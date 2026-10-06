package report

import (
	"testing"

	"jfrog_manager/internal/models"
)

func xrayArtifact(p string, issues ...models.XrayIssue) models.XrayArtifact {
	return models.XrayArtifact{General: models.XrayGeneral{Path: p}, Issues: issues}
}

func TestBuild_MatchesBothPathForms(t *testing.T) {
	latest := []models.Artifact{
		artifact("portal/opcore/opcore-v1.0.x86_64.rpm", ""),
		artifact("portal/opcontrol/opcontrol-v2.0.x86_64.rpm", ""),
	}
	summary := models.XraySummary{Available: true, Artifacts: []models.XrayArtifact{
		xrayArtifact("default/repo/portal/opcore/opcore-v1.0.x86_64.rpm", models.XrayIssue{Severity: "High"}),
		xrayArtifact("repo/portal/opcontrol/opcontrol-v2.0.x86_64.rpm", models.XrayIssue{Severity: "Low"}),
	}}

	report := Build("repo", latest, summary)

	if report.Scanned != 2 || len(report.NotScanned) != 0 {
		t.Fatalf("expected 2 scanned and none not scanned, got %d / %v", report.Scanned, report.NotScanned)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(report.Rows))
	}
	if report.Rows[0].Component != "opcore" || report.Rows[1].Component != "opcontrol" {
		t.Errorf("unexpected components: %q, %q", report.Rows[0].Component, report.Rows[1].Component)
	}
}

func TestBuild_PopulatesRowFields(t *testing.T) {
	latest := []models.Artifact{
		artifact("portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm", ""),
		artifact("portal/sso/sso-v1.0.x86_64.rpm", ""),
	}
	prefix := "default/repo/portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm"
	summary := models.XraySummary{Available: true, Artifacts: []models.XrayArtifact{
		xrayArtifact(prefix,
			models.XrayIssue{
				IssueID:  "XRAY-1",
				Summary:  "Parser recursion",
				Severity: "High",
				CVEs:     []models.XrayCVE{{ID: "CVE-2026-1"}, {CVSS3: "7.5"}, {ID: "CVE-2026-2"}},
				ImpactPaths: []string{
					prefix + "/./opt/opcore/bin/opcore/github.com/pelletier/go-toml/v2",
					prefix + "/opt/opcore/bin/tool/github.com/pelletier/go-toml/v2",
				},
			},
			models.XrayIssue{IssueID: "XRAY-2", Summary: "No CVE", Severity: "Unknown", CVEs: []models.XrayCVE{{CVSS3: "5.0"}}},
		),
		xrayArtifact("default/repo/portal/sso/sso-v1.0.x86_64.rpm"),
	}}

	report := Build("repo", latest, summary)

	if len(report.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(report.Rows))
	}
	row := report.Rows[0]
	if row.Component != "opcore" || row.Package != "opcore" {
		t.Errorf("unexpected component/package: %q / %q", row.Component, row.Package)
	}
	if row.ArtifactName != "opcore-v1.10.25-5314aa81.x86_64.rpm" || row.ArtifactPath != "portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm" {
		t.Errorf("unexpected artifact: %q / %q", row.ArtifactName, row.ArtifactPath)
	}
	if row.IssueID != "XRAY-1" || row.Severity != "High" || row.Summary != "Parser recursion" {
		t.Errorf("unexpected issue fields: %+v", row)
	}
	if !equalStrings(row.CVEs, []string{"CVE-2026-1", "CVE-2026-2"}) {
		t.Errorf("unexpected CVEs: %v", row.CVEs)
	}
	wantPaths := []string{
		"opt/opcore/bin/opcore/github.com/pelletier/go-toml/v2",
		"opt/opcore/bin/tool/github.com/pelletier/go-toml/v2",
	}
	if !equalStrings(row.ImpactPaths, wantPaths) {
		t.Errorf("unexpected impact paths: %v", row.ImpactPaths)
	}
	if len(report.Rows[1].CVEs) != 0 || report.Rows[1].IssueID != "XRAY-2" {
		t.Errorf("expected second row without CVEs and with its issue id, got %+v", report.Rows[1])
	}
}

