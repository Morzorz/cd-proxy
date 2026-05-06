package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

func main() {
	configPath := flag.String("config", "", "path to YAML config file")
	proxyAddr := flag.String("proxy-addr", "", "proxy listen address (overrides config)")
	adminAddr := flag.String("admin-addr", "", "admin web UI listen address (overrides config)")
	headless := flag.Bool("headless", false, "run without menu bar (CLI mode)")
	flag.Parse()

	configFile := resolveConfigPath(*configPath)

	cfg, err := loadConfig(configFile)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// CLI flags take precedence over config file
	pAddr := cfg.Listen
	if *proxyAddr != "" {
		pAddr = *proxyAddr
	}
	aAddr := cfg.AdminListen
	if *adminAddr != "" {
		aAddr = *adminAddr
	}

	app, err := NewApp(cfg, configFile, pAddr, aAddr)
	if err != nil {
		slog.Error("failed to initialize", "error", err)
		os.Exit(1)
	}

	if *headless {
		runHeadless(app)
		return
	}

	proxyOK := true
	adminOK := true
	if err := app.StartProxy(); err != nil {
		slog.Error("proxy start failed", "error", err)
		proxyOK = false
	}
	if err := app.StartAdmin(); err != nil {
		slog.Error("admin start failed", "error", err)
		adminOK = false
	}

	slog.Info("cd-proxy startup complete",
		"proxy", *proxyAddr,
		"proxy_ok", proxyOK,
		"admin", "http://"+*adminAddr,
		"admin_ok", adminOK,
		"config", configFile,
	)

	runSystray(app)
}

func resolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}

	// Search paths in order
	candidates := []string{
		"config.yaml",
		filepath.Join(configDir(), "config.yaml"),
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Create default config at ~/.config/cd-proxy/config.yaml
	defaultPath := filepath.Join(configDir(), "config.yaml")
	if err := createDefaultConfig(defaultPath); err != nil {
		slog.Error("failed to create default config", "path", defaultPath, "error", err)
		os.Exit(1)
	}
	slog.Info("created default config", "path", defaultPath)
	return defaultPath
}

func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "cd-proxy")
}

func createDefaultConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	defaultYAML := `# cd-proxy configuration
listen: ":48271"
admin_listen: ":59183"

# Timeouts for upstream requests
upstream_timeout: 120s
upstream_connect_timeout: 10s

# Default upstream: all requests go here
# Replace with your own API details
default:
  url: "https://api.deepseek.com/anthropic/v1/messages"
  api_key: "sk-your-api-key"
  model_name: "deepseek-v4-pro"

cors:
  enabled: true
  allowed_origins: ["*"]
`
	return os.WriteFile(path, []byte(defaultYAML), 0600)
}

func runHeadless(app *App) {
	if err := app.StartProxy(); err != nil {
		slog.Error("proxy start failed", "error", err)
		os.Exit(1)
	}
	if err := app.StartAdmin(); err != nil {
		slog.Error("admin start failed", "error", err)
	}

	slog.Info("cd-proxy started (headless)",
		"proxy", app.proxyAddr,
		"admin", "http://"+app.adminAddr,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	sig := <-sigCh
	slog.Info("shutting down", "signal", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	app.Shutdown(shutdownCtx)
	slog.Info("shutdown complete")
}
