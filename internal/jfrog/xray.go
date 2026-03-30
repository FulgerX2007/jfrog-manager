package jfrog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"jfrog_manager/internal/models"
)

// GetXraySummary retrieves the Xray vulnerability summary for an artifact.
// Uses "default" as the JFrog service ID in the artifact path, which is the
// standard service ID for single-server JFrog Platform installations.
// Returns a summary with Available=false if Xray is unreachable or the artifact is not indexed.
func (c Client) GetXraySummary(repo, path string) (models.XraySummary, error) {
	url := c.baseURL + "/xray/api/v1/summary/artifact"
	reqBody := fmt.Sprintf(`{"paths":["default/%s/%s"]}`, repo, path)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(reqBody))
	if err != nil {
		return models.XraySummary{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return models.XraySummary{Available: false}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return models.XraySummary{Available: false}, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return models.XraySummary{Available: false}, nil
	}

	var summary models.XraySummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return models.XraySummary{}, fmt.Errorf("parsing xray response: %w", err)
	}
	summary.Available = true

	return summary, nil
}
