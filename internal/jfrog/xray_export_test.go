package jfrog

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jfrog_manager/internal/models"
)

const exportPath = "/xray/api/v1/component/exportDetails"

// buildZip returns a zip archive holding the given name → content entries.
func buildZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("creating zip entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("writing zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buf.Bytes()
}

// exportJSON renders an export JSON entry with one row per issue id.
func exportJSON(issueIDs ...string) string {
	rows := make([]string, 0, len(issueIDs))
	for _, id := range issueIDs {
		rows = append(rows, `{"id":"`+id+`","severity":"High","issue_type":"security",`+
			`"component":"golang.org/x/net","source_comp_id":"go://golang.org/x/net:0.59.0",`+
			`"component_versions":{"id":"golang.org/x/net","vulnerable_versions":["< 0.60.0"],`+
			`"fixed_versions":["0.60.0"]}}`)
	}
	return `{"total_count":` + strconv.Itoa(len(issueIDs)) + `,"data":[` + strings.Join(rows, ",") + `]}`
}

func writeZip(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/zip")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func newExportClient(serverURL string, timeout time.Duration) Client {
	c := newTestClient(serverURL)
	c.exportTimeout = timeout
	return c
}

func general(path, sha string) models.XrayGeneral {
	return models.XrayGeneral{
		Name:        path,
		Path:        "default/rpm-local/" + path,
		PackageType: "Rpm",
		SHA256:      sha,
		ComponentID: "rpm://" + path + ":1.0",
	}
}

func decodeExportRequest(t *testing.T, r *http.Request) exportRequest {
	t.Helper()
	var req exportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Errorf("decoding request body: %v", err)
	}
	return req
}

