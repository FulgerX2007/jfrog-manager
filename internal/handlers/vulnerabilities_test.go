package handlers

import (
	"encoding/csv"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	if records[0][0] != "Component" || records[0][6] != "Vulnerable Version" || records[0][7] != "Fixed Version" || records[0][9] != "Impact Path" {
		t.Errorf("unexpected header: %v", records[0])
	}
	// Component ascending, most severe first inside a component.
	gotOrder := []string{records[1][8], records[2][8], records[3][8]}
	wantOrder := []string{"Critical issue in opcontrol", "Low issue in opcontrol", "High issue in opcore"}
	if strings.Join(gotOrder, "|") != strings.Join(wantOrder, "|") {
		t.Errorf("unexpected row order: %v", gotOrder)
	}
	if records[3][0] != "opcore" || records[3][2] != "portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm" {
		t.Errorf("unexpected opcore row: %v", records[3])
	}
	if records[3][9] != "opt/opcore/bin/opcore/github.com/pelletier/go-toml/v2" {
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

func TestExportVulnerabilities_IncludesVersions(t *testing.T) {
	mock := vulnMock()
	mock.fixes = models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"default/rpm-stable/portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm": {
			{IssueID: "XRAY-20", SourceComponentID: "go://github.com/pelletier/go-toml/v2:2.0.0", ComponentVersions: models.XrayComponentVersions{FixedVersions: []string{"2.0.1"}}},
		},
	}}
	w := getVulnerabilities(mock, "/vulnerabilities/export?repo=rpm-stable&severity=high")

	records, err := csv.NewReader(w.Body).ReadAll()
	if err != nil {
		t.Fatalf("response is not valid CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected header and 1 row, got %v", records)
	}
	if records[1][6] != "github.com/pelletier/go-toml/v2:2.0.0" || records[1][7] != "2.0.1" {
		t.Errorf("unexpected version cells: %q, %q", records[1][6], records[1][7])
	}
}

func buildReport(t *testing.T, mock *mockService, q vulnerabilityQuery) vulnerabilityResult {
	t.Helper()
	h := NewHandler(mock, nil, "")
	res, err := h.buildVulnerabilityReport(q)
	if err != nil {
		t.Fatalf("buildVulnerabilityReport: %v", err)
	}
	return res
}

func TestBuildVulnerabilityReport_AttachesFixes(t *testing.T) {
	mock := vulnMock()
	mock.fixes = models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"default/rpm-stable/portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm": {
			{IssueID: "XRAY-20", SourceComponentID: "go://github.com/pelletier/go-toml/v2:2.0.0", ComponentVersions: models.XrayComponentVersions{FixedVersions: []string{"2.0.1"}}},
		},
	}}
	res := buildReport(t, mock, vulnerabilityQuery{repo: "rpm-stable", sort: "severity", dir: "desc"})

	if res.fixesFailed != 0 {
		t.Errorf("expected no failed exports, got %d", res.fixesFailed)
	}
	if len(res.rows) != 3 || len(res.report.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d filtered and %d total", len(res.rows), len(res.report.Rows))
	}
	for _, rows := range [][]models.VulnRow{res.rows, res.report.Rows} {
		for _, row := range rows {
			if row.IssueID != "XRAY-20" {
				if row.FixesMatched || len(row.Fixes) != 0 {
					t.Errorf("row %s must not carry fixes, got %+v", row.IssueID, row.Fixes)
				}
				continue
			}
			want := []models.ComponentFix{{Component: "github.com/pelletier/go-toml/v2:2.0.0", Fixed: []string{"2.0.1"}}}
			if !row.FixesMatched || !reflect.DeepEqual(row.Fixes, want) {
				t.Errorf("row XRAY-20: got matched=%v fixes=%+v, want %+v", row.FixesMatched, row.Fixes, want)
			}
		}
	}
}

