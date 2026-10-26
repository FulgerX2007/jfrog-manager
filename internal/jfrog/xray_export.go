package jfrog

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"jfrog_manager/internal/models"
)

// xrayExportWorkers is the number of component export calls run at once.
var xrayExportWorkers = 4

// Size caps for one component export response. The zipped body is read into
// memory; the JSON entry is capped separately as a zip-bomb guard.
var (
	xrayExportMaxZipBytes  int64 = 20 << 20
	xrayExportMaxJSONBytes int64 = 100 << 20
)

// exportRequest is the body of POST /xray/api/v1/component/exportDetails.
type exportRequest struct {
	ComponentName   string `json:"component_name"`
	PackageType     string `json:"package_type"`
	SHA256          string `json:"sha_256"`
	Security        bool   `json:"security"`
	Violations      bool   `json:"violations"`
	License         bool   `json:"license"`
	OperationalRisk bool   `json:"operational_risk"`
	OutputFormat    string `json:"output_format"`
}

// exportResponse is the JSON entry inside the export zip.
type exportResponse struct {
	Data []models.XrayExportRow `json:"data"`
}

// exportResult is the outcome of one artifact's export call.
type exportResult struct {
	rows []models.XrayExportRow
	err  error
}

// GetComponentFixes fetches the vulnerable components and fixed versions for
// the given scanned artifacts from the Xray component export, one call per
// artifact, xrayExportWorkers at a time, all bounded by one exportTimeout
// derived from ctx. Every per-artifact failure (missing component id or
// sha256, network error, non-2xx, deadline, malformed or oversized body) is
// logged and counted in Failed; the other artifacts keep their rows. An error
// is returned only when ctx itself is done, e.g. the browser went away.
func (c Client) GetComponentFixes(ctx context.Context, artifacts []models.XrayGeneral) (models.ComponentFixes, error) {
	fixes := models.ComponentFixes{ByArtifact: map[string][]models.XrayExportRow{}}
	if len(artifacts) == 0 {
		return fixes, nil
	}

	exportCtx, cancel := context.WithTimeout(ctx, c.exportTimeout)
	defer cancel()

	results := make([]exportResult, len(artifacts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(xrayExportWorkers, len(artifacts)) {
		wg.Go(func() {
			for i := range jobs {
				rows, err := c.exportComponent(exportCtx, artifacts[i])
				results[i] = exportResult{rows: rows, err: err}
			}
		})
	}
	for i := range artifacts {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return models.ComponentFixes{}, fmt.Errorf("exporting component details: %w", err)
	}

	for i, res := range results {
		if res.err != nil {
			slog.Warn("xray component export failed", "path", artifacts[i].Path, "error", res.err)
			fixes.Failed++
			continue
		}
		fixes.ByArtifact[artifacts[i].Path] = res.rows
	}
	return fixes, nil
}

// exportComponent runs one export call and decodes its zipped JSON body.
func (c Client) exportComponent(ctx context.Context, art models.XrayGeneral) ([]models.XrayExportRow, error) {
	if art.ComponentID == "" || art.SHA256 == "" {
		return nil, errors.New("missing component id or sha256")
	}
	name := art.ComponentID
	if _, rest, ok := strings.Cut(name, "://"); ok {
		name = rest
	}
	body, err := json.Marshal(exportRequest{
		ComponentName: name,
		PackageType:   strings.ToLower(art.PackageType),
		SHA256:        art.SHA256,
		Security:      true,
		OutputFormat:  "json",
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/xray/api/v1/component/exportDetails", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling export: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Error bodies are not reliable JSON; decide on the status code alone.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	zipped, err := io.ReadAll(io.LimitReader(resp.Body, xrayExportMaxZipBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	if int64(len(zipped)) > xrayExportMaxZipBytes {
		return nil, fmt.Errorf("export body exceeds %d bytes", xrayExportMaxZipBytes)
	}
	return decodeExportZip(zipped)
}

// decodeExportZip reads the first .json entry of an export zip.
func decodeExportZip(zipped []byte) ([]models.XrayExportRow, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		return nil, fmt.Errorf("opening export zip: %w", err)
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".json") {
			continue
		}
		if f.UncompressedSize64 > uint64(xrayExportMaxJSONBytes) {
			return nil, fmt.Errorf("export entry %s exceeds %d bytes", f.Name, xrayExportMaxJSONBytes)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("opening export entry %s: %w", f.Name, err)
		}
		// Decode as a stream so only the kept fields are held in memory; the
		// limited reader also guards against a header that understates the size.
		limited := &io.LimitedReader{R: rc, N: xrayExportMaxJSONBytes + 1}
		var out exportResponse
		err = json.NewDecoder(limited).Decode(&out)
		_ = rc.Close()
		if limited.N <= 0 {
			return nil, fmt.Errorf("export entry %s exceeds %d bytes", f.Name, xrayExportMaxJSONBytes)
		}
		if err != nil {
			return nil, fmt.Errorf("parsing export entry %s: %w", f.Name, err)
		}
		return out.Data, nil
	}
	return nil, errors.New("export zip has no .json entry")
}
