package jfrog

import (
	"net/http"
	"strings"
	"testing"
)

func TestListRepos_ParsesMultipleRepos(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
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
		w.Write([]byte(`[]`))
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
		w.Write([]byte(`{"errors":[{"message":"Bad credentials"}]}`))
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
		w.Write([]byte("internal server error"))
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
		w.Write([]byte(`not valid json`))
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

func TestListRepos_CorrectHTTPMethod(t *testing.T) {
	var method string
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
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
