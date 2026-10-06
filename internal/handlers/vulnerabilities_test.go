package handlers

import (
	"encoding/csv"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jfrog_manager/internal/models"
	"jfrog_manager/internal/templates"

	"github.com/gin-gonic/gin"
)

func setupVulnerabilitiesTestRouter(mock *mockService) *gin.Engine {
	gin.SetMode(gin.TestMode)

	tmpl, err := templates.Load("../../templates")
	if err != nil {
		panic("failed to load templates: " + err.Error())
	}

	h := NewHandler(mock, tmpl, "")

	r := gin.New()
	r.GET("/vulnerabilities", h.GetVulnerabilities)
	r.GET("/vulnerabilities/export", h.ExportVulnerabilities)

	return r
}

func vulnArtifact(p, modified string) models.Artifact {
	return models.Artifact{Name: p[strings.LastIndex(p, "/")+1:], Path: p, LastModified: modified, Repo: "rpm-stable"}
}

// vulnMock returns a repository with two versions of opcontrol, one opcore and
// one sso package; Xray knows the latest opcontrol and opcore but not sso.
func vulnMock() *mockService {
	return &mockService{
		artifacts: []models.Artifact{
			vulnArtifact("portal/opcontrol/opcontrol-v1.10.45-6c62b1ec.x86_64.rpm", "2026-09-24T08:39:45.536Z"),
			vulnArtifact("portal/opcontrol/opcontrol-v1.10.52-5608d1c8.x86_64.rpm", "2026-10-06T06:20:23.510Z"),
			vulnArtifact("portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm", "2026-09-15T08:11:50.629Z"),
			vulnArtifact("portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm", "2026-10-06T07:14:47.134Z"),
		},
		xray: models.XraySummary{
			Available: true,
			Artifacts: []models.XrayArtifact{
				{
					General: models.XrayGeneral{Path: "default/rpm-stable/portal/opcontrol/opcontrol-v1.10.52-5608d1c8.x86_64.rpm"},
					Issues: []models.XrayIssue{
						{IssueID: "XRAY-10", Severity: "Low", Summary: "Low issue in opcontrol", CVEs: []models.XrayCVE{{ID: "CVE-2026-10"}}},
						{IssueID: "XRAY-11", Severity: "Critical", Summary: "Critical issue in opcontrol", CVEs: []models.XrayCVE{{ID: "CVE-2026-11"}}},
					},
				},
				{
					General: models.XrayGeneral{Path: "default/rpm-stable/portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm"},
					Issues: []models.XrayIssue{
						{
							IssueID: "XRAY-20", Severity: "High", Summary: "High issue in opcore",
							ImpactPaths: []string{"default/rpm-stable/portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm/./opt/opcore/bin/opcore/github.com/pelletier/go-toml/v2"},
						},
					},
				},
			},
			Errors: []models.XrayError{
				{Identifier: "default/rpm-stable/portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm", Error: "Artifact doesn't exist or not indexed/cached in Xray"},
			},
		},
	}
}

func getVulnerabilities(mock *mockService, target string) *httptest.ResponseRecorder {
	r := setupVulnerabilitiesTestRouter(mock)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, target, nil)
	r.ServeHTTP(w, req)
	return w
}

// assertOrder fails unless the given substrings appear in body in that order.
func assertOrder(t *testing.T, body string, parts ...string) {
	t.Helper()
	pos := 0
	for _, part := range parts {
		idx := strings.Index(body[pos:], part)
		if idx < 0 {
			t.Fatalf("expected %q after position %d in output", part, pos)
		}
		pos += idx + len(part)
	}
}

func TestGetVulnerabilities_Success(t *testing.T) {
	mock := vulnMock()
	w := getVulnerabilities(mock, "/vulnerabilities?repo=rpm-stable")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", ct)
	}

	wantPaths := []string{
		"portal/opcontrol/opcontrol-v1.10.52-5608d1c8.x86_64.rpm",
		"portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm",
		"portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm",
	}
	if strings.Join(mock.xrayPaths, ",") != strings.Join(wantPaths, ",") {
		t.Errorf("expected only the latest artifacts to be sent to Xray, got %v", mock.xrayPaths)
	}

	body := html.UnescapeString(w.Body.String())
	// Components with the worst findings come first; inside one, most severe first.
	assertOrder(t, body, "Critical issue in opcontrol", "Low issue in opcontrol", "High issue in opcore")
	for _, s := range []string{
		"Findings: 3", "Packages scanned: 2", "Not scanned: 1",
		"Latest version of each package only.",
		"Packages not scanned by Xray: 1",
		"portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm",
		"Artifact doesn't exist or not indexed/cached in Xray",
		"opt/opcore/bin/opcore/github.com/pelletier/go-toml/v2",
		"<code>XRAY-20</code>",
		`href="/vulnerabilities/export?repo=rpm-stable&sort=severity&dir=desc"`,
	} {
		if !strings.Contains(body, s) {
			t.Errorf("expected output to contain %q", s)
		}
	}
	if strings.Contains(body, "opcontrol-v1.10.45") {
		t.Error("an older version must not appear in the report")
	}
}

