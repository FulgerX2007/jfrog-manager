# Vulnerable and Fixed Version Columns in the Vulnerability Report

## Overview
- Add two columns to the vulnerability report (`GET /vulnerabilities`) and its CSV export (`GET /vulnerabilities/export`): **Vulnerable version** (the library and version inside the artifact that carries the issue, e.g. `openssl:1.0.2k`) and **Fixed version** (the version(s) Xray says fix it).
- Goal: remediation guidance — a reader sees what to upgrade and to which version without opening Xray.
- The current data source, `POST /xray/api/v1/summary/artifact`, returns `components: null` on this instance (see `docs/plans/completed/20261006-vulnerability-report.md`), so these fields need a new source: the **Xray Vulnerabilities Report API**.
- The new data is joined onto the existing rows. If it can't be fetched, the report still renders as today, with the two columns showing `—` and a notice. Missing version data never hides or drops a finding.

## Context (from discovery)
- files/components involved:
  - `internal/jfrog/xray.go`, `internal/jfrog/service.go`, `internal/jfrog/client.go` (`Do`, timeout), `internal/jfrog/xray_test.go`
  - `internal/models` (`XrayIssue`, `VulnRow`)
  - `internal/report/report.go` (`Build`, `normalizePath`, `trimImpactPaths`), `internal/report/csv.go`
  - `internal/handlers/vulnerabilities.go` (`buildVulnerabilityReport`, `GetVulnerabilities`, `ExportVulnerabilities`)
  - `templates/partials/vuln_report.html`, `templates/layout.html` (CSS)
  - mocks: `mockService` in `main_test.go` and `internal/handlers/artifacts_test.go`
  - docs: `docs/api/README.md`, `CLAUDE.md`
- related patterns found:
  - Xray degrades instead of failing: unreachable / 404 / non-2xx → `Available: false`, nil error; only a malformed 2xx body is an error.
  - Report logic is pure Go in `internal/report`; handlers only orchestrate.
  - Value receivers, no pointers; `slog` logging; `fmt.Errorf("...: %w")`; stdlib-only tests; `internal/jfrog` tests run the real `Client` against `httptest.Server`.
  - CSV cells go through `guardCell` (formula-injection guard).
