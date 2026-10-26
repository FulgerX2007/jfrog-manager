package report

import (
	"reflect"
	"testing"

	"jfrog_manager/internal/models"
)

func TestComponentLabel(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"go://golang.org/x/net:0.59.0", "golang.org/x/net:0.59.0"},
		{"rpm://9:openssl-libs:3.0.7-27.el9", "9:openssl-libs:3.0.7-27.el9"},
		{"npm://lodash:4.17.20", "lodash:4.17.20"},
		{"golang.org/x/net:0.59.0", "golang.org/x/net:0.59.0"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := componentLabel(tt.in); got != tt.want {
			t.Errorf("componentLabel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

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
	if !got[0].FixesMatched || !reflect.DeepEqual(got[0].Fixes, want) {
		t.Errorf("row 0: matched=%v fixes=%v, want %v", got[0].FixesMatched, got[0].Fixes, want)
	}
	if got[1].FixesMatched || len(got[1].Fixes) != 0 {
		t.Errorf("row 1 should be unmatched, got matched=%v fixes=%v", got[1].FixesMatched, got[1].Fixes)
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
			if !got[0].FixesMatched {
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

	if got[0].Fixes[0].Component != "x:1" || got[1].Fixes[0].Component != "x:5" {
		t.Errorf("fixes mixed between artifacts: %v / %v", got[0].Fixes, got[1].Fixes)
	}
	if len(got[0].Fixes) != 1 || len(got[1].Fixes) != 1 {
		t.Errorf("expected one fix per row, got %v / %v", got[0].Fixes, got[1].Fixes)
	}
}

func TestAttachVersions_MatchedWithoutFix(t *testing.T) {
	rows := []models.VulnRow{vulnRow("/a.rpm", "XRAY-1")}
	fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{
		"repo/a.rpm": {exportRow("XRAY-1", "go://x:1")},
	}}

	got := AttachVersions("repo", rows, fixes)

	if !got[0].FixesMatched || len(got[0].Fixes) != 1 {
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

	if got[0].FixesMatched || len(got[0].Fixes) != 0 {
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

	if rows[0].FixesMatched || rows[0].Fixes != nil {
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