func TestBuild_CoverageAccounting(t *testing.T) {
	latest := []models.Artifact{
		artifact("portal/api/api-v1.0.x86_64.rpm", ""),
		artifact("portal/sso/sso-v1.0.x86_64.rpm", ""),
		artifact("portal/nginx/nginx-1.0.x86_64.rpm", ""),
		artifact("portal/kafka/kafka-1.0.x86_64.rpm", ""),
	}
	summary := models.XraySummary{
		Available: true,
		Artifacts: []models.XrayArtifact{
			xrayArtifact("default/repo/portal/api/api-v1.0.x86_64.rpm",
				models.XrayIssue{Severity: "Critical"}, models.XrayIssue{Severity: "high"}, models.XrayIssue{Severity: "High"}),
			xrayArtifact("default/repo/portal/sso/sso-v1.0.x86_64.rpm"),
			xrayArtifact("default/repo/portal/other/unrequested-1.0.x86_64.rpm", models.XrayIssue{Severity: "Critical"}),
		},
		Errors: []models.XrayError{
			{Identifier: "default/repo/portal/nginx/nginx-1.0.x86_64.rpm", Error: "not indexed"},
		},
	}

	report := Build("repo", latest, summary)

	if report.Scanned != 2 {
		t.Errorf("expected 2 scanned (one clean, one with issues), got %d", report.Scanned)
	}
	if len(report.Rows) != 3 {
		t.Errorf("expected 3 rows, none from the unrequested artifact, got %d", len(report.Rows))
	}
	if len(report.NotScanned) != 2 {
		t.Fatalf("expected 2 not scanned, got %v", report.NotScanned)
	}
	if report.NotScanned[0].Path != "portal/nginx/nginx-1.0.x86_64.rpm" || report.NotScanned[0].Reason != "not indexed" {
		t.Errorf("unexpected not-scanned entry with reason: %+v", report.NotScanned[0])
	}
	if report.NotScanned[1].Path != "portal/kafka/kafka-1.0.x86_64.rpm" || report.NotScanned[1].Reason != "" {
		t.Errorf("unexpected not-scanned entry without reason: %+v", report.NotScanned[1])
	}
	if report.Counts["critical"] != 1 || report.Counts["high"] != 2 {
		t.Errorf("unexpected counts: %v", report.Counts)
	}
}

func TestBuild_Empty(t *testing.T) {
	report := Build("repo", nil, models.XraySummary{Available: true})
	if report.Rows == nil || len(report.Rows) != 0 || report.Scanned != 0 || len(report.NotScanned) != 0 {
		t.Errorf("expected an empty report, got %+v", report)
	}
}

func row(component, pkg, severity string) models.VulnRow {
	return models.VulnRow{Component: component, Package: pkg, Severity: severity}
}

func describe(rows []models.VulnRow) []string {
	result := make([]string, 0, len(rows))
	for _, r := range rows {
		result = append(result, r.Component+"/"+r.Package+"/"+r.Severity)
	}
	return result
}

