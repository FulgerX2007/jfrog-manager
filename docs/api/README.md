# API Documentation

Reference for this project's external interface(s).

> Keep in sync with the code: when an endpoint, message, or contract changes, update
> this folder in the same change (see `.ai/rules.md`).

JFrog Manager exposes a single interface: an HTTP server built with Gin whose routes
are registered in `setupRouter` in `main.go`. It is a server-rendered web UI, not a
JSON API: handlers in `internal/handlers/` respond with HTML (a full page or htmx
fragments rendered from `templates/`), plus one binary download stream. There is no
CLI and no gRPC interface in the repository.

## Contract source of truth

- **Style:** HTTP, server-rendered HTML with htmx partial swaps (`README.md`; `main.go` `setupRouter`). Responses are `text/html; charset=utf-8` fragments, not JSON (`internal/handlers/artifacts.go`, `internal/handlers/xray.go`). The only JSON on the wire is the request body of `POST /artifacts/bulk-delete` (`internal/handlers/artifacts.go` `BulkDeleteArtifacts`).
- **Spec file:** None. The repository contains no OpenAPI/Swagger or proto file. The route table in `main.go` (`setupRouter`) is the source of truth; `README.md` ("Routes") carries a human-readable copy.
- **Generated docs / UI:** None. No Swagger UI or generated reference is served; the registered routes in `main.go` are the ten listed below.

## Conventions

- **Versioning:** Routes carry no version prefix (`main.go` `setupRouter`).
  TODO: decide and document a versioning / compatibility policy for these routes (none is stated in the repo).
- **Authentication:** None at the application level. `setupRouter` in `main.go` registers only the `securityHeaders` middleware, and `README.md` ("Security notes") states that anyone who can reach the port has full access to the configured JFrog credentials and advises deploying behind a VPN, an SSO reverse proxy, or on `127.0.0.1` with an authenticating proxy. Upstream calls to Artifactory / Xray use HTTP Basic auth with `JFROG_USERNAME` and `JFROG_TOKEN` (`internal/jfrog/client.go`, `internal/config/config.go`); callers of this service never supply credentials.
  TODO: document the authenticating proxy / network boundary actually used in deployment (not in the repo).
- **Error format:** `renderError` in `internal/handlers/artifacts.go` responds with HTTP `422 Unprocessable Entity`, `Content-Type: text/html; charset=utf-8`, the `error` template fragment (`templates/partials/error.html`), and the headers `HX-Retarget: #error-container` and `HX-Reswap: innerHTML` so htmx swaps the message into the error container. It is used for both validation failures and upstream failures. Exceptions, all in `internal/handlers/artifacts.go`:
  - An upload over the 500 MB cap returns `413 Request Entity Too Large` with the same fragment and headers (`UploadArtifact`).
  - If the `error` template itself fails to render, the handler attempts a plain-text `500` fallback (`renderError`, `UploadArtifact`); the 422/413 status is set before rendering, so the status the client actually sees in that case is not covered by a test.
  - Upstream error details are logged with `slog` and replaced by a generic message in the response (for example "Failed to load artifacts").
- **Pagination / filtering:** None. No handler reads paging, sorting or filter parameters; `GET /artifacts` renders every artifact the service returns for the repository (`internal/handlers/artifacts.go` `ListArtifacts`). The only server-side filtering is fixed: folders and `repomd.xml` / `*.xml.gz` files are skipped by the client (`internal/jfrog/client.go`).
- **Input validation:** Handlers reject a `repo`, `path` or `folder` value containing `..` with the 422 error fragment (`internal/handlers/artifacts.go`, `internal/handlers/xray.go`).
- **Response headers:** Every response carries `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY` and `Referrer-Policy: no-referrer` (`main.go` `securityHeaders`).

## Endpoints / operations

One subsection per route registered in `setupRouter` (`main.go`).

All routes are registered in `main.go` (`setupRouter`). "None" in the Auth column means
the application performs no authentication (see Conventions).

| Operation | Purpose | Auth |
|-----------|---------|------|
| `GET /` | Main page (full `layout` template) | None |
| `GET /repos` | htmx fragment: `<option>` list of repositories | None |
| `GET /artifacts` | htmx fragment: artifact table for a repository | None |
| `POST /artifacts/upload` | Upload one file, then re-render the artifact table | None |
| `POST /artifacts/bulk-delete` | Delete several artifacts, then re-render the artifact table | None |
| `GET /artifacts/download` | Stream one artifact as a file download | None |
| `DELETE /artifacts` | Delete one artifact | None |
| `GET /xray` | htmx fragment: Xray vulnerability panel for an artifact | None |

