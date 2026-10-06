package jfrog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"jfrog_manager/internal/models"
)

// xrayBatchSize is the number of artifact paths sent to Xray per summary request.
const xrayBatchSize = 100

// GetXraySummary retrieves the Xray vulnerability summary for an artifact.
// Returns a summary with Available=false if Xray is unreachable or the artifact is not indexed.
func (c Client) GetXraySummary(repo, path string) (models.XraySummary, error) {
	return c.GetXraySummaries(repo, []string{path})
}

// GetXraySummaries retrieves the Xray vulnerability summary for several
// artifacts of one repository, asking Xray in batches of xrayBatchSize paths.
// Uses "default" as the JFrog service ID in the artifact path, which is the
// standard service ID for single-server JFrog Platform installations.
// Returns a summary with Available=false if any batch fails; paths Xray has
// no data for are reported in the summary's Errors.
func (c Client) GetXraySummaries(repo string, paths []string) (models.XraySummary, error) {
	summary := models.XraySummary{Available: true}
	for start := 0; start < len(paths); start += xrayBatchSize {
		end := min(start+xrayBatchSize, len(paths))
		batch, err := c.xraySummaryBatch(repo, paths[start:end])
		if err != nil {
			return models.XraySummary{}, err
		}
		if !batch.Available {
			slog.Warn("xray batch unavailable", "batch", start/xrayBatchSize, "size", end-start)
			return models.XraySummary{Available: false}, nil
		}
		summary.Artifacts = append(summary.Artifacts, batch.Artifacts...)
		summary.Errors = append(summary.Errors, batch.Errors...)
	}
	return summary, nil
}

// xraySummaryBatch sends one summary request for the given paths.
func (c Client) xraySummaryBatch(repo string, paths []string) (models.XraySummary, error) {
	url := c.baseURL + "/xray/api/v1/summary/artifact"
	payload := struct {
		Paths []string `json:"paths"`
	}{
		Paths: make([]string, 0, len(paths)),
	}
	for _, path := range paths {
		payload.Paths = append(payload.Paths, fmt.Sprintf("default/%s/%s", repo, path))
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return models.XraySummary{}, fmt.Errorf("marshaling request body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return models.XraySummary{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		slog.Warn("xray request failed", "error", err)
		return models.XraySummary{Available: false}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return models.XraySummary{Available: false}, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("xray returned non-OK status", "status", resp.StatusCode)
		return models.XraySummary{Available: false}, nil
	}

	var summary models.XraySummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return models.XraySummary{}, fmt.Errorf("parsing xray response: %w", err)
	}
	summary.Available = true

	return summary, nil
}
