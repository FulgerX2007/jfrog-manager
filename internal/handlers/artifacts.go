package handlers

import (
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"jfrog_manager/internal/jfrog"

	"github.com/gin-gonic/gin"
)

// Handler holds the JFrog service and templates needed by HTTP handlers.
type Handler struct {
	service     jfrog.Service
	tmpl        *template.Template
	defaultRepo string
}

// NewHandler creates a new Handler with the given JFrog service and templates.
func NewHandler(service jfrog.Service, tmpl *template.Template, defaultRepo string) Handler {
	return Handler{
		service:     service,
		tmpl:        tmpl,
		defaultRepo: defaultRepo,
	}
}

// Index renders the full index page with layout.
func (h Handler) Index(c *gin.Context) {
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{"DefaultRepo": h.defaultRepo}
	if err := h.tmpl.ExecuteTemplate(c.Writer, "layout", data); err != nil {
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

	defaultRepo := c.Query("default")

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	for _, repo := range repos {
		selected := ""
		if repo.Key == defaultRepo {
			selected = " selected"
		}
		_, _ = c.Writer.WriteString(`<option value="` + template.HTMLEscapeString(repo.Key) + `"` + selected + `>` + template.HTMLEscapeString(repo.Key) + ` (` + template.HTMLEscapeString(repo.PackageType) + `)</option>`)
	}
}

// ListArtifacts returns the artifact list table fragment for a given repository.
func (h Handler) ListArtifacts(c *gin.Context) {
	repo := c.Query("repo")
	if repo == "" {
		h.renderError(c, "Repository parameter is required")
		return
	}
	if strings.Contains(repo, "..") {
		h.renderError(c, "Invalid repository")
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
	const maxUploadSize = 500 << 20 // 500 MB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)

	// Parse the multipart form explicitly so we can detect MaxBytesError
	// before reading individual fields. Without this, PostForm triggers
	// ParseMultipartForm which silently swallows the size error and returns
	// empty strings, causing a misleading "required" validation message.
	if err := c.Request.ParseMultipartForm(maxUploadSize); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.Status(http.StatusRequestEntityTooLarge)
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Header("HX-Retarget", "#error-container")
			c.Header("HX-Reswap", "innerHTML")
			data := map[string]any{"Error": "File exceeds the 500 MB upload limit"}
			if tmplErr := h.tmpl.ExecuteTemplate(c.Writer, "error", data); tmplErr != nil {
				slog.Error("rendering error template", "error", tmplErr)
				c.String(http.StatusInternalServerError, "File too large")
			}
			return
		}
		h.renderError(c, "Failed to parse upload")
		return
	}

	repo := c.PostForm("repo")
	folder := strings.Trim(c.PostForm("folder"), "/ ")
	if repo == "" {
		h.renderError(c, "Repository is required")
		return
	}
	if strings.Contains(repo, "..") || strings.Contains(folder, "..") {
		h.renderError(c, "Invalid repository or path")
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		h.renderError(c, "File is required")
		return
	}
	defer func() { _ = file.Close() }()

	filename := path.Base(header.Filename)
	if filename == "" || filename == "." || filename == "/" {
		h.renderError(c, "Invalid file name")
		return
	}

	uploadPath := filename
	if folder != "" {
		uploadPath = folder + "/" + filename
	}

	if err := h.service.UploadArtifact(repo, uploadPath, file); err != nil {
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

// BulkDeleteArtifacts deletes multiple artifacts and re-renders the artifact list.
func (h Handler) BulkDeleteArtifacts(c *gin.Context) {
	var req struct {
		Repo  string   `json:"repo"`
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		h.renderError(c, "Invalid request body")
		return
	}
	if req.Repo == "" || len(req.Paths) == 0 {
		h.renderError(c, "Repository and at least one path are required")
		return
	}
	if strings.Contains(req.Repo, "..") {
		h.renderError(c, "Invalid repository")
		return
	}

	var errs []string
	for _, p := range req.Paths {
		if strings.Contains(p, "..") {
			errs = append(errs, "invalid path: "+p)
			continue
		}
		if err := h.service.DeleteArtifact(req.Repo, p); err != nil {
			slog.Error("bulk deleting artifact", "path", p, "error", err)
			errs = append(errs, p)
		}
	}

	if len(errs) > 0 {
		h.renderError(c, "Failed to delete: "+strings.Join(errs, ", "))
		return
	}

	artifacts, err := h.service.ListArtifacts(req.Repo)
	if err != nil {
		slog.Error("listing artifacts after bulk delete", "error", err)
		h.renderError(c, "Delete succeeded but failed to refresh list")
		return
	}

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{"Artifacts": artifacts}
	if err := h.tmpl.ExecuteTemplate(c.Writer, "artifact_list", data); err != nil {
		slog.Error("rendering artifact list", "error", err)
	}
}

// renderError renders the error template fragment.
// It uses HX-Retarget to ensure errors display in the error container,
// not inside whatever element triggered the request.
func (h Handler) renderError(c *gin.Context, message string) {
	c.Status(http.StatusUnprocessableEntity)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("HX-Retarget", "#error-container")
	c.Header("HX-Reswap", "innerHTML")
	data := map[string]any{"Error": message}
	if err := h.tmpl.ExecuteTemplate(c.Writer, "error", data); err != nil {
		slog.Error("rendering error template", "error", err)
		c.String(http.StatusInternalServerError, message)
	}
}