Unless stated otherwise, failures use the 422 error fragment described under
Conventions.

### `GET /`

- **Handler:** `Handler.Index` (`internal/handlers/artifacts.go`).
- **Request:** no parameters.
- **Response:** `200`, the `layout` template rendered with `DefaultRepo` taken from the `DEFAULT_REPO` setting (`main.go`, `internal/config/config.go`).

### `GET /repos`

- **Handler:** `Handler.ListRepos` (`internal/handlers/artifacts.go`).
- **Request:** optional query parameter `default`; the repository whose key matches it is marked `selected`.
- **Response:** `200`, one `<option value="{key}">{key} ({packageType})</option>` element per repository, values HTML-escaped.
- **Errors:** 422 "Failed to load repositories" when the upstream call fails.

### `GET /artifacts`

- **Handler:** `Handler.ListArtifacts` (`internal/handlers/artifacts.go`).
- **Request:** query parameters `repo` (required), `view` (`latest`, the default, shows the most recently modified version of each package; `all` shows every file) and `component` (optional; narrows the list to one component). Unknown `view` values fall back to `latest` (`report.NormalizeView` in `internal/report/artifacts.go`).
- **Response:** `200`, the `artifact_list` fragment (`templates/partials/artifact_list.html`): the component filter with counts and one row per artifact with package, version, component, modification time and size. The Xray button is rendered for latest versions only.
- **Errors:** 422 when `repo` is missing ("Repository parameter is required"), contains `..` ("Invalid repository"), or the upstream call fails ("Failed to load artifacts").

### `POST /artifacts/upload`

- **Handler:** `Handler.UploadArtifact` (`internal/handlers/artifacts.go`).
- **Request:** `multipart/form-data` with fields `repo` (required), `folder` (optional; leading/trailing `/` and spaces are trimmed) and `file` (required). The artifact is stored at `{folder}/{filename}`, or `{filename}` when no folder is given; only the base name of the uploaded file name is used.
- **Limits:** request body capped at 500 MB via `http.MaxBytesReader`; multipart parts above 32 MB spill to disk (`r.MaxMultipartMemory` in `main.go`).
- **Response:** `200`, the re-rendered `artifact_list` fragment for `repo`.
- **Errors:** `413` "File exceeds the 500 MB upload limit" when the cap is exceeded; 422 for an unparsable form, missing `repo`, `..` in `repo` or `folder`, missing `file`, an invalid file name, an upstream upload failure ("Upload failed"), or a failed list refresh after a successful upload ("Upload succeeded but failed to refresh list").

### `POST /artifacts/bulk-delete`

- **Handler:** `Handler.BulkDeleteArtifacts` (`internal/handlers/artifacts.go`).
- **Request:** JSON body `{"repo": "<repo>", "paths": ["<path>", ...]}`.
- **Limits:** body capped at 1 MiB; at most 500 paths per request. An oversized body fails JSON decoding and returns 422 "Invalid request body", not 413.
- **Behaviour:** paths are deleted one at a time and the loop continues past failures, so a request that returns an error may still have deleted some of the paths.
- **Response:** `200`, the re-rendered `artifact_list` fragment for `repo`, only when every path was deleted.
- **Errors:** 422 for an invalid body, an empty `repo` or `paths`, more than 500 paths, `..` in `repo`, any failed or invalid path ("Failed to delete: " followed by the affected paths), or a failed list refresh ("Delete succeeded but failed to refresh list").

### `GET /artifacts/download`

- **Handler:** `Handler.DownloadArtifact` (`internal/handlers/artifacts.go`).
- **Request:** query parameters `repo` and `path` (both required).
- **Response:** `200`, the artifact bytes streamed from Artifactory. `Content-Type` is the upstream value, or `application/octet-stream` when upstream gives none; `Content-Disposition: attachment` carries the base name of `path` as the file name (`download` if it has none); `Content-Length` is set when the upstream length is known.
- **Errors:** 422 when a parameter is missing, contains `..`, or the upstream call fails ("Download failed"). A failure after streaming has started is only logged.

### `DELETE /artifacts`

