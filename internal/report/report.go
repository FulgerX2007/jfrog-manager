package report

import (
	"log/slog"
	"sort"
	"strings"

	"jfrog_manager/internal/models"
)

// Sort keys and directions accepted by SortRows.
const (
	SortBySeverity  = "severity"
	SortByComponent = "component"
	DirAsc          = "asc"
	DirDesc         = "desc"
)

// severityRank orders the known severities from most to least severe.
// Anything else, including Xray's "Unknown", ranks after them.
var severityRank = map[string]int{
	"critical": 0,
	"high":     1,
	"medium":   2,
	"low":      3,
}

const unknownSeverityRank = 4

// NotScanned is a latest artifact Xray returned no data for.
type NotScanned struct {
	Path   string
	Reason string
}

// Report is the vulnerability report of one repository.
type Report struct {
	Rows []models.VulnRow
	// Scanned is the number of latest artifacts Xray returned data for,
	// whether or not they have issues.
	Scanned    int
	NotScanned []NotScanned
	// Clean lists the scanned artifacts that have no issues.
	Clean []models.Artifact
	// Counts holds the number of rows per severity key (see SeverityKey).
	Counts map[string]int
}

// Group is the part of a report that belongs to one component.
type Group struct {
	Component string
	// Artifacts are the names of the artifacts the rows were found in.
	Artifacts []string
	Counts    map[string]int
	Rows      []models.VulnRow
}

// Build flattens the Xray summary of the latest artifacts into one row per
// issue. Latest artifacts missing from the summary are listed as not scanned,
// never treated as clean.
func Build(repo string, latest []models.Artifact, summary models.XraySummary) Report {
	root := CommonRoot(latest)

	scanned := make(map[string]models.XrayArtifact, len(summary.Artifacts))
	for _, xa := range summary.Artifacts {
		scanned[normalizePath(repo, xa.General.Path)] = xa
	}
	reasons := make(map[string]string, len(summary.Errors))
	for _, e := range summary.Errors {
		reasons[normalizePath(repo, e.Identifier)] = e.Error
	}

	report := Report{Rows: []models.VulnRow{}, Counts: map[string]int{}}
	for _, a := range latest {
		key := strings.Trim(a.Path, "/")
		xa, ok := scanned[key]
		if !ok {
			report.NotScanned = append(report.NotScanned, NotScanned{Path: a.Path, Reason: reasons[key]})
			continue
		}
		delete(scanned, key)
		report.Scanned++

		if len(xa.Issues) == 0 {
			report.Clean = append(report.Clean, a)
		}

		component := ComponentOf(a.Path, root)
		pkg := PackageName(a.Name)
		for _, issue := range xa.Issues {
			report.Rows = append(report.Rows, models.VulnRow{
				Component:    component,
				Package:      pkg,
				ArtifactName: a.Name,
				ArtifactPath: a.Path,
				IssueID:      issue.IssueID,
				Severity:     issue.Severity,
				CVEs:         cveIDs(issue.CVEs),
				Summary:      issue.Summary,
				ImpactPaths:  trimImpactPaths(repo, key, issue.ImpactPaths),
			})
			report.Counts[SeverityKey(issue.Severity)]++
		}
	}

	if len(scanned) > 0 {
		slog.Warn("xray artifacts matched no requested artifact", "count", len(scanned))
	}

	return report
}

// SortRows returns a copy of rows ordered by the given key and direction.
// Unknown keys and directions fall back to severity, most severe first.
func SortRows(rows []models.VulnRow, by, dir string) []models.VulnRow {
	by, dir = NormalizeSort(by, dir)
	sorted := make([]models.VulnRow, len(rows))
	copy(sorted, rows)

	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		ra, rb := rankOf(a.Severity), rankOf(b.Severity)
		if by == SortByComponent {
			if a.Component != b.Component {
				if dir == DirDesc {
					return a.Component > b.Component
				}
				return a.Component < b.Component
			}
			if ra != rb {
				return ra < rb
			}
			return a.Package < b.Package
		}
		if ra != rb {
			// Descending severity means most severe first, i.e. lowest rank first.
			if dir == DirDesc {
				return ra < rb
			}
			return ra > rb
		}
		if a.Component != b.Component {
			return a.Component < b.Component
		}
		return a.Package < b.Package
	})
	return sorted
}

