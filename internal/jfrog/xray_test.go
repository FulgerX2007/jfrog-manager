package jfrog

import (
	"encoding/json"
	"fmt"
	"io"
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
						"components": ["gav://com.example:lib:1.0"]
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
		JFrogURL:      "http://localhost:1",
		JFrogUsername: "user",
		JFrogToken:    "token",
		Timeout:       1,
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

func TestGetXraySummary_DecodesIssueIDImpactPathAndErrors(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"artifacts": [{
				"general": {
					"name": "opcore-v1.10.25-5314aa81.x86_64.rpm",
					"path": "default/rpm-stable/portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm",
					"pkg_type": "Rpm"
				},
				"issues": [{
					"issue_id": "XRAY-1065708",
					"summary": "Unbounded parser recursion",
					"severity": "High",
					"cves": [{"cvss_v3": "7.5/CVSS:3.1/AV:N"}],
					"components": null,
					"impact_path": [
						"default/rpm-stable/portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm/./opt/opcore/bin/opcore/github.com/pelletier/go-toml/v2",
						"default/rpm-stable/portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm/./opt/opcore/bin/tool/github.com/pelletier/go-toml/v2"
					]
				}]
			}],
			"errors": [{
				"identifier": "default/rpm-stable/portal/sso/sso-v1.0.0.x86_64.rpm",
				"error": "Artifact doesn't exist or not indexed/cached in Xray"
			}]
		}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	summary, err := client.GetXraySummary("rpm-stable", "portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summary.Artifacts) != 1 || len(summary.Artifacts[0].Issues) != 1 {
		t.Fatalf("expected 1 artifact with 1 issue, got %+v", summary.Artifacts)
	}

	issue := summary.Artifacts[0].Issues[0]
	if issue.IssueID != "XRAY-1065708" {
		t.Errorf("expected issue id XRAY-1065708, got %q", issue.IssueID)
	}
	if len(issue.ImpactPaths) != 2 {
		t.Fatalf("expected 2 impact paths, got %d", len(issue.ImpactPaths))
	}
	if !strings.HasSuffix(issue.ImpactPaths[0], "/./opt/opcore/bin/opcore/github.com/pelletier/go-toml/v2") {
		t.Errorf("unexpected impact path: %q", issue.ImpactPaths[0])
	}
	if len(issue.Components) != 0 {
		t.Errorf("expected no components for null, got %d", len(issue.Components))
	}

	if len(summary.Errors) != 1 {
		t.Fatalf("expected 1 error entry, got %d", len(summary.Errors))
	}
	if summary.Errors[0].Identifier != "default/rpm-stable/portal/sso/sso-v1.0.0.x86_64.rpm" {
		t.Errorf("unexpected error identifier: %q", summary.Errors[0].Identifier)
	}
	if summary.Errors[0].Error == "" {
		t.Error("expected error reason to be decoded")
	}
}