func TestBuildVulnerabilityReport_PassesOnlyArtifactsWithFindings(t *testing.T) {
	mock := vulnMock()
	mock.artifacts = append(mock.artifacts, vulnArtifact("portal/clean/clean-v1.0.0-aaaa.x86_64.rpm", "2026-10-01T00:00:00.000Z"))
	mock.xray.Artifacts = append(mock.xray.Artifacts, models.XrayArtifact{
		General: models.XrayGeneral{Path: "default/rpm-stable/portal/clean/clean-v1.0.0-aaaa.x86_64.rpm"},
	})
	buildReport(t, mock, vulnerabilityQuery{repo: "rpm-stable", sort: "severity", dir: "desc"})

	got := make([]string, 0, len(mock.fixesArtifacts))
	for _, g := range mock.fixesArtifacts {
		got = append(got, g.Path)
	}
	want := []string{
		"default/rpm-stable/portal/opcontrol/opcontrol-v1.10.52-5608d1c8.x86_64.rpm",
		"default/rpm-stable/portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fixes requested for %v, want %v", got, want)
	}
}

func TestBuildVulnerabilityReport_FixesPartiallyFailed(t *testing.T) {
	mock := vulnMock()
	mock.fixes = models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{}, Failed: 1}
	res := buildReport(t, mock, vulnerabilityQuery{repo: "rpm-stable", sort: "severity", dir: "desc"})
	if res.fixesFailed != 1 {
		t.Errorf("expected 1 failed export, got %d", res.fixesFailed)
	}
}

func TestVulnerabilities_FixesErrorStillRenders(t *testing.T) {
	mock := vulnMock()
	mock.fixesErr = errors.New("malformed export")

	res := buildReport(t, mock, vulnerabilityQuery{repo: "rpm-stable", sort: "severity", dir: "desc"})
	if res.fixesFailed != 2 {
		t.Errorf("expected both artifacts counted as failed, got %d", res.fixesFailed)
	}
	if len(res.rows) != 3 {
		t.Fatalf("expected all 3 rows kept, got %d", len(res.rows))
	}
	for _, row := range res.rows {
		if row.FixesMatched || len(row.Fixes) != 0 {
			t.Errorf("row %s must not carry fixes after an error", row.IssueID)
		}
	}

	w := getVulnerabilities(mock, "/vulnerabilities?repo=rpm-stable")
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(html.UnescapeString(w.Body.String()), "Critical issue in opcontrol") {
		t.Error("expected the findings to render despite the fixes error")
	}

	w = getVulnerabilities(mock, "/vulnerabilities/export?repo=rpm-stable")
	if w.Code != http.StatusOK {
		t.Fatalf("expected export status 200, got %d", w.Code)
	}
}

func TestVulnerabilities_NoFixesCallWithoutSummaryOrFindings(t *testing.T) {
	unavailable := vulnMock()
	unavailable.xray = models.XraySummary{Available: false}
	clean := vulnMock()
	for i := range clean.xray.Artifacts {
		clean.xray.Artifacts[i].Issues = nil
	}

	for name, mock := range map[string]*mockService{"unavailable": unavailable, "no findings": clean} {
		t.Run(name, func(t *testing.T) {
			mock.fixesErr = errors.New("must not be called")
			w := getVulnerabilities(mock, "/vulnerabilities?repo=rpm-stable")
			if w.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", w.Code)
			}
			if mock.fixesArtifacts != nil {
				t.Errorf("expected no fixes call, got %v", mock.fixesArtifacts)
			}
		})
	}
}

// findingRow returns the <tr> of the rendered report that mentions summary.
func findingRow(t *testing.T, body, summary string) string {
	t.Helper()
	idx := strings.Index(body, summary)
	if idx < 0 {
		t.Fatalf("summary %q not found in output", summary)
	}
	start := strings.LastIndex(body[:idx], "<tr>")
	end := strings.Index(body[idx:], "</tr>")
	if start < 0 || end < 0 {
		t.Fatalf("no table row around %q", summary)
	}
	return body[start : idx+end]
}

