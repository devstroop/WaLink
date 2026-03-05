package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := defaults()

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Server.Port)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected log level info, got %s", cfg.Logging.Level)
	}
	if cfg.Limits.MaxUploadSize != 10*1024*1024 {
		t.Errorf("expected max_upload_size 10MB, got %d", cfg.Limits.MaxUploadSize)
	}
	if !cfg.Swagger.Enabled {
		t.Error("expected swagger enabled by default")
	}
	if cfg.Swagger.Path != "/api-docs" {
		t.Errorf("expected swagger path /api-docs, got %s", cfg.Swagger.Path)
	}
	if cfg.Webhooks.RetryCount != 3 {
		t.Errorf("expected retry_count 3, got %d", cfg.Webhooks.RetryCount)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	os.MkdirAll(configDir, 0o755)

	content := `
[server]
host = "127.0.0.1"
port = 8080

[auth]
secret_key = "test-key"

[logging]
level = "debug"
`
	configPath := filepath.Join(configDir, "app.toml")
	os.WriteFile(configPath, []byte(content), 0o644)

	// Change to temp dir so config/app.toml is found
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Auth.SecretKey != "test-key" {
		t.Errorf("expected secret_key test-key, got %s", cfg.Auth.SecretKey)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Logging.Level)
	}

	// Defaults should still be applied for unset fields
	if cfg.Limits.MaxConcurrentRequests != 50 {
		t.Errorf("expected default max_concurrent_requests 50, got %d", cfg.Limits.MaxConcurrentRequests)
	}
}

func TestLoadNoFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Should get all defaults
	if cfg.Server.Port != 3000 {
		t.Errorf("expected default port 3000, got %d", cfg.Server.Port)
	}
}

func TestLoadInvalidToml(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "app.toml"), []byte("not valid [[[ toml"), 0o644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	_, err := Load()
	if err == nil {
		t.Error("expected error for invalid toml, got nil")
	}
}
