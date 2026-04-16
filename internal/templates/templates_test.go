package templates

import (
	"bytes"
	"path/filepath"
	"runtime"
	"testing"

	"jfrog_manager/internal/models"
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
		"bootstrap",
		"htmx",
	}
	for _, s := range mustContain {
		if !bytes.Contains([]byte(output), []byte(s)) {
			t.Errorf("layout output missing %q", s)
		}
	}
}

func TestArtifactListRenders(t *testing.T) {
	tmpl, err := Load(templateDir())
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}

	t.Run("with artifacts", func(t *testing.T) {
		data := struct {
			Artifacts []models.Artifact
		}{
			Artifacts: []models.Artifact{
				{Name: "test.jar", Path: "com/test/test.jar", Size: 1024, LastModified: "2024-01-01", Repo: "libs-release"},
			},
		}
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "artifact_list", data); err != nil {
			t.Fatalf("failed to render: %v", err)
		}
		output := buf.String()
		if !bytes.Contains([]byte(output), []byte("test.jar")) {
			t.Error("artifact name not found in output")
		}
		// Regression: download href must be single-encoded. Previously we
		// combined urlEncode with html/template's URL auto-escape, which
		// produced %252F in place of %2F and made JFrog return 404.
		if bytes.Contains(buf.Bytes(), []byte("%252F")) {
			t.Error("download href is double-encoded (contains %252F)")
		}
		// And must not contain raw '/' in the query param — that would mean
		// no encoding at all was applied.
		if !bytes.Contains(buf.Bytes(), []byte("path=com%2ftest%2ftest.jar")) &&
			!bytes.Contains(buf.Bytes(), []byte("path=com%2Ftest%2Ftest.jar")) {
			t.Errorf("expected single-encoded path in href; got: %s", output)
		}
	})

	t.Run("empty", func(t *testing.T) {
		data := struct {
			Artifacts []models.Artifact
		}{}
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "artifact_list", data); err != nil {
			t.Fatalf("failed to render: %v", err)
		}
		if !bytes.Contains(buf.Bytes(), []byte("No artifacts found")) {
			t.Error("empty message not found")
		}
	})
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