func TestGetVulnerabilities_RendersVersionColumns(t *testing.T) {
	mock := vulnMock()
	mock.fixes = models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"default/rpm-stable/portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm": {
			{IssueID: "XRAY-20", SourceComponentID: "go://golang.org/x/net:0.59.0", ComponentVersions: models.XrayComponentVersions{FixedVersions: []string{"0.60.0"}}},
			{IssueID: "XRAY-20", SourceComponentID: "go://golang.org/x/net:0.58.0", ComponentVersions: models.XrayComponentVersions{FixedVersions: []string{"0.60.0", "0.59.1"}}},
		},
		"default/rpm-stable/portal/opcontrol/opcontrol-v1.10.52-5608d1c8.x86_64.rpm": {
			{IssueID: "XRAY-11", SourceComponentID: "npm://<script>x</script>:1.0", ComponentVersions: models.XrayComponentVersions{}},
		},
	}}
	w := getVulnerabilities(mock, "/vulnerabilities?repo=rpm-stable")
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()

	assertOrder(t, body, ">Package</th>", ">Vulnerable version</th>", ">Fixed version</th>", ">Summary</th>")
	if strings.Contains(body, "<script>x</script>") {
		t.Error("component must be HTML-escaped")
	}
	if strings.Contains(body, "could not be retrieved for") {
		t.Error("notice must not show when all exports succeeded")
	}

	// several components: one line each, the same order in both cells
	opcore := findingRow(t, body, "High issue in opcore")
	assertOrder(t, opcore,
		"<code>golang.org/x/net:0.58.0</code>", "<code>golang.org/x/net:0.59.0</code>",
		"<code>0.60.0, 0.59.1</code>", "<code>0.60.0</code>")
	if strings.Count(opcore, `class="o-fix-line"`) != 4 {
		t.Errorf("expected two aligned lines per cell, got %q", opcore)
	}
	if strings.Contains(opcore, "No fix available") {
		t.Error("fixed row must not say no fix available")
	}

	// matched, no fix: title only here
	critical := findingRow(t, body, "Critical issue in opcontrol")
	if !strings.Contains(critical, "&lt;script&gt;x&lt;/script&gt;:1.0") {
		t.Errorf("expected escaped component, got %q", critical)
	}
	if !strings.Contains(critical, `title="No fix available"`) {
		t.Errorf("expected the no-fix title on a matched unfixed row, got %q", critical)
	}

	// unmatched: plain dashes in both cells
	low := findingRow(t, body, "Low issue in opcontrol")
	if strings.Contains(low, "No fix available") || strings.Contains(low, "o-fix-line") {
		t.Errorf("unmatched row must show plain dashes, got %q", low)
	}
	dash := `<td><span class="o-muted">&mdash;</span></td>`
	assertOrder(t, low, ">opcontrol</td>", dash, dash, "Low issue in opcontrol")
	if strings.Count(body, `title="No fix available"`) != 1 {
		t.Errorf("expected exactly one no-fix title, got %d", strings.Count(body, `title="No fix available"`))
	}
}

func TestGetVulnerabilities_FixesNotice(t *testing.T) {
	const notice = "Vulnerable and fixed versions could not be retrieved for 1 packages."

	partial := vulnMock()
	partial.fixes = models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{}, Failed: 1}
	w := getVulnerabilities(partial, "/vulnerabilities?repo=rpm-stable")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), notice) {
		t.Errorf("expected the notice when an export failed (status %d)", w.Code)
	}

	unavailable := vulnMock()
	unavailable.xray = models.XraySummary{Available: false}
	unavailable.fixes = models.ComponentFixes{Failed: 1}
	w = getVulnerabilities(unavailable, "/vulnerabilities?repo=rpm-stable")
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "could not be retrieved for") {
		t.Error("notice must not show when the summary itself is unavailable")
	}
}

