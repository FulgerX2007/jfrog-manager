package templates

import (
	"bytes"
	"html"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"jfrog_manager/internal/models"
	"jfrog_manager/internal/report"
)

func templateDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "templates")
}

func TestTemplatesParse(t *testing.T) {
	tmpl, err := Load(templateDir())
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}

	expectedTemplates := []string{
		"layout",
		"content",
		"artifact_list",
		"upload_form",
		"xray_panel",
		"error",
	}
	for _, name := range expectedTemplates {
		if tmpl.Lookup(name) == nil {
			t.Errorf("template %q not found", name)
		}
	}
}

func TestFuncMap(t *testing.T) {
	fm := FuncMap()

	t.Run("toLower", func(t *testing.T) {
		fn := fm["toLower"].(func(string) string)
		if got := fn("Critical"); got != "critical" {
			t.Errorf("toLower(Critical) = %q, want %q", got, "critical")
		}
	})

	t.Run("countBySeverity", func(t *testing.T) {
		fn := fm["countBySeverity"].(func([]models.XrayIssue, string) int)
		issues := []models.XrayIssue{
			{Severity: "Critical"},
			{Severity: "High"},
			{Severity: "Critical"},
			{Severity: "Low"},
		}
		if got := fn(issues, "Critical"); got != 2 {
			t.Errorf("countBySeverity(Critical) = %d, want 2", got)
		}
		if got := fn(issues, "Medium"); got != 0 {
			t.Errorf("countBySeverity(Medium) = %d, want 0", got)
		}
	})
}

func TestLayoutRenders(t *testing.T) {
	tmpl, err := Load(templateDir())
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}

	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "layout", map[string]any{"DefaultRepo": ""})
	if err != nil {
		t.Fatalf("failed to execute layout: %v", err)
	}

	output := buf.String()
	mustContain := []string{
		"<!DOCTYPE html>",
		"JFrog Manager",
		"htmx",
		`hx-get="/vulnerabilities"`,
		"Vulnerabilities",
	}
	for _, s := range mustContain {
		if !bytes.Contains([]byte(output), []byte(s)) {
			t.Errorf("layout output missing %q", s)
		}
	}
}

func renderArtifactList(t *testing.T, artifacts []models.Artifact, view, component string) string {
	t.Helper()
	tmpl, err := Load(templateDir())
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}
	data := map[string]any{"Repo": "libs-release", "List": report.BuildArtifactView(artifacts, view, component)}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "artifact_list", data); err != nil {
		t.Fatalf("failed to render: %v", err)
	}
	return buf.String()
}

func TestArtifactListRenders(t *testing.T) {
	artifacts := []models.Artifact{
		{Name: "test.jar", Path: "com/test/test.jar", Size: 1024, LastModified: "2024-01-01", Repo: "libs-release"},
		{Name: "api-v1.1.x86_64.rpm", Path: "portal/api/api-v1.1.x86_64.rpm", Size: 2048, LastModified: "2026-10-06T07:14:47.134Z", Repo: "libs-release"},
		{Name: "api-v1.0.x86_64.rpm", Path: "portal/api/api-v1.0.x86_64.rpm", Size: 2048, LastModified: "2026-09-01T07:00:00.000Z", Repo: "libs-release"},
	}

	t.Run("latest view", func(t *testing.T) {
		output := renderArtifactList(t, artifacts, "", "")
		for _, s := range []string{
			"test.jar",
			"v1.1",
			"2026-10-06 07:14",
			"1.00 KiB",
			"+1 older",
			"Packages: 2",
			"Files in the repository: 3",
			`data-path="portal/api/api-v1.1.x86_64.rpm"`,
			"toggleXray(this)",
		} {
			if !strings.Contains(output, s) {
				t.Errorf("artifact_list output missing %q", s)
			}
		}
		if strings.Contains(output, "api-v1.0.x86_64.rpm") {
			t.Error("an older version must not be listed in the latest view")
		}
		// Regression: download href must be single-encoded. Previously we
		// combined urlEncode with html/template's URL auto-escape, which
		// produced %252F in place of %2F and made JFrog return 404.
		if strings.Contains(output, "%252F") {
			t.Error("download href is double-encoded (contains %252F)")
		}
		// And must not contain raw '/' in the query param — that would mean
		// no encoding at all was applied.
		if !strings.Contains(output, "path=com%2ftest%2ftest.jar") && !strings.Contains(output, "path=com%2Ftest%2Ftest.jar") {
			t.Errorf("expected single-encoded path in href; got: %s", output)
		}
		links := html.UnescapeString(output)
		for _, s := range []string{
			`hx-get="/artifacts?repo=libs-release&view=all&component=portal"`,
			`hx-get="/artifacts?repo=libs-release&view=latest&component=com"`,
			`hx-get="/artifacts?repo=libs-release&view=all&component="`,
		} {
			if !strings.Contains(links, s) {
				t.Errorf("artifact_list output missing link %q", s)
			}
		}
	})

	t.Run("all files view marks older versions", func(t *testing.T) {
		output := renderArtifactList(t, artifacts, "all", "portal")
		if !strings.Contains(output, "api-v1.0.x86_64.rpm") || !strings.Contains(output, "Older version") {
			t.Error("expected the older version to be listed and marked")
		}
		if !strings.Contains(output, `<span class="o-tag">Latest</span>`) {
			t.Error("expected the latest version to be tagged")
		}
		if strings.Contains(output, "test.jar") {
			t.Error("artifacts of another component must not be listed")
		}
		if strings.Count(output, "toggleXray(this)") != 1 {
			t.Errorf("Xray must be offered for the latest version only, got %d buttons", strings.Count(output, "toggleXray(this)"))
		}
	})

	t.Run("empty", func(t *testing.T) {
		output := renderArtifactList(t, nil, "", "")
		if !strings.Contains(output, "No artifacts found") {
			t.Error("empty message not found")
		}
	})
}