func TestSortRows(t *testing.T) {
	rows := []models.VulnRow{
		row("opcore", "opcore", "Medium"),
		row("api", "api", "Unknown"),
		row("opcontrol", "opcontrol", "critical"),
		row("api", "api", "High"),
		row("opcore", "opcore", "Critical"),
		row("api", "aaa", "High"),
		row("sso", "sso", "Low"),
	}

	tests := []struct {
		name string
		by   string
		dir  string
		want []string
	}{
		{
			name: "severity desc is most severe first",
			by:   "severity", dir: "desc",
			want: []string{
				"opcontrol/opcontrol/critical", "opcore/opcore/Critical",
				"api/aaa/High", "api/api/High",
				"opcore/opcore/Medium", "sso/sso/Low", "api/api/Unknown",
			},
		},
		{
			name: "severity asc is least severe first",
			by:   "severity", dir: "asc",
			want: []string{
				"api/api/Unknown", "sso/sso/Low", "opcore/opcore/Medium",
				"api/aaa/High", "api/api/High",
				"opcontrol/opcontrol/critical", "opcore/opcore/Critical",
			},
		},
		{
			name: "component asc, then most severe first",
			by:   "component", dir: "asc",
			want: []string{
				"api/aaa/High", "api/api/High", "api/api/Unknown",
				"opcontrol/opcontrol/critical",
				"opcore/opcore/Critical", "opcore/opcore/Medium",
				"sso/sso/Low",
			},
		},
		{
			name: "component desc, still most severe first inside a component",
			by:   "component", dir: "desc",
			want: []string{
				"sso/sso/Low",
				"opcore/opcore/Critical", "opcore/opcore/Medium",
				"opcontrol/opcontrol/critical",
				"api/aaa/High", "api/api/High", "api/api/Unknown",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describe(SortRows(rows, tt.by, tt.dir))
			if !equalStrings(got, tt.want) {
				t.Errorf("SortRows(%q, %q) =\n %v\nwant\n %v", tt.by, tt.dir, got, tt.want)
			}
		})
	}

	t.Run("unknown key and direction fall back to severity desc", func(t *testing.T) {
		got := describe(SortRows(rows, "bogus", "sideways"))
		if !equalStrings(got, tests[0].want) {
			t.Errorf("unexpected fallback order: %v", got)
		}
	})

	t.Run("input is not mutated", func(t *testing.T) {
		if rows[0].Severity != "Medium" || rows[6].Component != "sso" {
			t.Errorf("input slice was reordered: %v", describe(rows))
		}
	})
}

func TestNormalizeSort(t *testing.T) {
	tests := []struct {
		by, dir         string
		wantBy, wantDir string
	}{
		{"", "", "severity", "desc"},
		{"severity", "asc", "severity", "asc"},
		{"component", "", "component", "asc"},
		{"component", "desc", "component", "desc"},
		{"bogus", "asc", "severity", "asc"},
		{"severity", "bogus", "severity", "desc"},
	}
	for _, tt := range tests {
		by, dir := NormalizeSort(tt.by, tt.dir)
		if by != tt.wantBy || dir != tt.wantDir {
			t.Errorf("NormalizeSort(%q, %q) = %q, %q; want %q, %q", tt.by, tt.dir, by, dir, tt.wantBy, tt.wantDir)
		}
	}
}

func TestBuild_ListsCleanArtifacts(t *testing.T) {
	latest := []models.Artifact{
		artifact("portal/api/api-v1.0.x86_64.rpm", ""),
		artifact("portal/sso/sso-v1.0.x86_64.rpm", ""),
	}
	summary := models.XraySummary{Available: true, Artifacts: []models.XrayArtifact{
		xrayArtifact("default/repo/portal/api/api-v1.0.x86_64.rpm", models.XrayIssue{Severity: "Weird"}),
		xrayArtifact("default/repo/portal/sso/sso-v1.0.x86_64.rpm"),
	}}

	report := Build("repo", latest, summary)

	if len(report.Clean) != 1 || report.Clean[0].Name != "sso-v1.0.x86_64.rpm" {
		t.Errorf("expected only sso to be clean, got %+v", report.Clean)
	}
	if report.Counts["unknown"] != 1 {
		t.Errorf("expected an unrecognised severity to count as unknown, got %v", report.Counts)
	}
}

