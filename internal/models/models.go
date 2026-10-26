package models

import "strings"

// Repository represents a JFrog Artifactory repository.
type Repository struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	PackageType string `json:"packageType"`
	Description string `json:"description"`
}

// Artifact represents a file stored in a JFrog Artifactory repository.
type Artifact struct {
	Name         string `json:"name"`
	Path         string `json:"uri"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified"`
	Repo         string `json:"-"`
}

// XraySummary holds the Xray scan results for an artifact.
// When Available is false, Xray data could not be retrieved (e.g. Xray not configured or artifact not indexed).
type XraySummary struct {
	Artifacts []XrayArtifact `json:"artifacts"`
	Errors    []XrayError    `json:"errors"`
	Available bool           `json:"-"`
}

// XrayError reports a requested path Xray returned no data for,
// e.g. an artifact that is not indexed. Identifier has the form "default/<repo>/<path>".
type XrayError struct {
	Identifier string `json:"identifier"`
	Error      string `json:"error"`
}

// XrayArtifact contains general info and issues for a scanned artifact.
type XrayArtifact struct {
	General  XrayGeneral   `json:"general"`
	Issues   []XrayIssue   `json:"issues"`
	Licenses []XrayLicense `json:"licenses"`
}

// XrayGeneral holds general metadata about a scanned artifact.
type XrayGeneral struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	PackageType string `json:"pkg_type"`
	SHA256      string `json:"sha256"`
	// ComponentID is Xray's id of the artifact as a component, e.g.
	// "rpm://opcore:1.10.25"; the component export API needs it.
	ComponentID string `json:"component_id"`
}

// XrayIssue represents a single vulnerability or issue found by Xray.
type XrayIssue struct {
	IssueID     string          `json:"issue_id"`
	Summary     string          `json:"summary"`
	Description string          `json:"description"`
	Severity    string          `json:"severity"`
	IssueType   string          `json:"issue_type"`
	Provider    string          `json:"provider"`
	CVEs        []XrayCVE       `json:"cves"`
	Components  []XrayComponent `json:"components"`
	// ImpactPaths lists where the vulnerable component sits, each as
	// "default/<repo>/<artifact path>/<path inside the artifact>".
	ImpactPaths []string `json:"impact_path"`
}

// XrayCVE represents a CVE entry associated with an Xray issue.
type XrayCVE struct {
	ID    string `json:"cve"`
	CVSS2 string `json:"cvss_v2"`
	CVSS3 string `json:"cvss_v3"`
}

// XrayComponent represents an affected component in an Xray issue.
type XrayComponent struct {
	ID            string   `json:"component_id"`
	FixedVersions []string `json:"fixed_versions"`
}

// XrayLicense represents license information from an Xray scan.
type XrayLicense struct {
	Name       string   `json:"name"`
	FullName   string   `json:"full_name"`
	Components []string `json:"components"`
}

// VulnRow is one line of the vulnerability report: a single Xray issue found
// in the latest artifact of a package.
type VulnRow struct {
	Component    string
	Package      string
	ArtifactName string
	ArtifactPath string
	IssueID      string
	Severity     string
	CVEs         []string
	Summary      string
	// ImpactPaths are the paths inside the artifact down to the vulnerable module.
	ImpactPaths []string
	// Fixes are the vulnerable components of this issue inside the artifact
	// with the versions that fix them, from the Xray component export.
	Fixes []ComponentFix
}

// XrayExportRow is one issue × component row of an Xray component export.
type XrayExportRow struct {
	IssueID           string                `json:"id"`
	SourceComponentID string                `json:"source_comp_id"`
	ComponentVersions XrayComponentVersions `json:"component_versions"`
}

// XrayComponentVersions holds the version details of an export row.
type XrayComponentVersions struct {
	FixedVersions []string `json:"fixed_versions"`
}

// ComponentFixes holds the export rows per artifact, keyed by the artifact
// path exactly as the summary returned it (general.path).
type ComponentFixes struct {
	ByArtifact map[string][]XrayExportRow
	// Failed is the number of artifacts whose export failed or timed out.
	Failed int
}

// ComponentFix is one vulnerable component of a report row.
type ComponentFix struct {
	// Component is the installed component and version, e.g. golang.org/x/net:0.59.0.
	Component string
	// Fixed lists the fixing versions, e.g. ["0.60.0"]; nil when Xray knows no fix.
	Fixed []string
}

// ComponentName strips the "xxx://" scheme from an Xray component id:
// "go://golang.org/x/net:0.59.0" → "golang.org/x/net:0.59.0". An id without
// a scheme is returned unchanged.
func ComponentName(id string) string {
	if _, rest, ok := strings.Cut(id, "://"); ok {
		return rest
	}
	return id
}
