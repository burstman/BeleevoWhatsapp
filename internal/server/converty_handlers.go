package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/anthdm/superkit/kit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/converty"
)

const maxWebhookBody = 2 << 20

// handleConvertyWebhook is the target Converty POSTs order events to. The
// payload shape is not yet documented, so each delivery is persisted
// verbatim into order_events (deduplicated by body hash) with best-effort
// parsed fields, and we always acknowledge with 200.
func (a *App) handleConvertyWebhook(k *kit.Kit) error {
	body, err := io.ReadAll(io.LimitReader(k.Request.Body, maxWebhookBody))
	if err != nil {
		return err
	}

	event, err := a.Converty.CaptureWebhook(k.Request.Context(), body)
	if err != nil {
		a.Log.Error("converty webhook ingest failed", "error", err)
		return err
	}

	a.Log.Info("converty webhook captured",
		"duplicate", event.Duplicate,
		"shop_id", event.ShopID,
		"event_type", event.EventType,
		"order_id", event.OrderID,
		"order_status", event.OrderStatus,
		"body", truncateBytes(body, 800),
	)

	return k.Text(http.StatusOK, "ok")
}

func truncateBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

func (a *App) handleConvertyConnect(k *kit.Kit) error {
	if !a.Converty.Configured() {
		return fmt.Errorf("converty is not configured on this deployment")
	}

	principal := auth.FromKit(k)

	state, err := randomHex(32)
	if err != nil {
		return err
	}

	sess := k.GetSession(auth.SessionCookieName)
	sess.Values["converty_oauth_state"] = state
	if err := sess.Save(k.Request, k.Response); err != nil {
		return err
	}

	a.Log.Info("converty connect started", "shop_id", principal.User.ShopID, "redirect", a.Cfg.ConvertyRedirectURI)
	return k.Redirect(http.StatusFound, a.Converty.AuthorizeURL(state))
}

func (a *App) handleConvertyCallback(k *kit.Kit) error {
	principal := auth.FromKit(k)

	sess := k.GetSession(auth.SessionCookieName)
	savedState, _ := sess.Values["converty_oauth_state"].(string)
	delete(sess.Values, "converty_oauth_state")
	_ = sess.Save(k.Request, k.Response)

	state := k.Request.URL.Query().Get("state")
	code := k.Request.URL.Query().Get("code")

	if !secureEqual(savedState, state) {
		a.Log.Warn("converty callback rejected: state mismatch")
		return k.Redirect(http.StatusSeeOther, "/dashboard?connect=state")
	}
	if code == "" {
		return k.Redirect(http.StatusSeeOther, "/dashboard?connect=error")
	}

	ctx, cancel := context.WithTimeout(k.Request.Context(), 20*time.Second)
	defer cancel()

	tok, err := a.Converty.ExchangeCode(ctx, code)
	if err != nil {
		a.Log.Error("converty code exchange failed", "error", err)
		return k.Redirect(http.StatusSeeOther, "/dashboard?connect=error")
	}

	store, err := a.Converty.GetStore(ctx, tok.AccessToken)
	if err != nil {
		// A failed store sync should not discard the freshly connected tokens:
		// persist them and subscribe webhooks, but flag the sync problem.
		a.Log.Error("converty stores/me failed", "error", err)
		if err := a.Converty.SaveIntegration(ctx, principal.User.ShopID, converty.Store{}, converty.DefaultScopes, tok, time.Now()); err != nil {
			a.Log.Error("converty integration save failed", "error", err)
			return k.Redirect(http.StatusSeeOther, "/dashboard?connect=error")
		}
		a.subscribeWebhooks(ctx, principal.User.ShopID)
		return k.Redirect(http.StatusSeeOther, "/dashboard?connect=store")
	}

	if err := a.Converty.SaveIntegration(ctx, principal.User.ShopID, store, converty.DefaultScopes, tok, time.Now()); err != nil {
		a.Log.Error("converty integration save failed", "error", err)
		return k.Redirect(http.StatusSeeOther, "/dashboard?connect=error")
	}

	a.subscribeWebhooks(ctx, principal.User.ShopID)

	a.Log.Info("converty connected", "shop_id", principal.User.ShopID, "store", store.Name)
	return k.Redirect(http.StatusSeeOther, "/dashboard?connect=success")
}

