package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"
)

type ctxKey int

const ctxKeyBody ctxKey = iota

func cachedBody(r *http.Request) ([]byte, bool) {
	b, ok := r.Context().Value(ctxKeyBody).([]byte)
	return b, ok
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func requestLogMiddleware(logRing *LogRing, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}

		// Read body once, cache in context so downstream handler can reuse it.
		model, bodyBytes := extractModel(r)
		if bodyBytes != nil {
			r = r.WithContext(context.WithValue(r.Context(), ctxKeyBody, bodyBytes))
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		next.ServeHTTP(rec, r)

		logRing.Add(LogEntry{
			Timestamp: start,
			Level:     levelForStatus(rec.status),
			Method:    r.Method,
			Path:      r.URL.Path,
			Status:    rec.status,
			Duration:  time.Since(start).String(),
			Remote:    r.RemoteAddr,
			Model:     model,
		})
	})
}

func extractModel(r *http.Request) (model string, bodyBytes []byte) {
	if r.Method != "POST" || r.URL.Path != "/v1/messages" {
		return "", nil
	}
	var err error
	bodyBytes, err = io.ReadAll(r.Body)
	if err != nil {
		return "", nil
	}

	var body map[string]any
	if json.Unmarshal(bodyBytes, &body) != nil {
		return "", bodyBytes
	}
	model, _ = body["model"].(string)
	return model, bodyBytes
}

func levelForStatus(status int) string {
	if status >= 500 {
		return "error"
	}
	if status >= 400 {
		return "warn"
	}
	return "info"
}

// logMiddleware is a simpler version used in the admin server
func simpleLogMiddleware(logRing *LogRing, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)

		logRing.Add(LogEntry{
			Timestamp: start,
			Level:     levelForStatus(rec.status),
			Method:    r.Method,
			Path:      r.URL.Path,
			Status:    rec.status,
			Duration:  time.Since(start).String(),
			Remote:    r.RemoteAddr,
		})
	})
}
