// Package auth handles admin authentication backed by SQLite sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tgustinak/pohkapela.sk/internal/db"
)

// ErrInvalidCredentials is returned when the e-mail or password does not match.
// The same error is used for both cases so callers cannot probe which one failed.
var ErrInvalidCredentials = errors.New("invalid e-mail or password")

// SessionTTL is how long a login session stays valid.
const SessionTTL = 30 * 24 * time.Hour

// Service groups authentication operations around sqlc queries.
type Service struct {
	queries *db.Queries
}

// NewService creates an authentication service.
func NewService(queries *db.Queries) *Service {
	return &Service{queries: queries}
}

// HashPassword hashes a plain-text password with bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	return string(hash), nil
}

// Login verifies credentials and creates a new session token.
// It returns ErrInvalidCredentials when the credentials do not match.
func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidCredentials
		}

		return "", fmt.Errorf("find user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	token, err := newToken()
	if err != nil {
		return "", err
	}

	_, err = s.queries.CreateSession(ctx, db.CreateSessionParams{
		TokenHash: hashToken(token),
		UserID:    user.ID,
		ExpiresAt: time.Now().UTC().Add(SessionTTL),
	})
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return token, nil
}

// Authenticate resolves a session token to its user.
// Expired sessions are deleted as a side effect.
func (s *Service) Authenticate(ctx context.Context, token string) (*db.User, error) {
	if token == "" {
		return nil, ErrInvalidCredentials
	}

	session, err := s.queries.GetSessionByTokenHash(ctx, hashToken(token))
	if err != nil {
		return nil, fmt.Errorf("find session: %w", err)
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		_ = s.queries.DeleteSession(ctx, session.TokenHash)

		return nil, ErrInvalidCredentials
	}

	user, err := s.queries.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("find session user: %w", err)
	}

	return &user, nil
}

// Logout deletes the session belonging to token, if any.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	if err := s.queries.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// EnsureAdminUser creates the initial admin account when no users exist yet.
// It returns true when a user was created.
func (s *Service) EnsureAdminUser(ctx context.Context, email, password string) (bool, error) {
	count, err := s.queries.CountUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}

	if count > 0 {
		return false, nil
	}

	hash, err := HashPassword(password)
	if err != nil {
		return false, err
	}

	_, err = s.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
	})
	if err != nil {
		return false, fmt.Errorf("create admin user: %w", err)
	}

	return true, nil
}

// newToken generates a random URL-safe session token.
func newToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// hashToken hashes a token with SHA-256 for storage.
// The raw token only ever travels in the session cookie.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
