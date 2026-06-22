package gateway

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/zorcal/theapp/backend/internal/telemetry"
	"github.com/zorcal/theapp/backend/pkg/slogctx"
)

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
