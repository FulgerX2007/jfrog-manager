# Vulnerable and Fixed Version Columns in the Vulnerability Report

## Overview
- Add two columns to the vulnerability report (`GET /vulnerabilities`) and its CSV export (`GET /vulnerabilities/export`): **Vulnerable version** (the library and version inside the artifact that carries the issue, e.g. `golang.org/x/net:0.59.0`) and **Fixed version** (the version(s) Xray says fix it, e.g. `0.60.0`), matching the Component and Fix Version columns in the Xray UI.
- Goal: remediation guidance — a reader sees what to upgrade and to which version without opening Xray.
- The current data source, `POST /xray/api/v1/summary/artifact`, returns `components: null` on this instance, and the token has no Xray reports permission (Task 1). The source is the **Xray Export Component Details API** (`POST /xray/api/v1/component/exportDetails`), which the token can call and which returns installed version and fixed versions per issue and component.
- The new data is joined onto the existing rows. If it can't be fetched, the report still renders as today, with the two columns showing `—` and a notice. Missing version data never hides or drops a finding.

## Context (from discovery)
- files/components involved:
  - `internal/jfrog/xray.go`, `internal/jfrog/service.go`, `internal/jfrog/client.go` (`Do`, timeout), `internal/jfrog/xray_test.go`
  - `internal/models` (`XrayGeneral`, `XrayIssue`, `VulnRow`)
  - `internal/report/report.go` (`Build`, `normalizePath`), `internal/report/csv.go`
  - `internal/handlers/vulnerabilities.go` (`buildVulnerabilityReport`, `GetVulnerabilities`, `ExportVulnerabilities`)
  - `templates/partials/vuln_report.html`, `templates/layout.html` (CSS)
  - mocks: `mockService` in `main_test.go` and `internal/handlers/artifacts_test.go`
  - docs: `docs/api/README.md`, `CLAUDE.md`
- related patterns found:
  - Xray degrades instead of failing: unreachable / 404 / non-2xx → `Available: false`, nil error; only a malformed 2xx body is an error.
  - Report logic is pure Go in `internal/report`; handlers only orchestrate.
  - Value receivers, no pointers; `slog` logging; `fmt.Errorf("...: %w")`; stdlib-only tests; `internal/jfrog` tests run the real `Client` against `httptest.Server`.
  - CSV cells go through `guardCell` (formula-injection guard).
- dependencies identified: Xray `exportDetails` endpoint (works with the current read permission, Xray 3.124.42), `archive/zip` from the stdlib, the existing `TIMEOUT` config.

## Development Approach
- **testing approach**: Regular (code first, then tests)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional - they are a required part of the checklist
  - write unit tests for new functions/methods
  - write unit tests for modified functions/methods
  - add new test cases for new code paths
  - update existing test cases if behavior changes
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- run tests after each change (`go test ./...`) and `golangci-lint run`
- maintain backward compatibility: existing columns, sort, severity filter and not-scanned handling stay as they are

## Testing Strategy
- **unit tests**: required for every task (see Development Approach above)
- **e2e tests**: the project has no UI e2e suite; template output is covered by handler tests that render real templates (`internal/handlers/vulnerabilities_test.go`) and `main_test.go`

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope
- keep plan in sync with actual work done

