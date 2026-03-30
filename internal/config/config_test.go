package config

import (
	"os"
	"testing"
)

func clearEnv() {
	os.Unsetenv("JFROG_URL")
	os.Unsetenv("JFROG_API_KEY")
	os.Unsetenv("PORT")
	os.Unsetenv("TIMEOUT")
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv()
	os.Setenv("JFROG_URL", "https://example.jfrog.io")
	os.Setenv("JFROG_API_KEY", "test-key")
	defer clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %s", cfg.Port)
	}
	if cfg.Timeout != 30 {
		t.Errorf("expected default timeout 30, got %d", cfg.Timeout)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	clearEnv()
	os.Setenv("JFROG_URL", "https://custom.jfrog.io")
	os.Setenv("JFROG_API_KEY", "my-key")
	os.Setenv("PORT", "9090")
	os.Setenv("TIMEOUT", "60")
	defer clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JFrogURL != "https://custom.jfrog.io" {
		t.Errorf("expected custom URL, got %s", cfg.JFrogURL)
	}
	if cfg.JFrogAPIKey != "my-key" {
		t.Errorf("expected custom API key, got %s", cfg.JFrogAPIKey)
	}
	if cfg.Port != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.Port)
	}
	if cfg.Timeout != 60 {
		t.Errorf("expected timeout 60, got %d", cfg.Timeout)
	}
}

func TestLoad_MissingJFrogURL(t *testing.T) {
	clearEnv()
	os.Setenv("JFROG_API_KEY", "test-key")
	defer clearEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing JFROG_URL")
	}
}

func TestLoad_MissingJFrogAPIKey(t *testing.T) {
	clearEnv()
	os.Setenv("JFROG_URL", "https://example.jfrog.io")
	defer clearEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing JFROG_API_KEY")
	}
}

func TestLoad_InvalidTimeout(t *testing.T) {
	clearEnv()
	os.Setenv("JFROG_URL", "https://example.jfrog.io")
	os.Setenv("JFROG_API_KEY", "test-key")
	os.Setenv("TIMEOUT", "not-a-number")
	defer clearEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid TIMEOUT")
	}
}