func TestGetVulnerabilities_SortByComponent(t *testing.T) {
	w := getVulnerabilities(vulnMock(), "/vulnerabilities?repo=rpm-stable&sort=component&dir=desc")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := html.UnescapeString(w.Body.String())
	assertOrder(t, body, "High issue in opcore", "Critical issue in opcontrol", "Low issue in opcontrol")
	if !strings.Contains(body, `href="/vulnerabilities/export?repo=rpm-stable&sort=component&dir=desc"`) {
		t.Error("expected the export link to carry the requested sort")
	}
}

func TestGetVulnerabilities_SeverityFilter(t *testing.T) {
	w := getVulnerabilities(vulnMock(), "/vulnerabilities?repo=rpm-stable&severity=Critical")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := html.UnescapeString(w.Body.String())
	if !strings.Contains(body, "Critical issue in opcontrol") {
		t.Error("expected the critical finding")
	}
	if strings.Contains(body, "High issue in opcore") || strings.Contains(body, "Low issue in opcontrol") {
		t.Error("findings of other severities must not be listed")
	}
	// Totals stay those of the whole report, and the export keeps the filter.
	if !strings.Contains(body, "Findings: 3") {
		t.Error("expected the unfiltered total in the header")
	}
	if !strings.Contains(body, `href="/vulnerabilities/export?repo=rpm-stable&sort=severity&dir=desc&severity=critical"`) {
		t.Error("expected the export link to carry the severity filter")
	}
}

func TestGetVulnerabilities_ListsCleanPackages(t *testing.T) {
	mock := vulnMock()
	mock.xray.Artifacts[1].Issues = nil
	w := getVulnerabilities(mock, "/vulnerabilities?repo=rpm-stable")

	body := w.Body.String()
	if !strings.Contains(body, "scanned and clean: 1") || !strings.Contains(body, `<span class="o-clean" title="portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm">`) {
		t.Error("expected opcore to be listed as scanned and clean")
	}
	if strings.Contains(body, `title="portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm">sso`) {
		t.Error("a package Xray did not scan must not be listed as clean")
	}
}

func TestGetVulnerabilities_NoVulnerabilities(t *testing.T) {
	mock := vulnMock()
	for i := range mock.xray.Artifacts {
		mock.xray.Artifacts[i].Issues = nil
	}
	w := getVulnerabilities(mock, "/vulnerabilities?repo=rpm-stable")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "No vulnerabilities found in the scanned packages (2).") {
		t.Error("expected the empty state")
	}
	if !strings.Contains(body, "not scanned by Xray") {
		t.Error("expected the not-scanned warning next to the empty state")
	}
}

func TestGetVulnerabilities_EmptyRepository(t *testing.T) {
	mock := &mockService{xray: models.XraySummary{Available: true}}
	w := getVulnerabilities(mock, "/vulnerabilities?repo=rpm-stable")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "No vulnerabilities found in the scanned packages (0).") {
		t.Error("expected the empty state for an empty repository")
	}
}

func TestGetVulnerabilities_XrayUnavailable(t *testing.T) {
	mock := vulnMock()
	mock.xray = models.XraySummary{Available: false}
	w := getVulnerabilities(mock, "/vulnerabilities?repo=rpm-stable")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Xray data could not be retrieved") {
		t.Error("expected the unavailable notice")
	}
	if strings.Contains(body, "No vulnerabilities found") {
		t.Error("an unavailable Xray must not be reported as no vulnerabilities")
	}
}

