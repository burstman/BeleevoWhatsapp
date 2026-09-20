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
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
		RETURNING id, name, status, created_at, updated_at`,
		name,
	).Scan(&s.ID, &s.Name, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return Shop{}, err
	}
	return s, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Shop, error) {
	var s Shop
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, status, created_at, updated_at
		FROM shops
		WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.Name, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Shop{}, ErrNotFound
	}
	if err != nil {
		return Shop{}, err
	}
	return s, nil
}
