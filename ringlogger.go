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

type bodyRecorder struct {
	http.ResponseWriter
	status  int
	body    bytes.Buffer
	maxBody int
}

func (r *bodyRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *bodyRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	if r.body.Len() < r.maxBody {
		remaining := r.maxBody - r.body.Len()
		if n > remaining {
			r.body.Write(b[:remaining])
		} else {
			r.body.Write(b[:n])
		}
	}
	return n, err
}

func (r *bodyRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (r *bodyRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func requestLogMiddleware(logRing *LogRing, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &bodyRecorder{ResponseWriter: w, status: 200, maxBody: 8 * 1024}

		model, bodyBytes := extractModel(r)
		if bodyBytes != nil {
			r = r.WithContext(context.WithValue(r.Context(), ctxKeyBody, bodyBytes))
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		next.ServeHTTP(rec, r)

		logRing.Add(LogEntry{
			Timestamp:    start,
			Level:        levelForStatus(rec.status),
			Source:       "proxy",
			Method:       r.Method,
			Path:         r.URL.Path,
			Status:       rec.status,
			Duration:     time.Since(start).String(),
			Remote:       r.RemoteAddr,
			Model:        model,
			RequestBody:  truncateBytesToString(bodyBytes, 8*1024),
			ResponseBody: truncateBytesToString(rec.body.Bytes(), 8*1024),
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

func truncateBytesToString(b []byte, maxSize int) string {
	if len(b) == 0 {
		return ""
	}
	if len(b) > maxSize {
		return string(b[:maxSize]) + "...[truncated]"
	}
	return string(b)
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
			Source:    "admin",
			Method:    r.Method,
			Path:      r.URL.Path,
			Status:    rec.status,
			Duration:  time.Since(start).String(),
			Remote:    r.RemoteAddr,
		})
	})
}
