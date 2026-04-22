package app

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func (a *App) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				a.Logger.Error("recover middleware", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := wrapResponseWriter(w)
		next.ServeHTTP(wrapped, r)
		slog.Info("http request",
			"method", r.Method,
			"status", wrapped.status,
			"path", r.URL.Path,
			"latency", time.Since(start).String(),
			"ua", r.UserAgent(),
		)
	})
}

func (a *App) authMiddleware(next http.Handler) http.Handler {
	if a.Config == nil || strings.TrimSpace(a.Config.AuthToken) == "" {
		return next
	}

	expected := strings.TrimSpace(a.Config.AuthToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queryToken := strings.TrimSpace(r.URL.Query().Get("token"))
		headerToken := strings.TrimSpace(r.Header.Get("Authorization"))
		if queryToken == expected || headerToken == expected {
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w}
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}

	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
	rw.wroteHeader = true
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
