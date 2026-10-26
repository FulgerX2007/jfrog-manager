package jfrog

import (
	"archive/zip"
	"bytes"
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
		b, _ := json.Marshal(m)
		var req exportRequest
		_ = json.Unmarshal(b, &req)
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
	fixes, err := client.GetComponentFixes([]models.XrayGeneral{general("a.rpm", "sha-a"), general("b.rpm", "sha-b")})
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

	fixes, err := newExportClient(server.URL, time.Second).GetComponentFixes(nil)
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

	fixes, err := newExportClient(server.URL, 5*time.Second).GetComponentFixes([]models.XrayGeneral{
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

func TestGetComponentFixes_DeadlineCountsSlowArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := decodeExportRequest(t, r)
		if strings.HasPrefix(req.SHA256, "slow") {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
			return
		}
		writeZip(w, buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1")}))
	}))
	defer server.Close()

	fixes, err := newExportClient(server.URL, 200*time.Millisecond).GetComponentFixes([]models.XrayGeneral{
		general("fast.rpm", "fast"),
		general("slow1.rpm", "slow1"),
		general("slow2.rpm", "slow2"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes.Failed != 2 {
		t.Errorf("expected Failed 2, got %d", fixes.Failed)
	}
	if rows := fixes.ByArtifact["default/rpm-local/fast.rpm"]; len(rows) != 1 {
		t.Errorf("expected the fast artifact's rows, got %+v", rows)
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
	fixes, err := newExportClient(server.URL, 5*time.Second).GetComponentFixes([]models.XrayGeneral{
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

func TestGetComponentFixes_MalformedBodiesAreErrors(t *testing.T) {
	tests := []struct {
		name string
		body func(t *testing.T) []byte
		cap  int64
	}{
		{name: "not a zip", body: func(t *testing.T) []byte { return []byte(`{"data":[]}`) }},
		{name: "no json entry", body: func(t *testing.T) []byte {
			return buildZip(t, map[string]string{"readme.txt": "hello"})
		}},
		{name: "bad json", body: func(t *testing.T) []byte {
			return buildZip(t, map[string]string{"x.json": "{not json"})
		}},
		{name: "oversized entry", cap: 64, body: func(t *testing.T) []byte {
			return buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1", "XRAY-2")})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cap > 0 {
				orig := xrayExportMaxJSONBytes
				xrayExportMaxJSONBytes = tt.cap
				defer func() { xrayExportMaxJSONBytes = orig }()
			}
			body := tt.body(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeZip(w, body)
			}))
			defer server.Close()

			_, err := newExportClient(server.URL, 5*time.Second).GetComponentFixes([]models.XrayGeneral{general("a.rpm", "sha-a")})
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestGetComponentFixes_OversizedBodyIsError(t *testing.T) {
	orig := xrayExportMaxZipBytes
	xrayExportMaxZipBytes = 16
	defer func() { xrayExportMaxZipBytes = orig }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeZip(w, buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1")}))
	}))
	defer server.Close()

	_, err := newExportClient(server.URL, 5*time.Second).GetComponentFixes([]models.XrayGeneral{general("a.rpm", "sha-a")})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestGetComponentFixes_WorkerPoolCoversAllArtifacts(t *testing.T) {
	orig := xrayExportWorkers
	xrayExportWorkers = 2
	defer func() { xrayExportWorkers = orig }()

	var inFlight, peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		writeZip(w, buildZip(t, map[string]string{"x.json": exportJSON("XRAY-1")}))
	}))
	defer server.Close()

	arts := make([]models.XrayGeneral, 0, 7)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		arts = append(arts, general(name+".rpm", "sha-"+name))
	}
	fixes, err := newExportClient(server.URL, 5*time.Second).GetComponentFixes(arts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fixes.ByArtifact) != 7 || fixes.Failed != 0 {
		t.Errorf("expected 7 artifacts and no failures, got %d / %d", len(fixes.ByArtifact), fixes.Failed)
	}
	if peak.Load() > 2 {
		t.Errorf("expected at most 2 concurrent calls, got %d", peak.Load())
	}
}

func TestStripScheme(t *testing.T) {
	tests := map[string]string{
		"rpm://opcore:1.10.25":         "opcore:1.10.25",
		"go://golang.org/x/net:0.59.0": "golang.org/x/net:0.59.0",
		"opcore:1.10.25":               "opcore:1.10.25",
		"":                             "",
	}
	for in, want := range tests {
		if got := stripScheme(in); got != want {
			t.Errorf("stripScheme(%q) = %q, want %q", in, got, want)
		}
	}
}
