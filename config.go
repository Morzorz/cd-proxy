package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen                 string          `yaml:"listen"                  json:"listen"`
	AdminListen            string          `yaml:"admin_listen"            json:"admin_listen"`
	UpstreamTimeout        time.Duration   `yaml:"upstream_timeout"        json:"upstream_timeout"`
	UpstreamConnectTimeout time.Duration   `yaml:"upstream_connect_timeout" json:"upstream_connect_timeout"`
	Default                *UpstreamConfig `yaml:"default"                 json:"default"`
	Models                 []ModelConfig   `yaml:"models"                  json:"models"`
	CORS                   CORSConfig      `yaml:"cors"                    json:"cors"`
}

type ModelConfig struct {
	Name     string         `yaml:"name"     json:"name"`
	Upstream UpstreamConfig `yaml:"upstream" json:"upstream"`
}

type UpstreamConfig struct {
	URL       string `yaml:"url"        json:"url"`
	APIKey    string `yaml:"api_key"    json:"api_key"`
	ModelName string `yaml:"model_name" json:"model_name"`
}

type CORSConfig struct {
	Enabled        bool     `yaml:"enabled"         json:"enabled"`
	AllowedOrigins []string `yaml:"allowed_origins" json:"allowed_origins"`
}

type ProxyState struct {
	Config         Config
	ModelMap       map[string]ModelConfig
	DefaultUpstream *UpstreamConfig
	HTTPClient     *http.Client
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Listen == "" {
		cfg.Listen = ":48271"
	}
	if cfg.AdminListen == "" {
		cfg.AdminListen = ":59183"
	}
	if cfg.UpstreamTimeout == 0 {
		cfg.UpstreamTimeout = 120 * time.Second
	}
	if cfg.UpstreamConnectTimeout == 0 {
		cfg.UpstreamConnectTimeout = 10 * time.Second
	}

	return &cfg, nil
}

func validateConfig(cfg *Config) error {
	if len(cfg.Models) == 0 && cfg.Default == nil {
		return fmt.Errorf("at least one model or a default upstream must be configured")
	}

	if cfg.Default != nil {
		if cfg.Default.URL == "" {
			return fmt.Errorf("default upstream.url is required")
		}
		u, err := url.Parse(cfg.Default.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("default upstream.url must be a valid HTTP/HTTPS URL")
		}
		if cfg.Default.APIKey == "" {
			return fmt.Errorf("default upstream.api_key is required")
		}
	}

	seen := make(map[string]bool)
	for i, m := range cfg.Models {
		if m.Name == "" {
			return fmt.Errorf("models[%d]: name is required", i)
		}
		if seen[m.Name] {
			return fmt.Errorf("duplicate model name %q", m.Name)
		}
		seen[m.Name] = true

		if m.Upstream.URL == "" {
			return fmt.Errorf("models[%d] (%s): upstream.url is required", i, m.Name)
		}
		u, err := url.Parse(m.Upstream.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("models[%d] (%s): upstream.url must be a valid HTTP/HTTPS URL", i, m.Name)
		}
		if m.Upstream.APIKey == "" {
			return fmt.Errorf("models[%d] (%s): upstream.api_key is required", i, m.Name)
		}
	}

	return nil
}

func buildState(cfg *Config) *ProxyState {
	m := make(map[string]ModelConfig, len(cfg.Models))
	for _, mc := range cfg.Models {
		m[mc.Name] = mc
	}
	return &ProxyState{
		Config:         *cfg,
		ModelMap:       m,
		DefaultUpstream: cfg.Default,
		HTTPClient: &http.Client{
			Timeout: cfg.UpstreamTimeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   cfg.UpstreamConnectTimeout,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: cfg.UpstreamConnectTimeout,
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (s *ProxyState) modelNames() []string {
	names := make([]string, 0, len(s.ModelMap))
	for name := range s.ModelMap {
		names = append(names, name)
	}
	return names
}
