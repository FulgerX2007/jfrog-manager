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
	r.MaxMultipartMemory = 500 << 20 // 500 MB

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
