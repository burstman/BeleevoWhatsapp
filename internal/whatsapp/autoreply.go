package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"whatsappconverty/internal/queue"
)

// FirstContactAutoreplyKind values.
const (
	AutoreplyText  = "text"
	AutoreplyImage = "image"
	AutoreplyAudio = "audio"
)

// FirstContactAutoreply is a shop's greeting for a customer's first inbound
// message. The media fields mirror the chat_messages media columns; media_bytes
// is only populated by the send path, never by the settings page.
type FirstContactAutoreply struct {
	ShopID          uuid.UUID
	Enabled         bool
	Kind            string
	TextBody        string
	Caption         string
	MediaMime       string
	MediaBytes      []byte
	MediaDurationMS int
	MediaFilename   string
}

// GetFirstContactAutoreply returns the shop's greeting configuration without
// the media bytes, so the settings page never loads a 16 MB blob. A missing row
// is not an error: it reports an unconfigured, disabled greeting.
func (s *Service) GetFirstContactAutoreply(ctx context.Context, shopID uuid.UUID) (FirstContactAutoreply, error) {
	cfg := FirstContactAutoreply{ShopID: shopID, Kind: AutoreplyText}
	err := s.pool.QueryRow(ctx, `
		SELECT enabled, kind, text_body, caption, media_mime, media_duration_ms, media_filename
		FROM shop_autoreplies WHERE shop_id = $1`, shopID,
	).Scan(&cfg.Enabled, &cfg.Kind, &cfg.TextBody, &cfg.Caption, &cfg.MediaMime, &cfg.MediaDurationMS, &cfg.MediaFilename)
	if errors.Is(err, pgx.ErrNoRows) {
		return FirstContactAutoreply{ShopID: shopID, Kind: AutoreplyText}, nil
	}
	if err != nil {
		return FirstContactAutoreply{}, err
	}
	return cfg, nil
}

// FirstContactAutoreplyMedia returns the stored media bytes and mime for the
// preview, or nil when the shop has no saved media.
func (s *Service) FirstContactAutoreplyMedia(ctx context.Context, shopID uuid.UUID) ([]byte, string, error) {
	var media []byte
	var mimeT string
	err := s.pool.QueryRow(ctx, `
		SELECT media_bytes, media_mime FROM shop_autoreplies
		WHERE shop_id = $1 AND media_bytes IS NOT NULL`, shopID,
	).Scan(&media, &mimeT)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return media, mimeT, nil
}