func TestFuncMap_ShortTime(t *testing.T) {
	cases := map[string]string{
		"2026-10-06T07:14:47.134Z":     "2026-10-06 07:14",
		"2026-10-06T10:00:00.000+0300": "2026-10-06 07:00",
		"2024-01-01":                   "2024-01-01",
		"":                             "",
	}
	fn := FuncMap()["shortTime"].(func(string) string)
	for in, want := range cases {
		if got := fn(in); got != want {
			t.Errorf("shortTime(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrorRenders(t *testing.T) {
	tmpl, err := Load(templateDir())
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}

	data := struct{ Error string }{Error: "something went wrong"}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "error", data); err != nil {
		t.Fatalf("failed to render: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("something went wrong")) {
		t.Error("error message not found in output")
	}
}

func TestXrayPanelRenders(t *testing.T) {
	tmpl, err := Load(templateDir())
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}

	t.Run("unavailable", func(t *testing.T) {
		data := models.XraySummary{Available: false}
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "xray_panel", data); err != nil {
			t.Fatalf("failed to render: %v", err)
		}
		if !bytes.Contains(buf.Bytes(), []byte("not configured")) {
			t.Error("unavailable message not found")
		}
	})

	t.Run("with vulnerabilities", func(t *testing.T) {
		data := models.XraySummary{
			Available: true,
			Artifacts: []models.XrayArtifact{
				{
					Issues: []models.XrayIssue{
						{
							Severity: "Critical",
							Summary:  "Test vulnerability",
							CVEs:     []models.XrayCVE{{ID: "CVE-2024-1234"}},
						},
					},
				},
			},
		}
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "xray_panel", data); err != nil {
			t.Fatalf("failed to render: %v", err)
		}
		output := buf.String()
		if !bytes.Contains([]byte(output), []byte("CVE-2024-1234")) {
			t.Error("CVE not found in output")
		}
		if !bytes.Contains([]byte(output), []byte("Test vulnerability")) {
			t.Error("vulnerability summary not found")
		}
	})
}

func TestUploadFormRenders(t *testing.T) {
	tmpl, err := Load(templateDir())
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "upload_form", nil); err != nil {
		t.Fatalf("failed to render upload_form: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("uploadWithProgress")) {
		t.Error("upload form missing progress upload handler")
	}
}

func TestFuncMap_CssID(t *testing.T) {
	fm := FuncMap()
	fn := fm["cssID"].(func(string) string)

	if got := fn("com/example/app.jar"); got != "com_sexample_sapp_djar" {
		t.Errorf("cssID(com/example/app.jar) = %q, want %q", got, "com_sexample_sapp_djar")
	}
	if got := fn("simple"); got != "simple" {
		t.Errorf("cssID(simple) = %q, want %q", got, "simple")
	}
	// Verify injectivity: paths that previously collided must now differ
	if fn("a/b") == fn("a_sb") {
		t.Errorf("cssID is not injective: a/b and a_sb both produce %q", fn("a/b"))
	}
	// Underscores in input must be escaped
	if got := fn("a_b"); got != "a__b" {
		t.Errorf("cssID(a_b) = %q, want %q", got, "a__b")
	}
}

