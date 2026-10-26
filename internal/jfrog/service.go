package jfrog

import (
	"context"
	"io"

	"jfrog_manager/internal/models"
)

// Service defines the interface for JFrog Artifactory operations.
// Handlers depend on this interface for testability.
type Service interface {
	ListRepos() ([]models.Repository, error)
	ListArtifacts(repo string) ([]models.Artifact, error)
	UploadArtifact(repo, path string, reader io.Reader) error
	DeleteArtifact(repo, path string) error
	DownloadArtifact(repo, path string) (io.ReadCloser, int64, string, error)
	GetXraySummary(repo, path string) (models.XraySummary, error)
	GetXraySummaries(repo string, paths []string) (models.XraySummary, error)
	// GetComponentFixes honours ctx so a report load stops its export calls
	// when the browser request goes away.
	GetComponentFixes(ctx context.Context, artifacts []models.XrayGeneral) (models.ComponentFixes, error)
}
