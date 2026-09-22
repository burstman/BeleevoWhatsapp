package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
)

// StatusUpdate is one delivery-status event sent by Meta for an outbound
// message. It is the only signal that advances a message from "sent".
type StatusUpdate struct {
	MetaMessageID string
	Status        string // sent | delivered | read | failed
	Timestamp     int64  // seconds since epoch, Meta's clock
	RecipientID   string
	Errors        []StatusError
}

// StatusError captures a failed-delivery reason from the webhook.
type StatusError struct {
	Code  int
	Title string
}

// VerifyWebhookChallenge implements Meta's GET subscription handshake:
// hub.mode=subscribe, hub.verify_token must equal the configured token, then
// echo hub.challenge verbatim.
func VerifyWebhookChallenge(configuredToken, mode, token, challenge string) (ok bool) {
	return mode == "subscribe" && subtle.ConstantTimeCompare([]byte(configuredToken), []byte(token)) == 1 && challenge != ""
}

// hmacSHA256 computes the HMAC-SHA256 mac for the raw body, used both by
// signature verification and by tests.
func hmacSHA256(secret string, rawBody []byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	return mac.Sum(nil)
}

// VerifyWebhookSignature checks X-Hub-Signature-256 against HMAC-SHA256 of
// the raw body using the app secret. Constant-time compare against the raw
// base, not the base64 form Meta sends.
func VerifyWebhookSignature(appSecret, signatureHeader string, rawBody []byte) bool {
	const prefix = "sha256="
	if len(signatureHeader) <= len(prefix) || signatureHeader[:len(prefix)] != prefix {
		return false
	}
	got, err := hex.DecodeString(signatureHeader[len(prefix):])
	if err != nil {
		return false
	}
	return hmac.Equal(got, hmacSHA256(appSecret, rawBody))
}

// ParseWebhook decodes one Meta webhook delivery into ordered status updates.
// Unknown fields are ignored; a delivery with no statuses returns an error.
func ParseWebhook(body []byte) ([]StatusUpdate, error) {
	var p struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					MessagingProduct string `json:"messaging_product"`
					Statuses         []struct {
						ID          string `json:"id"`
						Status      string `json:"status"`
						Timestamp   string `json:"timestamp"`
						RecipientID string `json:"recipient_id"`
						Errors      []struct {
							Code  int    `json:"code"`
							Title string `json:"title"`
						} `json:"errors"`
					} `json:"statuses"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}

	var out []StatusUpdate
	for _, entry := range p.Entry {
		for _, ch := range entry.Changes {
			if ch.Value.MessagingProduct != "" && ch.Value.MessagingProduct != "whatsapp" {
				continue
			}
			for _, s := range ch.Value.Statuses {
				u := StatusUpdate{
					MetaMessageID: s.ID,
					Status:        s.Status,
					RecipientID:   s.RecipientID,
				}
				if n, err := strconv.ParseInt(s.Timestamp, 10, 64); err == nil {
					u.Timestamp = n
				}
				for _, e := range s.Errors {
					u.Errors = append(u.Errors, StatusError{Code: e.Code, Title: e.Title})
				}
				out = append(out, u)
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("webhook delivery carried no statuses")
	}
	return out, nil
}

// webhookStatusRank orders statuses so regressions are never applied:
// sent(1) < delivered(2) < read(3), failed(4) is terminal.
func webhookStatusRank(status string) int {
	switch status {
	case "sent":
		return 1
	case "delivered":
		return 2
	case "read":
		return 3
	case "failed":
		return 4
	default:
		return 0
	}
}

// ShouldAdvanceStatus reports whether incoming supersedes the stored status.
func ShouldAdvanceStatus(stored, incoming string) bool {
	storedRank := webhookStatusRank(stored)
	incRank := webhookStatusRank(incoming)
	if incRank == 0 {
		return false
	}
	return incRank > storedRank
}

// webhookStatusColumn maps a status to the timestamp column name.
func webhookStatusColumn(status string) string {
	switch status {
	case "sent":
		return "sent_at"
	case "delivered":
		return "delivered_at"
	case "read":
		return "read_at"
	case "failed":
		return "failed_at"
	default:
		return ""
	}
}

// ApplyStatusUpdate advances one message to a newer delivery status. The
// message is located by its Meta message id (globally unique to the platform's
// own WABA). Regressive updates are ignored.
func (s *Service) ApplyStatusUpdate(ctx context.Context, u StatusUpdate) error {
	var current string
	err := s.pool.QueryRow(ctx, `
		SELECT status FROM messages WHERE meta_message_id = $1`,
		u.MetaMessageID,
	).Scan(&current)
	if err != nil {
		return err
	}
	if !ShouldAdvanceStatus(current, u.Status) {
		return nil
	}

	col := webhookStatusColumn(u.Status)
	if col == "" {
		return nil
	}

	var errsJSON []byte
	if len(u.Errors) > 0 {
		errsJSON, _ = json.Marshal(u.Errors)
	}

	sql := `UPDATE messages SET status = $2, updated_at = now()`
	args := []any{u.MetaMessageID, u.Status}
	switch u.Status {
	case "failed":
		sql += `, error_code = 'meta_webhook_failed', failed_at = COALESCE(failed_at, now())`
		if len(u.Errors) > 0 {
			sql += `, error_message = $3, meta_errors = $4::jsonb`
			args = append(args, u.Errors[0].Title, errsJSON)
		}
	case "delivered":
		sql += `, delivered_at = COALESCE(delivered_at, now())`
	case "read":
		sql += `, read_at = COALESCE(read_at, now())`
	}
	sql += ` WHERE meta_message_id = $1`

	_, err = s.pool.Exec(ctx, sql, args...)
	return err
}
