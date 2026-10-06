package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jfrog_manager/internal/models"
	"jfrog_manager/internal/templates"

	"github.com/gin-gonic/gin"
)

func setupXrayTestRouter(mock *mockService) *gin.Engine {
	gin.SetMode(gin.TestMode)

	tmpl, err := templates.Load("../../templates")
	if err != nil {
		panic("failed to load templates: " + err.Error())
	}

	h := NewHandler(mock, tmpl, "")

	r := gin.New()
	r.GET("/xray", h.GetXray)

	return r
}

func TestGetXray_WithVulnerabilities(t *testing.T) {
	mock := &mockService{
		xray: models.XraySummary{
			Available: true,
			Artifacts: []models.XrayArtifact{
				{
					Issues: []models.XrayIssue{
						{
							Severity: "Critical",
							Summary:  "Remote code execution",
							CVEs:     []models.XrayCVE{{ID: "CVE-2024-1234"}},
							Components: []models.XrayComponent{{ID: "org.example:lib"}},
						},
						{
							Severity: "High",
							Summary:  "SQL injection",
							CVEs:     []models.XrayCVE{{ID: "CVE-2024-5678"}},
							Components: []models.XrayComponent{{ID: "org.example:db"}},
						},
						{
							Severity: "Medium",
							Summary:  "Information disclosure",
							CVEs:     []models.XrayCVE{{ID: "CVE-2024-9999"}},
						},
					},
				},
			},
		},
	}

	r := setupXrayTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/xray?repo=libs-release&path=com/example/app.jar", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "CVE-2024-1234") {
		t.Error("expected body to contain CVE-2024-1234")
	}
	if !strings.Contains(body, "CVE-2024-5678") {
		t.Error("expected body to contain CVE-2024-5678")
	}
	if !strings.Contains(body, "Remote code execution") {
		t.Error("expected body to contain vulnerability summary")
	}
	if !strings.Contains(body, "Critical") {
		t.Error("expected body to contain Critical severity")
	}
	if !strings.Contains(body, "High") {
		t.Error("expected body to contain High severity")
	}
}

func TestGetXray_NoVulnerabilities(t *testing.T) {
	mock := &mockService{
		xray: models.XraySummary{
			Available: true,
			Artifacts: []models.XrayArtifact{
				{
					Issues: []models.XrayIssue{},
				},
			},
		},
	}

	r := setupXrayTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/xray?repo=libs-release&path=com/example/app.jar", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "No vulnerabilities found") {
		t.Error("expected 'No vulnerabilities found' message")
	}
}

func TestGetXray_Unavailable(t *testing.T) {
	mock := &mockService{
		xray: models.XraySummary{
			Available: false,
		},
	}

	r := setupXrayTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/xray?repo=libs-release&path=com/example/app.jar", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Xray not configured or artifact not indexed") {
		t.Error("expected unavailable message")
	}
}

func TestGetXray_MissingParams(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"missing both", "/xray"},
		{"missing path", "/xray?repo=libs-release"},
		{"missing repo", "/xray?path=com/example/app.jar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupXrayTestRouter(&mockService{})
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, tt.url, nil)
			r.ServeHTTP(w, req)

			body := w.Body.String()
			if !strings.Contains(body, "Repository and path are required") {
				t.Error("expected missing params error")
			}
		})
	}
}

func TestGetXray_PathTraversal(t *testing.T) {
	r := setupXrayTestRouter(&mockService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/xray?repo=libs-release&path=../../etc/passwd", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Invalid repository or path") {
		t.Error("expected path traversal rejection")
	}
}

func TestGetXray_ClientError(t *testing.T) {
	mock := &mockService{
		xrayErr: errors.New("xray service unavailable"),
	}

	r := setupXrayTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/xray?repo=libs-release&path=com/example/app.jar", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Failed to load Xray data") {
		t.Error("expected error message in response")
	}
	if !strings.Contains(body, "alert-danger") {
		t.Error("expected error alert in response")
	}
}

func TestGetXray_ShowsImpactPathInsideArtifactAndIssueID(t *testing.T) {
	mock := &mockService{
		xray: models.XraySummary{
			Available: true,
			Artifacts: []models.XrayArtifact{{
				Issues: []models.XrayIssue{{
					IssueID:  "XRAY-77",
					Summary:  "No CVE yet",
					Severity: "High",
					CVEs:     []models.XrayCVE{{CVSS3: "7.5"}},
					ImpactPaths: []string{
						"default/libs-release/com/example/app.rpm/./opt/app/bin/app/github.com/acme/mod",
					},
				}},
			}},
		},
	}
	r := setupXrayTestRouter(mock)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/xray?repo=libs-release&path=com/example/app.rpm", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "opt/app/bin/app/github.com/acme/mod") {
		t.Error("expected the impact path inside the artifact")
	}
	if strings.Contains(body, "default/libs-release") {
		t.Error("the artifact's own location should be trimmed from the impact path")
	}
	if !strings.Contains(body, "<code>XRAY-77</code>") {
		t.Error("expected the Xray issue id when the issue has no CVE id")
	}
}