func TestGetVulnerabilities_AllFixesFailedShowsDashesAndNotice(t *testing.T) {
	mock := vulnMock()
	mock.fixesErr = errors.New("export timed out")
	w := getVulnerabilities(mock, "/vulnerabilities?repo=rpm-stable")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Vulnerable and fixed versions could not be retrieved for 2 packages.") {
		t.Error("expected the notice counting both packages with findings")
	}
	if strings.Contains(body, "o-fix-line") || strings.Contains(body, "No fix available") {
		t.Error("no version lines may render when every export failed")
	}
	dash := `<td><span class="o-muted">&mdash;</span></td>`
	for _, summary := range []string{"Critical issue in opcontrol", "Low issue in opcontrol", "High issue in opcore"} {
		row := findingRow(t, body, summary)
		if strings.Count(row, dash) < 2 {
			t.Errorf("expected dashes in both version cells of %q, got %q", summary, row)
		}
	}
}

// TestGetVulnerabilities_FixesKeepExistingBehaviour checks that attaching fixes
// (with one export failed) changes neither not-scanned handling, the severity
// filter, sorting nor the clean list.
func TestGetVulnerabilities_FixesKeepExistingBehaviour(t *testing.T) {
	withFixes := func() *mockService {
		mock := vulnMock()
		mock.fixes = models.ComponentFixes{
			ByArtifact: map[string][]models.XrayExportRow{
				"default/rpm-stable/portal/opcontrol/opcontrol-v1.10.52-5608d1c8.x86_64.rpm": {
					{IssueID: "XRAY-11", SourceComponentID: "go://golang.org/x/net:0.59.0", ComponentVersions: models.XrayComponentVersions{FixedVersions: []string{"0.60.0"}}},
				},
			},
			Failed: 1,
		}
		return mock
	}

	t.Run("not scanned", func(t *testing.T) {
		body := html.UnescapeString(getVulnerabilities(withFixes(), "/vulnerabilities?repo=rpm-stable").Body.String())
		for _, s := range []string{"Findings: 3", "Packages scanned: 2", "Not scanned: 1", "Packages not scanned by Xray: 1", "portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm"} {
			if !strings.Contains(body, s) {
				t.Errorf("expected output to contain %q", s)
			}
		}
		assertOrder(t, body, "Critical issue in opcontrol", "Low issue in opcontrol", "High issue in opcore")
	})

	t.Run("sort", func(t *testing.T) {
		body := html.UnescapeString(getVulnerabilities(withFixes(), "/vulnerabilities?repo=rpm-stable&sort=component&dir=desc").Body.String())
		assertOrder(t, body, "High issue in opcore", "Critical issue in opcontrol", "Low issue in opcontrol")
	})

	t.Run("severity filter", func(t *testing.T) {
		body := html.UnescapeString(getVulnerabilities(withFixes(), "/vulnerabilities?repo=rpm-stable&severity=Critical").Body.String())
		if !strings.Contains(body, "Critical issue in opcontrol") || !strings.Contains(body, "<code>golang.org/x/net:0.59.0</code>") {
			t.Error("expected the critical finding with its versions")
		}
		if strings.Contains(body, "High issue in opcore") || strings.Contains(body, "Low issue in opcontrol") {
			t.Error("findings of other severities must not be listed")
		}
		if !strings.Contains(body, "Findings: 3") {
			t.Error("expected the unfiltered total in the header")
		}

		w := getVulnerabilities(withFixes(), "/vulnerabilities/export?repo=rpm-stable&severity=critical")
		records, err := csv.NewReader(w.Body).ReadAll()
		if err != nil {
			t.Fatalf("response is not valid CSV: %v", err)
		}
		if len(records) != 2 || records[1][6] != "golang.org/x/net:0.59.0" || records[1][7] != "0.60.0" {
			t.Errorf("expected one filtered CSV row with versions, got %v", records)
		}
	})

	t.Run("clean list", func(t *testing.T) {
		mock := withFixes()
		mock.xray.Artifacts[1].Issues = nil
		body := getVulnerabilities(mock, "/vulnerabilities?repo=rpm-stable").Body.String()
		if !strings.Contains(body, "scanned and clean: 1") || !strings.Contains(body, `<span class="o-clean" title="portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm">`) {
			t.Error("expected opcore to be listed as scanned and clean")
		}
		if strings.Contains(body, `title="portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm">sso`) {
			t.Error("a package Xray did not scan must not be listed as clean")
		}
	})
}
