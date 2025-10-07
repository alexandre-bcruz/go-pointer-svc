package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

/* ===================== req_id no contexto ===================== */

type ctxKey int

const (
	ctxKeyReqID ctxKey = iota
)

func WithReqID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyReqID, id)
}

func ReqIDFrom(ctx context.Context) string {
	if v := ctx.Value(ctxKeyReqID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func newStatusWriter(w http.ResponseWriter) *statusWriter {
	return &statusWriter{ResponseWriter: w}
}

/* ===================== Middleware de request ===================== */
func RequestLog() func(http.Handler) http.Handler {
	mustJSON := func(v any) string {
		b, _ := json.Marshal(v)
		return string(b)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get("X-Request-Id")
			if reqID == "" {
				reqID = uuid.NewString()
			}
			w.Header().Set("X-Request-Id", reqID)

			// injeta no contexto
			ctx := WithReqID(r.Context(), reqID)
			r = r.WithContext(ctx)

			start := time.Now()
			sw := newStatusWriter(w)

			next.ServeHTTP(sw, r)

			dur := time.Since(start)

			// log amigável + json; silencie /healthz
			if r.URL.Path != "/pointer/healthz" {
				msg := fmt.Sprintf("📥 %s %s", r.Method, r.URL.Path)
				fields := map[string]any{
					"event":       "http_request",
					"request_id":  reqID,
					"status":      sw.status,
					"duration_ms": dur.Milliseconds(),
					"bytes":       sw.bytes,
				}
				log.Printf("%s %s", msg, mustJSON(fields))
			}
		})
	}
}

/* ===================== Helpers HTTP ===================== */

func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		ErrJSON(w, r, http.StatusBadRequest, "invalid_json", err)
		return false
	}
	return true
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ErrJSON(w http.ResponseWriter, r *http.Request, status int, code string, err error) {
	if err == nil {
		err = fmt.Errorf(code)
	}
	reqID := ReqIDFrom(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   code,
		"message": err.Error(),
	})

	// mensagem amigável + json compacto (padronizado)
	msg := fmt.Sprintf("❌ %s %s", r.Method, r.URL.Path)
	fields := map[string]any{
		"event":      "http_error",
		"request_id": reqID,
		"status":     status,
		"code":       code,
		"error":      err.Error(),
	}
	log.Printf("%s %s", msg, mustJSON(fields))
}

/* ===================== statusWriter ===================== */

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

/* ===================== Timeouts ===================== */

func withTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

/* ===================== App logs (mensagem + JSON) ===================== */

func InfoJ(ctx context.Context, prefix, message string, kv ...any) {
	appLog(ctx, "info", prefix, message, kv...)
}
func WarnJ(ctx context.Context, prefix, message string, kv ...any) {
	appLog(ctx, "warn", prefix, message, kv...)
}
func ErrorJ(ctx context.Context, prefix, message string, kv ...any) {
	appLog(ctx, "error", prefix, message, kv...)
}

func appLog(ctx context.Context, level, prefix, message string, kv ...any) {
	reqID := ReqIDFrom(ctx)

	// JSON com os mesmos dados + extras
	m := kvToMap(kv...)
	m["event"] = "app_log"
	m["level"] = level
	if reqID != "" {
		m["request_id"] = reqID
	}
	log.Printf("%s %s %s", prefix, message, mustJSON(m))
}

func kvToMap(kv ...any) map[string]any {
	m := make(map[string]any, len(kv)/2+4)
	for i := 0; i+1 < len(kv); i += 2 {
		k := fmt.Sprint(kv[i])
		m[k] = kv[i+1]
	}
	return m
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"marshal_error":%q}`, err.Error())
	}
	return string(b)
}
