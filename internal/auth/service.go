package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/database"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already registered")
)

const (
	SessionCookieName = "user-session"
	sessionLifetime   = 30 * 24 * time.Hour
)

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
	rand func([]byte) (int, error)
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool: pool,
		now:  time.Now,
	}
}

// EnsureAdmin seeds the single operator account if it does not exist. The
// admin user has no shop binding (shop_id NULL) and manages every shop.
func (s *Service) EnsureAdmin(ctx context.Context, cfg config.Config) error {
	email := lowerEmail(cfg.AdminEmail)
	var existingID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM users
		WHERE email = $1`,
		email,
	).Scan(&existingID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lookup admin: %w", err)
	}
	if err == nil {
		_, _ = s.pool.Exec(ctx, `UPDATE users SET role = 'admin', updated_at = now() WHERE id = $1`, existingID)
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO users (shop_id, email, password_hash, name, role)
		VALUES (NULL, $1, $2, $3, 'admin')`,
		email, string(hash), cfg.Name,
	); err != nil {
		return fmt.Errorf("insert admin: %w", err)
	}
	return nil
}

// Login verifies credentials and returns a session token for the operator.
func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	var user User
	var passwordHash string
	if err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, role, password_hash
		FROM users
		WHERE email = lower($1)`,
		email,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &passwordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrInvalidCredentials
		}
		return "", fmt.Errorf("query user: %w", err)
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return "", ErrInvalidCredentials
	}

	user.Password = ""
	token, err := s.createSession(ctx, s.pool, user.ID)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

// Logout invalidates the session token server-side.
func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM auth_sessions WHERE token = $1`, token)
	return err
}

func (s *Service) createSession(ctx context.Context, q database.Querier, userID uuid.UUID) (string, error) {
	random, err := randomToken(32)
	if err != nil {
		return "", err
	}
	token := random
	expires := s.now().Add(sessionLifetime)

	query := `
		INSERT INTO auth_sessions (user_id, token, expires_at)
		VALUES ($1, $2, $3)`
	if _, err := q.Exec(ctx, query, userID, token, expires); err != nil {
		return "", err
	}
	return token, nil
}

func lowerEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
