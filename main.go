package main

import (
	"log/slog"
	"os"

	"jfrog_manager/internal/config"
	"jfrog_manager/internal/handlers"
	"jfrog_manager/internal/jfrog"
	"jfrog_manager/internal/templates"

	"github.com/gin-gonic/gin"
)

func main() {
	// Default gin to release mode unless explicitly overridden by GIN_MODE.
	// Debug mode prints every registered route and full request logs, which
	// is noisy in production and leaks URL query params into stdout.
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	client := jfrog.NewClient(cfg)

	tmpl, err := templates.Load("templates")
	if err != nil {
		slog.Error("failed to load templates", "error", err)
		os.Exit(1)
	}

	h := handlers.NewHandler(client, tmpl, cfg.DefaultRepo)
	r := setupRouter(h)

	slog.Info("starting server", "port", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func setupRouter(h handlers.Handler) *gin.Engine {
	r := gin.Default()
	// Spill uploads >32 MB to disk rather than buffering whole files in RAM.
	// Individual upload size is still capped at 500 MB by the handler.
	r.MaxMultipartMemory = 32 << 20

	r.Use(securityHeaders)

	r.GET("/", h.Index)
	r.GET("/repos", h.ListRepos)
	r.GET("/artifacts", h.ListArtifacts)
	r.POST("/artifacts/upload", h.UploadArtifact)
	r.POST("/artifacts/bulk-delete", h.BulkDeleteArtifacts)
	r.GET("/artifacts/download", h.DownloadArtifact)
	r.DELETE("/artifacts", h.DeleteArtifact)
	r.GET("/xray", h.GetXray)

	return r
}

// securityHeaders sets conservative response headers on every request:
//   - nosniff prevents browsers from overriding our Content-Disposition on downloads
//   - DENY frames blocks clickjacking embeds
//   - no-referrer keeps internal paths out of outbound Referer headers
func securityHeaders(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Referrer-Policy", "no-referrer")
	c.Next()
}