func TestSeverityKeyAndNormalize(t *testing.T) {
	keys := map[string]string{"Critical": "critical", "HIGH": "high", "medium": "medium", "Low": "low", "Unknown": "unknown", "": "unknown", "Weird": "unknown"}
	for in, want := range keys {
		if got := SeverityKey(in); got != want {
			t.Errorf("SeverityKey(%q) = %q, want %q", in, got, want)
		}
	}
	filters := map[string]string{"critical": "critical", "High": "high", "unknown": "unknown", "": "", "bogus": ""}
	for in, want := range filters {
		if got := NormalizeSeverity(in); got != want {
			t.Errorf("NormalizeSeverity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilterBySeverity(t *testing.T) {
	rows := []models.VulnRow{
		row("api", "api", "High"), row("api", "api", "Unknown"), row("sso", "sso", "high"), row("sso", "sso", "Weird"),
	}
	if got := describe(FilterBySeverity(rows, "high")); !equalStrings(got, []string{"api/api/High", "sso/sso/high"}) {
		t.Errorf("unexpected high rows: %v", got)
	}
	if got := describe(FilterBySeverity(rows, "unknown")); !equalStrings(got, []string{"api/api/Unknown", "sso/sso/Weird"}) {
		t.Errorf("unexpected unknown rows: %v", got)
	}
	if got := FilterBySeverity(rows, ""); len(got) != 4 {
		t.Errorf("an empty key should keep all rows, got %d", len(got))
	}
	if got := FilterBySeverity(rows, "critical"); len(got) != 0 {
		t.Errorf("expected no critical rows, got %d", len(got))
	}
}

func groupNames(groups []Group) []string {
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Component)
	}
	return names
}

func TestGroupRows(t *testing.T) {
	artifactRow := func(component, artifactName, severity string) models.VulnRow {
		return models.VulnRow{Component: component, Package: component, ArtifactName: artifactName, Severity: severity}
	}
	rows := []models.VulnRow{
		artifactRow("nginx", "nginx-1.rpm", "Medium"),
		artifactRow("kafka", "kafka-1.rpm", "High"),
		artifactRow("kafka", "kafka-1.rpm", "Critical"),
		artifactRow("grafana", "grafana-1.rpm", "Critical"),
		artifactRow("grafana", "grafana-1.rpm", "Critical"),
		artifactRow("clickhouse", "clickhouse-client-1.rpm", "Low"),
		artifactRow("clickhouse", "clickhouse-server-1.rpm", "High"),
	}

	tests := []struct {
		name    string
		by, dir string
		want    []string
	}{
		{"most severe first", "severity", "desc", []string{"grafana", "kafka", "clickhouse", "nginx"}},
		{"least severe first", "severity", "asc", []string{"nginx", "clickhouse", "kafka", "grafana"}},
		{"component ascending", "component", "asc", []string{"clickhouse", "grafana", "kafka", "nginx"}},
		{"component descending", "component", "desc", []string{"nginx", "kafka", "grafana", "clickhouse"}},
		{"defaults", "", "", []string{"grafana", "kafka", "clickhouse", "nginx"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupNames(GroupRows(rows, tt.by, tt.dir)); !equalStrings(got, tt.want) {
				t.Errorf("GroupRows(%q, %q) order = %v, want %v", tt.by, tt.dir, got, tt.want)
			}
		})
	}

	groups := GroupRows(rows, "component", "asc")
	clickhouse := groups[0]
	if !equalStrings(clickhouse.Artifacts, []string{"clickhouse-server-1.rpm", "clickhouse-client-1.rpm"}) {
		t.Errorf("unexpected artifacts in group: %v", clickhouse.Artifacts)
	}
	if clickhouse.Rows[0].Severity != "High" || clickhouse.Rows[1].Severity != "Low" {
		t.Errorf("rows inside a group should run from most to least severe: %v", describe(clickhouse.Rows))
	}
	if clickhouse.Counts["high"] != 1 || clickhouse.Counts["low"] != 1 {
		t.Errorf("unexpected group counts: %v", clickhouse.Counts)
	}
	if kafka := groups[2]; len(kafka.Artifacts) != 1 || len(kafka.Rows) != 2 {
		t.Errorf("unexpected kafka group: %+v", kafka)
	}
	if got := GroupRows(nil, "", ""); len(got) != 0 {
		t.Errorf("expected no groups for no rows, got %d", len(got))
	}
}

func TestTrimImpactPaths(t *testing.T) {
	got := TrimImpactPaths("repo", "/portal/api/api-1.rpm", []string{
		"default/repo/portal/api/api-1.rpm/./opt/api/mod/a",
		"default/repo/portal/api/api-1.rpm",
	})
	if !equalStrings(got, []string{"opt/api/mod/a"}) {
		t.Errorf("unexpected trimmed paths: %v", got)
	}
}
