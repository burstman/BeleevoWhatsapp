package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// customerServiceWindow is how long a free-form reply stays allowed after the
// customer's last inbound message.
const customerServiceWindow = 24 * time.Hour

// Conversation is one inbox thread: a customer's messages with the operator's
// WhatsApp number.
type Conversation struct {
	ID            uuid.UUID
	ShopID        uuid.UUID
	CustomerPhone string
	CustomerName  string
	LastDirection string
	LastBody      string
	LastMessageAt time.Time
	UnreadCount   int
	WindowOpen    bool
}

// ChatMessage is one row of a conversation thread.
type ChatMessage struct {
	ID             uuid.UUID
	Direction      string
	Body           string
	MetaMessageID  string
	Status         string
	CreatedAt      time.Time
	MediaKind      string
	MediaMime      string
	MediaBytes     []byte
	MediaDurationMS int
	MediaFilename  string
}

// ErrReplyWindowClosed is returned when a free-form reply is attempted after
// the 24h customer service window expired.
var ErrReplyWindowClosed = errors.New("customer service window closed; free-form replies are only available for 24h after the customer's last message")

// ErrConversationNotFound is returned when a thread id does not resolve.
var ErrConversationNotFound = errors.New("conversation not found")

// shopByPhoneNumberID resolves the shop owning the operator number that
// received an inbound message; pgx.ErrNoRows when it is not ours.
func (s *Service) shopByPhoneNumberID(ctx context.Context, phoneNumberID string) (uuid.UUID, error) {
	var shopID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT shop_id FROM whatsapp_integrations WHERE phone_number_id = $1`,
		phoneNumberID,
	).Scan(&shopID)
	return shopID, err
}

// ShopByPhoneNumberID resolves the shop owning the operator number that
// received an inbound message; pgx.ErrNoRows when it is not ours.
func (s *Service) ShopByPhoneNumberID(ctx context.Context, phoneNumberID string) (uuid.UUID, error) {
	return s.shopByPhoneNumberID(ctx, phoneNumberID)
}

// ApplyChatStatusUpdate applies a webhook delivery status to a free-form reply
// (chat_messages), a no-op when the id does not belong to one.
func (s *Service) ApplyChatStatusUpdate(ctx context.Context, u StatusUpdate) error {
	return s.updateChatMessageStatus(ctx, u)
}

// UpsertInbound records a customer message against the shop owning the
// receiving number, creating the conversation when it is new. Re-deliveries of
// the same Meta message id are ignored. The customer service window restarts
// from the message timestamp.
func (s *Service) UpsertInbound(ctx context.Context, shopID uuid.UUID, m InboundMessage) error {
	if m.From == "" || m.MetaMessageID == "" {
		return nil
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM chat_messages WHERE meta_message_id = $1 AND direction = 'inbound')`,
		m.MetaMessageID,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	ts := time.Unix(m.Timestamp, 0).UTC()
	if ts.Before(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		ts = time.Now().UTC()
	}

	body := m.TextBody
	mediaKind := ""
	mediaMime := ""
	mediaBytes := []byte(nil)
	durationMS := m.MediaDurationMS
	filename := m.MediaFilename
	if m.Type == "audio" || m.Type == "voice" {
		mediaKind = "audio"
		body = "🎤 Voice message"
		if m.MediaID == "" {
			s.RecordWebhookReceipt(ctx, WebhookReceipt{
				Kind:          "media_missing_id",
				MetaMessageID: m.MetaMessageID,
				FromPhone:     m.From,
				PhoneNumberID: s.phoneNumberID(ctx, shopID),
				ShopID:        &shopID,
				Detail:        "audio message carried no media id",
			})
		} else if downloaded, mimeT, derr := s.downloadInboundMedia(ctx, shopID, m.MediaID); derr != nil {
			s.log.Warn("inbound voice note: media download failed; storing label only",
				"media_id", m.MediaID, "error", derr.Error())
			s.RecordWebhookReceipt(ctx, WebhookReceipt{
				Kind:          "media_download_failed",
				MetaMessageID: m.MetaMessageID,
				FromPhone:     m.From,
				PhoneNumberID: s.phoneNumberID(ctx, shopID),
				ShopID:        &shopID,
				Detail:        truncate(derr.Error(), 240),
			})
		} else {
			mediaBytes, mediaMime = downloaded, mimeT
		}
	} else if m.Type != "" && m.Type != "text" && body == "" {
		body = "[" + m.Type + "]"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var convID uuid.UUID
	// Upsert the conversation: keep the id, refresh name/window/last fields.
	err = tx.QueryRow(ctx, `
		INSERT INTO conversations (
			shop_id, customer_phone, customer_name, last_message_direction,
			last_message_body, last_message_at, unread_count, window_expires_at
		) VALUES ($1, $2, $3, 'inbound', $4, $5, 1, $6)
		ON CONFLICT (shop_id, customer_phone) DO UPDATE SET
			customer_name = CASE WHEN $3 <> '' THEN $3 ELSE conversations.customer_name END,
			last_message_direction = 'inbound',
			last_message_body = $4,
			last_message_at = $5,
			unread_count = conversations.unread_count + 1,
			window_expires_at = $6,
			updated_at = now()
		RETURNING id`,
		shopID, m.From, m.ProfileName, body, ts, ts.Add(customerServiceWindow),
	).Scan(&convID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO chat_messages (
			conversation_id, direction, body, meta_message_id, status,
			media_kind, media_mime, media_bytes, media_duration_ms, media_filename,
			sent_at, created_at
		) VALUES ($1, 'inbound', $2, $3, 'delivered', $4, $5, $6, $7, $8, $9, $9)`,
		convID, body, m.MetaMessageID, mediaKind, mediaMime, mediaBytes, durationMS, filename, ts,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// downloadInboundMedia fetches a customer's media with the shop's own Meta
// credentials. ErrNotConfigured or a network failure falls back to storing the
// message with its label only, so an isolated media hiccup never loses the
// conversation itself. Transient Graph errors are retried once before giving up.
func (s *Service) downloadInboundMedia(ctx context.Context, shopID uuid.UUID, mediaID string) ([]byte, string, error) {
	creds, err := s.Credentials(ctx, shopID)
	if err != nil {
		return nil, "", err
	}
	bytes, mime, err := s.DownloadMedia(ctx, creds.AccessToken, creds.PhoneNumberID, mediaID)
	if err != nil {
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
		bytes, mime, err = s.DownloadMedia(ctx, creds.AccessToken, creds.PhoneNumberID, mediaID)
	}
	return bytes, mime, err
}

// phoneNumberID is a diagnostic helper for webhook receipts: it returns the
// number that received the message, or empty if the shop has no credentials.
func (s *Service) phoneNumberID(ctx context.Context, shopID uuid.UUID) string {
	creds, err := s.Credentials(ctx, shopID)
	if err != nil {
		return ""
	}
	return creds.PhoneNumberID
}

// truncate caps diagnostic detail strings so the settings trail stays compact.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

// Threads lists the operator's inbox: every conversation across their shops,
// most recent first.
func (s *Service) Threads(ctx context.Context) ([]Conversation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, customer_phone, customer_name, last_message_direction,
		       last_message_body, last_message_at, unread_count,
		       window_expires_at > now() AS window_open
		FROM conversations
		ORDER BY last_message_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.ShopID, &c.CustomerPhone, &c.CustomerName,
			&c.LastDirection, &c.LastBody, &c.LastMessageAt, &c.UnreadCount, &c.WindowOpen); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Thread returns one conversation with its messages (older first), plus
// whether free-form replies are currently allowed. ErrConversationNotFound
// when the id does not resolve.
func (s *Service) Thread(ctx context.Context, conversationID uuid.UUID) (Conversation, []ChatMessage, error) {
	var c Conversation
	err := s.pool.QueryRow(ctx, `
		SELECT id, shop_id, customer_phone, customer_name, last_message_direction,
		       last_message_body, last_message_at, unread_count,
		       window_expires_at > now() AS window_open
		FROM conversations WHERE id = $1`, conversationID,
	).Scan(&c.ID, &c.ShopID, &c.CustomerPhone, &c.CustomerName,
		&c.LastDirection, &c.LastBody, &c.LastMessageAt, &c.UnreadCount, &c.WindowOpen)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c, nil, ErrConversationNotFound
		}
		return c, nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, direction, body, meta_message_id, status, created_at,
		       media_kind, media_mime, media_bytes, media_duration_ms, media_filename
		FROM chat_messages WHERE conversation_id = $1 ORDER BY created_at, id`,
		conversationID)
	if err != nil {
		return c, nil, err
	}
	defer rows.Close()

	var msgs []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.Direction, &m.Body, &m.MetaMessageID, &m.Status, &m.CreatedAt,
			&m.MediaKind, &m.MediaMime, &m.MediaBytes, &m.MediaDurationMS, &m.MediaFilename); err != nil {
			return c, nil, err
		}
		msgs = append(msgs, m)
	}
	return c, msgs, rows.Err()
}

