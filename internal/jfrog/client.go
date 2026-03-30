package jfrog

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"jfrog_manager/internal/config"
)

// Client implements the Service interface for JFrog Artifactory API calls.
type Client struct {
	httpClient http.Client
	baseURL    string
	apiKey     string
}

// NewClient creates a new JFrog API client from the application config.
func NewClient(cfg config.Config) Client {
	return Client{
		httpClient: http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		baseURL: strings.TrimRight(cfg.JFrogURL, "/"),
		apiKey:  cfg.JFrogAPIKey,
	}
}

// Do executes an HTTP request with the JFrog API key header injected.
func (c Client) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("X-JFrog-Art-Api", c.apiKey)
	return c.httpClient.Do(req)
}

func (c Client) ListArtifacts(repo string) ([]byte, error) {
	url := c.baseURL + "/artifactory/api/storage/" + repo + "/?list&deep=1"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	return c.doAndReadBody(req)
}

func (c Client) UploadArtifact(repo, path string, reader io.Reader) error {
	url := c.baseURL + "/artifactory/" + repo + "/" + path
	req, err := http.NewRequest(http.MethodPut, url, reader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("uploading artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c Client) DeleteArtifact(repo, path string) error {
	url := c.baseURL + "/artifactory/" + repo + "/" + path
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("deleting artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c Client) GetXraySummary(repo, path string) ([]byte, error) {
	url := c.baseURL + "/xray/api/v1/summary/artifact"
	body := fmt.Sprintf(`{"paths":["default/%s/%s"]}`, repo, path)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doAndReadBody(req)
}

// doAndReadBody executes a request and returns the response body bytes.
// Returns an error for non-2xx status codes.
func (c Client) doAndReadBody(req *http.Request) ([]byte, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
