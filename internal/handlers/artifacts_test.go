package handlers

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jfrog_manager/internal/models"
	"jfrog_manager/internal/templates"

	"github.com/gin-gonic/gin"
)

// mockService is a test double for jfrog.Service.
type mockService struct {
	repos     []models.Repository
	reposErr  error
	artifacts []models.Artifact
	artsErr   error
	uploadErr error
	deleteErr error
	xray      models.XraySummary
	xrayErr   error

	uploadedRepo string
	uploadedPath string
	deletedRepo  string
	deletedPath  string
}

func (m *mockService) ListRepos() ([]models.Repository, error) {
	return m.repos, m.reposErr
}

func (m *mockService) ListArtifacts(repo string) ([]models.Artifact, error) {
	return m.artifacts, m.artsErr
}

func (m *mockService) UploadArtifact(repo, path string, reader io.Reader) error {
	m.uploadedRepo = repo
	m.uploadedPath = path
	return m.uploadErr
}

func (m *mockService) DeleteArtifact(repo, path string) error {
	m.deletedRepo = repo
	m.deletedPath = path
	return m.deleteErr
}

func (m *mockService) GetXraySummary(repo, path string) (models.XraySummary, error) {
	return m.xray, m.xrayErr
}

func setupTestRouter(mock *mockService) *gin.Engine {
	gin.SetMode(gin.TestMode)

	tmpl, err := templates.Load("../../templates")
	if err != nil {
		panic("failed to load templates: " + err.Error())
	}

	h := NewHandler(mock, tmpl, "")

	r := gin.New()
	r.GET("/", h.Index)
	r.GET("/repos", h.ListRepos)
	r.GET("/artifacts", h.ListArtifacts)
	r.POST("/artifacts/upload", h.UploadArtifact)
	r.POST("/artifacts/bulk-delete", h.BulkDeleteArtifacts)
	r.DELETE("/artifacts", h.DeleteArtifact)

	return r
}

func TestIndex(t *testing.T) {
	r := setupTestRouter(&mockService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "JFrog Manager") {
		t.Error("expected page to contain 'JFrog Manager'")
	}
	if !strings.Contains(body, "repo-select") {
		t.Error("expected page to contain repo-select element")
	}
}

func TestListRepos_Success(t *testing.T) {
	mock := &mockService{
		repos: []models.Repository{
			{Key: "libs-release", PackageType: "maven"},
			{Key: "docker-local", PackageType: "docker"},
		},
	}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/repos", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "libs-release") {
		t.Error("expected body to contain 'libs-release'")
	}
	if !strings.Contains(body, "docker-local") {
		t.Error("expected body to contain 'docker-local'")
	}
	if !strings.Contains(body, "<option") {
		t.Error("expected body to contain option elements")
	}
}

func TestListRepos_Error(t *testing.T) {
	mock := &mockService{
		reposErr: errors.New("connection refused"),
	}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/repos", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Failed to load repositories") {
		t.Error("expected error message in response")
	}
	if !strings.Contains(body, "alert-danger") {
		t.Error("expected error alert in response")
	}
}

func TestListArtifacts_Success(t *testing.T) {
	mock := &mockService{
		artifacts: []models.Artifact{
			{Name: "app.jar", Path: "com/example/app.jar", Size: 1024, LastModified: "2024-01-01", Repo: "libs-release"},
		},
	}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/artifacts?repo=libs-release", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "app.jar") {
		t.Error("expected body to contain artifact name")
	}
	if !strings.Contains(body, "com/example/app.jar") {
		t.Error("expected body to contain artifact path")
	}
}

func TestListArtifacts_MissingRepo(t *testing.T) {
	r := setupTestRouter(&mockService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/artifacts", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Repository parameter is required") {
		t.Error("expected missing repo error message")
	}
}

func TestListArtifacts_PathTraversal(t *testing.T) {
	r := setupTestRouter(&mockService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/artifacts?repo=../evil", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Invalid repository") {
		t.Error("expected path traversal rejection")
	}
}

func TestListArtifacts_ClientError(t *testing.T) {
	mock := &mockService{
		artsErr: errors.New("request failed with status 500"),
	}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/artifacts?repo=libs-release", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Failed to load artifacts") {
		t.Error("expected error message in response")
	}
}

func TestListArtifacts_Empty(t *testing.T) {
	mock := &mockService{
		artifacts: []models.Artifact{},
	}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/artifacts?repo=libs-release", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "No artifacts found") {
		t.Error("expected empty message")
	}
}