- **Handler:** `Handler.DeleteArtifact` (`internal/handlers/artifacts.go`).
- **Request:** query parameters `repo` and `path` (both required).
- **Response:** `200` with an empty body; the htmx caller removes the table row.
- **Errors:** 422 when a parameter is missing, contains `..`, or the upstream call fails ("Delete failed").

### `GET /xray`

- **Handler:** `Handler.GetXray` (`internal/handlers/xray.go`).
- **Request:** query parameters `repo` and `path` (both required).
- **Response:** `200`, the `xray_panel` fragment (`templates/partials/xray_panel.html`). When Xray is unreachable, returns 404, or returns any non-2xx status, the service reports the summary as unavailable rather than failing, so the panel still renders with `200` (`internal/jfrog/xray.go`).
- **Errors:** 422 when a parameter is missing, contains `..`, or the Xray response body is malformed ("Failed to load Xray data").

### `GET /vulnerabilities`

- **Handler:** `Handler.GetVulnerabilities` (`internal/handlers/vulnerabilities.go`).
- **Purpose:** every Xray vulnerability in the latest version of each package of a repository.
- **Request:** query parameters `repo` (required), `severity` (optional: `critical`, `high`, `medium`, `low` or `unknown`; narrows the findings to that severity, anything else means no filter), `sort` (`severity` or `component`, default `severity`) and `dir` (`asc` or `desc`; default `desc` for severity, meaning most severe first, and `asc` for component). Unknown `sort` / `dir` values fall back to the defaults (`report.NormalizeSort` in `internal/report/report.go`).
- **Behaviour:** lists the repository, keeps the most recently modified artifact per folder, package name and variant (`report.LatestArtifacts`), and asks Xray about those in batches of 100 paths (`GetXraySummaries` in `internal/jfrog/xray.go`). Nothing is cached; each request repeats these calls.
- **Response:** `200`, the `vuln_report` fragment (`templates/partials/vuln_report.html`): totals, the packages Xray returned no data for ("not scanned"), the packages scanned and clean, and the findings grouped by component (`report.GroupRows`), each with severity, CVE ids (or the Xray issue id), package, summary and impact paths. `sort` orders the groups; inside a group findings run from most to least severe. Only the latest version of each package is reported. When Xray is unreachable or any batch returns a non-2xx status, the fragment still renders with `200` and a notice that Xray data could not be retrieved.
- **Errors:** 422 when `repo` is missing or contains `..`, when listing the repository fails, or when an Xray response body is malformed ("Failed to load vulnerabilities").

### `GET /vulnerabilities/export`

- **Handler:** `Handler.ExportVulnerabilities` (`internal/handlers/vulnerabilities.go`).
- **Request:** the same query parameters as `GET /vulnerabilities`, including `severity`. The file is a flat list ordered by `sort` and `dir`.
- **Response:** `200`, `Content-Type: text/csv; charset=utf-8`, `Content-Disposition: attachment` with file name `vulnerabilities-<repo>-<yyyymmdd>.csv`. Columns: `Component, Package, Artifact, Severity, Issue ID, CVEs, Summary, Impact Path`. Cells starting with `=`, `+`, `-`, `@`, tab or carriage return are prefixed with `'` (`internal/report/csv.go`).
- **Errors:** 422 with the error fragment for the same cases as `GET /vulnerabilities`, and also when Xray data could not be retrieved.

## Upstream APIs consumed

Not part of this service's own contract, but changes here alter what the routes above
can do. All calls are made by `jfrog.Client` (`internal/jfrog/client.go`,
`internal/jfrog/artifacts.go`, `internal/jfrog/xray.go`) against `JFROG_URL`:

| Upstream call | Used by |
|---------------|---------|
| `GET {base}/artifactory/api/repositories` | `GET /repos` |
| `GET {base}/artifactory/api/storage/{repo}/?list&deep=1` | `GET /artifacts`, and the list refresh after upload / bulk delete |
| `PUT {base}/artifactory/{repo}/{path}` | `POST /artifacts/upload` |
| `GET {base}/artifactory/{repo}/{path}` | `GET /artifacts/download` |
| `DELETE {base}/artifactory/{repo}/{path}` | `DELETE /artifacts`, `POST /artifacts/bulk-delete` |
| `POST {base}/xray/api/v1/summary/artifact` | `GET /xray` |

TODO: record the Artifactory / Xray versions this service is expected to work against (not stated in the repo).
