package models

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
	Available bool           `json:"-"`
}

// XrayArtifact contains general info and issues for a scanned artifact.
type XrayArtifact struct {
	General  XrayGeneral  `json:"general"`
	Issues   []XrayIssue  `json:"issues"`
	Licenses []XrayLicense `json:"licenses"`
}

// XrayGeneral holds general metadata about a scanned artifact.
type XrayGeneral struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	PackageType string `json:"pkg_type"`
	SHA256     string `json:"sha256"`
}

// XrayIssue represents a single vulnerability or issue found by Xray.
type XrayIssue struct {
	Summary     string           `json:"summary"`
	Description string           `json:"description"`
	Severity    string           `json:"severity"`
	IssueType   string           `json:"issue_type"`
	Provider    string           `json:"provider"`
	CVEs        []XrayCVE        `json:"cves"`
	Components  []XrayComponent  `json:"components"`
}

// XrayCVE represents a CVE entry associated with an Xray issue.
type XrayCVE struct {
	ID     string `json:"cve"`
	CVSS2  string `json:"cvss_v2"`
	CVSS3  string `json:"cvss_v3"`
}

// XrayComponent represents an affected component in an Xray issue.
type XrayComponent struct {
	ID              string   `json:"component_id"`
	FixedVersions   []string `json:"fixed_versions"`
}

// XrayLicense represents license information from an Xray scan.
type XrayLicense struct {
	Name       string   `json:"name"`
	FullName   string   `json:"full_name"`
	Components []string `json:"components"`
}
