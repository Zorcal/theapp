package gateway

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/zorcal/theapp/backend/internal/telemetry"
	"github.com/zorcal/theapp/backend/pkg/slogctx"
)

type middleware func(http.Handler) http.Handler

func wrapMiddleware(h http.Handler, mw ...middleware) http.Handler {
	for _, v := range slices.Backward(mw) {
		h = v(h)
	}
	return h
}

func basicAuth(username, password string) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUsername, gotPassword, ok := r.BasicAuth()
			isUsernameMatch := subtle.ConstantTimeCompare([]byte(gotUsername), []byte(username)) == 1
			isPasswordMatch := subtle.ConstantTimeCompare([]byte(gotPassword), []byte(password)) == 1
			if !ok || !isUsernameMatch || !isPasswordMatch {
				w.Header().Set("WWW-Authenticate", `Basic realm="internal API docs", charset="UTF-8"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func loggingMiddleware(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		ctx := r.Context()

		if traceID := telemetry.GetTraceID(ctx); traceID != "" {
			ctx = slogctx.Attach(ctx, "trace_id", traceID)
			r = r.WithContext(ctx)
		}

		log.InfoContext(
			ctx, "HTTP Gateway Request",
			"method", r.Method,
			"path", r.URL.Path,
		)

		rrw := &recordingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rrw, r)

		log.InfoContext(
			ctx, "HTTP Gateway Response",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rrw.status,
			"duration_ms", time.Since(now).Milliseconds(),
		)
	})
}

type recordingResponseWriter struct {
	http.ResponseWriter

	status int
}

func (rrw *recordingResponseWriter) WriteHeader(code int) {
	rrw.status = code
	rrw.ResponseWriter.WriteHeader(code)
}
