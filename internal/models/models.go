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
