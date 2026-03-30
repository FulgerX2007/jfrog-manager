package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetXray returns the Xray vulnerability panel fragment for a given artifact.
func (h Handler) GetXray(c *gin.Context) {
	repo := c.Query("repo")
	path := c.Query("path")
	if repo == "" || path == "" {
		h.renderError(c, "Repository and path are required")
		return
	}
	if strings.Contains(repo, "..") || strings.Contains(path, "..") {
		h.renderError(c, "Invalid repository or path")
		return
	}

	summary, err := h.service.GetXraySummary(repo, path)
	if err != nil {
		slog.Error("getting xray summary", "error", err)
		h.renderError(c, "Failed to load Xray data: "+err.Error())
		return
	}

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(c.Writer, "xray_panel", summary); err != nil {
		slog.Error("rendering xray panel", "error", err)
	}
}
