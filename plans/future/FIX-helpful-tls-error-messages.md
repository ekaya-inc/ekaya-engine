# FIX: Improve TLS Handshake Error Messages

## Problem

When clients (like Claude Code) connect to ekaya-engine but don't trust the server's certificate, the server logs this cryptic message:

```
http: TLS handshake error from 127.0.0.1:49512: EOF
```

This is unhelpful because:
1. The `http:` prefix is misleading (it's Go's package name, not the protocol)
2. `EOF` doesn't explain why the handshake failed
3. Developers waste time debugging when the fix is simply setting `NODE_EXTRA_CA_CERTS`

## Solution

Intercept TLS handshake errors and emit actionable messages.

### Implementation

Set a custom `ErrorLog` on the HTTP server to intercept and enhance error messages:

```go
// pkg/middleware/tls_error_logger.go
package middleware

import (
    "io"
    "strings"
    "go.uber.org/zap"
)

// TLSErrorLogger intercepts http.Server error logs and provides helpful messages
type TLSErrorLogger struct {
    logger *zap.Logger
}

func NewTLSErrorLogger(logger *zap.Logger) *TLSErrorLogger {
    return &TLSErrorLogger{logger: logger}
}

func (t *TLSErrorLogger) Write(p []byte) (n int, err error) {
    msg := string(p)

    switch {
    case strings.Contains(msg, "TLS handshake error") && strings.Contains(msg, "EOF"):
        t.logger.Warn("TLS handshake failed - client disconnected (likely rejected our certificate)",
            zap.String("hint", "If client is Claude Code/Node.js, set NODE_EXTRA_CA_CERTS to trust this server's CA"),
            zap.String("original", strings.TrimSpace(msg)),
        )

    case strings.Contains(msg, "tls: unknown certificate"):
        t.logger.Warn("TLS handshake failed - client doesn't trust our certificate",
            zap.String("hint", "Client needs to add our CA to its trust store (NODE_EXTRA_CA_CERTS for Node.js)"),
            zap.String("original", strings.TrimSpace(msg)),
        )

    case strings.Contains(msg, "client sent an HTTP request to an HTTPS server"):
        t.logger.Warn("Client used HTTP instead of HTTPS",
            zap.String("hint", "Ensure client URL uses https:// scheme"),
            zap.String("original", strings.TrimSpace(msg)),
        )

    default:
        // Pass through other errors unchanged
        t.logger.Warn("HTTP server error", zap.String("message", strings.TrimSpace(msg)))
    }

    return len(p), nil
}

// Ensure TLSErrorLogger implements io.Writer
var _ io.Writer = (*TLSErrorLogger)(nil)
```

### Update main.go

```go
import (
    "log"
    "github.com/ekaya-inc/ekaya-engine/pkg/middleware"
)

// In server setup:
server := &http.Server{
    Addr:     cfg.BindAddr + ":" + cfg.Port,
    Handler:  handler,
    ErrorLog: log.New(middleware.NewTLSErrorLogger(logger), "", 0),
}
```

## Expected Output

Before:
```
2026/01/11 20:03:06 http: TLS handshake error from 127.0.0.1:49512: EOF
```

After:
```json
{
  "level": "warn",
  "msg": "TLS handshake failed - client disconnected (likely rejected our certificate)",
  "hint": "If client is Claude Code/Node.js, set NODE_EXTRA_CA_CERTS to trust this server's CA",
  "original": "http: TLS handshake error from 127.0.0.1:49512: EOF"
}
```

## Files to Modify

1. `pkg/middleware/tls_error_logger.go` - NEW
2. `main.go` - Set `ErrorLog` on HTTP server

## Testing

1. Start ekaya-engine with mkcert certificate
2. Attempt connection from Claude Code without `NODE_EXTRA_CA_CERTS`
3. Verify helpful error message appears in logs
4. Verify normal operation is unaffected
