package auth

import (
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// Common authentication errors.
var (
	ErrMissingAuthorization = errors.New("missing authorization")
	ErrInvalidAuthFormat    = errors.New("invalid authorization header format")
	ErrMissingProjectID     = errors.New("missing project ID in token")
	ErrProjectIDMismatch    = errors.New("project ID mismatch between token and URL")
)

// AuthService defines the interface for authentication operations.
// This abstraction enables clean separation between HTTP handling
// and authentication logic, making both easier to test.
type AuthService interface {
	// ValidateRequest extracts and validates a JWT from the request.
	// It checks for the token in:
	//   1. Cookie named "ekaya_jwt" (browser clients)
	//   2. Authorization header with "Bearer" scheme (API clients)
	// Returns the validated claims, the raw token string, or an error.
	ValidateRequest(r *http.Request) (*Claims, string, error)

	// RequireProjectID validates that the claims contain a project ID.
	RequireProjectID(claims *Claims) error

	// ValidateProjectIDMatch ensures the URL project ID matches the token project ID.
	// If urlProjectID is empty, validation is skipped.
	ValidateProjectIDMatch(claims *Claims, urlProjectID string) error
}

// authService implements AuthService.
type authService struct {
	jwksClient JWKSClientInterface
	logger     *zap.Logger
}

// NewAuthService creates a new AuthService with the given JWKS client and logger.
func NewAuthService(jwksClient JWKSClientInterface, logger *zap.Logger) AuthService {
	return &authService{
		jwksClient: jwksClient,
		logger:     logger,
	}
}

// ValidateRequest extracts and validates a JWT from the request.
func (s *authService) ValidateRequest(r *http.Request) (*Claims, string, error) {
	var tokenString string
	var tokenSource string

	// Try cookie first (browser clients)
	if cookie, err := r.Cookie("ekaya_jwt"); err == nil {
		tokenString = cookie.Value
		tokenSource = "cookie"
	} else {
		// Fallback to Authorization header (API clients)
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.logger.Debug("No JWT found in request",
				zap.String("path", r.URL.Path),
				zap.String("method", r.Method))
			return nil, "", ErrMissingAuthorization
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			s.logger.Debug("Invalid Authorization header format",
				zap.String("path", r.URL.Path),
				zap.String("header", authHeader))
			return nil, "", ErrInvalidAuthFormat
		}
		tokenString = parts[1]
		tokenSource = "header"
	}

	claims, err := s.jwksClient.ValidateToken(tokenString)
	if err != nil {
		s.logger.Debug("JWT validation failed",
			zap.Error(err),
			zap.String("path", r.URL.Path),
			zap.String("token_source", tokenSource))
		return nil, "", err
	}

	return claims, tokenString, nil
}

// RequireProjectID validates that the claims contain a project ID.
func (s *authService) RequireProjectID(claims *Claims) error {
	if claims.ProjectID == "" {
		return ErrMissingProjectID
	}
	return nil
}

// ValidateProjectIDMatch ensures the URL project ID matches the token project ID.
func (s *authService) ValidateProjectIDMatch(claims *Claims, urlProjectID string) error {
	if urlProjectID != "" && claims.ProjectID != urlProjectID {
		s.logger.Warn("Project ID mismatch",
			zap.String("url_project_id", urlProjectID),
			zap.String("token_project_id", claims.ProjectID))
		return ErrProjectIDMismatch
	}
	return nil
}

// Ensure authService implements AuthService at compile time.
var _ AuthService = (*authService)(nil)
