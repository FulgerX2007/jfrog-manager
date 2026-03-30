package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	JFrogURL    string
	JFrogAPIKey string
	Port        string
	Timeout     int
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, using environment variables")
	}

	cfg := Config{
		JFrogURL:    os.Getenv("JFROG_URL"),
		JFrogAPIKey: os.Getenv("JFROG_API_KEY"),
		Port:        os.Getenv("PORT"),
		Timeout:     30,
	}

	if cfg.JFrogURL == "" {
		return Config{}, fmt.Errorf("JFROG_URL is required")
	}
	if cfg.JFrogAPIKey == "" {
		return Config{}, fmt.Errorf("JFROG_API_KEY is required")
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

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
