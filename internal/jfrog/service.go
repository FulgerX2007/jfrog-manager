package jfrog

import (
	"io"

	"jfrog_manager/internal/models"
)

// Service defines the interface for JFrog Artifactory operations.
// Handlers depend on this interface for testability.
type Service interface {
	ListRepos() ([]models.Repository, error)
	ListArtifacts(repo string) ([]byte, error)
	UploadArtifact(repo, path string, reader io.Reader) error
	DeleteArtifact(repo, path string) error
	GetXraySummary(repo, path string) ([]byte, error)
}
