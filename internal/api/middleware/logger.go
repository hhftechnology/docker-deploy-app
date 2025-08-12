package middleware

import (
	"log"
	"net/http"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = 200
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// Logger middleware logs HTTP requests
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap the response writer
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     0,
		}

		// Process the request
		next.ServeHTTP(wrapped, r)

		// Log the request
		duration := time.Since(start)
		log.Printf(
			"%s %s %d %d bytes %v %s",
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			wrapped.written,
			duration,
			getClientIP(r),
		)
	})
}

// JSONContentType middleware sets JSON content type for API responses
func JSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip for WebSocket upgrades
		if r.Header.Get("Upgrade") == "websocket" {
			next.ServeHTTP(w, r)
			return
		}

		// Skip for file downloads
		if r.URL.Path == "/api/backups" && r.Method == "GET" && r.URL.Query().Get("download") == "true" {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}