## Solution Overview
- **Data source**: one `exportDetails` call per latest artifact that has at least one issue. The summary call the report already makes returns each artifact's `component_id`, `pkg_type` and `sha256` — exactly what the export call needs — so no extra lookup is required.
- **Flow**: summary (as today) → `report.Build` → collect the scanned artifacts that have issues → `GetComponentFixes` for them → `report.AttachVersions` → filter, sort, group as today. The fixes call depends on the summary, so it runs after it, not in parallel.
- **Concurrency**: the client runs the export calls with a small worker pool (`xrayExportWorkers = 4`), writing each result into its own slot of a pre-sized slice (race-free). All calls share one `context.WithTimeout` equal to `TIMEOUT`. Measured: ~0.4 s per artifact, so ~20 artifacts take ~2 s.
- **Join**: per artifact, then by issue id. Because each export covers one artifact, there is no path matching inside an artifact: a `VulnRow` takes every export row with the same `id` (Xray issue id). One issue often hits several components or versions in one artifact (e.g. `golang.org/x/net:0.58.0` and `:0.59.0` in different bundled binaries), so a row carries a list of fixes.
- **Degrade, never hide**: a failed export for one artifact leaves only that artifact's rows at `—`; the others still get versions. If any export failed or the deadline hit, the page shows a muted notice. Only a malformed 2xx body is an error, and the handler logs it and degrades rather than failing the page, because the summary data is still good.
- **Why not other APIs** (checked in Task 1): reports API → 403; `summary/artifact` and `summary/component` → `components: null`, no fix fields; dependency graph → versions but no fixes and no issue link.

## Technical Details
- **Request**: `POST /xray/api/v1/component/exportDetails`, `Content-Type: application/json`, via `c.Do` with `http.NewRequestWithContext`:
  ```json
  {"component_name": "<component_id without the scheme>", "package_type": "<lowercased pkg_type>",
   "sha_256": "<sha256>", "security": true, "violations": false, "license": false,
   "operational_risk": false, "output_format": "json"}
  ```
  - `component_name` is the summary's `general.component_id` (e.g. `<rpm name>:<version>`); with or without the scheme both work. The file name (`general.name`) does **not** work (404 "Component is not found for given sha").
  - `package_type` is `general.pkg_type` lowercased (`Rpm` → `rpm`).
  - `"vulnerabilities": true` is not an option name: without `security` the API answers 400 "Non of the export options were selected".
- **Response**: `200`, `Content-Type: application/zip`, one entry `<Type>_<sha prefix>_Security_Export.json` (≈25 KB zipped / 320 KB unzipped for 101 rows):
  ```json
  {"total_count": 53, "data": [{
     "id": "XRAY-1104848", "severity": "Critical", "issue_type": "security",
     "component": "golang.org/x/net", "source_comp_id": "go://golang.org/x/net:0.59.0",
     "component_versions": {"id": "golang.org/x/net", "vulnerable_versions": ["< 0.60.0"],
                            "fixed_versions": ["0.60.0"], "more_details": {"cves": [...]}},
     "component_physical_paths": ["<path inside the artifact>/golang.org/x/net"]}]}
  ```
  `total_count` counts issues; `data` has one row per issue × component version. Errors: 400/404 with a JSON `{"error": ...}` body (sometimes followed by a second JSON object), so decide on the status code, never parse error bodies.
