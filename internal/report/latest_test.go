package report

import (
	"testing"

	"jfrog_manager/internal/models"
)

func artifact(p, modified string) models.Artifact {
	name := p
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			name = p[i+1:]
			break
		}
	}
	return models.Artifact{Name: name, Path: p, LastModified: modified, Repo: "repo"}
}

func paths(artifacts []models.Artifact) []string {
	result := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		result = append(result, a.Path)
	}
	return result
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCommonRoot(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  string
	}{
		{"shared top folder", []string{"portal/opcore/a-1.0.rpm", "portal/opcontrol/b-1.0.rpm", "portal/kafka/x86_64/k-1.0.rpm"}, "portal"},
		{"no shared folder", []string{"opcore/a-1.0.rpm", "opcontrol/b-1.0.rpm"}, ""},
		{"single folder", []string{"portal/opcore/a-1.0.rpm", "portal/opcore/a-1.1.rpm"}, "portal/opcore"},
		{"root level file breaks the root", []string{"portal/opcore/a-1.0.rpm", "b-1.0.rpm"}, ""},
		{"only root level files", []string{"a-1.0.rpm", "b-1.0.rpm"}, ""},
		{"whole segments only", []string{"portal-a/x/a-1.0.rpm", "portal-b/x/b-1.0.rpm"}, ""},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var artifacts []models.Artifact
			for _, p := range tt.paths {
				artifacts = append(artifacts, artifact(p, ""))
			}
			if got := CommonRoot(artifacts); got != tt.want {
				t.Errorf("CommonRoot() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComponentOf(t *testing.T) {
	tests := []struct {
		name string
		path string
		root string
		want string
	}{
		{"folder below shared root", "portal/opcore/opcore-v1.0.rpm", "portal", "opcore"},
		{"deeper folder below shared root", "portal/kafka/x86_64/kafka-4.3.1-1.x86_64.rpm", "portal", "kafka"},
		{"no shared root", "opcontrol/opcontrol-v1.0.rpm", "", "opcontrol"},
		{"artifact directly in root", "portal/opcore/opcore-v1.0.rpm", "portal/opcore", "opcore"},
		{"root level file", "opcore-v1.0.rpm", "", "(root)"},
		{"leading slash", "/portal/sso/sso-v1.0.rpm", "portal", "sso"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComponentOf(tt.path, tt.root); got != tt.want {
				t.Errorf("ComponentOf(%q, %q) = %q, want %q", tt.path, tt.root, got, tt.want)
			}
		})
	}
}

func TestPackageName(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"api-v1.9.41-a4fdb6e9.x86_64.rpm", "api"},
		{"grafana-13.2.3-4.x86_64.rpm", "grafana"},
		{"clickhouse-common-static-26.8.11.7.x86_64.rpm", "clickhouse-common-static"},
		{"nginx-1.30.5-1.el9.ngx.x86_64.rpm", "nginx"},
		{"app-1.0.jar", "app"},
		{"README.txt", "README"},
		{"noextension", "noextension"},
		// Known limit: a digit-led name segment is taken for the version.
		{"foo-2fa-1.0.rpm", "foo"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if got := PackageName(tt.file); got != tt.want {
				t.Errorf("PackageName(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestVariantOf(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"api-v1.9.41-a4fdb6e9.x86_64.rpm", "x86_64.rpm"},
		{"api-v1.9.41-a4fdb6e9.aarch64.rpm", "aarch64.rpm"},
		{"api-v1.9.41-a4fdb6e9.src.rpm", "src.rpm"},
		{"app-1.0.jar", "jar"},
		{"app-1.0.jar.sha256", "sha256"},
		{"noextension", ""},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if got := variantOf(tt.file); got != tt.want {
				t.Errorf("variantOf(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestLatestArtifacts(t *testing.T) {
	tests := []struct {
		name string
		in   []models.Artifact
		want []string
	}{
		{
			name: "newest version of one package wins",
			in: []models.Artifact{
				artifact("portal/sso/sso-v1.10.13-7bc4634e.x86_64.rpm", "2026-09-30T12:06:31.368Z"),
				artifact("portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm", "2026-10-06T07:14:47.134Z"),
				artifact("portal/sso/sso-v1.10.8-5f4ce4a0.x86_64.rpm", "2026-09-25T09:29:29.808Z"),
			},
			want: []string{"portal/sso/sso-v1.10.15-cc1a8b75.x86_64.rpm"},
		},
		{
			name: "target repository layout",
			in: []models.Artifact{
				artifact("portal/grafana/grafana-12.3.11-10.x86_64.rpm", "2026-09-24T07:57:20.355Z"),
				artifact("portal/grafana/grafana-13.2.3-4.x86_64.rpm", "2026-10-05T14:07:49.862Z"),
				artifact("portal/clickhouse/clickhouse-client-26.8.11.7.x86_64.rpm", "2026-09-23T13:30:04.850Z"),
				artifact("portal/clickhouse/clickhouse-server-26.8.11.7.x86_64.rpm", "2026-09-23T13:30:04.150Z"),
				artifact("portal/kafka/x86_64/kafka-4.3.1-1.x86_64.rpm", "2026-09-23T13:35:08.581Z"),
			},
			want: []string{
				"portal/clickhouse/clickhouse-client-26.8.11.7.x86_64.rpm",
				"portal/clickhouse/clickhouse-server-26.8.11.7.x86_64.rpm",
				"portal/grafana/grafana-13.2.3-4.x86_64.rpm",
				"portal/kafka/x86_64/kafka-4.3.1-1.x86_64.rpm",
			},
		},
		{
			name: "same package name in two folders kept separately",
			in: []models.Artifact{
				artifact("a/api-1.0.x86_64.rpm", "2026-01-01T00:00:00.000Z"),
				artifact("b/api-2.0.x86_64.rpm", "2026-02-01T00:00:00.000Z"),
			},
			want: []string{"a/api-1.0.x86_64.rpm", "b/api-2.0.x86_64.rpm"},
		},
		{
			name: "architectures and sidecar files kept separately",
			in: []models.Artifact{
				artifact("p/api-1.0.x86_64.rpm", "2026-01-01T00:00:00.000Z"),
				artifact("p/api-1.0.aarch64.rpm", "2026-01-02T00:00:00.000Z"),
				artifact("p/api-1.0.src.rpm", "2026-01-03T00:00:00.000Z"),
				artifact("p/api-1.1.x86_64.rpm", "2026-01-04T00:00:00.000Z"),
			},
			want: []string{"p/api-1.0.aarch64.rpm", "p/api-1.0.src.rpm", "p/api-1.1.x86_64.rpm"},
		},
		{name: "empty input", in: nil, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths(LatestArtifacts(tt.in))
			if !equalStrings(got, tt.want) {
				t.Errorf("LatestArtifacts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModifiedAfter(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"later Z timestamp", "2026-10-06T07:14:47.134Z", "2026-09-30T12:06:31.368Z", true},
		{"earlier Z timestamp", "2026-09-30T12:06:31.368Z", "2026-10-06T07:14:47.134Z", false},
		{"equal", "2026-10-06T07:14:47.134Z", "2026-10-06T07:14:47.134Z", false},
		// 10:00+0300 is 07:00Z, earlier than 08:00Z although it sorts later as a string.
		{"colon-less offset compared by instant", "2026-10-06T10:00:00.000+0300", "2026-10-06T08:00:00.000Z", false},
		{"colon-less offsets on both sides", "2026-10-06T10:00:00.000+0100", "2026-10-06T10:00:00.000+0300", true},
		{"unparseable falls back to string comparison", "b", "a", true},
		{"one side unparseable falls back to string comparison", "2026-10-06T07:14:47.134Z", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modifiedAfter(tt.a, tt.b); got != tt.want {
				t.Errorf("modifiedAfter(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
