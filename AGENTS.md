# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Build & development

```bash
go build -o cd-proxy .                    # local build (macOS, requires CGO for systray)
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o cd-proxy .  # release build
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o cd-proxy.exe .
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o cd-proxy .

bash scripts/build.sh darwin               # .app bundle + .dmg
bash scripts/build.sh windows              # .exe
bash scripts/build.sh all                  # both

go test ./...                              # run all tests
go test -run TestHandleMessages -v ./...   # run specific test
```

Single Go module (`cd-proxy`, Go 1.24), dependencies: `gopkg.in/yaml.v3`, `github.com/getlantern/systray`.

## Architecture

cd-proxy is a local reverse proxy that sits between Codex Desktop (or any Anthropic API client) and third-party Anthropic-compatible backends (DeepSeek, etc.). It serves two HTTP servers:

- **Proxy server** (default `:48271`): handles `POST /v1/messages` and `GET /health`. Clients point their Anthropic base URL here.
- **Admin server** (default `:59183`): serves the embedded web dashboard and REST API for configuration, status, and live logs.

## Request flow

`POST /v1/messages` arrives at the proxy server → `app_handlers.go` increments request counter, delegates to `ProxyState.HandleMessages` in `proxy.go` → extracts `model` from JSON body → `resolveUpstream()` checks explicit model map first, falls back to `DefaultUpstream`, returns 400 if neither exists → rewrites model name in body if `model_name` differs → forwards with `x-api-key` header to upstream → relays SSE stream or JSON response back to client.

## Key design decisions

- **Hot-reloadable config**: `ProxyState` (containing config, model map, and HTTP client) is stored in an `atomic.Pointer` on `App`. Config changes swap the pointer — no server restart needed. `app_handlers.go` reads the pointer on every request.
- **Atomic config writes**: `UpdateConfig` marshals to a `.tmp` file then `os.Rename` — prevents partial reads.
- **API key masking**: `GET /api/config` always masks keys (first 4 + last 5 chars). When keys arrive as masked in `PUT /api/config`, they are preserved from the existing config rather than overwritten.
- **Platform split**: `systray.go` (build tag `darwin`) for macOS menu bar; `systray_stub.go` (build tag `!darwin`) auto-opens the browser and waits for SIGINT. `--headless` bypasses both for a pure CLI server.
- **Log ring with pub/sub**: `LogRing` is a ring buffer (capacity 2000) that also fans out new entries to subscriber channels. The `/api/logs` SSE endpoint subscribes, sends the snapshot backlog, then streams live entries.
- **Web dashboard is embedded**: `//go:embed web` in `admin.go` embeds `web/index.html`, `web/css/style.css`, `web/js/app.js` into the binary.

## Config YAML structure

See `config.example.yaml`. `models` is a list of explicit model-to-upstream mappings. `default` is the catch-all upstream. At least one must be configured. Each upstream has `url`, `api_key`, and optional `model_name` (rewrites the model field sent upstream; omit to pass through unchanged).

## File map

| File | Purpose |
|---|---|
| `main.go` | Entry point, flag parsing, config file resolution, headless/systray dispatch |
| `app.go` | `App` struct — owns both HTTP servers, atomic state, reload/update/shutdown lifecycle |
| `config.go` | Config structs, YAML loading, validation, `ProxyState` + HTTP client construction |
| `proxy.go` | Core proxy logic: body parse, upstream resolution, model rewrite, request forwarding |
| `stream.go` | SSE relay (`relayStream`) and non-stream relay (`relayNonStream`) with hop-header stripping |
| `middleware.go` | CORS middleware (proxy), `isAllowedOrigin` |
| `logring.go` | Ring buffer with pub/sub for live log streaming |
| `ringlogger.go` | `requestLogMiddleware` (proxy, extracts model from body), `simpleLogMiddleware` (admin), `statusRecorder` hijack/flush wrapper |
| `admin.go` | Admin HTTP mux, `//go:embed web`, admin CORS |
| `admin_handlers.go` | REST API: config CRUD with masking/merge, status, SSE logs, YAML import/export |
| `app_handlers.go` | Thin wrappers: `handleMessages`, `handleHealth` — counter + delegate to `ProxyState` |
| `systray.go` | macOS menu bar app (build tag: darwin) |
| `systray_stub.go` | Non-macOS stub: auto-open browser, SIGINT shutdown |
| `systray_icon.go` | Programmatic tray icon PNG generation |
| `proxy_test.go` | Tests for proxying (streaming/non-streaming), model rewriting, default upstream, CORS, config validation |
| `web/` | Dashboard SPA (embedded): `index.html`, `css/style.css`, `js/app.js` |
| `scripts/build.sh` | Cross-platform build script (`.app` + `.dmg` for macOS, `.exe` for Windows) |