func createMultipartRequest(t *testing.T, repo, folder, filename, content string) *http.Request {
	t.Helper()
	body := &strings.Builder{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("repo", repo)
	_ = writer.WriteField("folder", folder)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(content))
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "/artifacts/upload", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestUploadArtifact_Success(t *testing.T) {
	mock := &mockService{
		artifacts: []models.Artifact{
			{Name: "app.jar", Path: "com/example/app.jar", Size: 1024, Repo: "libs-release"},
		},
	}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req := createMultipartRequest(t, "libs-release", "com/example", "app.jar", "file-content")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if mock.uploadedRepo != "libs-release" {
		t.Errorf("expected repo 'libs-release', got '%s'", mock.uploadedRepo)
	}
	if mock.uploadedPath != "com/example/app.jar" {
		t.Errorf("expected path 'com/example/app.jar', got '%s'", mock.uploadedPath)
	}
	body := w.Body.String()
	if !strings.Contains(body, "app.jar") {
		t.Error("expected refreshed artifact list")
	}
}

func TestUploadArtifact_NoFolder(t *testing.T) {
	mock := &mockService{
		artifacts: []models.Artifact{
			{Name: "app.jar", Path: "app.jar", Size: 1024, Repo: "libs-release"},
		},
	}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req := createMultipartRequest(t, "libs-release", "", "app.jar", "file-content")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if mock.uploadedPath != "app.jar" {
		t.Errorf("expected path 'app.jar', got '%s'", mock.uploadedPath)
	}
}

func TestUploadArtifact_MissingRepo(t *testing.T) {
	r := setupTestRouter(&mockService{})
	w := httptest.NewRecorder()
	req := createMultipartRequest(t, "", "somefolder", "file.txt", "content")
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Repository is required") {
		t.Error("expected missing repo error")
	}
}

func TestUploadArtifact_MissingFile(t *testing.T) {
	r := setupTestRouter(&mockService{})
	w := httptest.NewRecorder()

	body := &strings.Builder{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("repo", "libs-release")
	_ = writer.WriteField("folder", "some/path")
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "/artifacts/upload", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(w, req)

	respBody := w.Body.String()
	if !strings.Contains(respBody, "File is required") {
		t.Error("expected missing file error")
	}
}

func TestUploadArtifact_ClientError(t *testing.T) {
	mock := &mockService{
		uploadErr: errors.New("upload failed with status 403"),
	}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req := createMultipartRequest(t, "libs-release", "somefolder", "file.txt", "content")
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Upload failed") {
		t.Error("expected upload error message")
	}
}

func TestUploadArtifact_RefreshFails(t *testing.T) {
	mock := &mockService{
		uploadErr: nil,
		artsErr:   errors.New("connection refused"),
	}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req := createMultipartRequest(t, "libs-release", "com/example", "app.jar", "file-content")
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Upload succeeded but failed to refresh list") {
		t.Error("expected refresh failure message after successful upload")
	}
}

func TestUploadArtifact_PathTraversal(t *testing.T) {
	r := setupTestRouter(&mockService{})
	w := httptest.NewRecorder()
	req := createMultipartRequest(t, "libs-release", "../../etc", "passwd", "content")
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Invalid repository or path") {
		t.Error("expected path traversal rejection")
	}
}

func TestDeleteArtifact_PathTraversal(t *testing.T) {
	r := setupTestRouter(&mockService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/artifacts?repo=../evil&path=file.jar", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Invalid repository or path") {
		t.Error("expected path traversal rejection")
	}
}

func TestDeleteArtifact_Success(t *testing.T) {
	mock := &mockService{}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/artifacts?repo=libs-release&path=com/example/app.jar", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if mock.deletedRepo != "libs-release" {
		t.Errorf("expected repo 'libs-release', got '%s'", mock.deletedRepo)
	}
	if mock.deletedPath != "com/example/app.jar" {
		t.Errorf("expected path 'com/example/app.jar', got '%s'", mock.deletedPath)
	}
	// Should return empty body for htmx row removal
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %d bytes", w.Body.Len())
	}
}

func TestDeleteArtifact_MissingParams(t *testing.T) {
	r := setupTestRouter(&mockService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/artifacts?repo=libs-release", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Repository and path are required") {
		t.Error("expected missing params error")
	}
}

func TestDeleteArtifact_ClientError(t *testing.T) {
	mock := &mockService{
		deleteErr: errors.New("delete failed with status 404"),
	}
	r := setupTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/artifacts?repo=libs-release&path=some/file.jar", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Delete failed") {
		t.Error("expected delete error message")
	}
}