func TestVulnerabilities_Errors(t *testing.T) {
	listFails := vulnMock()
	listFails.artsErr = errors.New("upstream secret detail")
	xrayFails := vulnMock()
	xrayFails.xrayErr = errors.New("upstream secret detail")
	unavailable := vulnMock()
	unavailable.xray = models.XraySummary{Available: false}

	tests := []struct {
		name    string
		mock    *mockService
		target  string
		message string
	}{
		{"fragment: missing repo", vulnMock(), "/vulnerabilities", "Repository parameter is required"},
		{"fragment: path traversal", vulnMock(), "/vulnerabilities?repo=../etc", "Invalid repository"},
		{"fragment: listing fails", listFails, "/vulnerabilities?repo=rpm-stable", "Failed to load vulnerabilities"},
		{"fragment: xray fails", xrayFails, "/vulnerabilities?repo=rpm-stable", "Failed to load vulnerabilities"},
		{"export: missing repo", vulnMock(), "/vulnerabilities/export", "Repository parameter is required"},
		{"export: path traversal", vulnMock(), "/vulnerabilities/export?repo=a/../b", "Invalid repository"},
		{"export: listing fails", listFails, "/vulnerabilities/export?repo=rpm-stable", "Failed to export vulnerabilities"},
		{"export: xray fails", xrayFails, "/vulnerabilities/export?repo=rpm-stable", "Failed to export vulnerabilities"},
		{"export: xray unavailable", unavailable, "/vulnerabilities/export?repo=rpm-stable", "Xray data could not be retrieved"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := getVulnerabilities(tt.mock, tt.target)

			if w.Code != http.StatusUnprocessableEntity {
				t.Errorf("expected status 422, got %d", w.Code)
			}
			if w.Header().Get("HX-Retarget") != "#error-container" {
				t.Errorf("expected HX-Retarget header, got %q", w.Header().Get("HX-Retarget"))
			}
			body := w.Body.String()
			if !strings.Contains(body, tt.message) {
				t.Errorf("expected message %q, got %s", tt.message, body)
			}
			if strings.Contains(body, "upstream secret detail") {
				t.Error("upstream error details must not reach the browser")
			}
		})
	}
}

func TestExportVulnerabilities_Success(t *testing.T) {
	w := getVulnerabilities(vulnMock(), "/vulnerabilities/export?repo=rpm-stable&sort=component&dir=asc")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Errorf("unexpected Content-Type %q", ct)
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment; filename=vulnerabilities-rpm-stable-") || !strings.HasSuffix(cd, ".csv") {
		t.Errorf("unexpected Content-Disposition %q", cd)
	}

	records, err := csv.NewReader(w.Body).ReadAll()
	if err != nil {
		t.Fatalf("response is not valid CSV: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("expected header and 3 rows, got %d records", len(records))
	}
	if records[0][0] != "Component" || records[0][7] != "Impact Path" {
		t.Errorf("unexpected header: %v", records[0])
	}
	// Component ascending, most severe first inside a component.
	gotOrder := []string{records[1][6], records[2][6], records[3][6]}
	wantOrder := []string{"Critical issue in opcontrol", "Low issue in opcontrol", "High issue in opcore"}
	if strings.Join(gotOrder, "|") != strings.Join(wantOrder, "|") {
		t.Errorf("unexpected row order: %v", gotOrder)
	}
	if records[3][0] != "opcore" || records[3][2] != "portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm" {
		t.Errorf("unexpected opcore row: %v", records[3])
	}
	if records[3][7] != "opt/opcore/bin/opcore/github.com/pelletier/go-toml/v2" {
		t.Errorf("unexpected impact path cell: %q", records[3][7])
	}
}

func TestExportVulnerabilities_DefaultSortMatchesFragment(t *testing.T) {
	w := getVulnerabilities(vulnMock(), "/vulnerabilities/export?repo=rpm-stable")

	records, err := csv.NewReader(w.Body).ReadAll()
	if err != nil {
		t.Fatalf("response is not valid CSV: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("expected header and 3 rows, got %d records", len(records))
	}
	gotOrder := []string{records[1][3], records[2][3], records[3][3]}
	if strings.Join(gotOrder, "|") != "Critical|High|Low" {
		t.Errorf("expected most severe first by default, got %v", gotOrder)
	}
}

func TestExportVulnerabilities_SeverityFilter(t *testing.T) {
	w := getVulnerabilities(vulnMock(), "/vulnerabilities/export?repo=rpm-stable&severity=high")

	records, err := csv.NewReader(w.Body).ReadAll()
	if err != nil {
		t.Fatalf("response is not valid CSV: %v", err)
	}
	if len(records) != 2 || records[1][3] != "High" || records[1][0] != "opcore" {
		t.Errorf("expected only the high finding, got %v", records)
	}
}
