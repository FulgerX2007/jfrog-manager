# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A single-binary Go web admin tool for JFrog Artifactory: browse repos, list/upload/download/delete artifacts, view Xray vulnerabilities. Gin serves server-rendered HTML; the browser uses htmx partial swaps. There is no database and no application-level auth — the server holds one set of JFrog credentials and proxies every call.

## Commands

```bash
go run main.go                              # run (needs .env or env vars; must be run from the repo root)
go build -o jfrog_manager .                 # build (binary name is gitignored)
go test ./...                               # all tests
go test ./internal/handlers -run TestUploadArtifact_Success -v   # single test
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
golangci-lint run                           # no repo config; default linters
```

Config is env-only (`internal/config`), loaded via godotenv from `.env` in the working directory. `JFROG_URL`, `JFROG_USERNAME`, `JFROG_TOKEN` are required; `PORT` (8080), `TIMEOUT` (30s), `DEFAULT_REPO`, `GIN_MODE` (defaults to release) are optional. See `.env.example`.

Releases: pushing a `v*` tag runs GoReleaser (`.github/workflows/release.yml`, `.goreleaser.yaml`). The changelog is grouped by conventional-commit prefix (`feat:`/`fix:`; `docs:`/`test:`/`chore:`/`ci:` are excluded), so commit prefixes matter.

The default branch is `main`; there is no `develop` branch in this repo.

## Architecture

Request flow: `main.go` (routes + `securityHeaders` middleware) → `internal/handlers` → `jfrog.Service` interface → `jfrog.Client` → Artifactory/Xray REST APIs. Responses are HTML fragments rendered from `templates/`.

- **`jfrog.Service` (`internal/jfrog/service.go`) is the seam.** Handlers depend only on this interface. Adding a JFrog operation means: add to the interface, implement on `Client`, and update **both** mock implementations — `mockService` in `main_test.go` and in `internal/handlers/artifacts_test.go` — or the build breaks.
- **Routes are registered in three places**: `setupRouter` in `main.go`, plus the test routers in `internal/handlers/artifacts_test.go` and `xray_test.go`. Handler tests build their own `gin.New()` router and do not go through `setupRouter`; `main_test.go` covers the real wiring.
- **Two HTTP clients in `jfrog.Client`**: `Do` uses the `TIMEOUT`-bounded client for short API calls; `doStream` uses a client with no timeout for upload/download. Pick deliberately. `DownloadArtifact` returns an open body the caller must close.
- **URL building**: repo goes through `url.PathEscape`; artifact paths go through `escapePathSegments`, which escapes per segment and preserves `/`. Never escape a whole artifact path in one call.
- **Xray degrades instead of failing**: `GetXraySummary` returns `XraySummary{Available: false}` with a nil error when Xray is unreachable, returns 404, or any non-2xx. Only a malformed 2xx body is an error. The `xray_panel` template branches on `.Available`.
- **`ListArtifacts` filters** folders and RPM repo metadata (`repomd.xml`, `*.xml.gz`) out of the deep storage listing.

### Templates and the htmx contract

- Templates are **not embedded**. `templates.Load("templates")` globs `templates/*.html` and `templates/partials/*.html` relative to the working directory at startup. Tests reach them via relative paths (`"templates"`, `"../../templates"`) or `runtime.Caller`. The GoReleaser archive ships `templates/` next to the binary (`.goreleaser.yaml`, `archives.files`), so a released binary must be started from its unpacked directory.
- Every file defines a named template (`layout`, `artifact_list`, `error`, `xray_panel`, `upload_form`); handlers call `ExecuteTemplate` by name. A new partial placed outside those two globs will not be loaded.
- All CSS and client-side JS (`bulkDelete`, `toggleXray`, `uploadWithProgress`, …) live inline in `templates/layout.html`; Bootstrap, htmx and fonts come from CDNs. Upload and bulk-delete are driven by that JS (XHR for progress, JSON body for bulk-delete), not by htmx attributes.
- **Errors**: handlers call `h.renderError`, which returns **422** with the `error` fragment and sets `HX-Retarget: #error-container` / `HX-Reswap: innerHTML` so the message never lands inside the element that triggered the request. Oversized uploads return 413 with the same headers. Keep new handlers on this path.
- **DOM ids** derived from artifact paths must go through the `cssID` template func (injective escaping of `/ . space -`); each artifact is a `<tbody id="artifact-group-…">` containing its row plus an `xray-panel-…` row, and single delete swaps out the whole `<tbody>`.
- Custom template funcs are in `internal/templates/templates.go` (`humanSize`, `cssID`, `urlEncode`, `toLower`, `countBySeverity`, `sortBySeverity`); new ones must be added to `FuncMap()` before templates parse.

### Input limits and validation

These came out of a security audit — preserve them when touching handlers:

- Every handler rejects `repo`/`path`/`folder` values containing `..`.
- Upload: `http.MaxBytesReader` at 500 MB, and `ParseMultipartForm` is called explicitly *before* `PostForm` so the size error is detectable; `r.MaxMultipartMemory` is 32 MB (larger uploads spill to disk). The uploaded filename is reduced with `path.Base`.
- Bulk delete: body capped at 1 MiB, max 500 paths.
- Download: filename via `path.Base` and `mime.FormatMediaType` for `Content-Disposition`.
- Upstream error bodies are logged with `slog` but never sent to the browser; users get a generic message.

## Conventions

- Structs are passed and returned by value and methods use value receivers (`Handler`, `Client`, `Config`). The exceptions are types the libraries hand out as pointers (`*gin.Context`, `*template.Template`, `*http.Request`) and the pointer-receiver `mockService` in handler tests, which records calls.
- Logging is `log/slog` only. Wrap errors with `fmt.Errorf("doing thing: %w", err)`.
- Tests use only the standard library (`httptest`, no assertion framework); `internal/jfrog` tests run the real `Client` against an `httptest.Server`.