func TestFuncMap_HumanSize(t *testing.T) {
	fm := FuncMap()
	fn := fm["humanSize"].(func(int64) string)

	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.00 KiB"},
		{1536, "1.50 KiB"},
		{1024 * 1024, "1.00 MiB"},
		{int64(1.5 * 1024 * 1024), "1.50 MiB"},
		{1024 * 1024 * 1024, "1.00 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.00 TiB"},
	}
	for _, c := range cases {
		if got := fn(c.in); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFuncMap_UrlEncode(t *testing.T) {
	fm := FuncMap()
	fn := fm["urlEncode"].(func(string) string)

	if got := fn("com/example/app.jar"); got != "com%2Fexample%2Fapp.jar" {
		t.Errorf("urlEncode(com/example/app.jar) = %q, want %q", got, "com%%2Fexample%%2Fapp.jar")
	}
}

func renderVulnReport(t *testing.T, data map[string]any) string {
	t.Helper()
	tmpl, err := Load(templateDir())
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "vuln_report", data); err != nil {
		t.Fatalf("failed to render vuln_report: %v", err)
	}
	return buf.String()
}

func vulnReportRows() []models.VulnRow {
	return []models.VulnRow{
		{
			Component: "opcore", Package: "opcore",
			ArtifactName: "opcore-v1.0.x86_64.rpm", ArtifactPath: "portal/opcore/opcore-v1.0.x86_64.rpm",
			IssueID: "XRAY-1", Severity: "Critical", CVEs: []string{"CVE-2026-1"},
			Summary:     "Parser <overflow>",
			ImpactPaths: []string{"opt/bin/opcore/mod/a", "opt/bin/tool/mod/a", "opt/bin/other/mod/a"},
		},
		{
			Component: "sso", Package: "sso", ArtifactName: "sso-v1.0.x86_64.rpm",
			IssueID: "XRAY-2", Severity: "Unknown", Summary: "No CVE assigned",
		},
	}
}

type testTile struct {
	Key    string
	Label  string
	Count  int
	Active bool
}

func vulnReportData(rows []models.VulnRow, sortBy, dir, severity string) map[string]any {
	labels := map[string]string{"critical": "Critical", "high": "High", "medium": "Medium", "low": "Low", "unknown": "Unknown"}
	counts := map[string]int{}
	for _, r := range rows {
		counts[report.SeverityKey(r.Severity)]++
	}
	var tiles []testTile
	for _, key := range report.SeverityKeys {
		tiles = append(tiles, testTile{Key: key, Label: labels[key], Count: counts[key], Active: key == severity})
	}
	shown := report.FilterBySeverity(rows, severity)
	return map[string]any{
		"Repo":       "rpm stable",
		"Available":  true,
		"Sort":       sortBy,
		"Dir":        dir,
		"Severity":   severity,
		"Total":      len(rows),
		"Shown":      len(shown),
		"Tiles":      tiles,
		"Groups":     report.GroupRows(shown, sortBy, dir),
		"Scanned":    3,
		"NotScanned": []report.NotScanned{},
		"Excluded":   9,
		"Clean":      []models.Artifact{{Name: "nginx-1.0.x86_64.rpm", Path: "portal/nginx/nginx-1.0.x86_64.rpm"}},
	}
}

