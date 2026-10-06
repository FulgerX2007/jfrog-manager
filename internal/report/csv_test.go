package report

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"jfrog_manager/internal/models"
)

func writeAndParse(t *testing.T, rows []models.VulnRow) [][]string {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteCSV(&buf, rows); err != nil {
		t.Fatalf("WriteCSV failed: %v", err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v", err)
	}
	return records
}

func TestWriteCSV_HeaderAndRow(t *testing.T) {
	records := writeAndParse(t, []models.VulnRow{{
		Component:    "opcore",
		Package:      "opcore",
		ArtifactName: "opcore-v1.0.x86_64.rpm",
		ArtifactPath: "portal/opcore/opcore-v1.0.x86_64.rpm",
		IssueID:      "XRAY-1",
		Severity:     "High",
		CVEs:         []string{"CVE-2026-1", "CVE-2026-2"},
		Summary:      "Overflow in \"parser\", with commas\nand a newline",
		ImpactPaths:  []string{"opt/bin/opcore/mod/a", "opt/bin/tool/mod/a"},
	}})

	if len(records) != 2 {
		t.Fatalf("expected header and 1 row, got %d records", len(records))
	}
	wantHeader := []string{"Component", "Package", "Artifact", "Severity", "Issue ID", "CVEs", "Summary", "Impact Path"}
	if !equalStrings(records[0], wantHeader) {
		t.Errorf("unexpected header: %v", records[0])
	}
	want := []string{
		"opcore", "opcore", "portal/opcore/opcore-v1.0.x86_64.rpm", "High", "XRAY-1",
		"CVE-2026-1; CVE-2026-2",
		"Overflow in \"parser\", with commas\nand a newline",
		"opt/bin/opcore/mod/a | opt/bin/tool/mod/a",
	}
	if !equalStrings(records[1], want) {
		t.Errorf("unexpected row:\n %q\nwant\n %q", records[1], want)
	}
}

func TestWriteCSV_EmptyRowsWritesHeaderOnly(t *testing.T) {
	records := writeAndParse(t, nil)
	if len(records) != 1 || records[0][0] != "Component" {
		t.Errorf("expected only the header, got %v", records)
	}
}

func TestWriteCSV_GuardsFormulaCells(t *testing.T) {
	tests := []struct {
		summary string
		want    string
	}{
		{"=HYPERLINK(\"http://evil\")", "'=HYPERLINK(\"http://evil\")"},
		{"+1+1", "'+1+1"},
		{"-2+3", "'-2+3"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"\tindented", "'\tindented"},
		{"plain = text", "plain = text"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.summary, func(t *testing.T) {
			records := writeAndParse(t, []models.VulnRow{{Component: "=cmd", Summary: tt.summary}})
			if got := records[1][6]; got != tt.want {
				t.Errorf("summary cell = %q, want %q", got, tt.want)
			}
			if got := records[1][0]; got != "'=cmd" {
				t.Errorf("guard must apply to every column, component cell = %q", got)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWriteCSV_ReportsWriteError(t *testing.T) {
	rows := []models.VulnRow{{Summary: strings.Repeat("x", 8192)}}
	if err := WriteCSV(failingWriter{}, rows); err == nil {
		t.Fatal("expected a write error")
	}
}
