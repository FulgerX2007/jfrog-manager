package handlers

import (
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"jfrog_manager/internal/models"
	"jfrog_manager/internal/report"

	"github.com/gin-gonic/gin"
)

const xrayUnavailableMessage = "Xray data could not be retrieved (Xray unavailable or the request timed out)"

// vulnerabilityQuery holds the validated parameters of a report request.
type vulnerabilityQuery struct {
	repo string
	sort string
	dir  string
	// severity is a severity key to narrow the report to, or empty for all.
	severity string
}

// vulnerabilityResult is a built report with what the views need around it.
type vulnerabilityResult struct {
	report report.Report
	// rows are the report rows narrowed to the requested severity and sorted.
	rows []models.VulnRow
	// available is false when Xray data could not be retrieved.
	available bool
	// excluded is the number of older files left out of the report.
	excluded int
}

// parseVulnerabilityQuery reads and validates the report parameters.
// It renders the error itself and returns false when they are invalid.
func (h Handler) parseVulnerabilityQuery(c *gin.Context) (vulnerabilityQuery, bool) {
	repo := c.Query("repo")
	if repo == "" {
		h.renderError(c, "Repository parameter is required")
		return vulnerabilityQuery{}, false
	}
	if strings.Contains(repo, "..") {
		h.renderError(c, "Invalid repository")
		return vulnerabilityQuery{}, false
	}
	sortBy, dir := report.NormalizeSort(c.Query("sort"), c.Query("dir"))
	return vulnerabilityQuery{repo: repo, sort: sortBy, dir: dir, severity: report.NormalizeSeverity(c.Query("severity"))}, true
}

// buildVulnerabilityReport lists the repository, keeps the latest artifact of
// each package, asks Xray about those only and returns the report.
func (h Handler) buildVulnerabilityReport(q vulnerabilityQuery) (vulnerabilityResult, error) {
	artifacts, err := h.service.ListArtifacts(q.repo)
	if err != nil {
		return vulnerabilityResult{}, fmt.Errorf("listing artifacts: %w", err)
	}

	latest := report.LatestArtifacts(artifacts)
	paths := make([]string, 0, len(latest))
	for _, a := range latest {
		paths = append(paths, a.Path)
	}

	summary, err := h.service.GetXraySummaries(q.repo, paths)
	if err != nil {
		return vulnerabilityResult{}, fmt.Errorf("getting xray summaries: %w", err)
	}
	if !summary.Available {
		return vulnerabilityResult{}, nil
	}

	rep := report.Build(q.repo, latest, summary)
	rows := report.SortRows(report.FilterBySeverity(rep.Rows, q.severity), q.sort, q.dir)
	return vulnerabilityResult{report: rep, rows: rows, available: true, excluded: len(artifacts) - len(latest)}, nil
}

// severityTile is one entry of the severity filter of the report.
type severityTile struct {
	Key    string
	Label  string
	Count  int
	Active bool
}

// GetVulnerabilities returns the vulnerability report fragment for the latest
// version of every package in a repository.
func (h Handler) GetVulnerabilities(c *gin.Context) {
	q, ok := h.parseVulnerabilityQuery(c)
	if !ok {
		return
	}

	res, err := h.buildVulnerabilityReport(q)
	if err != nil {
		slog.Error("building vulnerability report", "error", err)
		h.renderError(c, "Failed to load vulnerabilities")
		return
	}

	labels := map[string]string{"critical": "Critical", "high": "High", "medium": "Medium", "low": "Low", "unknown": "Unknown"}
	tiles := make([]severityTile, 0, len(report.SeverityKeys))
	for _, key := range report.SeverityKeys {
		tiles = append(tiles, severityTile{Key: key, Label: labels[key], Count: res.report.Counts[key], Active: key == q.severity})
	}

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{
		"Repo":       q.repo,
		"Sort":       q.sort,
		"Dir":        q.dir,
		"Severity":   q.severity,
		"Available":  res.available,
		"Total":      len(res.report.Rows),
		"Shown":      len(res.rows),
		"Tiles":      tiles,
		"Groups":     report.GroupRows(res.rows, q.sort, q.dir),
		"Scanned":    res.report.Scanned,
		"NotScanned": res.report.NotScanned,
		"Clean":      res.report.Clean,
		"Excluded":   res.excluded,
	}
	if err := h.tmpl.ExecuteTemplate(c.Writer, "vuln_report", data); err != nil {
		slog.Error("rendering vulnerability report", "error", err)
	}
}

// ExportVulnerabilities streams the vulnerability report as a CSV download,
// holding the same findings as the fragment for the same parameters.
func (h Handler) ExportVulnerabilities(c *gin.Context) {
	q, ok := h.parseVulnerabilityQuery(c)
	if !ok {
		return
	}

	res, err := h.buildVulnerabilityReport(q)
	if err != nil {
		slog.Error("building vulnerability export", "error", err)
		h.renderError(c, "Failed to export vulnerabilities")
		return
	}
	if !res.available {
		h.renderError(c, xrayUnavailableMessage)
		return
	}

	filename := fmt.Sprintf("vulnerabilities-%s-%s.csv", q.repo, time.Now().Format("20060102"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Status(http.StatusOK)
	if err := report.WriteCSV(c.Writer, res.rows); err != nil {
		slog.Error("writing vulnerability export", "error", err)
	}
}
