package jfrog

import (
	"net/http"
	"strings"
	"testing"
)

func TestListRepos_ParsesMultipleRepos(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"key":"libs-release","type":"local","packageType":"maven","description":"Release repo"},
			{"key":"libs-snapshot","type":"local","packageType":"maven","description":"Snapshot repo"},
			{"key":"docker-remote","type":"remote","packageType":"docker","description":"Docker Hub proxy"}
		]`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	repos, err := client.ListRepos()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(repos))
	}

	if repos[0].Key != "libs-release" || repos[0].Type != "local" || repos[0].PackageType != "maven" {
		t.Errorf("unexpected first repo: %+v", repos[0])
	}
	if repos[2].Key != "docker-remote" || repos[2].Type != "remote" || repos[2].PackageType != "docker" {
		t.Errorf("unexpected third repo: %+v", repos[2])
	}
}

func TestListRepos_EmptyList(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	repos, err := client.ListRepos()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repos) != 0 {
		t.Errorf("expected empty list, got %d repos", len(repos))
	}
}

func TestListRepos_Unauthorized401(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":[{"message":"Bad credentials"}]}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.ListRepos()
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to contain '401', got: %v", err)
	}
}

func TestListRepos_ServerError500(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.ListRepos()
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain '500', got: %v", err)
	}
}

func TestListRepos_MalformedJSON(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not valid json`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.ListRepos()
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parsing repositories response") {
		t.Errorf("expected parsing error, got: %v", err)
	}
}

func TestListArtifacts_ParsesFiles(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"uri": "https://example.com/artifactory/api/storage/my-repo",
			"files": [
				{"uri":"/com/example/app-1.0.jar","size":12345,"lastModified":"2025-01-15T10:30:00.000Z","folder":false},
				{"uri":"/com/example/app-2.0.jar","size":67890,"lastModified":"2025-02-20T14:00:00.000Z","folder":false},
				{"uri":"/com/example","size":-1,"lastModified":"2025-01-01T00:00:00.000Z","folder":true}
			]
		}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	artifacts, err := client.ListArtifacts("my-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts (folders excluded), got %d", len(artifacts))
	}

	if artifacts[0].Name != "app-1.0.jar" {
		t.Errorf("expected name 'app-1.0.jar', got '%s'", artifacts[0].Name)
	}
	if artifacts[0].Path != "com/example/app-1.0.jar" {
		t.Errorf("expected path 'com/example/app-1.0.jar', got '%s'", artifacts[0].Path)
	}
	if artifacts[0].Size != 12345 {
		t.Errorf("expected size 12345, got %d", artifacts[0].Size)
	}
	if artifacts[0].LastModified != "2025-01-15T10:30:00.000Z" {
		t.Errorf("expected lastModified '2025-01-15T10:30:00.000Z', got '%s'", artifacts[0].LastModified)
	}
	if artifacts[0].Repo != "my-repo" {
		t.Errorf("expected repo 'my-repo', got '%s'", artifacts[0].Repo)
	}

	if artifacts[1].Name != "app-2.0.jar" {
		t.Errorf("expected name 'app-2.0.jar', got '%s'", artifacts[1].Name)
	}
}

func TestListArtifacts_EmptyRepo(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"files":[]}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	artifacts, err := client.ListArtifacts("empty-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(artifacts) != 0 {
		t.Errorf("expected empty list, got %d artifacts", len(artifacts))
	}
}

func TestListArtifacts_Unauthorized401(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":[{"message":"Bad credentials"}]}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.ListArtifacts("my-repo")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to contain '401', got: %v", err)
	}
}

func TestListArtifacts_ServerError500(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.ListArtifacts("my-repo")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain '500', got: %v", err)
	}
}

func TestListArtifacts_MalformedJSON(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not valid json`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.ListArtifacts("my-repo")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parsing artifacts response") {
		t.Errorf("expected parsing error, got: %v", err)
	}
}

func TestListArtifacts_FileAtRoot(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"files":[{"uri":"/readme.txt","size":100,"lastModified":"2025-03-01T00:00:00.000Z","folder":false}]}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	artifacts, err := client.ListArtifacts("my-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Name != "readme.txt" {
		t.Errorf("expected name 'readme.txt', got '%s'", artifacts[0].Name)
	}
	if artifacts[0].Path != "readme.txt" {
		t.Errorf("expected path 'readme.txt', got '%s'", artifacts[0].Path)
	}
}

func TestListRepos_CorrectHTTPMethod(t *testing.T) {
	var method string
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.ListRepos()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if method != http.MethodGet {
		t.Errorf("expected GET, got %s", method)
	}
}
