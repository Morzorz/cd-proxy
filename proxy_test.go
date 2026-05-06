package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHandleMessages_NonStreaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "test-key-123" {
			t.Errorf("upstream got x-api-key = %q, want %q", got, "test-key-123")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "req-001")
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_001",
			"model": body["model"],
			"content": []map[string]any{
				{"type": "text", "text": "Hello from upstream"},
			},
		})
	}))
	defer upstream.Close()

	state := &ProxyState{
		HTTPClient: http.DefaultClient,
		ModelMap: map[string]ModelConfig{
			"claude-sonnet": {
				Name:     "claude-sonnet",
				Upstream: UpstreamConfig{URL: upstream.URL + "/v1/messages", APIKey: "test-key-123"},
			},
		},
	}

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(
		`{"model":"claude-sonnet","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	w := httptest.NewRecorder()
	state.HandleMessages(w, req)

	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, w.Body.String())
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if rid := resp.Header.Get("x-request-id"); rid != "req-001" {
		t.Errorf("x-request-id = %q, want req-001", rid)
	}
}

func TestHandleMessages_Streaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher := w.(http.Flusher)
		for _, evt := range []string{
			`data: {"type":"message_start","message":{"id":"msg_001"}}`,
			`data: {"type":"content_block_delta","delta":{"text":"Hello"}}`,
			`data: {"type":"content_block_delta","delta":{"text":" world"}}`,
			`data: {"type":"message_stop"}`,
		} {
			fmt.Fprintln(w, evt)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	state := &ProxyState{
		HTTPClient: http.DefaultClient,
		ModelMap: map[string]ModelConfig{
			"claude-sonnet": {
				Name:     "claude-sonnet",
				Upstream: UpstreamConfig{URL: upstream.URL + "/v1/messages", APIKey: "test-key-123"},
			},
		},
	}

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(
		`{"model":"claude-sonnet","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	state.HandleMessages(w, req)

	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 SSE lines, got %d: %q", len(lines), string(body))
	}
}