- **Reading the zip**: read the body through `io.LimitReader` (cap 20 MB), `zip.NewReader` on the bytes, take the first `.json` entry, decode it through `io.LimitReader` (cap 100 MB uncompressed) as a zip-bomb guard. No JSON entry or undecodable JSON in a 2xx → error.
- **Model changes** (`internal/models`):
  ```go
  // XrayGeneral gets: ComponentID string `json:"component_id"`

  // XrayExportRow is one issue × component row of an Xray component export.
  type XrayExportRow struct {
      IssueID           string                `json:"id"`
      SourceComponentID string                `json:"source_comp_id"`
      ComponentVersions XrayComponentVersions `json:"component_versions"`
  }
  type XrayComponentVersions struct {
      FixedVersions []string `json:"fixed_versions"`
  }

  // ComponentFixes holds the export rows per artifact, keyed by the artifact
  // path exactly as the summary returned it (general.path).
  type ComponentFixes struct {
      ByArtifact map[string][]XrayExportRow
      Failed     int  // artifacts whose export failed or timed out
  }

  // ComponentFix is one vulnerable component of a report row.
  type ComponentFix struct {
      Component string   // installed component and version, e.g. golang.org/x/net:0.59.0
      Fixed     []string // e.g. ["0.60.0"]; empty when Xray knows no fix
  }
  ```
  `VulnRow` gets `Fixes []ComponentFix` and `FixesMatched bool` (an export row was found for this row's issue).
- **Service**: `GetComponentFixes(artifacts []models.XrayGeneral) (models.ComponentFixes, error)`. Per-artifact failures (network, non-2xx, deadline) are counted in `Failed` and logged with `slog.Warn`; a malformed 2xx body is returned as a wrapped error.
- **`report.AttachVersions(repo string, rows []models.VulnRow, fixes models.ComponentFixes) []models.VulnRow`**: returns a copy. Keys `fixes.ByArtifact` through `normalizePath(repo, key)` (the same rule `Build` uses for summary paths) and matches rows by `strings.Trim(row.ArtifactPath, "/")`, then by issue id. For each matching export row it adds `ComponentFix{Component: componentLabel(source_comp_id), Fixed: fixed_versions}`, deduplicated by component, sorted by component; sets `FixesMatched`. Never drops or adds rows. If rows exist, at least one artifact export succeeded, and nothing matched, it logs `slog.Warn` so a broken join is visible.
- **`componentLabel`**: strips the `xxx://` scheme only (`go://golang.org/x/net:0.59.0` → `golang.org/x/net:0.59.0`); a value with no scheme is returned as is.
- **Template** (`vuln_report.html`): columns become Severity · ID · Package · **Vulnerable version** · **Fixed version** · Summary · Source. One line per `ComponentFix`, aligned across the two cells: `<code>golang.org/x/net:0.59.0</code>` | `<code>0.60.0</code>` (several fixed versions joined with `, `). Empty fixed list on a matched row → `—` with `title="No fix available"`; unmatched row → plain `—` in both cells. When `.Available` and `.FixesFailed > 0`, a muted `o-alert` notice: "Vulnerable and fixed versions could not be retrieved for N packages."
- **CSV**: header becomes `Component, Package, Artifact, Severity, Issue ID, CVEs, Vulnerable Version, Fixed Version, Summary, Impact Path`. `Vulnerable Version` = components joined with `"; "`; `Fixed Version` = the matching fixed lists in the same order, each joined with `", "`, entries joined with `"; "` (empty list → `none`). Every cell still goes through `guardCell`. `Summary` and `Impact Path` move two positions — documented in `docs/api/README.md`.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): code, tests, docs in this repo
- **Post-Completion** (no checkboxes): manual verification against the real Xray

## Implementation Steps

### Task 1: Spike — find an Xray API that returns fixed versions on this instance

**Files:**
- Modify: `docs/plans/20261026-vuln-fixed-versions.md` (record findings)

- [x] try the Xray Reports API with the `.env` credentials (create / list) — 403, no reports permission
- [x] try read-only alternatives: `summary/artifact`, `summary/component`, `dependencyGraph/artifact`, `component/exportDetails`
- [x] confirm `exportDetails` returns installed version and fixed versions for a known finding (CVE-2026-78663: `golang.org/x/net:0.59.0` → `0.60.0`, as in the Xray UI)
- [x] record request fields, response shape, error behavior and timing in Technical Details
- [x] decide the approach with the user: switch to `exportDetails`; "Vulnerable version" shows the installed version
- [x] no code change in this task, so no tests; `go test ./...` still passes

**Spike findings (2026-10-26):**
- Auth as the app does it (basic auth). Xray `3.124.42`; ping 200; `summary/artifact` 200.
- Reports API: create `POST /xray/api/v1/reports/vulnerabilities` → 403 with empty body on the default repo and the largest local repo; the reports list → 403 too. No report was created. Bearer auth → 401.
- `summary/artifact` and `summary/component` (`go://golang.org/x/net:0.59.0` → 200, 5 issues): `components` is `null`, no fix fields anywhere. A component id without the scheme → 400.
- `dependencyGraph/artifact` → 200, 1,448 nested components with versions (e.g. `golang.org/x/net:0.58.0/0.59.0/0.60.0/0.61.0` in different bundled binaries) but no issues or fixes.
- Impact paths end with the component name without a version (`…/gpx_reporting_linux_amd64/golang.org/x/net`), so the version can't be parsed from them.
- `component/exportDetails` → 200 zip, ~0.4 s, 101 rows / 53 issues for one RPM, **every row has `fixed_versions`**. Summary `general` already includes `component_id`, equal to the dependency graph's artifact id.