// MarkThreadRead zeroes the thread's unread counter.
func (s *Service) MarkThreadRead(ctx context.Context, conversationID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE conversations SET unread_count = 0, updated_at = now() WHERE id = $1`,
		conversationID)
	return err
}

// MediaForMessage returns the stored media bytes for one chat message, proving
// it belongs to the given conversation. A text message (or one whose 30-day
// retention purge already cleared it) returns no bytes.
func (s *Service) MediaForMessage(ctx context.Context, conversationID, messageID uuid.UUID) (media []byte, mime string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT media_bytes, media_mime
		FROM chat_messages WHERE id = $1 AND conversation_id = $2`,
		messageID, conversationID,
	).Scan(&media, &mime)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrConversationNotFound
	}
	return media, mime, err
}

// PurgeChatMedia drops the stored bytes of media messages older than the
// cutoff, keeping the message row and its label so the conversation history
// still reads "[🎤 Voice message]" with no play button. It is the inbox's
// retention job (media older than 30 days is deleted).
func (s *Service) PurgeChatMedia(ctx context.Context, olderThan time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE chat_messages
		SET media_bytes = NULL, media_mime = '', media_duration_ms = 0, media_filename = ''
		WHERE media_kind <> '' AND media_bytes IS NOT NULL AND created_at < $1`,
		olderThan)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// ReplyResult reports a sent free-form reply.
type ReplyResult struct {
	MessageID uuid.UUID
	MetaMessageID string
}

// SendReply delivers a free-form customer service message inside the 24h
// window. Outside the window it returns ErrReplyWindowClosed so the UI can
// tell the operator to use an approved template instead.
func (s *Service) SendReply(ctx context.Context, conversationID uuid.UUID, text string) (ReplyResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ReplyResult{}, errors.New("empty reply")
	}

	conv, _, err := s.Thread(ctx, conversationID)
	if err != nil {
		return ReplyResult{}, err
	}
	if !conv.WindowOpen {
		return ReplyResult{}, ErrReplyWindowClosed
	}

	creds, err := s.Credentials(ctx, conv.ShopID)
	if err != nil {
		return ReplyResult{}, err
	}
	if s.rate != nil {
		if err := s.rate.Allow(ctx, conv.ShopID); err != nil {
			return ReplyResult{}, err
		}
	}

	var msgID uuid.UUID
	err = s.pool.QueryRow(ctx, `
		INSERT INTO chat_messages (conversation_id, direction, body, status)
		VALUES ($1, 'outbound', $2, 'queued') RETURNING id`,
		conversationID, text,
	).Scan(&msgID)
	if err != nil {
		return ReplyResult{}, err
	}

	metaID, sendErr := s.SendText(ctx, creds.AccessToken, creds.PhoneNumberID,
		conv.CustomerPhone, text, MessagingAccountParam(creds.MessagingAccountID))
	if sendErr != nil {
		_, _ = s.pool.Exec(ctx, `
			UPDATE chat_messages SET status = 'failed', error_message = $2 WHERE id = $1`,
			msgID, sendErr.Error())
		return ReplyResult{}, sendErr
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE chat_messages SET status = 'sent', meta_message_id = $2, sent_at = COALESCE(sent_at, now())
		WHERE id = $1`, msgID, metaID)
	if err != nil {
		s.log.Error("chat: could not mark reply sent", "error", err)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE conversations
		SET last_message_direction = 'outbound', last_message_body = $2,
		    last_message_at = now(), updated_at = now()
		WHERE id = $1`, conversationID, text)
	if err != nil {
		return ReplyResult{}, err
	}

	s.log.Info("chat reply sent", "conversation_id", conversationID,
		"shop_id", conv.ShopID, "to", conv.CustomerPhone, "meta_message_id", metaID)
	return ReplyResult{MessageID: msgID, MetaMessageID: metaID}, nil
}

// SendAudioReply records, uploads and delivers operator audio (web microphone
// or attached file) to the customer, inside the 24h window. The sender's copy
// stores the bytes locally so the web thread shows a replayable bubble.
func (s *Service) SendAudioReply(ctx context.Context, conversationID uuid.UUID, mime string, filename string, data []byte, durationMS int) (ReplyResult, error) {
	if len(data) == 0 {
		return ReplyResult{}, errors.New("empty audio")
	}
	if len(data) > maxOutboundMediaBytes {
		return ReplyResult{}, fmt.Errorf("audio too large: %d bytes", len(data))
	}

	conv, _, err := s.Thread(ctx, conversationID)
	if err != nil {
		return ReplyResult{}, err
	}
	if !conv.WindowOpen {
		return ReplyResult{}, ErrReplyWindowClosed
	}

	creds, err := s.Credentials(ctx, conv.ShopID)
	if err != nil {
		return ReplyResult{}, err
	}
	if s.rate != nil {
		if err := s.rate.Allow(ctx, conv.ShopID); err != nil {
			return ReplyResult{}, err
		}
	}

	body := "🎤 Voice message"
	var msgID uuid.UUID
	err = s.pool.QueryRow(ctx, `
		INSERT INTO chat_messages (
			conversation_id, direction, body, status,
			media_kind, media_mime, media_bytes, media_duration_ms, media_filename
		) VALUES ($1, 'outbound', $2, 'queued', 'audio', $3, $4, $5, $6) RETURNING id`,
		conversationID, body, mime, data, durationMS, filename,
	).Scan(&msgID)
	if err != nil {
		return ReplyResult{}, err
	}

	mediaID, upErr := s.UploadMedia(ctx, creds.AccessToken, creds.PhoneNumberID, mime, data, filename)
	if upErr != nil {
		_, _ = s.pool.Exec(ctx, `
			UPDATE chat_messages SET status = 'failed', error_message = $2 WHERE id = $1`,
			msgID, upErr.Error())
		return ReplyResult{}, upErr
	}
	metaID, sendErr := s.SendAudio(ctx, creds.AccessToken, creds.PhoneNumberID,
		conv.CustomerPhone, mediaID, isVoiceNote(mime))
	if sendErr != nil {
		_, _ = s.pool.Exec(ctx, `
			UPDATE chat_messages SET status = 'failed', error_message = $2 WHERE id = $1`,
			msgID, sendErr.Error())
		return ReplyResult{}, sendErr
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE chat_messages SET status = 'sent', meta_message_id = $2, sent_at = COALESCE(sent_at, now())
		WHERE id = $1`, msgID, metaID)
	if err != nil {
		s.log.Error("chat: could not mark audio response sent", "error", err)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE conversations
		SET last_message_direction = 'outbound', last_message_body = $2,
		    last_message_at = now(), updated_at = now()
		WHERE id = $1`, conversationID, body)
	if err != nil {
		return ReplyResult{}, err
	}

	s.log.Info("chat audio reply sent", "conversation_id", conversationID,
		"shop_id", conv.ShopID, "to", conv.CustomerPhone, "meta_message_id", metaID,
		"mime", mime, "bytes", len(data))
	return ReplyResult{MessageID: msgID, MetaMessageID: metaID}, nil
}

// isVoiceNote reports whether a mime is OPUS-in-OGG, i.e. a real WhatsApp
// voice note. Anything else plays as a basic audio attachment.
func isVoiceNote(mime string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(mime)), "audio/ogg")
}

// updateChatMessageStatus advances a free-form reply's delivery status from a
// Meta webhook status event, mirroring the template ledger updates. A nil
// result with err == nil means the id does not belong to a chat reply (the
// template ledger already handled it, or it is a duplicate).
func (s *Service) updateChatMessageStatus(ctx context.Context, u StatusUpdate) error {
	var current string
	err := s.pool.QueryRow(ctx, `
		SELECT status FROM chat_messages WHERE meta_message_id = $1 AND direction = 'outbound'`,
		u.MetaMessageID,
	).Scan(&current)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if !ShouldAdvanceStatus(current, u.Status) {
		return nil
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE chat_messages SET status = $2, updated_at = now()
		WHERE meta_message_id = $1 AND direction = 'outbound'`,
		u.MetaMessageID, u.Status)
	return err
}