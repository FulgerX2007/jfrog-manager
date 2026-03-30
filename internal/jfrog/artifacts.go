package jfrog

import (
	"encoding/json"
	"fmt"
	"net/http"

	"jfrog_manager/internal/models"
)

// ListRepos returns all repositories from the JFrog Artifactory instance.
func (c Client) ListRepos() ([]models.Repository, error) {
	url := c.baseURL + "/artifactory/api/repositories"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	body, err := c.doAndReadBody(req)
	if err != nil {
		return nil, err
	}

	var repos []models.Repository
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, fmt.Errorf("parsing repositories response: %w", err)
	}

	return repos, nil
}
