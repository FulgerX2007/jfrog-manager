package jfrog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"jfrog_manager/internal/config"
	"jfrog_manager/internal/models"
)

// storageListResponse represents the JFrog storage list API response.
type storageListResponse struct {
	Files []storageListFile `json:"files"`
}

type storageListFile struct {
	URI          string `json:"uri"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified"`
	Folder       bool   `json:"folder"`
}

// Client implements the Service interface for JFrog Artifactory API calls.
type Client struct {
	httpClient       http.Client
	streamHTTPClient http.Client
	baseURL          string
	username         string
	token            string
}

// NewClient creates a new JFrog API client from the application config.
// The streamHTTPClient has no overall timeout since uploads and downloads of
// large artifacts can legitimately take longer than the short API timeout.
func NewClient(cfg config.Config) Client {
	return Client{
		httpClient: http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		streamHTTPClient: http.Client{},
		baseURL:          strings.TrimRight(cfg.JFrogURL, "/"),
		username:         cfg.JFrogUsername,
		token:            cfg.JFrogToken,
	}
}

// Do executes an HTTP request with basic auth injected.
func (c Client) Do(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(c.username, c.token)
	return c.httpClient.Do(req)
}

// doStream executes a long-running upload or download without a fixed timeout.
func (c Client) doStream(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(c.username, c.token)
	return c.streamHTTPClient.Do(req)
}

func (c Client) ListArtifacts(repo string) ([]models.Artifact, error) {
	reqURL := c.baseURL + "/artifactory/api/storage/" + url.PathEscape(repo) + "/?list&deep=1"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	body, err := c.doAndReadBody(req)
	if err != nil {
		return nil, err
	}

	var listResp storageListResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("parsing artifacts response: %w", err)
	}

	artifacts := make([]models.Artifact, 0, len(listResp.Files))
	for _, f := range listResp.Files {
		if f.Folder {
			continue
		}
		name := f.URI
		if idx := strings.LastIndex(f.URI, "/"); idx >= 0 {
			name = f.URI[idx+1:]
		}
		if name == "repomd.xml" || strings.HasSuffix(name, ".xml.gz") {
			continue
		}
		artifacts = append(artifacts, models.Artifact{
			Name:         name,
			Path:         strings.TrimPrefix(f.URI, "/"),
			Size:         f.Size,
			LastModified: f.LastModified,
			Repo:         repo,
		})
	}

	return artifacts, nil
}

func (c Client) UploadArtifact(repo, path string, reader io.Reader) error {
	reqURL := c.baseURL + "/artifactory/" + url.PathEscape(repo) + "/" + escapePathSegments(path)
	req, err := http.NewRequest(http.MethodPut, reqURL, reader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := c.doStream(req)
	if err != nil {
		return fmt.Errorf("uploading artifact: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// DownloadArtifact fetches an artifact and returns the open response body,
// content length (may be -1 when unknown), and content type. The caller must
// close the returned ReadCloser.
func (c Client) DownloadArtifact(repo, path string) (io.ReadCloser, int64, string, error) {
	reqURL := c.baseURL + "/artifactory/" + url.PathEscape(repo) + "/" + escapePathSegments(path)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, "", fmt.Errorf("creating request: %w", err)
	}
	resp, err := c.doStream(req)
	if err != nil {
		return nil, 0, "", fmt.Errorf("downloading artifact: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, 0, "", fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}
	return resp.Body, resp.ContentLength, resp.Header.Get("Content-Type"), nil
}

func (c Client) DeleteArtifact(repo, path string) error {
	reqURL := c.baseURL + "/artifactory/" + url.PathEscape(repo) + "/" + escapePathSegments(path)
	req, err := http.NewRequest(http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("deleting artifact: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// doAndReadBody executes a request and returns the response body bytes.
// Returns an error for non-2xx status codes.
func (c Client) doAndReadBody(req *http.Request) ([]byte, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// escapePathSegments escapes each segment of a slash-separated path individually,
// preserving the slash separators that JFrog Artifactory expects in URLs.
func escapePathSegments(p string) string {
	segments := strings.Split(p, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}
