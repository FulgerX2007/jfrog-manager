package jfrog

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jfrog_manager/internal/config"
)

func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func newTestClient(serverURL string) Client {
	cfg := config.Config{
		JFrogURL:    serverURL,
		JFrogAPIKey: "test-api-key",
		Port:        "8080",
		Timeout:     5,
	}
	return NewClient(cfg)
}

func TestNewClient_SetsFields(t *testing.T) {
	cfg := config.Config{
		JFrogURL:    "https://artifactory.example.com/",
		JFrogAPIKey: "my-key",
		Port:        "9090",
		Timeout:     10,
	}
	client := NewClient(cfg)

	if client.baseURL != "https://artifactory.example.com" {
		t.Errorf("expected trailing slash trimmed, got %s", client.baseURL)
	}
	if client.apiKey != "my-key" {
		t.Errorf("expected apiKey 'my-key', got %s", client.apiKey)
	}
}

func TestDo_InjectsAuthHeader(t *testing.T) {
	var receivedHeader string
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-JFrog-Art-Api")
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	client := newTestClient(server.URL)
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	_, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedHeader != "test-api-key" {
		t.Errorf("expected auth header 'test-api-key', got '%s'", receivedHeader)
	}
}

func TestListRepos_CorrectURLAndAuth(t *testing.T) {
	var requestPath string
	var authHeader string
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authHeader = r.Header.Get("X-JFrog-Art-Api")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"key":"repo1","type":"local","packageType":"maven","description":"test repo"}]`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	repos, err := client.ListRepos()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if requestPath != "/artifactory/api/repositories" {
		t.Errorf("expected path '/artifactory/api/repositories', got '%s'", requestPath)
	}
	if authHeader != "test-api-key" {
		t.Errorf("expected auth header 'test-api-key', got '%s'", authHeader)
	}
	if len(repos) != 1 || repos[0].Key != "repo1" {
		t.Errorf("unexpected repos: %+v", repos)
	}
}

func TestListArtifacts_CorrectURL(t *testing.T) {
	var requestURL string
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		requestURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"files":[]}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	artifacts, err := client.ListArtifacts("my-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/artifactory/api/storage/my-repo/?list&deep=1"
	if requestURL != expected {
		t.Errorf("expected URL '%s', got '%s'", expected, requestURL)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected empty list, got %d artifacts", len(artifacts))
	}
}

func TestUploadArtifact_SendsBodyAndCorrectPath(t *testing.T) {
	var requestPath string
	var requestMethod string
	var receivedBody string
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusCreated)
	})
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.UploadArtifact("my-repo", "path/to/file.jar", strings.NewReader("file-content"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if requestMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", requestMethod)
	}
	if requestPath != "/artifactory/my-repo/path/to/file.jar" {
		t.Errorf("expected path '/artifactory/my-repo/path/to/file.jar', got '%s'", requestPath)
	}
	if receivedBody != "file-content" {
		t.Errorf("expected body 'file-content', got '%s'", receivedBody)
	}
}

func TestDeleteArtifact_CorrectMethodAndPath(t *testing.T) {
	var requestPath string
	var requestMethod string
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.DeleteArtifact("my-repo", "path/to/file.jar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if requestMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", requestMethod)
	}
	if requestPath != "/artifactory/my-repo/path/to/file.jar" {
		t.Errorf("expected path '/artifactory/my-repo/path/to/file.jar', got '%s'", requestPath)
	}
}

func TestUploadArtifact_PreservesSlashesInPath(t *testing.T) {
	var rawURL string
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		rawURL = r.RequestURI
		w.WriteHeader(http.StatusCreated)
	})
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.UploadArtifact("my-repo", "com/example/app/1.0/app-1.0.jar", strings.NewReader("data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(rawURL, "%2F") {
		t.Errorf("path slashes should not be encoded as %%2F, got raw URL: %s", rawURL)
	}
	expected := "/artifactory/my-repo/com/example/app/1.0/app-1.0.jar"
	if rawURL != expected {
		t.Errorf("expected raw URL '%s', got '%s'", expected, rawURL)
	}
}

func TestDeleteArtifact_PreservesSlashesInPath(t *testing.T) {
	var rawURL string
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		rawURL = r.RequestURI
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.DeleteArtifact("my-repo", "com/example/app/1.0/app-1.0.jar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(rawURL, "%2F") {
		t.Errorf("path slashes should not be encoded as %%2F, got raw URL: %s", rawURL)
	}
	expected := "/artifactory/my-repo/com/example/app/1.0/app-1.0.jar"
	if rawURL != expected {
		t.Errorf("expected raw URL '%s', got '%s'", expected, rawURL)
	}
}

func TestGetXraySummary_CorrectURLAndBody(t *testing.T) {
	var requestPath string
	var requestMethod string
	var receivedBody string
	var contentType string
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestMethod = r.Method
		contentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"artifacts":[]}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	summary, err := client.GetXraySummary("my-repo", "path/to/file.jar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if requestMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", requestMethod)
	}
	if requestPath != "/xray/api/v1/summary/artifact" {
		t.Errorf("expected path '/xray/api/v1/summary/artifact', got '%s'", requestPath)
	}
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}
	expectedBody := `{"paths":["default/my-repo/path/to/file.jar"]}`
	if receivedBody != expectedBody {
		t.Errorf("expected body '%s', got '%s'", expectedBody, receivedBody)
	}
	if !summary.Available {
		t.Error("expected Available to be true")
	}
	if len(summary.Artifacts) != 0 {
		t.Errorf("expected empty artifacts, got %d", len(summary.Artifacts))
	}
}

// Error scenario tests

func TestListRepos_ConnectionRefused(t *testing.T) {
	cfg := config.Config{
		JFrogURL:    "http://localhost:1", // unlikely to have anything listening
		JFrogAPIKey: "key",
		Timeout:     1,
	}
	client := NewClient(cfg)
	_, err := client.ListRepos()
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestUploadArtifact_Non2xxReturnsError(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("access denied"))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.UploadArtifact("repo", "path", strings.NewReader("data"))
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected error to contain '403', got: %v", err)
	}
}

func TestDeleteArtifact_404ReturnsError(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.DeleteArtifact("repo", "missing/file.jar")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain '404', got: %v", err)
	}
}

func TestDeleteArtifact_403ReturnsError(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	err := client.DeleteArtifact("repo", "file.jar")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected error to contain '403', got: %v", err)
	}
}

// Verify Client satisfies Service interface at compile time
var _ Service = Client{}