func TestGetComponentFixes_RequestAndDecode(t *testing.T) {
	var mu sync.Mutex
	got := map[string]exportRequest{}
	var raw []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != exportPath {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected JSON content type, got %q", ct)
		}
		if user, _, ok := r.BasicAuth(); !ok || user != "test-user" {
			t.Errorf("expected basic auth, got %q", user)
		}
		var m map[string]any
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Errorf("decoding body: %v", err)
		}
		b, err := json.Marshal(m)
		if err != nil {
			t.Errorf("re-encoding body: %v", err)
		}
		var req exportRequest
		if err := json.Unmarshal(b, &req); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		mu.Lock()
		got[req.SHA256] = req
		raw = append(raw, m)
		mu.Unlock()
		issue := "XRAY-1"
		if req.SHA256 == "sha-b" {
			issue = "XRAY-2"
		}
		writeZip(w, buildZip(t, map[string]string{"Rpm_" + req.SHA256 + "_Security_Export.json": exportJSON(issue, issue+"0")}))
	}))
	defer server.Close()

	client := newExportClient(server.URL, 5*time.Second)
	fixes, err := client.GetComponentFixes(context.Background(), []models.XrayGeneral{general("a.rpm", "sha-a"), general("b.rpm", "sha-b")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes.Failed != 0 {
		t.Errorf("expected Failed 0, got %d", fixes.Failed)
	}

	reqA := got["sha-a"]
	if reqA.ComponentName != "a.rpm:1.0" {
		t.Errorf("expected scheme stripped component name, got %q", reqA.ComponentName)
	}
	if reqA.PackageType != "rpm" {
		t.Errorf("expected lowercased package type, got %q", reqA.PackageType)
	}
	if !reqA.Security || reqA.Violations || reqA.License || reqA.OperationalRisk || reqA.OutputFormat != "json" {
		t.Errorf("unexpected export options: %+v", reqA)
	}
	if got["sha-b"].ComponentName != "b.rpm:1.0" {
		t.Errorf("unexpected component name for b: %q", got["sha-b"].ComponentName)
	}
	for _, m := range raw {
		for _, key := range []string{"component_name", "package_type", "sha_256", "security", "violations", "license", "operational_risk", "output_format"} {
			if _, ok := m[key]; !ok {
				t.Errorf("request body missing %q: %v", key, m)
			}
		}
	}

	rowsA := fixes.ByArtifact["default/rpm-local/a.rpm"]
	if len(rowsA) != 2 || rowsA[0].IssueID != "XRAY-1" {
		t.Fatalf("unexpected rows for a: %+v", rowsA)
	}
	if rowsA[0].SourceComponentID != "go://golang.org/x/net:0.59.0" {
		t.Errorf("unexpected source component: %q", rowsA[0].SourceComponentID)
	}
	if fv := rowsA[0].ComponentVersions.FixedVersions; len(fv) != 1 || fv[0] != "0.60.0" {
		t.Errorf("unexpected fixed versions: %v", fv)
	}
	rowsB := fixes.ByArtifact["default/rpm-local/b.rpm"]
	if len(rowsB) != 2 || rowsB[0].IssueID != "XRAY-2" {
		t.Errorf("unexpected rows for b: %+v", rowsB)
	}
}

func TestGetComponentFixes_EmptyInputMakesNoCalls(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	fixes, err := newExportClient(server.URL, time.Second).GetComponentFixes(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 0 {
		t.Errorf("expected no calls, got %d", calls.Load())
	}
	if fixes.Failed != 0 || len(fixes.ByArtifact) != 0 {
		t.Errorf("expected empty fixes, got %+v", fixes)
	}
}

func TestGetComponentFixes_PerArtifactFailuresDegrade(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeExportRequest(t, r)
		switch req.SHA256 {
		case "sha-404":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Component is not found for given sha"}`))
		case "sha-400":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Non of the export options were selected"}{"error":"second"}`))
		default:
			writeZip(w, buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1")}))
		}
	}))
	defer server.Close()

	fixes, err := newExportClient(server.URL, 5*time.Second).GetComponentFixes(context.Background(), []models.XrayGeneral{
		general("missing.rpm", "sha-404"),
		general("ok.rpm", "sha-ok"),
		general("bad.rpm", "sha-400"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes.Failed != 2 {
		t.Errorf("expected Failed 2, got %d", fixes.Failed)
	}
	if rows := fixes.ByArtifact["default/rpm-local/ok.rpm"]; len(rows) != 1 {
		t.Errorf("expected the healthy artifact's rows, got %+v", rows)
	}
	if _, ok := fixes.ByArtifact["default/rpm-local/missing.rpm"]; ok {
		t.Error("failed artifact must not be in ByArtifact")
	}
}

// setVar overrides a package-level setting for one test. Tests that use it
// must not call t.Parallel.
func setVar[T any](t *testing.T, v *T, val T) {
	t.Helper()
	orig := *v
	*v = val
	t.Cleanup(func() { *v = orig })
}

func TestGetComponentFixes_OneDeadlineSharedByAllCalls(t *testing.T) {
	// One worker runs the calls one after another. Each fits in the timeout on
	// its own, but not both: only a deadline shared by the whole load fails the
	// second one.
	setVar(t, &xrayExportWorkers, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeExportRequest(t, r)
		select {
		case <-r.Context().Done():
			return
		case <-time.After(700 * time.Millisecond):
		}
		writeZip(w, buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1")}))
	}))
	defer server.Close()

	start := time.Now()
	fixes, err := newExportClient(server.URL, time.Second).GetComponentFixes(context.Background(), []models.XrayGeneral{
		general("first.rpm", "sha-1"),
		general("second.rpm", "sha-2"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes.Failed != 1 {
		t.Errorf("expected Failed 1, got %d", fixes.Failed)
	}
	if rows := fixes.ByArtifact["default/rpm-local/first.rpm"]; len(rows) != 1 {
		t.Errorf("expected the first artifact's rows, got %+v", rows)
	}
	if elapsed := time.Since(start); elapsed > 1800*time.Millisecond {
		t.Errorf("expected the load to stop near the shared deadline, took %v", elapsed)
	}
}

func TestGetComponentFixes_CanceledContextIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reading the body lets the server notice the client going away.
		decodeExportRequest(t, r)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := newExportClient(server.URL, 5*time.Second).GetComponentFixes(ctx, []models.XrayGeneral{general("a.rpm", "sha-a")})
	if err == nil {
		t.Fatal("expected an error when the request context is canceled")
	}
}

func TestGetComponentFixes_ReadErrorAfterSuccessStatusDegrades(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeExportRequest(t, r)
		if req.SHA256 == "sha-ok" {
			writeZip(w, buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1")}))
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK\x03\x04partial"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer server.Close()

	fixes, err := newExportClient(server.URL, 500*time.Millisecond).GetComponentFixes(context.Background(), []models.XrayGeneral{
		general("ok.rpm", "sha-ok"),
		general("cut.rpm", "sha-cut"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes.Failed != 1 {
		t.Errorf("expected Failed 1, got %d", fixes.Failed)
	}
	if rows := fixes.ByArtifact["default/rpm-local/ok.rpm"]; len(rows) != 1 {
		t.Errorf("expected the healthy artifact's rows, got %+v", rows)
	}
}

func TestGetComponentFixes_SkipsMissingIdentifiers(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeZip(w, buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1")}))
	}))
	defer server.Close()

	noID := general("noid.rpm", "sha-1")
	noID.ComponentID = ""
	noSHA := general("nosha.rpm", "")
	fixes, err := newExportClient(server.URL, 5*time.Second).GetComponentFixes(context.Background(), []models.XrayGeneral{
		noID, noSHA, general("ok.rpm", "sha-ok"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call, got %d", calls.Load())
	}
	if fixes.Failed != 2 {
		t.Errorf("expected Failed 2, got %d", fixes.Failed)
	}
	if len(fixes.ByArtifact) != 1 {
		t.Errorf("expected 1 artifact, got %+v", fixes.ByArtifact)
	}
}

func TestGetComponentFixes_MalformedBodiesDegrade(t *testing.T) {
	tests := []struct {
		name    string
		body    func(t *testing.T) []byte
		jsonCap int64
		// capZip lowers the zip cap to the healthy body's size.
		capZip bool
	}{
		{name: "not a zip", body: func(t *testing.T) []byte { return []byte(`{"data":[]}`) }},
		{name: "no json entry", body: func(t *testing.T) []byte {
			return buildZip(t, map[string]string{"readme.txt": "hello"})
		}},
		{name: "bad json", body: func(t *testing.T) []byte {
			return buildZip(t, map[string]string{"x.json": "{not json"})
		}},
		{name: "oversized entry", jsonCap: 300, body: func(t *testing.T) []byte {
			return buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1", "XRAY-2")})
		}},
		{name: "oversized body", capZip: true, body: func(t *testing.T) []byte {
			return buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1", "XRAY-2", "XRAY-3", "XRAY-4")})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The healthy body fits the lowered caps; only the bad one exceeds them.
			healthy := buildZip(t, map[string]string{"x.json": exportJSON("XRAY-9")})
			if tt.jsonCap > 0 {
				setVar(t, &xrayExportMaxJSONBytes, tt.jsonCap)
			}
			bad := tt.body(t)
			if tt.capZip {
				if len(bad) <= len(healthy) {
					t.Fatalf("test setup: bad body (%d bytes) must be larger than the healthy one (%d)", len(bad), len(healthy))
				}
				setVar(t, &xrayExportMaxZipBytes, int64(len(healthy)))
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if decodeExportRequest(t, r).SHA256 == "sha-ok" {
					writeZip(w, healthy)
					return
				}
				writeZip(w, bad)
			}))
			defer server.Close()

			fixes, err := newExportClient(server.URL, 5*time.Second).GetComponentFixes(context.Background(), []models.XrayGeneral{
				general("ok.rpm", "sha-ok"),
				general("bad.rpm", "sha-bad"),
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fixes.Failed != 1 {
				t.Errorf("expected Failed 1, got %d", fixes.Failed)
			}
			if rows := fixes.ByArtifact["default/rpm-local/ok.rpm"]; len(rows) != 1 || rows[0].IssueID != "XRAY-9" {
				t.Errorf("expected the healthy artifact's rows, got %+v", rows)
			}
			if _, ok := fixes.ByArtifact["default/rpm-local/bad.rpm"]; ok {
				t.Error("malformed artifact must not be in ByArtifact")
			}
		})
	}
}

func TestGetComponentFixes_WorkerPoolCoversAllArtifacts(t *testing.T) {
	setVar(t, &xrayExportWorkers, 2)

	// The first two calls wait for each other, so the test only passes when
	// two run at once; peak shows no more than two ever do.
	var inFlight, peak atomic.Int32
	var arrived sync.WaitGroup
	arrived.Add(2)
	var seen atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		if seen.Add(1) <= 2 {
			arrived.Done()
			done := make(chan struct{})
			go func() { arrived.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("two export calls never ran at the same time")
			}
		}
		time.Sleep(5 * time.Millisecond)
		writeZip(w, buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1")}))
	}))
	defer server.Close()

	arts := make([]models.XrayGeneral, 0, 7)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		arts = append(arts, general(name+".rpm", "sha-"+name))
	}
	fixes, err := newExportClient(server.URL, 10*time.Second).GetComponentFixes(context.Background(), arts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixes.ByArtifact) != 7 || fixes.Failed != 0 {
		t.Errorf("expected 7 artifacts and no failures, got %d / %d", len(fixes.ByArtifact), fixes.Failed)
	}
	if peak.Load() != 2 {
		t.Errorf("expected exactly 2 concurrent calls at peak, got %d", peak.Load())
	}
}
