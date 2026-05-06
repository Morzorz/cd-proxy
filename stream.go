package main

import (
	"bufio"
	"io"
	"log/slog"
	"net/http"
)

var hopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"TE":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

func relayNonStream(w http.ResponseWriter, resp *http.Response) {
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func relayStream(w http.ResponseWriter, resp *http.Response) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(resp.StatusCode)

	flusher, ok := w.(http.Flusher)
	if !ok {
		io.Copy(w, resp.Body)
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		_, err := w.Write(append(scanner.Bytes(), '\n'))
		if err != nil {
			slog.Debug("stream write failed, client likely disconnected", "error", err)
			return
		}
		flusher.Flush()
	}

	if err := scanner.Err(); err != nil {
		slog.Debug("upstream stream read error", "error", err)
	}
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		if hopHeaders[key] {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
	// Strip hop headers from downstream that might leak through
	for h := range hopHeaders {
		dst.Del(h)
	}
	dst.Del("Content-Length") // let Go set it based on body
}

func copyHeader(src, dst http.Header, key string) {
	if v := src.Get(key); v != "" {
		dst.Set(key, v)
	}
}
