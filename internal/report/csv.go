package report

import (
	"encoding/csv"
	"io"
	"strings"

	"jfrog_manager/internal/models"
)

var csvHeader = []string{"Component", "Package", "Artifact", "Severity", "Issue ID", "CVEs", "Summary", "Impact Path"}

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