### Task 2: Add models and the Service method

**Files:**
- Modify: `internal/models/models.go` (or the file holding `XrayGeneral` / `VulnRow`)
- Modify: `internal/jfrog/service.go`
- Modify: `main_test.go`
- Modify: `internal/handlers/artifacts_test.go`
- Modify: `internal/jfrog/xray_test.go`

- [x] add `ComponentID` to `XrayGeneral`; add `XrayExportRow`, `XrayComponentVersions`, `ComponentFixes`, `ComponentFix`; add `Fixes` and `FixesMatched` to `VulnRow`
- [x] add `GetComponentFixes(artifacts []models.XrayGeneral) (models.ComponentFixes, error)` to `jfrog.Service`
- [x] add the method to both `mockService` implementations: fields `fixes models.ComponentFixes`, `fixesErr error`; the handler mock also records `fixesArtifacts`
- [x] add a stub on `Client` returning `ComponentFixes{}` so the build passes (replaced in Task 3)
- [x] extend an existing `GetXraySummaries` test so the fixture has `component_id` and the decoded `General.ComponentID` is asserted
- [x] run tests - must pass before task 3

### Task 3: Implement `Client.GetComponentFixes`

**Files:**
- Modify: `internal/jfrog/client.go` (`exportTimeout` field, set in `NewClient`)
- Create: `internal/jfrog/xray_export.go`
- Create: `internal/jfrog/xray_export_test.go`

- [x] add `exportTimeout time.Duration` to `Client`, set from `cfg.Timeout` in `NewClient`; tests in the same package build a `Client` with a few-millisecond value
- [x] implement the request body from Technical Details (scheme stripped from `component_id`, `pkg_type` lowercased) and the zip reading with both size caps
- [x] run the calls with `xrayExportWorkers` (package var, 4) sharing one `context.WithTimeout(exportTimeout)`; results into a pre-sized slice, merged after `Wait`; skip artifacts with an empty `component_id` or `sha256` (counted as failed)
- [x] degrade rules: network error / non-2xx / deadline for an artifact → counted in `Failed`, `slog.Warn`, no error; malformed 2xx (not a zip, no `.json` entry, bad JSON, over the cap) → wrapped error
- [x] replace the Task 2 stub
- [x] write tests (httptest.Server serving zips built in the test with `archive/zip`): request body fields per artifact, two artifacts decoded into `ByArtifact`, empty input → no calls
- [x] write tests for: one artifact 404 and another 200 (`Failed` = 1, the other present); 400 with a two-object error body; deadline hit (`Failed` counts the slow ones); not-a-zip 200 → error; zip without a `.json` entry → error; oversized entry → error; missing `component_id` skipped; run with `go test -race ./internal/jfrog`
- [x] run tests - must pass before task 4

### Task 4: Join fixes onto report rows

**Files:**
- Create: `internal/report/versions.go`
- Create: `internal/report/versions_test.go`

- [ ] implement `AttachVersions(repo, rows, fixes)` and `componentLabel` as described in Technical Details
- [ ] set `FixesMatched`; deduplicate by component, sort; never drop or add rows; `slog.Warn` when exports succeeded, rows exist and none matched
- [ ] write table-driven tests for `componentLabel` (`go://golang.org/x/net:0.59.0`, `rpm://…`, `npm://lodash:4.17.20`, no scheme, empty)
- [ ] write tests for `AttachVersions`: match by artifact + issue id; artifact key with `default/<repo>/` prefix vs `ArtifactPath` with a leading `/`; several components for one issue; duplicate component rows deduplicated; same issue id on two artifacts kept apart; matched row with empty `fixed_versions` (`FixesMatched` true, empty `Fixed`); no match → empty `Fixes`, `FixesMatched` false; empty `ComponentFixes` → rows unchanged; input slice not mutated
- [ ] run tests - must pass before task 5

