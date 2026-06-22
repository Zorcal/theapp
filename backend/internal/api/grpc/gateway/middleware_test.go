package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestRecordingResponseWriter(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{
			name:   "ok",
			status: http.StatusOK,
		},
		{
			name:   "not found",
			status: http.StatusNotFound,
		},
		{
			name:   "internal server error",
			status: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rrw := &recordingResponseWriter{
				ResponseWriter: rec,
				status:         http.StatusOK,
			}

			rrw.WriteHeader(tt.status)

			if rrw.status != tt.status {
				t.Errorf("recordingResponseWriter.WriteHeader(%d) = %d, want %d", tt.status, rrw.status, tt.status)
			}
			if rec.Code != tt.status {
				t.Errorf("recordingResponseWriter.WriteHeader(%d): underlying code = %d, want %d", tt.status, rec.Code, tt.status)
			}
		})
	}
}

func TestLoggingMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.Handler
		wantStatus int
	}{
		{
			name:       "passes through 200",
			handler:    http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			wantStatus: http.StatusOK,
		},
		{
			name: "captures 404",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}),
			wantStatus: http.StatusNotFound,
		},
		{
			name: "captures 500",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}),
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := loggingMiddleware(testingx.NewLogger(t), tt.handler)

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("loggingMiddleware: status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
