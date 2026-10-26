package report

import (
	"reflect"
	"testing"

	"jfrog_manager/internal/models"
)

func exportRow(issue, comp string, fixed ...string) models.XrayExportRow {
	return models.XrayExportRow{
		IssueID:           issue,
		SourceComponentID: comp,
		ComponentVersions: models.XrayComponentVersions{FixedVersions: fixed},
	}
}

func vulnRow(path, issue string) models.VulnRow {
	return models.VulnRow{ArtifactPath: path, IssueID: issue}
}

func TestAttachVersions_MatchesByArtifactAndIssue(t *testing.T) {
	rows := []models.VulnRow{
		vulnRow("/portal/opcore/opcore-1.0.rpm", "XRAY-1"),
		vulnRow("/portal/opcore/opcore-1.0.rpm", "XRAY-2"),
	}
	fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"default/repo/portal/opcore/opcore-1.0.rpm": {
			exportRow("XRAY-1", "go://golang.org/x/net:0.59.0", "0.60.0"),
			exportRow("XRAY-3", "go://other:1.0", "1.1"),
		},
	}}

	got := AttachVersions("repo", rows, fixes)

	want := []models.ComponentFix{{Component: "golang.org/x/net:0.59.0", Fixed: []string{"0.60.0"}}}
	if !reflect.DeepEqual(got[0].Fixes, want) {
		t.Errorf("row 0: fixes=%v, want %v", got[0].Fixes, want)
	}
	if len(got[1].Fixes) != 0 {
		t.Errorf("row 1 should be unmatched, got fixes=%v", got[1].Fixes)
	}
}

func TestAttachVersions_StripsComponentScheme(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"go://golang.org/x/net:0.59.0", "golang.org/x/net:0.59.0"},
		{"rpm://9:openssl-libs:3.0.7-27.el9", "9:openssl-libs:3.0.7-27.el9"},
		{"npm://lodash:4.17.20", "lodash:4.17.20"},
		{"golang.org/x/net:0.59.0", "golang.org/x/net:0.59.0"},
	}
	for _, tt := range tests {
		fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
			"repo/a.rpm": {exportRow("XRAY-1", tt.in, "2")},
		}}
		got := AttachVersions("repo", []models.VulnRow{vulnRow("/a.rpm", "XRAY-1")}, fixes)
		if len(got[0].Fixes) != 1 || got[0].Fixes[0].Component != tt.want {
			t.Errorf("component %q: got fixes %+v, want label %q", tt.in, got[0].Fixes, tt.want)
		}
	}
}

func TestAttachVersions_IgnoresExportRowsWithoutIDOrComponent(t *testing.T) {
	rows := []models.VulnRow{vulnRow("/a.rpm", ""), vulnRow("/a.rpm", "XRAY-1"), vulnRow("/a.rpm", "XRAY-2")}
	fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"repo/a.rpm": {
			exportRow("", "go://x:1", "2"),
			exportRow("XRAY-1", "", "3"),
			exportRow("XRAY-2", "", "4"),
			exportRow("XRAY-2", "go://y:1", "5"),
		},
	}}

	got := AttachVersions("repo", rows, fixes)

	if len(got[0].Fixes) != 0 {
		t.Errorf("row without issue id must not match, got %+v", got[0].Fixes)
	}
	if len(got[1].Fixes) != 0 {
		t.Errorf("export row without component must be ignored, got %+v", got[1].Fixes)
	}
	want := []models.ComponentFix{{Component: "y:1", Fixed: []string{"5"}}}
	if !reflect.DeepEqual(got[2].Fixes, want) {
		t.Errorf("row XRAY-2: fixes=%+v, want %+v", got[2].Fixes, want)
	}
}