// SaveFirstContactAutoreply upserts the greeting. Switching away from a media
// kind clears the stored bytes so a later text greeting cannot inherit a stale
// image, and switching to a media kind with no new upload keeps whatever was
// already stored.
func (s *Service) SaveFirstContactAutoreply(ctx context.Context, cfg FirstContactAutoreply) error {
	if cfg.Kind == "" {
		cfg.Kind = AutoreplyText
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO shop_autoreplies (
			shop_id, enabled, kind, text_body, caption,
			media_mime, media_bytes, media_duration_ms, media_filename, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (shop_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			kind = EXCLUDED.kind,
			text_body = EXCLUDED.text_body,
			caption = EXCLUDED.caption,
			media_mime = CASE
				WHEN EXCLUDED.kind = 'text' THEN ''
				WHEN EXCLUDED.media_bytes IS NOT NULL THEN EXCLUDED.media_mime
				ELSE shop_autoreplies.media_mime END,
			media_bytes = CASE
				WHEN EXCLUDED.kind = 'text' THEN NULL
				WHEN EXCLUDED.media_bytes IS NOT NULL THEN EXCLUDED.media_bytes
				ELSE shop_autoreplies.media_bytes END,
			media_duration_ms = CASE
				WHEN EXCLUDED.kind = 'text' THEN 0
				WHEN EXCLUDED.media_bytes IS NOT NULL THEN EXCLUDED.media_duration_ms
				ELSE shop_autoreplies.media_duration_ms END,
			media_filename = CASE
				WHEN EXCLUDED.kind = 'text' THEN ''
				WHEN EXCLUDED.media_bytes IS NOT NULL THEN EXCLUDED.media_filename
				ELSE shop_autoreplies.media_filename END,
			updated_at = now()`,
		cfg.ShopID, cfg.Enabled, cfg.Kind, cfg.TextBody, cfg.Caption,
		cfg.MediaMime, cfg.MediaBytes, cfg.MediaDurationMS, cfg.MediaFilename)
	return err
}

// ScheduleFirstContactReply enqueues the greeting for a brand-new conversation.
// It is best-effort: a disabled or unconfigured shop is a no-op. The once-only
// claim happens in the job, not here, so a failed enqueue does not consume the
// conversation's first-contact slot.
func (s *Service) ScheduleFirstContactReply(ctx context.Context, shopID, conversationID uuid.UUID) error {
	cfg, err := s.GetFirstContactAutoreply(ctx, shopID)
	if err != nil {
		return err
	}
	if !cfg.Enabled || !autoreplyConfigured(cfg) {
		return nil
	}
	payload, err := json.Marshal(FirstContactReplyJob{
		ShopID:         shopID,
		ConversationID: conversationID,
	})
	if err != nil {
		return err
	}
	return queue.Enqueue(ctx, s.pool, queue.Params{
		Kind:      queue.TaskSendFirstContactReply,
		ShopID:    shopID,
		Payload:   payload,
		DedupeKey: "fc:" + conversationID.String(),
	})
}

// FirstContactReplyJob is the queue payload for a first-contact greeting.
type FirstContactReplyJob struct {
	ShopID         uuid.UUID `json:"shop_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
}

// SendFirstContactReplyJob is the queue handler: it claims the conversation
// exactly once, loads the greeting and sends it with the same free-form paths
// the operator inbox uses. The inbound message that created the conversation has
// already opened the 24h service window, so a template is not required.
func (s *Service) SendFirstContactReplyJob(ctx context.Context, payload []byte) error {
	var job FirstContactReplyJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return queue.Permanent(fmt.Errorf("first-contact reply: bad payload: %w", err))
	}
	if job.ShopID == uuid.Nil || job.ConversationID == uuid.Nil {
		return queue.Permanent(errors.New("first-contact reply: missing shop or conversation"))
	}

	cfg, err := s.GetFirstContactAutoreply(ctx, job.ShopID)
	if err != nil {
		return err
	}
	if !cfg.Enabled || !autoreplyConfigured(cfg) {
		return nil
	}
	media, mime, err := s.FirstContactAutoreplyMedia(ctx, job.ShopID)
	if err != nil {
		return err
	}

	// Claim the conversation: only the first worker to flip the column sends.
	tag, err := s.pool.Exec(ctx, `
		UPDATE conversations SET first_contact_autoreplied_at = now()
		WHERE id = $1 AND shop_id = $2 AND first_contact_autoreplied_at IS NULL`,
		job.ConversationID, job.ShopID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil // already greeted, or conversation not this shop's
	}

	switch cfg.Kind {
	case AutoreplyImage:
		if len(media) == 0 {
			s.log.Warn("first-contact reply: image configured but no bytes stored",
				"shop_id", job.ShopID, "conversation_id", job.ConversationID)
			return nil
		}
		_, err = s.SendImageReply(ctx, job.ConversationID, mime, cfg.MediaFilename, media, cfg.Caption)
	case AutoreplyAudio:
		if len(media) == 0 {
			s.log.Warn("first-contact reply: audio configured but no bytes stored",
				"shop_id", job.ShopID, "conversation_id", job.ConversationID)
			return nil
		}
		_, err = s.SendAudioReply(ctx, job.ConversationID, mime, cfg.MediaFilename, media, cfg.MediaDurationMS)
	default:
		text := strings.TrimSpace(cfg.TextBody)
		if text == "" {
			return nil
		}
		_, err = s.SendReply(ctx, job.ConversationID, text)
	}
	if err != nil {
		// The failure is already visible as a failed bubble in the inbox; do not
		// retry a closed window or a broken integration forever.
		if errors.Is(err, ErrReplyWindowClosed) || errors.Is(err, ErrNotConfigured) {
			return queue.Permanent(err)
		}
		return err
	}
	s.log.Info("first-contact reply sent",
		"shop_id", job.ShopID, "conversation_id", job.ConversationID, "kind", cfg.Kind)
	return nil
}

// autoreplyConfigured reports whether a saved greeting has the content its kind
// needs, so an empty row never produces a silent no-op send.
func autoreplyConfigured(cfg FirstContactAutoreply) bool {
	switch cfg.Kind {
	case AutoreplyImage, AutoreplyAudio:
		return cfg.MediaMime != ""
	case AutoreplyText:
		return strings.TrimSpace(cfg.TextBody) != ""
	}
	return false
}
