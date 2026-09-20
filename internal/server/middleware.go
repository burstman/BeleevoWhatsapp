package server

import (
	"net/http"
	"strings"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// shouldLogRequest filters high-frequency noise (health checks, static assets)
// from the request log.
func shouldLogRequest(path string) bool {
	if path == "/healthz" || path == "/readyz" {
		return false
	}
	if strings.HasPrefix(path, "/static/") {
		return false
	}
	return true
}

func (a *App) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := chimiddleware.GetReqID(r.Context())

		logger := a.Log.With(
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
		)

		// Capture the response status for the log line.
		rr := newStatusRecorder(w)
		next.ServeHTTP(rr, r)

		if !shouldLogRequest(r.URL.Path) {
			return
		}

		logger.Info("request",
			"status", rr.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