### Task 5: Fetch fixes in the handler

**Files:**
- Modify: `internal/handlers/vulnerabilities.go`
- Modify: `internal/handlers/vulnerabilities_test.go`

- [ ] in `buildVulnerabilityReport`, after `report.Build`, collect the `General` of each scanned latest artifact that has issues (skip the call when there are none or the summary is unavailable), call `GetComponentFixes`, then `report.AttachVersions` on `rep.Rows` before filter/sort
- [ ] an error from `GetComponentFixes` is logged and treated as "all failed" — the page still renders
- [ ] expose `FixesFailed` (count) in the result and template data; the export uses the same rows
- [ ] write handler tests: fixes available → rows carry fixes; error → 200, rows without fixes, `FixesFailed` > 0; summary unavailable → no fixes call; only artifacts with issues are passed to the call
- [ ] run tests - must pass before task 6

### Task 6: Show the columns in the report page

**Files:**
- Modify: `templates/partials/vuln_report.html`
- Modify: `templates/layout.html` (column width CSS)
- Modify: `internal/handlers/vulnerabilities_test.go`

- [ ] add the two `<th>` / `<td>` columns between Package and Summary, one aligned line per `ComponentFix`, `—` rules as in Technical Details
- [ ] add the muted notice when `.Available` and `.FixesFailed` > 0
- [ ] add `col-ver` / `col-fix` widths in `layout.html` so the table still fits; check the narrow-screen layout
- [ ] write rendering tests: headers present, components and fixed versions rendered and HTML-escaped, several components aligned, "No fix available" title only on matched-unfixed rows, plain `—` on unmatched rows, notice shown only when some exports failed and absent when the summary itself is unavailable
- [ ] run tests - must pass before task 7

### Task 7: Add the columns to the CSV export

**Files:**
- Modify: `internal/report/csv.go`
- Modify: `internal/report/csv_test.go`
- Modify: `internal/handlers/vulnerabilities_test.go` (export assertions, if any check the header)

- [ ] extend `csvHeader` and the record with `Vulnerable Version` and `Fixed Version` after `CVEs`, formatted as in Technical Details, through `guardCell`
- [ ] update existing CSV tests for the new header/column positions
- [ ] write tests for several components, an empty fixed list (`none`), an unmatched row (empty cells), and the formula guard on a value starting with `=` or `-`
- [ ] run tests - must pass before task 8

### Task 8: Verify acceptance criteria
- [ ] verify both columns show on the page and in the CSV for scanned findings
- [ ] verify the page still renders (200) with `—` and the notice when some or all exports fail or time out
- [ ] verify not-scanned packages, severity filter, sort and clean list are unchanged
- [ ] run full test suite: `go test ./...` and `go test -race ./...`
- [ ] run `golangci-lint run`
- [ ] check coverage of the new code: `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`

### Task 9: [Final] Update documentation
- [ ] update `docs/api/README.md`: new columns on the page, new CSV header (note that `Summary` and `Impact Path` moved two positions), the extra `exportDetails` call per artifact with findings
- [ ] update `CLAUDE.md`: `exportDetails` as the source of vulnerable/fixed versions (and why not reports or summaries), its degrade rule, the worker pool, and the new `Service` method in the mock-update note
- [ ] update `README.md` if it lists the report columns
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention or external systems - no checkboxes, informational only*

**Manual verification**:
- open the report for a real repo and compare a few rows against the Xray UI (Component and Fix Version), including CVE-2026-78663
- time a page load on the largest repo; if the export calls regularly approach `TIMEOUT`, raise `xrayExportWorkers` or `TIMEOUT` as a follow-up
- try a non-RPM repo (e.g. docker or generic) to confirm `package_type` and `component_name` work the same way there