func TestGetXraySummary_WithoutIssueIDImpactPathOrErrors(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"artifacts": [{"general": {"name": "a.jar"}, "issues": [{"summary": "s", "severity": "Low"}]}]}`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	summary, err := client.GetXraySummary("repo", "a.jar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	issue := summary.Artifacts[0].Issues[0]
	if issue.IssueID != "" || len(issue.ImpactPaths) != 0 {
		t.Errorf("expected empty issue id and impact paths, got %q / %v", issue.IssueID, issue.ImpactPaths)
	}
	if len(summary.Errors) != 0 {
		t.Errorf("expected no errors, got %v", summary.Errors)
	}
}

// xrayBatchServer answers every summary request with one artifact per requested
// path and records the paths of each request.
func xrayBatchServer(t *testing.T, requests *[][]string, failOnRequest int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Paths []string `json:"paths"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("invalid request body: %v", err)
		}
		*requests = append(*requests, req.Paths)
		if len(*requests) == failOnRequest {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		type general struct {
			Path string `json:"path"`
		}
		type artifact struct {
			General general `json:"general"`
		}
		resp := struct {
			Artifacts []artifact `json:"artifacts"`
			Errors    []any      `json:"errors"`
		}{}
		for _, p := range req.Paths {
			resp.Artifacts = append(resp.Artifacts, artifact{General: general{Path: p}})
		}
		resp.Errors = append(resp.Errors, map[string]string{"identifier": req.Paths[0], "error": "not indexed"})
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func numberedPaths(n int) []string {
	paths := make([]string, 0, n)
	for i := range n {
		paths = append(paths, fmt.Sprintf("pkg/app-%d.rpm", i))
	}
	return paths
}

func TestGetXraySummaries_SendsAllPathsInOneRequest(t *testing.T) {
	var requests [][]string
	server := newTestServer(xrayBatchServer(t, &requests, 0))
	defer server.Close()

	client := newTestClient(server.URL)
	summary, err := client.GetXraySummaries("my-repo", []string{"a/one.rpm", "b/two.rpm"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !summary.Available {
		t.Fatal("expected Available to be true")
	}
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	want := []string{"default/my-repo/a/one.rpm", "default/my-repo/b/two.rpm"}
	if len(requests[0]) != 2 || requests[0][0] != want[0] || requests[0][1] != want[1] {
		t.Errorf("expected paths %v, got %v", want, requests[0])
	}
	if len(summary.Artifacts) != 2 {
		t.Errorf("expected 2 artifacts, got %d", len(summary.Artifacts))
	}
}

func TestGetXraySummaries_SplitsIntoBatchesAndMerges(t *testing.T) {
	var requests [][]string
	server := newTestServer(xrayBatchServer(t, &requests, 0))
	defer server.Close()

	total := 2*xrayBatchSize + 5
	client := newTestClient(server.URL)
	summary, err := client.GetXraySummaries("my-repo", numberedPaths(total))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(requests))
	}
	if len(requests[0]) != xrayBatchSize || len(requests[1]) != xrayBatchSize || len(requests[2]) != 5 {
		t.Errorf("unexpected batch sizes: %d, %d, %d", len(requests[0]), len(requests[1]), len(requests[2]))
	}
	if len(summary.Artifacts) != total {
		t.Errorf("expected %d merged artifacts, got %d", total, len(summary.Artifacts))
	}
	if len(summary.Errors) != 3 {
		t.Errorf("expected errors merged from 3 batches, got %d", len(summary.Errors))
	}
	last := summary.Artifacts[total-1].General.Path
	if want := fmt.Sprintf("default/my-repo/pkg/app-%d.rpm", total-1); last != want {
		t.Errorf("expected last artifact %q, got %q", want, last)
	}
}

func TestGetXraySummaries_EmptyPathsMakesNoRequest(t *testing.T) {
	var requests [][]string
	server := newTestServer(xrayBatchServer(t, &requests, 0))
	defer server.Close()

	client := newTestClient(server.URL)
	summary, err := client.GetXraySummaries("my-repo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requests) != 0 {
		t.Errorf("expected no requests, got %d", len(requests))
	}
	if !summary.Available || len(summary.Artifacts) != 0 {
		t.Errorf("expected an available, empty summary, got %+v", summary)
	}
}

func TestGetXraySummaries_LaterBatchFailureIsUnavailable(t *testing.T) {
	var requests [][]string
	server := newTestServer(xrayBatchServer(t, &requests, 2))
	defer server.Close()

	client := newTestClient(server.URL)
	summary, err := client.GetXraySummaries("my-repo", numberedPaths(xrayBatchSize+1))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if summary.Available {
		t.Error("expected Available to be false when a batch fails")
	}
	if len(summary.Artifacts) != 0 {
		t.Errorf("expected partial results to be discarded, got %d artifacts", len(summary.Artifacts))
	}
}

func TestGetXraySummaries_MalformedJSON(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	})
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.GetXraySummaries("my-repo", []string{"a.rpm"})
	if err == nil {
		t.Fatal("expected an error for a malformed response")
	}
}
