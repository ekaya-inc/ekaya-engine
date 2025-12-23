package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// RequestLogger returns middleware that logs HTTP requests at DEBUG level.
// Pass nil logger to disable logging (makes it optional/injectable).
func RequestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// If no logger provided, pass through without logging
		if logger == nil {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			logger.Debug("HTTP request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", wrapped.statusCode),
				zap.Duration("duration", time.Since(start)),
				zap.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func (rw *responseWriter) WriteHeader(code int) {
	// Prevent duplicate WriteHeader calls to avoid "superfluous response.WriteHeader" warnings
	if rw.headerWritten {
		return
	}
	rw.headerWritten = true
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write implements http.ResponseWriter. It ensures WriteHeader is called with 200
// if no explicit status code was set, matching the standard library behavior.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.headerWritten {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}
