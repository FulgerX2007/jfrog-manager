package jfrog

import (
	"net/http"
	"strings"
	"testing"

	"jfrog_manager/internal/config"
)

func TestGetXraySummary_WithVulnerabilities(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"artifacts": [{
				"general": {
					"name": "app-1.0.jar",
					"path": "libs-release/com/example/app-1.0.jar",
					"pkg_type": "maven",
					"sha256": "abc123"
				},
				"issues": [
					{
						"summary": "Remote Code Execution",
						"description": "A critical RCE vulnerability",
						"severity": "Critical",
						"issue_type": "security",
						"provider": "JFrog",
						"cves": [{"cve": "CVE-2024-1234", "cvss_v2": "9.0", "cvss_v3": "9.8"}],
						"components": [{"component_id": "gav://com.example:lib:1.0", "fixed_versions": ["1.1", "2.0"]}]
					},
					{
						"summary": "Information Disclosure",
						"description": "A medium severity info leak",
						"severity": "Medium",
						"issue_type": "security",
						"provider": "JFrog",
						"cves": [{"cve": "CVE-2024-5678", "cvss_v3": "5.3"}],
						"components": [{"component_id": "gav://com.example:lib:1.0", "fixed_versions": ["1.2"]}]
					}
				],
				"licenses": [
					{
						"name": "Apache-2.0",
						"full_name": "Apache License 2.0",
						"components": [{"component_id": "gav://com.example:lib:1.0"}]
					}
				]
			}]
		}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	summary, err := client.GetXraySummary("libs-release", "com/example/app-1.0.jar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !summary.Available {
		t.Fatal("expected Available to be true")
	}
	if len(summary.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(summary.Artifacts))
	}

	art := summary.Artifacts[0]
	if art.General.Name != "app-1.0.jar" {
		t.Errorf("expected name 'app-1.0.jar', got '%s'", art.General.Name)
	}
	if art.General.PackageType != "maven" {
		t.Errorf("expected pkg_type 'maven', got '%s'", art.General.PackageType)
	}
	if len(art.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(art.Issues))
	}
	if art.Issues[0].Severity != "Critical" {
		t.Errorf("expected severity 'Critical', got '%s'", art.Issues[0].Severity)
	}
	if len(art.Issues[0].CVEs) != 1 || art.Issues[0].CVEs[0].ID != "CVE-2024-1234" {
		t.Errorf("unexpected CVEs: %+v", art.Issues[0].CVEs)
	}
	if art.Issues[0].CVEs[0].CVSS3 != "9.8" {
		t.Errorf("expected CVSS3 '9.8', got '%s'", art.Issues[0].CVEs[0].CVSS3)
	}
	if len(art.Issues[0].Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(art.Issues[0].Components))
	}
	if len(art.Issues[0].Components[0].FixedVersions) != 2 {
		t.Errorf("expected 2 fixed versions, got %d", len(art.Issues[0].Components[0].FixedVersions))
	}
	if len(art.Licenses) != 1 || art.Licenses[0].Name != "Apache-2.0" {
		t.Errorf("unexpected licenses: %+v", art.Licenses)
	}
}

func TestGetXraySummary_EmptyResults(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"artifacts":[{"general":{"name":"clean.jar"},"issues":[],"licenses":[]}]}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	summary, err := client.GetXraySummary("repo", "clean.jar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !summary.Available {
		t.Error("expected Available to be true")
	}
	if len(summary.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(summary.Artifacts))
	}
	if len(summary.Artifacts[0].Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(summary.Artifacts[0].Issues))
	}
}

func TestGetXraySummary_XrayUnavailable404(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Xray is not enabled"}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	summary, err := client.GetXraySummary("repo", "file.jar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Available {
		t.Error("expected Available to be false for 404")
	}
}

func TestGetXraySummary_XrayServerError(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	summary, err := client.GetXraySummary("repo", "file.jar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Available {
		t.Error("expected Available to be false for 500")
	}
}

func TestGetXraySummary_ConnectionError(t *testing.T) {
	cfg := config.Config{
		JFrogURL:    "http://localhost:1",
		JFrogAPIKey: "key",
		Timeout:     1,
	}
	client := NewClient(cfg)
	summary, err := client.GetXraySummary("repo", "file.jar")
	if err != nil {
		t.Fatalf("expected no error for connection failure (graceful degradation), got: %v", err)
	}

	if summary.Available {
		t.Error("expected Available to be false for connection error")
	}
}

func TestGetXraySummary_MalformedJSON(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not valid json`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.GetXraySummary("repo", "file.jar")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parsing xray response") {
		t.Errorf("expected parsing error, got: %v", err)
	}
}
