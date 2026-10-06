package report

import (
	"testing"

	"jfrog_manager/internal/models"
)

func sampleRepository() []models.Artifact {
	return []models.Artifact{
		artifact("portal/sso/sso-v1.10.13-7bc4634e.x86_64.rpm", "2026-09-30T12:06:31.368Z"),
		artifact("portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm", "2026-10-06T07:14:47.134Z"),
		artifact("portal/sso/sso-v1.10.8-5f4ce4a0.x86_64.rpm", "2026-09-25T09:29:29.808Z"),
		artifact("portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm", "2026-09-15T08:11:50.629Z"),
		artifact("portal/clickhouse/clickhouse-client-26.8.11.7.x86_64.rpm", "2026-09-23T13:30:04.850Z"),
		artifact("portal/clickhouse/clickhouse-server-26.8.11.7.x86_64.rpm", "2026-09-23T13:30:04.150Z"),
		artifact("portal/kafka/x86_64/kafka-4.3.1-1.x86_64.rpm", "2026-09-23T13:35:08.581Z"),
	}
}

func TestVersionOf(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"api-v1.10.16-456dc375.x86_64.rpm", "v1.10.16-456dc375"},
		{"grafana-13.2.3-4.x86_64.rpm", "13.2.3-4"},
		{"nginx-1.30.5-1.el9.ngx.x86_64.rpm", "1.30.5-1.el9.ngx"},
		{"clickhouse-common-static-26.8.11.7.x86_64.rpm", "26.8.11.7"},
		{"app-1.0.jar", "1.0"},
		{"README.txt", ""},
		{"noextension", ""},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if got := VersionOf(tt.file); got != tt.want {
				t.Errorf("VersionOf(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestBuildArtifactView_Latest(t *testing.T) {
	view := BuildArtifactView(sampleRepository(), "", "")

	if view.View != ViewLatest {
		t.Errorf("expected the latest view by default, got %q", view.View)
	}
	if view.Files != 7 || view.Packages != 5 || view.InView != 5 {
		t.Errorf("unexpected totals: files %d, packages %d, in view %d", view.Files, view.Packages, view.InView)
	}
	want := []string{
		"portal/clickhouse/clickhouse-client-26.8.11.7.x86_64.rpm",
		"portal/clickhouse/clickhouse-server-26.8.11.7.x86_64.rpm",
		"portal/kafka/x86_64/kafka-4.3.1-1.x86_64.rpm",
		"portal/opcore/opcore-v1.10.25-5314aa81.x86_64.rpm",
		"portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm",
	}
	var got []string
	for _, r := range view.Rows {
		got = append(got, r.Path)
		if !r.Latest {
			t.Errorf("row %s should be marked latest", r.Path)
		}
	}
	if !equalStrings(got, want) {
		t.Errorf("unexpected rows: %v", got)
	}

	sso := view.Rows[4]
	if sso.Package != "sso" || sso.Version != "v1.10.15-cc1a8b75" || sso.Component != "sso" || sso.Older != 2 {
		t.Errorf("unexpected sso row: %+v", sso)
	}
	if kafka := view.Rows[2]; kafka.Component != "kafka" || kafka.Older != 0 {
		t.Errorf("unexpected kafka row: %+v", kafka)
	}

	wantComponents := []ComponentCount{{Name: "clickhouse", Count: 2}, {Name: "kafka", Count: 1}, {Name: "opcore", Count: 1}, {Name: "sso", Count: 1}}
	if len(view.Components) != len(wantComponents) {
		t.Fatalf("unexpected components: %+v", view.Components)
	}
	for i, c := range wantComponents {
		if view.Components[i] != c {
			t.Errorf("component %d = %+v, want %+v", i, view.Components[i], c)
		}
	}
}

func TestBuildArtifactView_AllFilesNewestFirst(t *testing.T) {
	view := BuildArtifactView(sampleRepository(), ViewAll, "sso")

	if view.View != ViewAll || view.Component != "sso" {
		t.Errorf("unexpected view/component: %q / %q", view.View, view.Component)
	}
	if view.InView != 7 {
		t.Errorf("expected 7 files in view before the component filter, got %d", view.InView)
	}
	var got []string
	for _, r := range view.Rows {
		got = append(got, r.Version)
	}
	if !equalStrings(got, []string{"v1.10.15-cc1a8b75", "v1.10.13-7bc4634e", "v1.10.8-5f4ce4a0"}) {
		t.Errorf("expected sso versions newest first, got %v", got)
	}
	if !view.Rows[0].Latest || view.Rows[1].Latest || view.Rows[2].Latest {
		t.Errorf("only the newest version should be marked latest: %+v", view.Rows)
	}
	for _, c := range view.Components {
		if c.Active != (c.Name == "sso") {
			t.Errorf("unexpected active flag on %+v", c)
		}
		if c.Name == "sso" && c.Count != 3 {
			t.Errorf("expected 3 sso files, got %d", c.Count)
		}
	}
}

func TestBuildArtifactView_UnknownComponentAndEmpty(t *testing.T) {
	view := BuildArtifactView(sampleRepository(), ViewLatest, "missing")
	if len(view.Rows) != 0 || view.Rows == nil {
		t.Errorf("expected an empty, non-nil row list, got %v", view.Rows)
	}
	if len(view.Components) != 4 {
		t.Errorf("components should still be listed, got %+v", view.Components)
	}

	empty := BuildArtifactView(nil, "bogus", "")
	if empty.View != ViewLatest || empty.Files != 0 || len(empty.Rows) != 0 || len(empty.Components) != 0 {
		t.Errorf("unexpected empty view: %+v", empty)
	}
}
