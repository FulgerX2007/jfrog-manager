package report

import (
	"encoding/csv"
	"io"
	"strings"

	"jfrog_manager/internal/models"
)

var csvHeader = []string{"Component", "Package", "Artifact", "Severity", "Issue ID", "CVEs", "Vulnerable Version", "Fixed Version", "Summary", "Impact Path"}

// WriteCSV writes the rows as CSV, one line per vulnerability, with a header row.
func WriteCSV(w io.Writer, rows []models.VulnRow) error {
	writer := csv.NewWriter(w)
	if err := writer.Write(csvHeader); err != nil {
		return err
	}
	for _, r := range rows {
		record := []string{
			r.Component,
			r.Package,
			r.ArtifactPath,
			r.Severity,
			r.IssueID,
			strings.Join(r.CVEs, "; "),
			vulnerableVersions(r.Fixes),
			fixedVersions(r.Fixes),
			r.Summary,
			strings.Join(r.ImpactPaths, " | "),
		}
		for i, cell := range record {
			record[i] = guardCell(cell)
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

// vulnerableVersions joins the vulnerable components of a row with "; ".
// A row without fixes (no export match) yields an empty cell.
func vulnerableVersions(fixes []models.ComponentFix) string {
	parts := make([]string, len(fixes))
	for i, f := range fixes {
		parts[i] = f.Component
	}
	return strings.Join(parts, "; ")
}

// fixedVersions lists the fixed versions per component, in the same order as
// vulnerableVersions: each list joined with ", ", entries joined with "; ".
// A component Xray knows no fix for is written as "none".
func fixedVersions(fixes []models.ComponentFix) string {
	parts := make([]string, len(fixes))
	for i, f := range fixes {
		if len(f.Fixed) == 0 {
			parts[i] = "none"
			continue
		}
		parts[i] = strings.Join(f.Fixed, ", ")
	}
	return strings.Join(parts, "; ")
}

// guardCell neutralises spreadsheet formula injection: a cell that would be
// evaluated as a formula is prefixed with a single quote so it stays text.
func guardCell(cell string) string {
	if cell == "" {
		return cell
	}
	switch cell[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + cell
	}
	return cell
}
