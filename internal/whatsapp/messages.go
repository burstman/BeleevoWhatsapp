package whatsapp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Message is the API-facing shape of one sent message.
type Message struct {
	ID                uuid.UUID
	ShopID            uuid.UUID
	CustomerID        *uuid.UUID
	TemplateID        *uuid.UUID
	ConvertyOrderID   string
	RecipientPhone    string
	MetaMessageID     string
	Status            string
	ErrorCode         string
	ErrorMessage      string
	MetaErrors        []MetaError
	TemplateVariables map[string]string
	SentAt            *time.Time
	DeliveredAt       *time.Time
	ReadAt            *time.Time
	FailedAt          *time.Time
	CreatedAt         time.Time
}

// MetaError mirrors a webhook failure element for the dashboard/API.
type MetaError struct {
	Code  int    `json:"code"`
	Title string `json:"title"`
}

// MessagesAll lists every message across the operator's shops, newest first.
// The message history is shared; each row carries its own shop id.
func (s *Service) MessagesAll(ctx context.Context) ([]Message, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, customer_id, template_id, converty_order_id,
		       recipient_phone, meta_message_id, status, error_code, error_message,
		       meta_errors, template_variables, sent_at, delivered_at, read_at, failed_at, created_at
		FROM messages
		ORDER BY created_at DESC
		LIMIT 500`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Messages lists a merchant's messages, newest first.
func (s *Service) Messages(ctx context.Context, shopID uuid.UUID) ([]Message, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, shop_id, customer_id, template_id, converty_order_id,
		       recipient_phone, meta_message_id, status, error_code, error_message,
		       meta_errors, template_variables, sent_at, delivered_at, read_at, failed_at, created_at
		FROM messages
		WHERE shop_id = $1
		ORDER BY created_at DESC
		LIMIT 500`,
		shopID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Message returns one merchant message scoped to its shop.
func (s *Service) Message(ctx context.Context, shopID, messageID uuid.UUID) (Message, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, shop_id, customer_id, template_id, converty_order_id,
		       recipient_phone, meta_message_id, status, error_code, error_message,
		       meta_errors, template_variables, sent_at, delivered_at, read_at, failed_at, created_at
		FROM messages
		WHERE id = $1 AND shop_id = $2`,
		messageID, shopID,
	)
	return scanMessage(row)
}

type messageScanner interface {
	Scan(dest ...any) error
}

func scanMessage(row messageScanner) (Message, error) {
	var m Message
	var metaErrors []byte
	var vars []byte
	err := row.Scan(
		&m.ID, &m.ShopID, &m.CustomerID, &m.TemplateID, &m.ConvertyOrderID,
		&m.RecipientPhone, &m.MetaMessageID, &m.Status, &m.ErrorCode, &m.ErrorMessage,
		&metaErrors, &vars, &m.SentAt, &m.DeliveredAt, &m.ReadAt, &m.FailedAt, &m.CreatedAt,
	)
	if err != nil {
		return Message{}, err
	}
	if len(metaErrors) > 0 {
		_ = json.Unmarshal(metaErrors, &m.MetaErrors)
	}
	if len(vars) > 0 && string(vars) != "{}" && string(vars) != "null" {
		_ = json.Unmarshal(vars, &m.TemplateVariables)
	}
	if m.TemplateVariables == nil {
		m.TemplateVariables = map[string]string{}
	}
	return m, nil
}