// subscribeWebhooks registers the order events against the app's Converty
// webhook endpoint, fetching/refreshing the access token as needed. The
// returned Converty hook ids are merged into the stored subscription map (so
// ids from hooks that already existed survive a reconnect) and persisted for
// later removal on disconnect. Failures are logged but do not fail the
// connect (the integration is already saved). A 409 means the hook already
// exists and is treated as success.
func (a *App) subscribeWebhooks(ctx context.Context, shopID uuid.UUID) {
	targetURL := a.Converty.WebhookURL(a.Cfg.AppURL)

	subs := map[string]string{}
	existing, err := a.Converty.Integration(ctx, shopID)
	if err == nil && existing.WebhookSubscriptions != nil {
		subs = existing.WebhookSubscriptions
	}

	err = a.Converty.WithAccessToken(ctx, shopID, func(ctx context.Context, accessToken string) error {
		for _, event := range converty.SupportedEvents() {
			hookID, sErr := a.Converty.SubscribeHook(ctx, accessToken, targetURL, event)
			if sErr == nil {
				subs[event] = hookID
				a.Log.Info("converty hook subscribed", "shop_id", shopID, "event", event, "hook_id", hookID, "target_url", targetURL)
				continue
			}
			var apiErr converty.APIError
			if errors.As(sErr, &apiErr) && apiErr.StatusCode == http.StatusConflict {
				a.Log.Info("converty hook already subscribed", "shop_id", shopID, "event", event)
				continue
			}
			a.Log.Error("converty hook subscribe failed",
				"shop_id", shopID, "event", event, "target_url", targetURL, "error", sErr)
		}

		// Hooks that already existed (409) come back without an id, so pull
		// the live list and record their ids for later removal on disconnect.
		hooks, hErr := a.Converty.ListHooks(ctx, accessToken)
		if hErr != nil {
			a.Log.Warn("converty hooks list failed", "shop_id", shopID, "target_url", targetURL, "error", hErr)
			return nil
		}
		for _, h := range hooks {
			if h.TargetURL == targetURL && h.Event != "" && h.ID != "" {
				subs[h.Event] = h.ID
			}
		}
		return nil
	})
	if err != nil {
		a.Log.Error("converty hook subscribe skipped, no access token", "shop_id", shopID, "error", err)
	}

	if err := a.Converty.SaveWebhookSubscriptions(ctx, shopID, subs); err != nil {
		a.Log.Error("converty webhook subscriptions save failed", "shop_id", shopID, "error", err)
	}
}

// handleConvertyDisconnect removes every stored Converty webhook subscription
// (with an auto-refreshed token), then drops the integration row. Unsubscribe
// failures are logged but never block the local disconnect, so the user is
// always freed from their account.
func (a *App) handleConvertyDisconnect(k *kit.Kit) error {
	principal := auth.FromKit(k)
	ctx, cancel := context.WithTimeout(k.Request.Context(), 20*time.Second)
	defer cancel()

	integ, err := a.Converty.Integration(ctx, principal.User.ShopID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return k.Redirect(http.StatusSeeOther, "/dashboard")
		}
		return err
	}

	err = a.Converty.WithAccessToken(ctx, principal.User.ShopID, func(ctx context.Context, accessToken string) error {
		for event, hookID := range integ.WebhookSubscriptions {
			if hookID == "" {
				continue
			}
			if uErr := a.Converty.UnsubscribeHook(ctx, accessToken, hookID); uErr != nil {
				var apiErr converty.APIError
				alreadyGone := errors.As(uErr, &apiErr) &&
					(apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusConflict)
				if !alreadyGone {
					a.Log.Warn("converty hook unsubscribe failed",
						"shop_id", principal.User.ShopID, "event", event, "hook_id", hookID, "error", uErr)
				}
				continue
			}
			a.Log.Info("converty hook unsubscribed", "shop_id", principal.User.ShopID, "event", event, "hook_id", hookID)
		}
		return nil
	})
	if err != nil {
		a.Log.Warn("converty disconnect: hook cleanup skipped, no access token", "shop_id", principal.User.ShopID, "error", err)
	}

	if err := a.Converty.DeleteIntegration(ctx, principal.User.ShopID); err != nil {
		a.Log.Error("converty integration delete failed", "shop_id", principal.User.ShopID, "error", err)
		return err
	}

	a.Log.Info("converty disconnected", "shop_id", principal.User.ShopID)
	return k.Redirect(http.StatusSeeOther, "/dashboard?disconnect=success")
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func secureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
