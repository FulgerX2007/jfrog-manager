package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	JFrogURL      string
	JFrogUsername string
	JFrogToken    string
	Port          string
	Timeout       int
	DefaultRepo   string
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, using environment variables")
	}

	cfg := Config{
		JFrogURL:      os.Getenv("JFROG_URL"),
		JFrogUsername: os.Getenv("JFROG_USERNAME"),
		JFrogToken:    os.Getenv("JFROG_TOKEN"),
		Port:          os.Getenv("PORT"),
		Timeout:       30,
	}

	if cfg.JFrogURL == "" {
		return Config{}, fmt.Errorf("JFROG_URL is required")
	}
	if cfg.JFrogUsername == "" {
		return Config{}, fmt.Errorf("JFROG_USERNAME is required")
	}
	if cfg.JFrogToken == "" {
		return Config{}, fmt.Errorf("JFROG_TOKEN is required")
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	cfg.DefaultRepo = os.Getenv("DEFAULT_REPO")

	if t := os.Getenv("TIMEOUT"); t != "" {
		val, err := strconv.Atoi(t)
		if err != nil {
			return Config{}, fmt.Errorf("TIMEOUT must be a valid integer: %w", err)
		}
		if val <= 0 {
			return Config{}, fmt.Errorf("TIMEOUT must be a positive integer")
		}
		cfg.Timeout = val
	}

	return cfg, nil
}
