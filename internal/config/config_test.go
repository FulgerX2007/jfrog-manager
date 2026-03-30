package config

import (
	"os"
	"testing"
)

func clearEnv() {
	_ = os.Unsetenv("JFROG_URL")
	_ = os.Unsetenv("JFROG_USERNAME")
	_ = os.Unsetenv("JFROG_TOKEN")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("TIMEOUT")
}

func setRequiredEnv() {
	_ = os.Setenv("JFROG_URL", "https://example.jfrog.io")
	_ = os.Setenv("JFROG_USERNAME", "user@example.com")
	_ = os.Setenv("JFROG_TOKEN", "test-token")
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv()
	setRequiredEnv()
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
	_ = os.Setenv("JFROG_URL", "https://custom.jfrog.io")
	_ = os.Setenv("JFROG_USERNAME", "admin@example.com")
	_ = os.Setenv("JFROG_TOKEN", "my-token")
	_ = os.Setenv("PORT", "9090")
	_ = os.Setenv("TIMEOUT", "60")
	defer clearEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JFrogURL != "https://custom.jfrog.io" {
		t.Errorf("expected custom URL, got %s", cfg.JFrogURL)
	}
	if cfg.JFrogUsername != "admin@example.com" {
		t.Errorf("expected custom username, got %s", cfg.JFrogUsername)
	}
	if cfg.JFrogToken != "my-token" {
		t.Errorf("expected custom token, got %s", cfg.JFrogToken)
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
	_ = os.Setenv("JFROG_USERNAME", "user@example.com")
	_ = os.Setenv("JFROG_TOKEN", "token")
	defer clearEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing JFROG_URL")
	}
}

func TestLoad_MissingJFrogUsername(t *testing.T) {
	clearEnv()
	_ = os.Setenv("JFROG_URL", "https://example.jfrog.io")
	_ = os.Setenv("JFROG_TOKEN", "token")
	defer clearEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing JFROG_USERNAME")
	}
}

func TestLoad_MissingJFrogToken(t *testing.T) {
	clearEnv()
	_ = os.Setenv("JFROG_URL", "https://example.jfrog.io")
	_ = os.Setenv("JFROG_USERNAME", "user@example.com")
	defer clearEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing JFROG_TOKEN")
	}
}

func TestLoad_InvalidTimeout(t *testing.T) {
	clearEnv()
	setRequiredEnv()
	_ = os.Setenv("TIMEOUT", "not-a-number")
	defer clearEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid TIMEOUT")
	}
}

func TestLoad_ZeroTimeout(t *testing.T) {
	clearEnv()
	setRequiredEnv()
	_ = os.Setenv("TIMEOUT", "0")
	defer clearEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for zero TIMEOUT")
	}
}

func TestLoad_NegativeTimeout(t *testing.T) {
	clearEnv()
	setRequiredEnv()
	_ = os.Setenv("TIMEOUT", "-5")
	defer clearEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative TIMEOUT")
	}
}
