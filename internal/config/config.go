package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the top-level application configuration.
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Auth     AuthConfig     `toml:"auth"`
	Logging  LoggingConfig  `toml:"logging"`
	Database DatabaseConfig `toml:"database"`
	CORS     CORSConfig     `toml:"cors"`
	Limits   LimitsConfig   `toml:"limits"`
	Accounts AccountsConfig `toml:"accounts"`
	Webhooks WebhookConfig  `toml:"webhooks"`
	Swagger  SwaggerConfig  `toml:"swagger"`
}

type ServerConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

type AuthConfig struct {
	SecretKey string `toml:"secret_key"`
}

type LoggingConfig struct {
	Level string `toml:"level"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type CORSConfig struct {
	AllowOrigins []string `toml:"allow_origins"`
	AllowMethods []string `toml:"allow_methods"`
	AllowHeaders []string `toml:"allow_headers"`
}

type LimitsConfig struct {
	MaxConcurrentRequests int   `toml:"max_concurrent_requests"`
	RequestTimeoutMs      int64 `toml:"request_timeout_ms"`
	MaxUploadSize         int64 `toml:"max_upload_size"`
}

type AccountsConfig struct {
	BaseDirectory string                `toml:"base_directory"`
	Defaults      AccountDefaultsConfig `toml:"defaults"`
}

type AccountDefaultsConfig struct {
	IdleTimeout int64 `toml:"idle_timeout"`
}

type WebhookConfig struct {
	Enabled    bool   `toml:"enabled"`
	TimeoutMs  int64  `toml:"timeout_ms"`
	RetryCount int    `toml:"retry_count"`
	RetryDelay int64  `toml:"retry_delay_ms"`
}

type SwaggerConfig struct {
	Enabled bool   `toml:"enabled"`
	Path    string `toml:"path"`
}

// Load reads config from config/app.toml (next to binary or working dir),
// falling back to sensible defaults.
func Load() (*Config, error) {
	cfg := defaults()

	// Search paths for config
	paths := []string{
		"config/app.toml",
		filepath.Join(homeDir(), ".walink", "config.toml"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			if _, err := toml.DecodeFile(p, cfg); err != nil {
				return nil, fmt.Errorf("parse %s: %w", p, err)
			}
			break
		}
	}

	// Resolve database path
	if cfg.Database.Path == "" {
		cfg.Database.Path = filepath.Join(homeDir(), ".walink", "db", "walink.db")
	}

	// Resolve accounts base directory
	if cfg.Accounts.BaseDirectory == "" {
		cfg.Accounts.BaseDirectory = filepath.Join(homeDir(), ".walink", "accounts")
	}

	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Server:  ServerConfig{Host: "0.0.0.0", Port: 3000},
		Auth:    AuthConfig{SecretKey: "change-this-secret-key-in-production"},
		Logging: LoggingConfig{Level: "info"},
		CORS: CORSConfig{
			AllowOrigins: []string{"*"},
			AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders: []string{"authorization", "content-type"},
		},
		Limits: LimitsConfig{
			MaxConcurrentRequests: 50,
			RequestTimeoutMs:      30000,
			MaxUploadSize:         10 * 1024 * 1024,
		},
		Accounts: AccountsConfig{
			Defaults: AccountDefaultsConfig{IdleTimeout: 300},
		},
		Webhooks: WebhookConfig{
			TimeoutMs:  5000,
			RetryCount: 3,
			RetryDelay: 1000,
		},
		Swagger: SwaggerConfig{Enabled: true, Path: "/api-docs"},
	}
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}
