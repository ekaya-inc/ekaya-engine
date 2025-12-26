package handlers

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// uuidPattern matches valid UUID v4 format
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// OAuthServerMetadata represents the OAuth 2.0 Authorization Server Metadata (RFC 8414).
type OAuthServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint,omitempty"`
	JWKSUri                           string   `json:"jwks_uri,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
}

// ProtectedResourceMetadata represents OAuth 2.0 Protected Resource Metadata (RFC 9728).
// This tells MCP clients which authorization server(s) protect this resource.
type ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
}

// supportedScopes defines the OAuth scopes supported by this server.
var supportedScopes = []string{"project:access"}

// WellKnownHandler handles /.well-known/* endpoints.
type WellKnownHandler struct {
	cfg    *config.Config
	logger *zap.Logger
}

// NewWellKnownHandler creates a new WellKnownHandler.
func NewWellKnownHandler(cfg *config.Config, logger *zap.Logger) *WellKnownHandler {
	return &WellKnownHandler{
		cfg:    cfg,
		logger: logger,
	}
}

// RegisterRoutes registers well-known endpoints.
func (h *WellKnownHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", h.OAuthDiscovery)
	// Wildcard route for path-based OAuth discovery (RFC 8414 Section 3)
	// Handles requests like /.well-known/oauth-authorization-server/mcp/{project-id}
	mux.HandleFunc("GET /.well-known/oauth-authorization-server/{path...}", h.OAuthDiscovery)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", h.ProtectedResource)
	// Wildcard route to capture resource paths like /mcp/{project-id}
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/{path...}", h.ProtectedResource)
}

// buildBaseURL constructs the base URL from the request, handling TLS and proxy headers.
func buildBaseURL(r *http.Request) *url.URL {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return &url.URL{
		Scheme: scheme,
		Host:   r.Host,
	}
}

// joinPath is a helper that wraps url.JoinPath and returns an error if path joining fails.
// This ensures we follow fail-fast principles rather than silently ignoring errors.
func joinPath(base string, elem ...string) (string, error) {
	return url.JoinPath(base, elem...)
}

// OAuthDiscovery serves the OAuth 2.0 Authorization Server Metadata (RFC 8414).
//
// Supports two ways to specify project_id:
//  1. Path-based (RFC 8414 Section 3): /.well-known/oauth-authorization-server/mcp/{project-id}
//  2. Query parameter: /.well-known/oauth-authorization-server?project_id={project-id}
//
// Query parameters:
//   - auth_url: Optional auth server URL. Must be in JWKS endpoints whitelist.
//     If provided and valid, returns discovery metadata for that auth server.
//     If provided but invalid, returns 400 error.
//     If not provided, uses default config.AuthServerURL.
//   - project_id: Optional project ID to include in authorization_endpoint.
//     When provided, the authorization_endpoint will include ?project_id={value}
//     which tells ekaya-central to skip project selection UI.
func (h *WellKnownHandler) OAuthDiscovery(w http.ResponseWriter, r *http.Request) {
	authURL := r.URL.Query().Get("auth_url")
	projectID := r.URL.Query().Get("project_id")

	// Extract project_id from path if not provided as query param
	// Path format: /mcp/{project-id}
	if projectID == "" {
		path := r.PathValue("path")
		if strings.HasPrefix(path, "mcp/") {
			candidate := strings.TrimPrefix(path, "mcp/")
			if uuidPattern.MatchString(candidate) {
				projectID = candidate
			} else if candidate != "" {
				h.logger.Warn("Invalid project_id format in OAuth discovery path",
					zap.String("path", path),
					zap.String("candidate", candidate))
				if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid project_id format"); err != nil {
					h.logger.Error("Failed to write error response", zap.Error(err))
				}
				return
			}
		}
	}
	validatedAuthURL, errMsg := h.cfg.ValidateAuthURL(authURL)

	if errMsg != "" {
		h.logger.Warn("Invalid auth_url rejected in OAuth discovery",
			zap.String("auth_url", authURL),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("error", errMsg))
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_auth_url", "Invalid auth_url: not in allowed list"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Build base URL from request (for MCP-specific endpoints)
	baseURL := buildBaseURL(r)

	// Parse the validated auth URL for proper URL building
	authBaseURL, err := url.Parse(validatedAuthURL)
	if err != nil {
		h.logger.Error("Failed to parse validated auth URL",
			zap.String("auth_url", validatedAuthURL),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "server_error", "Invalid auth server configuration"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Build authorization endpoint, including project_id if provided
	authEndpoint, err := joinPath(authBaseURL.String(), "authorize")
	if err != nil {
		h.logger.Error("Failed to build authorization endpoint URL", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "server_error", "Failed to build OAuth URLs"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}
	authEndpointURL, _ := url.Parse(authEndpoint)
	if projectID != "" {
		q := authEndpointURL.Query()
		q.Set("project_id", projectID)
		authEndpointURL.RawQuery = q.Encode()
	}

	// Build local MCP endpoints
	tokenEndpoint, err := joinPath(baseURL.String(), "mcp", "oauth", "token")
	if err != nil {
		h.logger.Error("Failed to build token endpoint URL", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "server_error", "Failed to build OAuth URLs"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	registrationEndpoint, err := joinPath(baseURL.String(), "mcp", "oauth", "register")
	if err != nil {
		h.logger.Error("Failed to build registration endpoint URL", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "server_error", "Failed to build OAuth URLs"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Build JWKS URI
	jwksURI, err := joinPath(authBaseURL.String(), ".well-known", "jwks.json")
	if err != nil {
		h.logger.Error("Failed to build JWKS URI", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "server_error", "Failed to build OAuth URLs"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	metadata := OAuthServerMetadata{
		Issuer:                validatedAuthURL,
		AuthorizationEndpoint: authEndpointURL.String(),
		TokenEndpoint:         tokenEndpoint,
		RegistrationEndpoint:  registrationEndpoint,
		JWKSUri:               jwksURI,
		ResponseTypesSupported: []string{
			"code",
		},
		GrantTypesSupported: []string{
			"authorization_code",
		},
		CodeChallengeMethodsSupported: []string{
			"S256",
		},
		ScopesSupported: supportedScopes,
		TokenEndpointAuthMethodsSupported: []string{
			"none",
		},
	}

	// Don't cache if auth_url or project_id was provided (dynamic response)
	if authURL == "" && projectID == "" {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}

	if err := WriteJSON(w, http.StatusOK, metadata); err != nil {
		h.logger.Error("Failed to encode OAuth metadata", zap.Error(err))
	}
}

// ProtectedResource serves the OAuth 2.0 Protected Resource Metadata (RFC 9728).
// This tells MCP clients which authorization server(s) protect this resource.
// Per RFC 9728, authorization_servers contains base URLs where the client should
// look for OAuth Authorization Server Metadata by appending /.well-known/oauth-authorization-server.
//
// This handler supports path-aware resource discovery. When accessed at:
//   - /.well-known/oauth-protected-resource → Returns base resource metadata
//   - /.well-known/oauth-protected-resource/mcp/{project-id} → Returns metadata
//     with authorization_servers pointing to the resource path (e.g., /mcp/{project-id}),
//     enabling project-scoped OAuth discovery without project selection UI.
func (h *WellKnownHandler) ProtectedResource(w http.ResponseWriter, r *http.Request) {
	if h.cfg.AuthServerURL == "" {
		h.logger.Error("AuthServerURL not configured")
		if err := ErrorResponse(w, http.StatusInternalServerError, "server_error", "Authorization server not configured"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Build base URL from request
	baseURL := buildBaseURL(r)

	// Extract path suffix from wildcard route (e.g., "mcp/6089f231-...")
	// PathValue returns empty string if no wildcard match
	path := r.PathValue("path")

	// Extract and validate project_id if path is an MCP resource
	var projectID string
	if strings.HasPrefix(path, "mcp/") {
		candidate := strings.TrimPrefix(path, "mcp/")
		// Validate project_id is a valid UUID to prevent path traversal or injection
		if uuidPattern.MatchString(candidate) {
			projectID = candidate
		} else if candidate != "" {
			h.logger.Warn("Invalid project_id format in protected resource path",
				zap.String("path", path),
				zap.String("candidate", candidate))
			if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid project_id format"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
	}

	// Build resource URL (includes path if present)
	var resource string
	var err error
	if path != "" {
		resource, err = joinPath(baseURL.String(), path)
		if err != nil {
			h.logger.Error("Failed to build resource URL", zap.Error(err))
			if err := ErrorResponse(w, http.StatusInternalServerError, "server_error", "Failed to build resource URL"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
	} else {
		resource = baseURL.String()
	}

	// Build authorization server base URL
	// Per RFC 9728, authorization_servers contains base URLs.
	// The client appends /.well-known/oauth-authorization-server to discover metadata.
	// If project_id is present, we include it in the path so the client requests:
	// /.well-known/oauth-authorization-server/mcp/{project-id}
	var authServer string
	if projectID != "" {
		// Include the resource path so client discovers project-scoped OAuth metadata
		authServer, err = joinPath(baseURL.String(), "mcp", projectID)
		if err != nil {
			h.logger.Error("Failed to build authorization server URL", zap.Error(err))
			if err := ErrorResponse(w, http.StatusInternalServerError, "server_error", "Failed to build authorization server URL"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
	} else {
		authServer = baseURL.String()
	}

	metadata := ProtectedResourceMetadata{
		Resource:             resource,
		AuthorizationServers: []string{authServer},
		BearerMethodsSupported: []string{
			"header",
		},
		ScopesSupported: supportedScopes,
	}

	// Don't cache if path-specific (dynamic response)
	if path == "" {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}

	if err := WriteJSON(w, http.StatusOK, metadata); err != nil {
		h.logger.Error("Failed to encode protected resource metadata", zap.Error(err))
	}
}
