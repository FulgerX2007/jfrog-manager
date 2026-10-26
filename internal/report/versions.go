package report

import (
	"log/slog"
	"slices"
	"sort"
	"strings"

	"jfrog_manager/internal/models"
)

// AttachVersions joins the Xray component export onto the report rows and
// returns a copy of them. Rows are matched by artifact path, then by issue id;
// each matching export row adds the installed component and the versions that
// fix it. Rows are never dropped or added, and the input slice is not modified.
func AttachVersions(repo string, rows []models.VulnRow, fixes models.ComponentFixes) []models.VulnRow {
	out := make([]models.VulnRow, len(rows))
	copy(out, rows)
	if len(rows) == 0 || len(fixes.ByArtifact) == 0 {
		return out
	}

	byArtifact := make(map[string]map[string][]models.XrayExportRow, len(fixes.ByArtifact))
	exportRows := 0
	for key, exp := range fixes.ByArtifact {
		p := normalizePath(repo, key)
		byIssue := byArtifact[p]
		if byIssue == nil {
			byIssue = map[string][]models.XrayExportRow{}
			byArtifact[p] = byIssue
		}
		for _, r := range exp {
			byIssue[r.IssueID] = append(byIssue[r.IssueID], r)
			exportRows++
		}
	}

	matched := 0
	for i, row := range out {
		exp, ok := byArtifact[strings.Trim(row.ArtifactPath, "/")][row.IssueID]
		if !ok || row.IssueID == "" {
			continue
		}
		out[i].Fixes = componentFixes(exp)
		out[i].FixesMatched = true
		matched++
	}

	if matched == 0 && exportRows > 0 {
		slog.Warn("component export rows did not match any report row",
			"repo", repo, "rows", len(out), "export_rows", exportRows, "artifacts", len(fixes.ByArtifact))
	}
	return out
}

// componentFixes turns the export rows of one issue into fixes, one per
// component, sorted by component. Duplicate components are merged and their
// fixed versions deduplicated in first-seen order.
func componentFixes(exp []models.XrayExportRow) []models.ComponentFix {
	index := map[string]int{}
	fixes := []models.ComponentFix{}
	for _, r := range exp {
		label := componentLabel(r.SourceComponentID)
		i, ok := index[label]
		if !ok {
			i = len(fixes)
			index[label] = i
			fixes = append(fixes, models.ComponentFix{Component: label, Fixed: []string{}})
		}
		for _, v := range r.ComponentVersions.FixedVersions {
			if !slices.Contains(fixes[i].Fixed, v) {
				fixes[i].Fixed = append(fixes[i].Fixed, v)
			}
		}
	}
	sort.SliceStable(fixes, func(a, b int) bool { return fixes[a].Component < fixes[b].Component })
	return fixes
}

// componentLabel strips the "xxx://" scheme from an Xray component id, e.g.
// "go://golang.org/x/net:0.59.0" → "golang.org/x/net:0.59.0". A value without
// a scheme is returned as is.
func componentLabel(id string) string {
	if _, rest, ok := strings.Cut(id, "://"); ok {
		return rest
	}
	return id
}

// ArtifactsWithFindings returns the Xray metadata of the scanned artifacts
// that produced at least one report row, in summary order and without
// duplicates. These are the artifacts worth a component export call.
func ArtifactsWithFindings(repo string, rows []models.VulnRow, summary models.XraySummary) []models.XrayGeneral {
	withRows := make(map[string]bool, len(rows))
	for _, r := range rows {
		withRows[strings.Trim(r.ArtifactPath, "/")] = true
	}

	out := []models.XrayGeneral{}
	seen := map[string]bool{}
	for _, xa := range summary.Artifacts {
		key := normalizePath(repo, xa.General.Path)
		if len(xa.Issues) == 0 || !withRows[key] || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, xa.General)
	}
	return out
}
