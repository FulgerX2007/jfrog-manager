package handlers

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"jfrog_manager/internal/jfrog"

	"github.com/gin-gonic/gin"
)

// Handler holds the JFrog service and templates needed by HTTP handlers.
type Handler struct {
	service jfrog.Service
	tmpl    *template.Template
}

// NewHandler creates a new Handler with the given JFrog service and templates.
func NewHandler(service jfrog.Service, tmpl *template.Template) Handler {
	return Handler{
		service: service,
		tmpl:    tmpl,
	}
}

// Index renders the full index page with layout.
func (h Handler) Index(c *gin.Context) {
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(c.Writer, "layout", nil); err != nil {
		slog.Error("rendering index", "error", err)
	}
}

// ListRepos returns HTML option elements for the repository dropdown.
func (h Handler) ListRepos(c *gin.Context) {
	repos, err := h.service.ListRepos()
	if err != nil {
		slog.Error("listing repos", "error", err)
		h.renderError(c, "Failed to load repositories")
		return
	}

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	for _, repo := range repos {
		c.Writer.WriteString(`<option value="` + template.HTMLEscapeString(repo.Key) + `">` + template.HTMLEscapeString(repo.Key) + ` (` + template.HTMLEscapeString(repo.PackageType) + `)</option>`)
	}
}

// ListArtifacts returns the artifact list table fragment for a given repository.
func (h Handler) ListArtifacts(c *gin.Context) {
	repo := c.Query("repo")
	if repo == "" {
		h.renderError(c, "Repository parameter is required")
		return
	}

	artifacts, err := h.service.ListArtifacts(repo)
	if err != nil {
		slog.Error("listing artifacts", "error", err)
		h.renderError(c, "Failed to load artifacts")
		return
	}

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{"Artifacts": artifacts}
	if err := h.tmpl.ExecuteTemplate(c.Writer, "artifact_list", data); err != nil {
		slog.Error("rendering artifact list", "error", err)
	}
}

// UploadArtifact handles multipart file upload and re-renders the artifact list.
func (h Handler) UploadArtifact(c *gin.Context) {
	repo := c.PostForm("repo")
	path := c.PostForm("path")
	if repo == "" || path == "" {
		h.renderError(c, "Repository and path are required")
		return
	}
	if strings.Contains(repo, "..") || strings.Contains(path, "..") {
		h.renderError(c, "Invalid repository or path")
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		h.renderError(c, "File is required")
		return
	}
	defer file.Close()

	if err := h.service.UploadArtifact(repo, path, file); err != nil {
		slog.Error("uploading artifact", "error", err)
		h.renderError(c, "Upload failed")
		return
	}

	// Re-render the artifact list after successful upload
	artifacts, err := h.service.ListArtifacts(repo)
	if err != nil {
		slog.Error("listing artifacts after upload", "error", err)
		h.renderError(c, "Upload succeeded but failed to refresh list")
		return
	}

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{"Artifacts": artifacts}
	if err := h.tmpl.ExecuteTemplate(c.Writer, "artifact_list", data); err != nil {
		slog.Error("rendering artifact list", "error", err)
	}
}

// DeleteArtifact deletes an artifact and returns an empty response for htmx row removal.
func (h Handler) DeleteArtifact(c *gin.Context) {
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

	if err := h.service.DeleteArtifact(repo, path); err != nil {
		slog.Error("deleting artifact", "error", err)
		h.renderError(c, "Delete failed")
		return
	}

	// Return empty response — htmx will remove the row via hx-swap="outerHTML"
	c.Status(http.StatusOK)
}

// renderError renders the error template fragment.
// It uses HX-Retarget to ensure errors display in the error container,
// not inside whatever element triggered the request.
func (h Handler) renderError(c *gin.Context, message string) {
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("HX-Retarget", "#error-container")
	c.Header("HX-Reswap", "innerHTML")
	data := map[string]any{"Error": message}
	if err := h.tmpl.ExecuteTemplate(c.Writer, "error", data); err != nil {
		slog.Error("rendering error template", "error", err)
		c.String(http.StatusInternalServerError, message)
	}
}