func TestHandleMessages_ModelRewrite(t *testing.T) {
	var upstreamGotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		upstreamGotModel = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "msg_001", "model": upstreamGotModel})
	}))
	defer upstream.Close()

	state := &ProxyState{
		HTTPClient: http.DefaultClient,
		ModelMap: map[string]ModelConfig{
			"my-claude": {
				Name: "my-claude",
				Upstream: UpstreamConfig{
					URL: upstream.URL + "/v1/messages", APIKey: "test-key",
					ModelName: "claude-sonnet-4-20250514",
				},
			},
		},
	}

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(
		`{"model":"my-claude","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	state.HandleMessages(w, req)

	if upstreamGotModel != "claude-sonnet-4-20250514" {
		t.Errorf("upstream received model = %q, want claude-sonnet-4-20250514", upstreamGotModel)
	}
}

func TestHandleMessages_UnknownModel(t *testing.T) {
	state := &ProxyState{
		HTTPClient: http.DefaultClient,
		ModelMap:   map[string]ModelConfig{},
	}

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(
		`{"model":"nonexistent","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	state.HandleMessages(w, req)

	resp := w.Result()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	var errBody map[string]any
	json.NewDecoder(resp.Body).Decode(&errBody)
	errMap, ok := errBody["error"].(map[string]any)
	if !ok || !strings.Contains(errMap["message"].(string), "unknown model") {
		t.Fatal("expected unknown model error")
	}
}

func TestHandleMessages_MissingModel(t *testing.T) {
	state := &ProxyState{
		HTTPClient: http.DefaultClient,
		ModelMap:   map[string]ModelConfig{},
	}

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(
		`{"max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	state.HandleMessages(w, req)

	if w.Result().StatusCode != 400 {
		t.Errorf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleMessages_InvalidJSON(t *testing.T) {
	state := &ProxyState{
		HTTPClient: http.DefaultClient,
		ModelMap:   map[string]ModelConfig{},
	}

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	state.HandleMessages(w, req)

	if w.Result().StatusCode != 400 {
		t.Errorf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleHealth(t *testing.T) {
	state := &ProxyState{}
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	state.HandleHealth(w, req)

	if w.Result().StatusCode != 200 {
		t.Errorf("expected 200, got %d", w.Result().StatusCode)
	}
}

func TestCORS(t *testing.T) {
	cfg := CORSConfig{Enabled: true, AllowedOrigins: []string{"*"}}

	handler := corsMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("OPTIONS", "/v1/messages", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != 204 {
		t.Errorf("OPTIONS: expected 204, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("ACAO header missing or wrong")
	}
}

func TestConfigLoad(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "cd-proxy-test-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	configYAML := `
listen: ":9999"
upstream_timeout: 60s
models:
  - name: "test-model"
    upstream:
      url: "https://api.example.com/v1/messages"
      api_key: "sk-test"
      model_name: "real-model-v2"
cors:
  enabled: true
  allowed_origins: ["*"]
`
	if _, err := tmpfile.Write([]byte(configYAML)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := loadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if cfg.Listen != ":9999" {
		t.Errorf("listen = %q, want :9999", cfg.Listen)
	}
	if cfg.UpstreamTimeout != 60*time.Second {
		t.Errorf("timeout = %v, want 60s", cfg.UpstreamTimeout)
	}
	if len(cfg.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(cfg.Models))
	}
	if cfg.Models[0].Upstream.ModelName != "real-model-v2" {
		t.Errorf("model_name = %q", cfg.Models[0].Upstream.ModelName)
	}
}

func TestConfigValidation_DuplicateModel(t *testing.T) {
	cfg := &Config{
		Listen: ":8080",
		Models: []ModelConfig{
			{Name: "m1", Upstream: UpstreamConfig{URL: "https://a.com", APIKey: "k1"}},
			{Name: "m1", Upstream: UpstreamConfig{URL: "https://b.com", APIKey: "k2"}},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for duplicate model names")
	}
}

func TestConfigValidation_EmptyModels(t *testing.T) {
	if err := validateConfig(&Config{Listen: ":8080"}); err == nil {
		t.Error("expected error for empty model list")
	}
}

func TestConfigValidation_BadURL(t *testing.T) {
	cfg := &Config{
		Listen: ":8080",
		Models: []ModelConfig{
			{Name: "m1", Upstream: UpstreamConfig{URL: "not-a-url", APIKey: "k1"}},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for bad URL")
	}
}

func TestHandleMessages_DefaultUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "default-key" {
			t.Errorf("upstream got x-api-key = %q, want %q", got, "default-key")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg_001","model":"deepseek-v4-pro","content":[{"type":"text","text":"Hi"}]}`))
	}))
	defer upstream.Close()

	state := &ProxyState{
		HTTPClient: http.DefaultClient,
		ModelMap:   map[string]ModelConfig{},
		DefaultUpstream: &UpstreamConfig{
			URL:    upstream.URL + "/v1/messages",
			APIKey: "default-key",
		},
	}

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(
		`{"model":"claude-sonnet-4-20250514","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	state.HandleMessages(w, req)

	if w.Result().StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Result().StatusCode, w.Body.String())
	}
}

func TestHandleMessages_DefaultUpstreamModelRewrite(t *testing.T) {
	var upstreamGotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		upstreamGotModel = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "msg_001", "model": upstreamGotModel})
	}))
	defer upstream.Close()

	state := &ProxyState{
		HTTPClient: http.DefaultClient,
		ModelMap:   map[string]ModelConfig{},
		DefaultUpstream: &UpstreamConfig{
			URL:       upstream.URL + "/v1/messages",
			APIKey:    "default-key",
			ModelName: "deepseek-v4-pro",
		},
	}

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(
		`{"model":"claude-sonnet-4-20250514","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	state.HandleMessages(w, req)

	if upstreamGotModel != "deepseek-v4-pro" {
		t.Errorf("upstream received model = %q, want deepseek-v4-pro", upstreamGotModel)
	}
}

func TestHandleMessages_ExplicitOverridesDefault(t *testing.T) {
	var upstreamGotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		upstreamGotModel = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "msg_001", "model": upstreamGotModel})
	}))
	defer upstream.Close()

	state := &ProxyState{
		HTTPClient: http.DefaultClient,
		ModelMap: map[string]ModelConfig{
			"claude-opus": {
				Name: "claude-opus",
				Upstream: UpstreamConfig{
					URL:       upstream.URL + "/v1/messages",
					APIKey:    "explicit-key",
					ModelName: "deepseek-v4-pro-opus",
				},
			},
		},
		DefaultUpstream: &UpstreamConfig{
			URL:       upstream.URL + "/v1/messages",
			APIKey:    "default-key",
			ModelName: "deepseek-v4-pro",
		},
	}

	// Request with explicit model → should use explicit upstream
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(
		`{"model":"claude-opus","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	state.HandleMessages(w, req)

	if upstreamGotModel != "deepseek-v4-pro-opus" {
		t.Errorf("explicit: upstream model = %q, want deepseek-v4-pro-opus", upstreamGotModel)
	}

	// Request with unknown model → should use default upstream
	req2 := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(
		`{"model":"claude-sonnet","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}`))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	state.HandleMessages(w2, req2)

	if upstreamGotModel != "deepseek-v4-pro" {
		t.Errorf("default: upstream model = %q, want deepseek-v4-pro", upstreamGotModel)
	}
}

func TestConfigLoad_DefaultOnly(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "cd-proxy-test-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	configYAML := `
listen: ":8080"
default:
  url: "https://api.deepseek.com/anthropic/v1/messages"
  api_key: "sk-deepseek"
  model_name: "deepseek-v4-pro"
`
	if _, err := tmpfile.Write([]byte(configYAML)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	cfg, err := loadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if cfg.Default == nil {
		t.Fatal("default should not be nil")
	}
	if cfg.Default.ModelName != "deepseek-v4-pro" {
		t.Errorf("default model_name = %q", cfg.Default.ModelName)
	}

	if err := validateConfig(cfg); err != nil {
		t.Errorf("should be valid with default only: %v", err)
	}
}
