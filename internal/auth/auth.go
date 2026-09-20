package auth

import (
	"github.com/anthdm/superkit/kit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User is a merchant account. Users belong to exactly one shop.
type User struct {
	ID       uuid.UUID `json:"id"`
	ShopID   uuid.UUID `json:"shop_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	Password string    `json:"-"`
}

// Auth is the authenticated principal for a request. It implements
// superkit's kit.Auth interface and carries the shop context.
type Auth struct {
	LoggedIn bool
	User     User
}

func (a Auth) Check() bool { return a.LoggedIn }

// FromKit returns the authenticated principal (or the zero value).
func FromKit(k *kit.Kit) Auth {
	return k.Auth().(Auth)
}

type Account struct {
	pool *pgxpool.Pool
}

func NewAccount(pool *pgxpool.Pool) *Account {
	return &Account{pool: pool}
}
