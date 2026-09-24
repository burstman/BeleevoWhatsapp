package shops

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/database"
)

var ErrNotFound = errors.New("shop not found")

type Shop struct {
	ID                      uuid.UUID  `json:"id"`
	Name                    string     `json:"name"`
	Phone                   string     `json:"phone"`
	Status                  string     `json:"status"`
	WhatsappEnabled         bool       `json:"whatsapp_enabled"`
	WhatsappTermsAcceptedAt *time.Time `json:"whatsapp_terms_accepted_at"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, q database.Querier, name string) (Shop, error) {
	var s Shop
	err := q.QueryRow(ctx, `
		INSERT INTO shops (name)
		VALUES ($1)
		RETURNING id, name, status, updated_at, created_at`,
		name,
	).Scan(&s.ID, &s.Name, &s.Status, &s.UpdatedAt, &s.CreatedAt)
	if err != nil {
		return Shop{}, err
	}
	return s, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Shop, error) {
	var s Shop
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, phone, status, whatsapp_enabled, whatsapp_terms_accepted_at, created_at, updated_at
		FROM shops
		WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.Name, &s.Phone, &s.Status, &s.WhatsappEnabled, &s.WhatsappTermsAcceptedAt, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Shop{}, ErrNotFound
	}
	if err != nil {
		return Shop{}, err
	}
	return s, nil
}

// List returns every shop the operator manages, most recently created first.
// On this single-client deployment the operator owns all shops.
func (r *Repository) List(ctx context.Context) ([]Shop, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, phone, status, whatsapp_enabled, whatsapp_terms_accepted_at, created_at, updated_at
		FROM shops
		ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	shops := make([]Shop, 0)
	for rows.Next() {
		var s Shop
		if err := rows.Scan(&s.ID, &s.Name, &s.Phone, &s.Status, &s.WhatsappEnabled, &s.WhatsappTermsAcceptedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		shops = append(shops, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return shops, nil
}

// ListIntegrated returns the shops that have at least one connected Converty
// integration, most recently created first. A shop's display name is the store
// name of its first integration, falling back to the shop name when the store
// has no name yet. Un-integrated shops are never surfaced: on this platform a
// merchant only exists once their store is connected.
func (r *Repository) ListIntegrated(ctx context.Context) ([]Shop, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id,
		       COALESCE((SELECT ci.store_name FROM converty_integrations ci
		                 WHERE ci.shop_id = s.id AND ci.store_name <> ''
		                 ORDER BY ci.created_at LIMIT 1), s.name) AS name,
		       s.phone, s.status, s.whatsapp_enabled, s.whatsapp_terms_accepted_at,
		       s.created_at, s.updated_at
		FROM shops s
		WHERE EXISTS (SELECT 1 FROM converty_integrations ci2 WHERE ci2.shop_id = s.id)
		ORDER BY s.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	shops := make([]Shop, 0)
	for rows.Next() {
		var s Shop
		if err := rows.Scan(&s.ID, &s.Name, &s.Phone, &s.Status, &s.WhatsappEnabled, &s.WhatsappTermsAcceptedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		shops = append(shops, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return shops, nil
}

// Update renames a shop.
func (r *Repository) Update(ctx context.Context, id uuid.UUID, name string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE shops
		SET name = $2, updated_at = now()
		WHERE id = $1`,
		id, name,
	)
	return err
}

// Delete removes a shop. Every child row (converty integrations, templates,
// customers, messages) cascades; whatsapp numbers are nulled. Used to clean up
// placeholder shops whose OAuth connect was abandoned.
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM shops WHERE id = $1`, id)
	return err
}

// EnableWhatsApp marks the merchant as opted into the platform's WhatsApp
// service, records their contact phone and that they accepted the service
// terms. This is the onboarding gateway for sending.
func (r *Repository) EnableWhatsApp(ctx context.Context, id uuid.UUID, phone string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE shops
		SET whatsapp_enabled = true,
		    whatsapp_terms_accepted_at = COALESCE(whatsapp_terms_accepted_at, now()),
		    phone = CASE WHEN $2 <> '' THEN $2 ELSE phone END,
		    updated_at = now()
		WHERE id = $1`,
		id, phone,
	)
	return err
}