- dependencies identified: Xray Reports API (needs the token to have Xray "Manage Reports"/reports permission on the repo), the existing `TIMEOUT` config.

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
- **Data source**: Xray Vulnerabilities Report, scoped to the one repository being viewed. It is asynchronous: create → poll status → fetch rows page by page → delete.
- **Join**: each report row carries the impacted artifact, the issue id, the vulnerable component and the fixed versions. Rows are keyed by `(artifact path, issue id)` — the same pair a `VulnRow` already has — after normalizing the artifact path the way `normalizePath` does today.
- **Kept separate from the summary**: a new `Service` method and a new pure function `report.AttachVersions` that fills two new `VulnRow` fields. `report.Build` and its tests stay unchanged.
- **Latency**: the handler starts the report job in a goroutine at the same time as the summary call, so the page waits for the slower of the two, not both. The whole job (create, every poll, every page) shares **one** `context.WithTimeout` deadline equal to `TIMEOUT`; past it, the report is deleted with a fresh short context and the columns degrade to `—`.
- **Volume**: the report covers every version in the repo, not only the latest, and is re-created on every load, sort, filter and export. Task 1 times it on the largest repo; if it routinely exceeds `TIMEOUT`, narrow it (e.g. `include_path_patterns` set to the latest artifacts' paths) or stop and revisit. Caching stays out of scope.
- **Degrade, never hide**: if the report can't be created (403, 404, non-2xx), fails, or times out, `VulnDetails.Available` is false, the rows render as today, the two columns show `—`, and a muted notice says version details are unavailable. A row with no matching report entry shows `—` too. Only a malformed 2xx body is an error, and the handler logs it and degrades rather than failing the page, because the summary data is still good.
- **Why not the component summary API or impact-path parsing**: the component id would have to be guessed from impact paths and may hit the same `null` problem; parsing impact paths gives no fixed version at all. The Reports API returns both as documented fields.

## Technical Details
- **Timeout**: `Client` gets a `reportTimeout time.Duration` field, set in `NewClient` from `cfg.Timeout`. Tests build a `Client` with a few-millisecond `reportTimeout` directly (same package), since `cfg.Timeout` is in whole seconds.
- **Xray calls** (all via `c.Do` with `http.NewRequestWithContext`; create, poll and fetch share one `context.WithTimeout(context.Background(), c.reportTimeout)`):
  1. `POST /xray/api/v1/reports/vulnerabilities`
     body: `{"name": "jfrog-manager-<repo>-<unix-nanos>", "resources": {"repositories": [{"name": "<repo>"}]}}` → `{"report_id": 123, "status": "pending"}`. The name is a plain JSON string (no `url.PathEscape`); drop any characters Xray rejects in names (confirm in Task 1). The id goes into paths with `strconv.Itoa`.
  2. `GET /xray/api/v1/reports/{id}` → `{"status": ...}`; poll every `xrayReportPollInterval` (1s; a package var so tests can shorten it). `completed` → fetch; `failed` (and `aborted`, if Task 1 confirms it exists) → unavailable; **any other or unknown status keeps polling** until the deadline.
  3. `POST /xray/api/v1/reports/vulnerabilities/{id}?direction=asc&page_num=N&num_of_rows=<max>` → `{"total_rows": n, "rows": [...]}`. `page_num` starts at **1** (confirm in Task 1), `num_of_rows` is the max Xray allows (confirm; plan assumes 1000), send `{}` with `Content-Type: application/json` if Xray needs a body. Loop until `total_rows` rows are read or a page is empty, capped at `xrayReportMaxPages` (e.g. 100) so a server that keeps returning the same page can't loop forever.
  4. `DELETE /xray/api/v1/reports/{id}` — always, via `defer`, once a report id exists, with a **fresh** `context.WithTimeout(context.Background(), 5*time.Second)` so cleanup still runs when the job deadline has passed; failure is logged and ignored.
- **Report row fields used** (others ignored by the decoder): `issue_id`, `vulnerable_component` (e.g. `rpm://7:openssl:1.0.2k`), `impacted_artifact`, `path`, `fixed_versions` (`[]string`).
  - ⚠️ `path` is likely the artifact's **folder** (`repo/folder/sub/`, trailing slash) and `impacted_artifact` a component-style id, not a repo path. Task 1 records the exact rule for the join key, e.g. `trim(normalizePath(path), "/") + "/" + <file name from impacted_artifact or another field>`, and the `vulnerable_component` formats seen. Rows and fields here are updated from that.
- **New models** (`internal/models`):
  ```go
  // XrayReportRow is one row of an Xray vulnerabilities report.
  type XrayReportRow struct {
      IssueID             string   `json:"issue_id"`
      VulnerableComponent string   `json:"vulnerable_component"`
      ImpactedArtifact    string   `json:"impacted_artifact"`
      Path                string   `json:"path"`
      FixedVersions       []string `json:"fixed_versions"`
  }

  // VulnDetails holds the rows of an Xray vulnerabilities report.
  // Available is false when the report could not be produced.
  type VulnDetails struct {
      Rows      []XrayReportRow
      Available bool
  }
  ```
  `VulnRow` gets `VulnerableVersions []string`, `FixedVersions []string` (an issue can hit several components in one artifact, so both are lists, deduplicated, in report order) and `VersionsMatched bool` (a report entry was found for this row). License and operational-risk issues have no report entries, so they stay unmatched.
- **Service**: `GetVulnerabilityDetails(repo string) (models.VulnDetails, error)`.
- **`report.AttachVersions(repo string, rows []models.VulnRow, details models.VulnDetails) []models.VulnRow`**: returns a copy; builds `map[key][]XrayReportRow` with `key = artifactPath + "\x00" + issueID`, where both sides have leading/trailing `/` trimmed (`VulnRow.ArtifactPath` can start with `/`). For each matched row it sets `VersionsMatched`, appends `componentLabel(vulnerable_component)` and the fixed versions. `componentLabel` strips only the `xxx://` scheme, plus a numeric RPM epoch if Task 1 shows one; other formats (deb distro segments, docker, `gav://`) fall back to the value without the scheme. If details are available and rows exist but none matched, it logs `slog.Warn` so a broken join is visible. Unavailable details → rows returned unchanged.
- **Template** (`vuln_report.html`): columns become Severity · ID · Package · **Vulnerable version** · **Fixed version** · Summary · Source. Values are `<code>` lists; empty → `—`. `title="No fix available"` only when `.VersionsMatched` and the fixed list is empty; an unmatched row is a plain `—`. New template field `.VersionsAvailable`; when `.Available && !.VersionsAvailable`, a muted `o-alert` notice: "Vulnerable and fixed versions could not be retrieved from Xray." (not shown on top of the existing "Xray unavailable" notice).
- **CSV**: header becomes `Component, Package, Artifact, Severity, Issue ID, CVEs, Vulnerable Version, Fixed Version, Summary, Impact Path`; lists joined with `"; "`; every cell still goes through `guardCell`. `Summary` and `Impact Path` move two positions — documented in `docs/api/README.md` for anyone parsing by position.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): code, tests, docs in this repo
- **Post-Completion** (no checkboxes): permission checks and manual verification against the real Xray