func TestVulnReportRenders(t *testing.T) {
	t.Run("with rows", func(t *testing.T) {
		output := renderVulnReport(t, vulnReportData(vulnReportRows(), "severity", "desc", ""))
		mustContain := []string{
			"Findings: 2", "Packages scanned: 3", "Not scanned: 0",
			"Latest version of each package only.",
			"(9)",
			"o-badge-critical", "1 Critical", "1 Unknown",
			"<code>CVE-2026-1</code>",
			"<code>XRAY-2</code>",
			"Parser &lt;overflow&gt;",
			"opt/bin/opcore/mod/a",
			"+2 more",
			"opt/bin/other/mod/a",
			"opcore-v1.0.x86_64.rpm",
			"scanned and clean: 1",
			"nginx-1.0.x86_64.rpm",
			"Most severe first",
			`hx-target="#artifact-list"`,
		}
		for _, s := range mustContain {
			if !strings.Contains(output, s) {
				t.Errorf("vuln_report output missing %q", s)
			}
		}
		links := []string{
			`href="/vulnerabilities/export?repo=rpm%20stable&sort=severity&dir=desc"`,
			// Reversing the order, switching the key and filtering by severity.
			`hx-get="/vulnerabilities?repo=rpm+stable&sort=severity&dir=asc"`,
			`hx-get="/vulnerabilities?repo=rpm+stable&sort=component&dir=asc"`,
			`hx-get="/vulnerabilities?repo=rpm+stable&sort=severity&dir=desc&severity=critical"`,
		}
		for _, s := range links {
			if !strings.Contains(html.UnescapeString(output), s) {
				t.Errorf("vuln_report output missing link %q", s)
			}
		}
		if strings.Contains(output, "<code>XRAY-1</code>") {
			t.Error("issue id should not be shown when the row has CVEs")
		}
		if strings.Contains(output, "not scanned by Xray") {
			t.Error("not-scanned warning should not be shown when everything was scanned")
		}
		if strings.Index(output, "Parser") > strings.Index(output, "No CVE assigned") {
			t.Error("the group with the critical finding should come first")
		}
	})

	t.Run("component order and severity filter are carried by the links", func(t *testing.T) {
		output := html.UnescapeString(renderVulnReport(t, vulnReportData(vulnReportRows(), "component", "asc", "unknown")))
		for _, s := range []string{
			`hx-get="/vulnerabilities?repo=rpm+stable&sort=component&dir=desc&severity=unknown"`,
			`hx-get="/vulnerabilities?repo=rpm+stable&sort=severity&dir=desc&severity=unknown"`,
			`href="/vulnerabilities/export?repo=rpm%20stable&sort=component&dir=asc&severity=unknown"`,
			// The active tile clears the filter.
			`hx-get="/vulnerabilities?repo=rpm+stable&sort=component&dir=asc"`,
			"A to Z",
			"Show all severities",
			"No CVE assigned",
		} {
			if !strings.Contains(output, s) {
				t.Errorf("vuln_report output missing %q", s)
			}
		}
		if strings.Contains(output, "Parser <overflow>") {
			t.Error("rows of other severities must not be listed when filtering")
		}
	})

	t.Run("no rows", func(t *testing.T) {
		output := renderVulnReport(t, vulnReportData(nil, "severity", "desc", ""))
		if !strings.Contains(output, "No vulnerabilities found in the scanned packages (3).") {
			t.Error("expected the empty state")
		}
		if strings.Contains(output, "/vulnerabilities/export") {
			t.Error("export link should not be rendered without rows")
		}
	})

	t.Run("severity filter without matches", func(t *testing.T) {
		output := renderVulnReport(t, vulnReportData(vulnReportRows(), "severity", "desc", "low"))
		if !strings.Contains(output, "No findings of this severity.") {
			t.Error("expected the filtered empty state")
		}
		if strings.Contains(output, "No vulnerabilities found") {
			t.Error("a filtered empty list must not read as an all-clear")
		}
	})

	t.Run("not scanned warning alongside an empty result", func(t *testing.T) {
		data := vulnReportData(nil, "severity", "desc", "")
		data["NotScanned"] = []report.NotScanned{
			{Path: "portal/nginx/nginx-1.0.x86_64.rpm", Reason: "not indexed"},
			{Path: "portal/kafka/kafka-1.0.x86_64.rpm"},
		}
		output := renderVulnReport(t, data)
		for _, s := range []string{
			"Packages not scanned by Xray: 2",
			"Not scanned: 2",
			"portal/nginx/nginx-1.0.x86_64.rpm",
			"not indexed",
			"portal/kafka/kafka-1.0.x86_64.rpm",
			"No vulnerabilities found in the scanned packages (3).",
		} {
			if !strings.Contains(output, s) {
				t.Errorf("vuln_report output missing %q", s)
			}
		}
	})

	t.Run("xray unavailable", func(t *testing.T) {
		output := renderVulnReport(t, map[string]any{"Repo": "r", "Available": false})
		if !strings.Contains(output, "Xray data could not be retrieved") {
			t.Error("expected the unavailable notice")
		}
		if strings.Contains(output, "<table") || strings.Contains(output, "No vulnerabilities found") || strings.Contains(output, "Latest version of each package only") {
			t.Error("unavailable state must not render a table, an all-clear message or the report banner")
		}
	})
}

func TestFuncMap_SortBySeverityRanksUnknownLast(t *testing.T) {
	fn := FuncMap()["sortBySeverity"].(func([]models.XrayIssue) []models.XrayIssue)
	sorted := fn([]models.XrayIssue{{Severity: "Unknown"}, {Severity: "low"}, {Severity: "Weird"}, {Severity: "Critical"}, {Severity: "High"}})
	var got []string
	for _, issue := range sorted {
		got = append(got, issue.Severity)
	}
	if strings.Join(got, ",") != "Critical,High,low,Unknown,Weird" {
		t.Errorf("unexpected order: %v", got)
	}
}
