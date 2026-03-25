package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// NonceStore manages single-use nonces for callback validation.
// Nonces are tied to a specific (action, projectID, appID) tuple.
type NonceStore interface {
	// Generate creates and persists a new nonce tied to the given action, project, and app.
	Generate(ctx context.Context, action, projectID, appID string) (string, error)
	// Validate checks if the nonce is valid for the given action, project, and app.
	// Returns true and deletes the nonce if valid (single-use).
	Validate(ctx context.Context, nonce, action, projectID, appID string) (bool, error)
}

type nonceStore struct {
	repo repositories.NonceRepository
	ttl  time.Duration
}

const (
	defaultNonceTTL     = 15 * time.Minute
	maxNonceCreateTries = 3
)

// NewNonceStore creates a new Postgres-backed nonce store.
func NewNonceStore(repo repositories.NonceRepository, ttl time.Duration) NonceStore {
	if ttl <= 0 {
		ttl = defaultNonceTTL
	}

	return &nonceStore{
		repo: repo,
		ttl:  ttl,
	}
}

func (s *nonceStore) Generate(ctx context.Context, action, projectID, appID string) (string, error) {
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return "", fmt.Errorf("invalid project ID: %w", err)
	}

	expiresAt := time.Now().Add(s.ttl)
	for range maxNonceCreateTries {
		nonce, err := generateNonce()
		if err != nil {
			return "", err
		}

		if err := s.repo.Create(ctx, nonce, action, projectUUID, appID, expiresAt); err != nil {
			if isUniqueViolation(err) {
				continue
			}
			return "", err
		}

		return nonce, nil
	}

	return "", fmt.Errorf("failed to generate unique nonce after %d attempts", maxNonceCreateTries)
}

func (s *nonceStore) Validate(ctx context.Context, nonce, action, projectID, appID string) (bool, error) {
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return false, fmt.Errorf("invalid project ID: %w", err)
	}

	return s.repo.ValidateAndConsume(ctx, nonce, action, projectUUID, appID)
}

func generateNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random nonce: %w", err)
	}

	return hex.EncodeToString(b), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

var _ NonceStore = (*nonceStore)(nil)
