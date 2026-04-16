package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jfrog_manager/internal/handlers"
	"jfrog_manager/internal/models"
	"jfrog_manager/internal/templates"

	"github.com/gin-gonic/gin"
)

// mockService implements jfrog.Service for integration tests.
type mockService struct{}

func (m mockService) ListRepos() ([]models.Repository, error) {
	return []models.Repository{
		{Key: "libs-release", Type: "LOCAL", PackageType: "maven", Description: "Release repo"},
	}, nil
}

func (m mockService) ListArtifacts(repo string) ([]models.Artifact, error) {
	return []models.Artifact{
		{Name: "app.jar", Path: "com/example/app.jar", Size: 1024, LastModified: "2026-01-01T00:00:00Z", Repo: repo},
	}, nil
}

func (m mockService) UploadArtifact(repo, path string, reader io.Reader) error {
	return nil
}

func (m mockService) DeleteArtifact(repo, path string) error {
	return nil
}

func (m mockService) GetXraySummary(repo, path string) (models.XraySummary, error) {
	return models.XraySummary{Available: true}, nil
}

func (m mockService) DownloadArtifact(repo, path string) (io.ReadCloser, int64, string, error) {
	body := "file-contents"
	return io.NopCloser(strings.NewReader(body)), int64(len(body)), "application/octet-stream", nil
}

func setupTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpl, err := templates.Load("templates")
	if err != nil {
		t.Fatalf("loading templates: %v", err)
	}

	h := handlers.NewHandler(mockService{}, tmpl, "")
	return setupRouter(h)
}

func TestRouteIndex(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET / status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct == "" {
		t.Error("GET / missing Content-Type header")
	}
}

func TestRouteListRepos(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/repos", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /repos status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("GET /repos returned empty body")
	}
}

func TestRouteListArtifacts(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/artifacts?repo=libs-release", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /artifacts status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRouteDeleteArtifact(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/artifacts?repo=libs-release&path=com/example/app.jar", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("DELETE /artifacts status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRouteGetXray(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/xray?repo=libs-release&path=com/example/app.jar", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /xray status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRouteArtifactsMissingRepo(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/artifacts", nil)
	r.ServeHTTP(w, req)

	// Handler renders error fragment with 422 status for validation errors
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("GET /artifacts (no repo) status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}