## Implementation Steps

### Task 1: Spike — confirm the Reports API response on this instance

**Files:**
- Modify: `docs/plans/20261026-vuln-fixed-versions.md` (record findings)

- [ ] with the `.env` credentials, create a vulnerabilities report for one repo with `curl`, poll its status, fetch the first page of rows, then delete it
- [ ] confirm `fixed_versions` and `vulnerable_component` are populated (not null) for at least one known finding
- [ ] record the exact join-key rule: format of `path` (folder or file, trailing slash?) and `impacted_artifact`, and where the artifact file name comes from, so the key matches `"<artifact path>"` as `VulnRow` has it; record the `vulnerable_component` formats seen per package type
- [ ] record pagination: is `page_num` 1-based, max `num_of_rows`, does the content POST need a JSON body / `Content-Type`
- [ ] record status values (does `aborted` exist?) and characters Xray rejects in report names
- [ ] time report generation on the largest repo and record it
- [ ] update Technical Details with all of the above (remove the ⚠️ notes)
- [ ] save one trimmed real response (no secrets, no internal hostnames) as the fixture shape for Task 3 and Task 4 tests
- [ ] ⚠️ stop and revisit the approach with the user before Task 2 if: the token lacks report permission, the fields are empty, no reliable join key exists, or generation routinely exceeds `TIMEOUT` and can't be narrowed with `include_path_patterns`
- [ ] no code change in this task, so no tests; `go test ./...` still passes

### Task 2: Add models and the Service method

**Files:**
- Modify: `internal/models/models.go` (or the file holding `XrayIssue` / `VulnRow`)
- Modify: `internal/jfrog/service.go`
- Modify: `main_test.go`
- Modify: `internal/handlers/artifacts_test.go`

- [ ] add `XrayReportRow`, `VulnDetails`, and `VulnerableVersions` / `FixedVersions` on `VulnRow`
- [ ] add `GetVulnerabilityDetails(repo string) (models.VulnDetails, error)` to `jfrog.Service`
- [ ] add the method to both `mockService` implementations: fields `details models.VulnDetails`, `detailsErr error`; the handler mock also records `detailsRepo` (a separate field from the summary call's, so the parallel calls in Task 5 don't race)
- [ ] add a stub on `Client` returning `VulnDetails{Available: false}` so the build passes (replaced in Task 3)
- [ ] no new behavior to test; the existing `var _ Service = Client{}` check and the mocks cover compilation
- [ ] run tests - must pass before task 3

### Task 3: Implement `Client.GetVulnerabilityDetails`

**Files:**
- Modify: `internal/jfrog/client.go` (`reportTimeout` field, set in `NewClient`)
- Create: `internal/jfrog/xray_report.go`
- Create: `internal/jfrog/xray_report_test.go`

- [ ] add `reportTimeout` to `Client`; set it from `cfg.Timeout` in `NewClient`
- [ ] implement create → poll → paged fetch with one job-wide `context.WithTimeout(c.reportTimeout)` and `http.NewRequestWithContext`; report name as a plain JSON string, id via `strconv.Itoa`
- [ ] deferred DELETE with a fresh 5s context; `xrayReportPollInterval` and `xrayReportMaxPages` as package vars
- [ ] status handling: `completed` → fetch, `failed`/`aborted` → unavailable, unknown → keep polling
- [ ] degrade rules: network error / 403 / 404 / non-2xx on any step, failure status, or deadline → `VulnDetails{Available: false}`, nil error, `slog.Warn`; malformed 2xx JSON → wrapped error
- [ ] replace the Task 2 stub
- [ ] write tests (httptest.Server) for success: pending → running → completed, two result pages (1-based), rows decoded, DELETE called once
- [ ] write tests for: create 403 (no DELETE); status `failed` (DELETE called); unknown status → polls until deadline → unavailable, DELETE still arrives **after** the deadline; fetch page 500; `total_rows` larger than rows returned (stops on empty page); same page returned forever (stops at max pages); malformed create/status/page body → error
- [ ] run tests - must pass before task 4

### Task 4: Join versions onto report rows

**Files:**
- Modify: `internal/report/report.go` (or create `internal/report/versions.go`)
- Modify: `internal/report/report_test.go` (or create `internal/report/versions_test.go`)

