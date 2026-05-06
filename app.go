package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

type App struct {
	state        atomic.Pointer[ProxyState]
	proxySrv     *http.Server
	adminSrv     *http.Server
	logRing      *LogRing
	configPath   string
	proxyAddr    string
	adminAddr    string
	startTime    time.Time
	requestCount atomic.Int64
	transport    *http.Transport // reused across config reloads

	mu sync.Mutex // serializes start/stop
}

type AppStatus struct {
	Running            bool     `json:"running"`
	UptimeSeconds      int64    `json:"uptime_seconds"`
	ProxyAddr          string   `json:"proxy_addr"`
	AdminAddr          string   `json:"admin_addr"`
	RequestCount       int64    `json:"request_count"`
	ActiveModels       []string `json:"active_models"`
	DefaultUpstreamURL string   `json:"default_upstream_url,omitempty"`
	HasDefaultUpstream bool     `json:"has_default_upstream"`
	ConfigPath         string   `json:"config_path"`
}

func NewApp(cfg *Config, configPath, proxyAddr, adminAddr string) (*App, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	state := buildState(cfg, nil)

	app := &App{
		logRing:    NewLogRing(2000),
		configPath: configPath,
		proxyAddr:  proxyAddr,
		adminAddr:  adminAddr,
		startTime:  time.Now(),
		transport:  state.HTTPClient.Transport.(*http.Transport),
	}
	app.state.Store(state)
	return app, nil
}

func (a *App) CurrentState() *ProxyState {
	return a.state.Load()
}

func (a *App) Status() AppStatus {
	state := a.CurrentState()
	s := AppStatus{
		Running:            a.IsRunning(),
		UptimeSeconds:      int64(time.Since(a.startTime).Seconds()),
		ProxyAddr:          a.proxyAddr,
		AdminAddr:          a.adminAddr,
		RequestCount:       a.requestCount.Load(),
		ActiveModels:       state.modelNames(),
		HasDefaultUpstream: state.DefaultUpstream != nil,
		ConfigPath:         a.configPath,
	}
	if state.DefaultUpstream != nil {
		s.DefaultUpstreamURL = state.DefaultUpstream.URL
	}
	return s
}

func (a *App) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.proxySrv != nil
}

func (a *App) ReloadConfig() error {
	cfg, err := loadConfig(a.configPath)
	if err != nil {
		return err
	}
	if err := validateConfig(cfg); err != nil {
		return err
	}

	state := buildState(cfg, a.transport)
	a.state.Store(state)
	slog.Info("config reloaded")
	return nil
}

func (a *App) UpdateConfig(cfg *Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmpPath := a.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmpPath, a.configPath); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}

	state := buildState(cfg, a.transport)
	a.state.Store(state)
	slog.Info("config updated and reloaded")
	return nil
}

func (a *App) StartProxy() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.proxySrv != nil {
		return fmt.Errorf("proxy already running")
	}

	ln, err := net.Listen("tcp", a.proxyAddr)
	if err != nil {
		return fmt.Errorf("proxy listen %s: %w", a.proxyAddr, err)
	}

	state := a.CurrentState()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", a.handleMessages)
	mux.HandleFunc("GET /health", a.handleHealth)

	var handler http.Handler = mux
	handler = requestLogMiddleware(a.logRing, handler)
	handler = corsMiddleware(state.Config.CORS, handler)

	a.proxySrv = &http.Server{
		Handler: handler,
		BaseContext: func(l net.Listener) context.Context {
			return context.Background()
		},
		ReadTimeout:  30 * time.Second,
		WriteTimeout: state.Config.UpstreamTimeout,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("proxy started", "addr", a.proxyAddr)
		if err := a.proxySrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("proxy error", "error", err)
		}
	}()
	return nil
}

func (a *App) StopProxy(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.proxySrv == nil {
		return nil
	}

	slog.Info("stopping proxy")
	err := a.proxySrv.Shutdown(ctx)
	a.proxySrv = nil
	return err
}

func (a *App) StartAdmin() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.adminSrv != nil {
		return fmt.Errorf("admin already running")
	}

	ln, err := net.Listen("tcp", a.adminAddr)
	if err != nil {
		return fmt.Errorf("admin listen %s: %w", a.adminAddr, err)
	}

	a.adminSrv = &http.Server{
		Handler: newAdminMux(a),
		BaseContext: func(l net.Listener) context.Context {
			return context.Background()
		},
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("admin started", "addr", a.adminAddr)
		if err := a.adminSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("admin error", "error", err)
		}
	}()
	return nil
}

func (a *App) StopAdmin(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.adminSrv == nil {
		return nil
	}

	slog.Info("stopping admin")
	err := a.adminSrv.Shutdown(ctx)
	a.adminSrv = nil
	return err
}

func (a *App) Shutdown(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := a.StopProxy(ctx); err != nil {
			slog.Error("proxy shutdown error", "error", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := a.StopAdmin(ctx); err != nil {
			slog.Error("admin shutdown error", "error", err)
		}
	}()
	wg.Wait()
}