func TestAttachVersions_PathForms(t *testing.T) {
	keys := []string{
		"default/repo/a/b.rpm",
		"repo/a/b.rpm",
		"/repo/a/b.rpm/",
		"a/b.rpm",
	}
	for _, key := range keys {
		fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
			key: {exportRow("XRAY-1", "go://x:1", "2")},
		}}
		for _, path := range []string{"/a/b.rpm", "a/b.rpm"} {
			got := AttachVersions("repo", []models.VulnRow{vulnRow(path, "XRAY-1")}, fixes)
			if len(got[0].Fixes) != 1 {
				t.Errorf("key %q, path %q: expected a match", key, path)
			}
		}
	}
}

func TestAttachVersions_SeveralComponentsSortedAndDeduplicated(t *testing.T) {
	rows := []models.VulnRow{vulnRow("/a.rpm", "XRAY-1")}
	fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"repo/a.rpm": {
			exportRow("XRAY-1", "go://golang.org/x/net:0.59.0", "0.60.0"),
			exportRow("XRAY-1", "go://golang.org/x/net:0.58.0", "0.60.0", "0.58.5"),
			exportRow("XRAY-1", "go://golang.org/x/net:0.59.0", "0.60.0", "0.59.1"),
		},
	}}

	got := AttachVersions("repo", rows, fixes)

	want := []models.ComponentFix{
		{Component: "golang.org/x/net:0.58.0", Fixed: []string{"0.60.0", "0.58.5"}},
		{Component: "golang.org/x/net:0.59.0", Fixed: []string{"0.60.0", "0.59.1"}},
	}
	if !reflect.DeepEqual(got[0].Fixes, want) {
		t.Errorf("fixes = %v, want %v", got[0].Fixes, want)
	}
}

func TestAttachVersions_SameIssueOnTwoArtifactsKeptApart(t *testing.T) {
	rows := []models.VulnRow{vulnRow("/a.rpm", "XRAY-1"), vulnRow("/b.rpm", "XRAY-1")}
	fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"repo/a.rpm": {exportRow("XRAY-1", "go://x:1", "2")},
		"repo/b.rpm": {exportRow("XRAY-1", "go://x:5", "6")},
	}}

	got := AttachVersions("repo", rows, fixes)

	wantA := []models.ComponentFix{{Component: "x:1", Fixed: []string{"2"}}}
	wantB := []models.ComponentFix{{Component: "x:5", Fixed: []string{"6"}}}
	if !reflect.DeepEqual(got[0].Fixes, wantA) || !reflect.DeepEqual(got[1].Fixes, wantB) {
		t.Errorf("fixes mixed between artifacts: %v / %v", got[0].Fixes, got[1].Fixes)
	}
}

func TestAttachVersions_MatchedWithoutFix(t *testing.T) {
	rows := []models.VulnRow{vulnRow("/a.rpm", "XRAY-1")}
	fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"repo/a.rpm": {exportRow("XRAY-1", "go://x:1")},
	}}

	got := AttachVersions("repo", rows, fixes)

	if len(got[0].Fixes) != 1 {
		t.Fatalf("expected a matched row with one component, got %+v", got[0])
	}
	if len(got[0].Fixes[0].Fixed) != 0 {
		t.Errorf("expected an empty fixed list, got %v", got[0].Fixes[0].Fixed)
	}
}

func TestAttachVersions_NoMatch(t *testing.T) {
	rows := []models.VulnRow{vulnRow("/a.rpm", "XRAY-1")}
	fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"repo/other.rpm": {exportRow("XRAY-1", "go://x:1", "2")},
	}}

	got := AttachVersions("repo", rows, fixes)

	if len(got[0].Fixes) != 0 {
		t.Errorf("expected no match, got %+v", got[0])
	}
}

func TestAttachVersions_EmptyFixesLeavesRowsUnchanged(t *testing.T) {
	rows := []models.VulnRow{vulnRow("/a.rpm", "XRAY-1"), vulnRow("/b.rpm", "XRAY-2")}

	for _, fixes := range []models.ComponentFixes{{}, {ByArtifact: map[string][]models.XrayExportRow{}, Failed: 2}} {
		got := AttachVersions("repo", rows, fixes)
		if !reflect.DeepEqual(got, rows) {
			t.Errorf("rows changed: %+v", got)
		}
	}
	if got := AttachVersions("repo", nil, models.ComponentFixes{}); len(got) != 0 {
		t.Errorf("expected no rows, got %v", got)
	}
}

