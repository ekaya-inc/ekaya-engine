package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// JWKSClientInterface defines the interface for JWT token validation.
// This abstraction enables testing with mock implementations.
type JWKSClientInterface interface {
	// ValidateToken validates a JWT token string and returns the claims.
	// Returns an error if the token is invalid, expired, or has an unauthorized issuer.
	ValidateToken(tokenString string) (*Claims, error)
	// Close releases any resources held by the client.
	Close()
}

// JWKSConfig contains configuration for the JWKS client.
type JWKSConfig struct {
	// EnableVerification controls whether JWT signatures are verified.
	// Set to false for development mode (parses tokens without verification).
	EnableVerification bool
	// JWKSEndpoints maps issuer URLs to their JWKS endpoint URLs.
	// Only tokens from issuers in this map are accepted.
	JWKSEndpoints map[string]string
}

// JWKSClient validates JWT tokens using JWKS (JSON Web Key Set) endpoints.
// It fetches public keys from configured JWKS URLs and uses them to verify
// JWT signatures. Only tokens from whitelisted issuers are accepted.
type JWKSClient struct {
	endpoints map[string]keyfunc.Keyfunc
	config    *JWKSConfig
}

// NewJWKSClient creates a new JWKS client with the given configuration.
// If EnableVerification is true, it fetches JWKS from all configured endpoints.
// Returns an error if any JWKS endpoint fails to load.
func NewJWKSClient(config *JWKSConfig) (*JWKSClient, error) {
	client := &JWKSClient{
		endpoints: make(map[string]keyfunc.Keyfunc),
		config:    config,
	}

	if !config.EnableVerification {
		return client, nil
	}

	for issuer, jwksURL := range config.JWKSEndpoints {
		jwks, err := keyfunc.NewDefaultCtx(context.Background(), []string{jwksURL})
		if err != nil {
			return nil, fmt.Errorf("failed to create JWKS client for %s: %w", issuer, err)
		}
		client.endpoints[issuer] = jwks
	}

	return client, nil
}

// ValidateToken validates a JWT token and returns the claims.
// If verification is disabled, it parses the token without signature validation.
// Otherwise, it verifies the RSA signature using the issuer's JWKS public keys.
func (c *JWKSClient) ValidateToken(tokenString string) (*Claims, error) {
	if !c.config.EnableVerification {
		return c.parseUnverifiedToken(tokenString)
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method is RSA (ekaya-central uses RS256)
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		claims, ok := token.Claims.(*Claims)
		if !ok {
			return nil, errors.New("invalid claims type")
		}

		// Look up JWKS for this issuer
		jwks, exists := c.endpoints[claims.Issuer]
		if !exists {
			return nil, fmt.Errorf("unauthorized issuer: %s", claims.Issuer)
		}

		// Get the key function for signature verification
		keyfuncFn := jwks.KeyfuncCtx(context.Background())
		return keyfuncFn(token)
	})

	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid claims type")
	}

	return claims, nil
}

// parseUnverifiedToken parses a JWT without verifying the signature.
// Used in development mode when EnableVerification is false.
func (c *JWKSClient) parseUnverifiedToken(tokenString string) (*Claims, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid claims type")
	}

	return claims, nil
}

// Close releases any resources held by the client.
// Currently a no-op as keyfunc v3 doesn't require explicit cleanup.
func (c *JWKSClient) Close() {
	// No cleanup required with keyfunc v3
}

// Ensure JWKSClient implements JWKSClientInterface at compile time.
var _ JWKSClientInterface = (*JWKSClient)(nil)
