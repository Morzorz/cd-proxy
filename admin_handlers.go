package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"gopkg.in/yaml.v3"
)

// GET /api/config
func (a *App) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	state := a.CurrentState()
	cfg := state.Config

	// Mask API keys
	masked := maskConfig(&cfg)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(masked)
}

// PUT /api/config
func (a *App) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "failed to read body"})
		return
	}

	var incoming Config
	if err := json.Unmarshal(body, &incoming); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}

	// Merge: start from current config, overlay incoming non-zero fields
	state := a.CurrentState()
	merged := state.Config

	if incoming.Listen != "" {
		merged.Listen = incoming.Listen
	}
	if incoming.AdminListen != "" {
		merged.AdminListen = incoming.AdminListen
	}
	if incoming.UpstreamTimeout != 0 {
		merged.UpstreamTimeout = incoming.UpstreamTimeout
	}
	if incoming.UpstreamConnectTimeout != 0 {
		merged.UpstreamConnectTimeout = incoming.UpstreamConnectTimeout
	}
	if incoming.Default != nil {
		if incoming.Default.APIKey != "" && !isMasked(incoming.Default.APIKey) {
			merged.Default.APIKey = incoming.Default.APIKey
		} else if merged.Default != nil && incoming.Default.APIKey != "" {
			// Keep existing key
		}
		if incoming.Default.URL != "" {
			if merged.Default == nil {
				merged.Default = &UpstreamConfig{}
			}
			merged.Default.URL = incoming.Default.URL
		}
		if incoming.Default.ModelName != "" {
			if merged.Default == nil {
				merged.Default = &UpstreamConfig{}
			}
			merged.Default.ModelName = incoming.Default.ModelName
		}
		// If incoming.Default is empty or being unset
		if incoming.Default.URL == "" && incoming.Default.ModelName == "" && incoming.Default.APIKey == "" {
			merged.Default = nil
		}
	}

	if incoming.Models != nil {
		merged.Models = make([]ModelConfig, len(incoming.Models))
		for i, im := range incoming.Models {
			if i < len(state.Config.Models) && im.Upstream.APIKey != "" && isMasked(im.Upstream.APIKey) {
				im.Upstream.APIKey = state.Config.Models[i].Upstream.APIKey
			}
			merged.Models[i] = im
		}
	}

	if err := a.UpdateConfig(&merged); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	a.handleGetConfig(w, r)
}

// GET /api/status
func (a *App) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.Status())
}

// GET /api/logs (SSE)
func (a *App) handleLogsSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	// Send initial backlog
	entries := a.logRing.Snapshot()
	for _, e := range entries {
		writeSSEEvent(w, flusher, e)
	}

	// Subscribe to live log entries
	ch := a.logRing.Subscribe()
	defer a.logRing.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			writeSSEEvent(w, flusher, entry)
		}
	}
}

// GET /api/logs/recent
func (a *App) handleGetRecentLogs(w http.ResponseWriter, r *http.Request) {
	entries := a.logRing.Snapshot()
	if entries == nil {
		entries = []LogEntry{}
	}
	writeJSON(w, 200, entries)
}

// POST /api/config/import
func (a *App) handleImportConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "failed to read body"})
		return
	}

	var cfg Config
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		writeJSON(w, 400, map[string]string{"error": fmt.Sprintf("invalid YAML: %v", err)})
		return
	}

	if err := a.UpdateConfig(&cfg); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	slog.Info("config imported")
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// GET /api/config/export
func (a *App) handleExportConfig(w http.ResponseWriter, r *http.Request) {
	state := a.CurrentState()
	data, err := yaml.Marshal(&state.Config)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to marshal config"})
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", "attachment; filename=config.yaml")
	w.Write(data)
}

func maskConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	masked := *cfg
	if masked.Default != nil {
		dup := *masked.Default
		dup.APIKey = maskAPIKey(dup.APIKey)
		masked.Default = &dup
	}
	masked.Models = make([]ModelConfig, len(cfg.Models))
	for i, m := range cfg.Models {
		masked.Models[i] = m
		masked.Models[i].Upstream.APIKey = maskAPIKey(m.Upstream.APIKey)
	}
	return &masked
}

func maskAPIKey(key string) string {
	if len(key) <= 10 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-5:]
}

func isMasked(key string) bool {
	return strings.Contains(key, "...")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeSSEEvent(w io.Writer, flusher http.Flusher, entry LogEntry) {
	data, _ := json.Marshal(entry)
	fmt.Fprintf(w, "event: log\ndata: %s\n\n", data)
	flusher.Flush()
}
