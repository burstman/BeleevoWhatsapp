package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/anthdm/superkit/kit"
	"github.com/jackc/pgx/v5"
)

// SessionCookieName is defined in service.go.
const sessionTokenKey = "sessionToken"

// AuthenticateUser resolves the signed session cookie into an Auth principal.
// It is wired into superkit's kit.WithAuthentication middleware.
func (s *Service) AuthenticateUser(k *kit.Kit) (kit.Auth, error) {
	sess := k.GetSession(SessionCookieName)

	token, ok := sess.Values[sessionTokenKey].(string)
	if !ok || token == "" {
		return Auth{}, nil
	}

	auth, err := s.userByToken(context.Background(), token)
	if err != nil {
		return Auth{}, nil
	}
	return auth, nil
}

func (s *Service) userByToken(ctx context.Context, token string) (Auth, error) {
	var user User

	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.name, u.role
		FROM auth_sessions a
		JOIN users u ON u.id = a.user_id
		WHERE a.token = $1 AND a.expires_at > now()`,
		token,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Auth{}, nil
	}
	if err != nil {
		return Auth{}, fmt.Errorf("resolve session: %w", err)
	}

	return Auth{LoggedIn: true, User: user}, nil
}