// NormalizeSort returns the sort key and direction SortRows will use for the
// given input, applying the defaults for unknown values.
func NormalizeSort(by, dir string) (string, string) {
	if by != SortByComponent {
		by = SortBySeverity
	}
	if dir != DirAsc && dir != DirDesc {
		dir = DirDesc
		if by == SortByComponent {
			dir = DirAsc
		}
	}
	return by, dir
}

func rankOf(severity string) int {
	if rank, ok := severityRank[strings.ToLower(severity)]; ok {
		return rank
	}
	return unknownSeverityRank
}

// SeverityKeys lists the severity keys from most to least severe.
var SeverityKeys = []string{"critical", "high", "medium", "low", "unknown"}

// SeverityKey maps an Xray severity to its key: the lower-cased name of a
// known severity, or "unknown" for anything else.
func SeverityKey(severity string) string {
	key := strings.ToLower(severity)
	if _, ok := severityRank[key]; ok {
		return key
	}
	return "unknown"
}

// NormalizeSeverity returns the severity key a filter value stands for, or an
// empty string when it names none, meaning no filter.
func NormalizeSeverity(filter string) string {
	filter = strings.ToLower(filter)
	for _, key := range SeverityKeys {
		if filter == key {
			return key
		}
	}
	return ""
}

// FilterBySeverity keeps the rows of one severity key. An empty key keeps all rows.
func FilterBySeverity(rows []models.VulnRow, key string) []models.VulnRow {
	if key == "" {
		return rows
	}
	filtered := make([]models.VulnRow, 0, len(rows))
	for _, r := range rows {
		if SeverityKey(r.Severity) == key {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// GroupRows splits rows into one group per component. Inside a group rows run
// from most to least severe. Groups are ordered by component name, or by how
// severe their findings are, in the given direction; unknown keys and
// directions fall back as in NormalizeSort.
func GroupRows(rows []models.VulnRow, by, dir string) []Group {
	by, dir = NormalizeSort(by, dir)

	index := map[string]int{}
	groups := []Group{}
	for _, r := range SortRows(rows, SortBySeverity, DirDesc) {
		i, ok := index[r.Component]
		if !ok {
			i = len(groups)
			index[r.Component] = i
			groups = append(groups, Group{Component: r.Component, Counts: map[string]int{}})
		}
		g := groups[i]
		g.Rows = append(g.Rows, r)
		g.Counts[SeverityKey(r.Severity)]++
		if !containsString(g.Artifacts, r.ArtifactName) {
			g.Artifacts = append(g.Artifacts, r.ArtifactName)
		}
		groups[i] = g
	}

	sort.SliceStable(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if by == SortBySeverity {
			for _, key := range SeverityKeys {
				if a.Counts[key] != b.Counts[key] {
					// Descending severity means the group with more of the worst findings first.
					if dir == DirDesc {
						return a.Counts[key] > b.Counts[key]
					}
					return a.Counts[key] < b.Counts[key]
				}
			}
			return a.Component < b.Component
		}
		if dir == DirDesc {
			return a.Component > b.Component
		}
		return a.Component < b.Component
	})
	return groups
}

func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// TrimImpactPaths strips the artifact's own location from each impact path,
// leaving the path inside the artifact down to the vulnerable module.
func TrimImpactPaths(repo, artifactPath string, impactPaths []string) []string {
	return trimImpactPaths(repo, strings.Trim(artifactPath, "/"), impactPaths)
}

// normalizePath reduces an Xray path to the artifact path inside the
// repository, accepting both "default/<repo>/<path>" and "<repo>/<path>".
func normalizePath(repo, p string) string {
	p = strings.Trim(p, "/")
	p = strings.TrimPrefix(p, "default/")
	p = strings.TrimPrefix(p, repo+"/")
	return p
}

// trimImpactPaths strips the artifact's own location from each impact path,
// leaving the path inside the artifact down to the vulnerable module.
func trimImpactPaths(repo, artifactPath string, impactPaths []string) []string {
	trimmed := make([]string, 0, len(impactPaths))
	for _, p := range impactPaths {
		p = strings.TrimPrefix(normalizePath(repo, p), artifactPath)
		p = strings.TrimPrefix(strings.TrimPrefix(p, "/"), "./")
		if p != "" {
			trimmed = append(trimmed, p)
		}
	}
	return trimmed
}

func cveIDs(cves []models.XrayCVE) []string {
	ids := make([]string, 0, len(cves))
	for _, c := range cves {
		if c.ID != "" {
			ids = append(ids, c.ID)
		}
	}
	return ids
}
