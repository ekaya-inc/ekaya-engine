package auth

import (
	"crypto/sha256"
	"net/http"

	"github.com/gorilla/sessions"
)

// Store is the global session store for OAuth flows.
// It stores temporary state during the OAuth authorization process
// (PKCE code verifier, state parameter, original URL).
var Store *sessions.CookieStore

// SessionName is the name of the OAuth session cookie.
const SessionName = "oauth-session"

// Session value keys.
const (
	SessionKeyState        = "state"
	SessionKeyCodeVerifier = "code_verifier"
	SessionKeyOriginalURL  = "original_url"
)

// InitSessionStore initializes the cookie-based session store
// for managing OAuth state during the authentication flow.
//
// The secret parameter is used to sign session cookies. It can be any
// passphrase - it will be SHA-256 hashed to derive a 32-byte key.
// The secret must be consistent across server restarts and multiple
// servers in a load-balanced deployment.
//
// The session has a short TTL (10 minutes) since it only needs
// to persist during the OAuth redirect flow.
//
// Security settings:
// - HttpOnly: true (inaccessible to JavaScript)
// - Secure: true (HTTPS only in production)
// - SameSite: Strict (prevents CSRF)
func InitSessionStore(secret string) {
	// Hash the secret to get a consistent 32-byte key
	key := sha256.Sum256([]byte(secret))

	Store = sessions.NewCookieStore(key[:])
	Store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   600, // 10 minutes (OAuth flow duration)
		HttpOnly: true,
		Secure:   true, // HTTPS only
		SameSite: http.SameSiteStrictMode,
	}
}

// GetSession retrieves the OAuth session from the request.
// Creates a new session if one doesn't exist.
func GetSession(r *http.Request) (*sessions.Session, error) {
	return Store.Get(r, SessionName)
}

// SaveSession saves the session to the response.
func SaveSession(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	return session.Save(r, w)
}

// ClearSessionValues removes OAuth-related values from the session.
// Called after successful OAuth completion.
func ClearSessionValues(session *sessions.Session) {
	delete(session.Values, SessionKeyState)
	delete(session.Values, SessionKeyCodeVerifier)
	delete(session.Values, SessionKeyOriginalURL)
}