func TestAttachVersions_DoesNotMutateInput(t *testing.T) {
	rows := []models.VulnRow{vulnRow("/a.rpm", "XRAY-1")}
	fixed := []string{"2"}
	fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"repo/a.rpm": {exportRow("XRAY-1", "go://x:1", fixed...)},
	}}

	got := AttachVersions("repo", rows, fixes)

	if rows[0].Fixes != nil {
		t.Errorf("input row was mutated: %+v", rows[0])
	}
	if len(got) != len(rows) {
		t.Fatalf("expected %d rows, got %d", len(rows), len(got))
	}
	got[0].Fixes[0].Fixed[0] = "changed"
	if fixes.ByArtifact["repo/a.rpm"][0].ComponentVersions.FixedVersions[0] != "2" {
		t.Error("result shares the fixed versions slice with the export")
	}
}

func TestArtifactsWithFindings(t *testing.T) {
	withIssue := models.XrayArtifact{
		General: models.XrayGeneral{Path: "default/repo/a/one.rpm", ComponentID: "rpm://one:1"},
		Issues:  []models.XrayIssue{{IssueID: "XRAY-1"}},
	}
	clean := models.XrayArtifact{General: models.XrayGeneral{Path: "default/repo/a/clean.rpm", ComponentID: "rpm://clean:1"}}
	noRows := models.XrayArtifact{
		General: models.XrayGeneral{Path: "default/repo/a/other.rpm", ComponentID: "rpm://other:1"},
		Issues:  []models.XrayIssue{{IssueID: "XRAY-2"}},
	}
	summary := models.XraySummary{Available: true, Artifacts: []models.XrayArtifact{withIssue, clean, noRows, withIssue}}
	rows := []models.VulnRow{
		{ArtifactPath: "/a/one.rpm", IssueID: "XRAY-1"},
		{ArtifactPath: "a/one.rpm", IssueID: "XRAY-3"},
		{ArtifactPath: "a/clean.rpm", IssueID: "XRAY-9"},
	}

	got := ArtifactsWithFindings("repo", rows, summary)
	want := []models.XrayGeneral{withIssue.General}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ArtifactsWithFindings = %+v, want %+v", got, want)
	}

	if got := ArtifactsWithFindings("repo", nil, summary); len(got) != 0 {
		t.Errorf("expected no artifacts without rows, got %+v", got)
	}
}

func TestArtifactsWithFindings_DuplicatePathLastEntryWins(t *testing.T) {
	first := models.XrayArtifact{
		General: models.XrayGeneral{Path: "default/repo/a.rpm", SHA256: "old", ComponentID: "rpm://a:1"},
		Issues:  []models.XrayIssue{{IssueID: "XRAY-1"}},
	}
	second := first
	second.General.SHA256 = "new"
	other := models.XrayArtifact{
		General: models.XrayGeneral{Path: "default/repo/b.rpm", SHA256: "b", ComponentID: "rpm://b:1"},
		Issues:  []models.XrayIssue{{IssueID: "XRAY-2"}},
	}
	rows := []models.VulnRow{{ArtifactPath: "/a.rpm", IssueID: "XRAY-1"}, {ArtifactPath: "/b.rpm", IssueID: "XRAY-2"}}

	got := ArtifactsWithFindings("repo", rows, models.XraySummary{Artifacts: []models.XrayArtifact{first, other, second}})
	want := []models.XrayGeneral{second.General, other.General}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ArtifactsWithFindings = %+v, want %+v", got, want)
	}

	cleanLast := first
	cleanLast.Issues = nil
	got = ArtifactsWithFindings("repo", rows, models.XraySummary{Artifacts: []models.XrayArtifact{first, cleanLast}})
	if len(got) != 0 {
		t.Errorf("a later entry without issues must win, got %+v", got)
	}
}
