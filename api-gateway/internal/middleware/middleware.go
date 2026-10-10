// Package middleware holds cross-cutting HTTP concerns for the gateway.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Gayley007/event-ticketing/api-gateway/internal/apierror"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Logging logs method, path, status and latency for every request.
func Logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("http request", "method", r.Method, "path", r.URL.RequestURI(), "status", rec.status, "duration", time.Since(start).Round(time.Millisecond))
	})
}

// Timeout bounds every request, including the downstream gRPC calls (the
// deadline travels with the context), so the gateway never hangs.
func Timeout(d time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Recover turns a panic into a 500 instead of dropping the connection.
func Recover(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Error("panic while handling request", "panic", v, "path", r.URL.Path)
				apierror.Write(w, http.StatusInternalServerError, apierror.Body{Error: "internal server error", Code: "INTERNAL"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
