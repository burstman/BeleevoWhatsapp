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

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/converty"
)

const maxWebhookBody = 2 << 20

// handleConvertyWebhook is the target Converty POSTs order events to. The
// payload shape is not yet documented, so for now we acknowledge with 200 and
// log the raw body until a real event sample is captured.
func (a *App) handleConvertyWebhook(k *kit.Kit) error {
	body, err := io.ReadAll(io.LimitReader(k.Request.Body, maxWebhookBody))
	if err != nil {
		return err
	}

	a.Log.Info("converty webhook received",
		"content_type", k.Request.Header.Get("Content-Type"),
		"body", string(body),
	)

	return k.Text(http.StatusOK, "ok")
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
		a.subscribeWebhooks(ctx, principal.User.ShopID, tok.AccessToken)
		return k.Redirect(http.StatusSeeOther, "/dashboard?connect=store")
	}

	if err := a.Converty.SaveIntegration(ctx, principal.User.ShopID, store, converty.DefaultScopes, tok, time.Now()); err != nil {
		a.Log.Error("converty integration save failed", "error", err)
		return k.Redirect(http.StatusSeeOther, "/dashboard?connect=error")
	}

	a.subscribeWebhooks(ctx, principal.User.ShopID, tok.AccessToken)

	a.Log.Info("converty connected", "shop_id", principal.User.ShopID, "store", store.Name)
	return k.Redirect(http.StatusSeeOther, "/dashboard?connect=success")
}

// subscribeWebhooks registers the order events against the app's Converty
// webhook endpoint. Failures are logged but do not fail the connect (the
// integration is already saved). A 409 means the hook already exists and is
// treated as success.
func (a *App) subscribeWebhooks(ctx context.Context, shopID uuid.UUID, accessToken string) {
	targetURL := a.Converty.WebhookURL(a.Cfg.AppURL)
	for _, event := range converty.SupportedEvents() {
		err := a.Converty.SubscribeHook(ctx, accessToken, targetURL, event)
		if err == nil {
			a.Log.Info("converty hook subscribed", "shop_id", shopID, "event", event, "target_url", targetURL)
			continue
		}
		var apiErr converty.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
			a.Log.Info("converty hook already subscribed", "shop_id", shopID, "event", event)
			continue
		}
		a.Log.Error("converty hook subscribe failed",
			"shop_id", shopID, "event", event, "target_url", targetURL, "error", err)
	}
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