- [ ] implement `AttachVersions(repo, rows, details)` and `componentLabel` as described in Technical Details, using the join-key rule recorded in Task 1
- [ ] set `VersionsMatched`; deduplicate versions per row, keep report order; never drop or add rows; `slog.Warn` when details are available, rows exist and none matched
- [ ] write table-driven tests for `componentLabel` with the formats recorded in Task 1 (at least `rpm://7:openssl:1.0.2k`, `npm://lodash:4.17.20`, a deb and a docker id, no scheme, empty)
- [ ] write tests for `AttachVersions`: match by path + issue id, folder-style `path` with trailing slash, `ArtifactPath` with leading `/`, several components for one issue, same issue id on two artifacts kept apart, matched with empty fixed list (`VersionsMatched` true), no match → empty fields and `VersionsMatched` false, `Available: false` → rows unchanged, input slice not mutated
- [ ] run tests - must pass before task 5

### Task 5: Fetch details in the handler, in parallel with the summary

**Files:**
- Modify: `internal/handlers/vulnerabilities.go`
- Modify: `internal/handlers/vulnerabilities_test.go`

- [ ] in `buildVulnerabilityReport`, start `GetVulnerabilityDetails(q.repo)` in a goroutine before `GetXraySummaries`, sending its result (details + error) on a **buffered** `make(chan detailsResult, 1)`; after the summary, read the channel and call `report.AttachVersions` on `rep.Rows` before filter/sort
- [ ] on early return (summary error or unavailable) the result is not read and dropped; the buffered channel lets the goroutine finish and its DELETE still runs — no goroutine leak
- [ ] an error from `GetVulnerabilityDetails` is logged and treated as unavailable — the page still renders
- [ ] expose `VersionsAvailable` in the result and template data; the export uses the same rows
- [ ] write handler tests: details available → rows carry versions; details unavailable / error → 200, rows without versions, `VersionsAvailable` false; summary unavailable → still 200 with the existing notice; repo passed to the details call
- [ ] run tests with the race detector: `go test -race ./internal/handlers` - must pass before task 6

### Task 6: Show the columns in the report page

**Files:**
- Modify: `templates/partials/vuln_report.html`
- Modify: `templates/layout.html` (column width CSS, if needed)
- Modify: `internal/handlers/vulnerabilities_test.go`

- [ ] add the two `<th>` / `<td>` columns between Package and Summary, values as `<code>` lists, `—` when empty, `title="No fix available"` only when `.VersionsMatched` and the fixed list is empty
- [ ] add the muted notice when `.Available` and not `.VersionsAvailable`
- [ ] add `col-ver` / `col-fix` widths in `layout.html` so the table still fits; check the narrow-screen layout
- [ ] write rendering tests: headers present, versions rendered and HTML-escaped, `—` placeholder, "No fix available" title on matched-unfixed rows only, notice shown when versions are unavailable and absent when the summary itself is unavailable
- [ ] run tests - must pass before task 7

### Task 7: Add the columns to the CSV export

**Files:**
- Modify: `internal/report/csv.go`
- Modify: `internal/report/csv_test.go`
- Modify: `internal/handlers/vulnerabilities_test.go` (export assertions, if any check the header)

- [ ] extend `csvHeader` and the record with `Vulnerable Version` and `Fixed Version` after `CVEs`, joined with `"; "`, through `guardCell`
- [ ] update existing CSV tests for the new header/column positions
- [ ] write tests for multi-value cells, empty cells, and the formula guard on a version value starting with `=` or `-`
- [ ] run tests - must pass before task 8

### Task 8: Verify acceptance criteria
- [ ] verify both columns show on the page and in the CSV for scanned findings
- [ ] verify the page still renders (200) with `—` and the notice when the reports API is denied, fails or times out
- [ ] verify not-scanned packages, severity filter, sort and clean list are unchanged
- [ ] run full test suite: `go test ./...`
- [ ] run `golangci-lint run`
- [ ] check coverage of the new code: `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`

### Task 9: [Final] Update documentation
- [ ] update `docs/api/README.md`: new columns on the page, new CSV header (note that `Summary` and `Impact Path` moved two positions), the extra Xray report calls and the permission they need
- [ ] update `CLAUDE.md`: the Reports API as the source of vulnerable/fixed versions, its degrade rule, the parallel fetch, and the new `Service` method in the mock-update note
- [ ] update `README.md` if it lists the report columns or required token permissions
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention or external systems - no checkboxes, informational only*

**Manual verification**:
- open the report for a real repo and compare a few rows against the Xray UI (vulnerable component and fixed version)
- time a page load on the largest repo; if report generation regularly approaches `TIMEOUT`, consider raising `TIMEOUT` or a short-lived in-memory cache as a follow-up (not in scope here — every sort click re-runs the report)
- check the Xray "Reports" list after a few loads to confirm no leftover `jfrog-manager-*` reports pile up

**External system updates**:
- the JFrog token used by the server needs permission to create and read Xray reports for the repositories it serves; without it the columns stay `—`
