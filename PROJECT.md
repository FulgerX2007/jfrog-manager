# PROJECT.md — JFrog Manager

The governing specification for this repository. It is the **single source of truth**
for how this project is built, what standards apply, and how AI agents must behave
here. Read this before changing code. Keep it current (see CONTRIBUTING.md).

> Audience: maintainers and AI coding agents.
> Rule: only assert what the repo can prove. Unknowns stay as `TODO:`.

---

## 1. Business context

- **What this project does:** A web-based admin tool for JFrog Artifactory: browse repositories, list / upload / download / delete artifacts (individually or in bulk), and view Xray vulnerability information (`README.md`). It is a single Go binary that serves server-rendered HTML and proxies every call to Artifactory / Xray using one set of configured JFrog credentials (`CLAUDE.md`, `internal/jfrog/client.go`).
- **Who uses it / who depends on it:** TODO: no user, team, or downstream-consumer information exists in the repository.
- **Why it exists (the problem it solves):** TODO: the repository states what the tool does (`README.md`) but not the business motivation behind it.
- **Owners / points of contact:** TODO: no CODEOWNERS, MAINTAINERS, or contact information exists in the repository.
- **Status / lifecycle:** TODO: (active / maintenance / deprecated) — not stated in any file.
- **License:** TODO: no `LICENSE` file exists in the repository.

## 2. Technology stack

- **Language(s) & version:** Go 1.26.1 (`go.mod`, module `jfrog_manager`). HTML templates with inline CSS and JavaScript (`templates/layout.html`).
- **Frameworks / key libraries:**
  - `github.com/gin-gonic/gin` v1.12.0 — HTTP router and middleware (`go.mod`, `main.go`).
  - `github.com/joho/godotenv` v1.5.1 — loads `.env` (`go.mod`, `internal/config/config.go`).
  - Go standard library `html/template`, `net/http`, `log/slog` (`internal/templates/templates.go`, `internal/jfrog/client.go`, `main.go`).
  - Browser side: htmx and Bootstrap 5, plus fonts, loaded from CDNs; all project CSS and client JS is inline in `templates/layout.html` (`README.md`, `CLAUDE.md`).
- **Datastores:** None. The application has no database (`CLAUDE.md`); all state lives in the upstream Artifactory instance.
- **Messaging / queues:** None. No messaging dependency appears in `go.mod`.
- **External services:** JFrog Artifactory REST API and, optionally, JFrog Xray (`README.md`, `internal/jfrog/artifacts.go`, `internal/jfrog/xray.go`).
- **Build / package tooling:** Go modules (`go.mod`, `go.sum`); `go build -o jfrog_manager .` for local builds (`CLAUDE.md`); GoReleaser v2 for release builds (`.goreleaser.yaml`). There is no Makefile and no Dockerfile in the repository.
- **Runtime / deploy target:** A standalone binary listening on `PORT` (default `8080`) (`internal/config/config.go`, `main.go`). Release builds target linux / darwin / windows on amd64 / arm64 with `CGO_ENABLED=0` (`.goreleaser.yaml`). The binary loads templates from the relative directory `templates` at startup, so it must be started from a directory containing `templates/` (`main.go`, `internal/templates/templates.go`).
  - TODO: the actual hosting environment (host, container platform, reverse proxy) is not described in any file.

## 3. Coding standards

- **Style / formatter:** TODO: no formatter configuration or formatting instruction exists in the repository (`gofmt` is the Go default but no file mandates it).
- **Linter & config:** `golangci-lint run` with default linters; there is no `.golangci.yml` in the repository (`CLAUDE.md`).
- **Naming conventions:**
  - Commit messages use conventional-commit prefixes (`feat:`, `fix:`, `docs:`, `ci:`); the release changelog is grouped by these prefixes, so they matter (`CLAUDE.md`, `.goreleaser.yaml`).
  - Every template file defines a named template (`layout`, `artifact_list`, `error`, `xray_panel`, `upload_form`) and handlers render by name (`CLAUDE.md`).
  - TODO: no other naming rules (packages, files, identifiers) are written down.
- **Project-specific patterns to follow:**
  - Handlers depend only on the `jfrog.Service` interface (`internal/jfrog/service.go`). Adding a JFrog operation means adding it to the interface, implementing it on `Client`, and updating both mock implementations in `main_test.go` and `internal/handlers/artifacts_test.go` (`CLAUDE.md`).
  - Routes are registered in `setupRouter` in `main.go` and again in the test routers in `internal/handlers/artifacts_test.go` and `internal/handlers/xray_test.go` (`CLAUDE.md`).
  - Structs are passed and returned by value with value receivers (`Handler`, `Client`, `Config`); pointers appear only where libraries hand them out (`*gin.Context`, `*template.Template`, `*http.Request`) and on the call-recording `mockService` in handler tests (`CLAUDE.md`).
  - Logging uses `log/slog` only; errors are wrapped with `fmt.Errorf("...: %w", err)` (`CLAUDE.md`, `internal/config/config.go`).
  - Choose the HTTP client deliberately: `Do` uses the `TIMEOUT`-bounded client for short calls, `doStream` uses a client without a timeout for upload / download (`CLAUDE.md`, `internal/jfrog/client.go`).
  - Build upstream URLs with `url.PathEscape` for the repo and `escapePathSegments` for artifact paths (`CLAUDE.md`, `internal/jfrog/client.go`).
  - Handler errors go through `renderError` (`internal/handlers/artifacts.go`); see section 5.
  - DOM ids derived from artifact paths go through the `cssID` template func; new template funcs must be added to `FuncMap()` in `internal/templates/templates.go` (`CLAUDE.md`).
- **Patterns to avoid:**
  - Escaping a whole artifact path in a single call (`CLAUDE.md`).
  - Returning upstream error bodies to the browser; they are logged with `slog` and the user gets a generic message (`CLAUDE.md`).
  - Placing a new partial outside `templates/*.html` or `templates/partials/*.html`; it will not be loaded (`CLAUDE.md`, `internal/templates/templates.go`).
  - Removing or weakening the input limits listed in section 4 (`CLAUDE.md`).

## 4. Security requirements

There is no `SECURITY.md` in the repository. The statements below come from `README.md` ("Security notes"), `CLAUDE.md`, and the source.

- **Secrets handling:** Configuration is environment-only. `internal/config/config.go` loads `.env` from the working directory via godotenv and falls back to process environment variables. `JFROG_URL`, `JFROG_USERNAME`, and `JFROG_TOKEN` are required. `.env` is gitignored (`.gitignore`); `.env.example` holds placeholders only. `README.md` instructs `chmod 600 .env`.
  - TODO: no secret-rotation or secret-store policy is documented.
- **AuthN / AuthZ model:** The application implements no authentication or authorization. Anyone who can reach `PORT` has full access to the configured JFrog credentials (`README.md`). `README.md` says to deploy behind a VPN, an SSO reverse proxy, or on `127.0.0.1` with an authenticating proxy in front. Upstream calls use HTTP Basic auth with the configured username and token (`internal/jfrog/client.go`).
- **Input validation expectations** (preserve these when touching handlers — `CLAUDE.md`):
  - Handlers reject `repo`, `path`, and `folder` values containing `..` (`internal/handlers/artifacts.go`, `internal/handlers/xray.go`).
  - Upload bodies are capped at 500 MB with `http.MaxBytesReader` (`internal/handlers/artifacts.go`); `MaxMultipartMemory` is 32 MB, so larger uploads spill to disk (`main.go`).
  - Bulk-delete bodies are capped at 1 MiB and 500 paths (`internal/handlers/artifacts.go`).
  - Uploaded and downloaded filenames are reduced with `path.Base`; the download `Content-Disposition` is built with `mime.FormatMediaType` (`CLAUDE.md`).
  - Every response carries `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, and `Referrer-Policy: no-referrer` (`main.go`, `securityHeaders`).
- **Dependencies / supply chain policy:** Dependencies are pinned in `go.mod` / `go.sum`. Browser assets (Bootstrap, htmx, fonts) are loaded from third-party CDNs (`CLAUDE.md`).
  - TODO: no dependency-update, vulnerability-scanning, or CDN-integrity policy is documented.
- **Data classification & handling:** TODO: no data-classification policy exists in the repository.
- **Vulnerability reporting:** TODO: no reporting process or contact exists in the repository.

## 5. API conventions

- **Style:** Server-rendered HTML over HTTP. Routes return HTML fragments for htmx partial swaps, not JSON (`README.md`, `CLAUDE.md`). Routes are registered in `setupRouter` in `main.go`:

  | Method | Route | Purpose |
  |--------|-------|---------|
  | GET | `/` | Main page |
  | GET | `/repos` | Fragment: repository `<option>` list |
  | GET | `/artifacts?repo=X` | Fragment: artifact table |
  | POST | `/artifacts/upload` | Multipart upload (fields `repo`, `folder`, `file`) |
  | POST | `/artifacts/bulk-delete` | JSON body `{repo, paths[]}`; re-renders the list |
  | GET | `/artifacts/download?repo=X&path=Y` | Streams the artifact as an attachment |
  | DELETE | `/artifacts?repo=X&path=Y` | Deletes one artifact |
  | GET | `/xray?repo=X&path=Y` | Fragment: Xray vulnerability panel |

  (Descriptions from `README.md` "Routes".)
- **Versioning:** None. Routes carry no version prefix (`main.go`).
- **Error format:** `renderError` responds with HTTP 422, renders the `error` template fragment, and sets `HX-Retarget: #error-container` and `HX-Reswap: innerHTML`. Oversized uploads respond with HTTP 413 and the same headers (`internal/handlers/artifacts.go`). New handlers must stay on this path (`CLAUDE.md`).
- **Auth scheme:** None on inbound requests (see section 4). Outbound requests to JFrog use HTTP Basic auth (`internal/jfrog/client.go`).
- **Source of truth for the contract:** `setupRouter` in `main.go` and the handlers in `internal/handlers/`. No OpenAPI or other machine-readable spec exists in the repository.
- **Upstream behaviour worth knowing:** `GetXraySummary` returns `XraySummary{Available: false}` with a nil error when Xray is unreachable or returns a non-2xx status; only a malformed 2xx body is an error (`internal/jfrog/xray.go`, `CLAUDE.md`). `ListArtifacts` filters folders and RPM metadata (`repomd.xml`, `*.xml.gz`) out of the listing (`CLAUDE.md`).

## 6. Database conventions

- **Engine(s):** Not applicable — the application has no database (`CLAUDE.md`).
- **Migration tool & location:** Not applicable.
- **Migration naming:** Not applicable.
- **Rules:** Not applicable.
- **Schema docs:** Not applicable; there is no `docs/database/` content to maintain while the project has no datastore.

## 7. Testing requirements

- **Test command:** `go test ./...` (`README.md`, `CLAUDE.md`).
  - Single test: `go test ./internal/handlers -run TestUploadArtifact_Success -v` (`CLAUDE.md`).
  - Coverage: `go test -coverprofile=coverage.out ./...` then `go tool cover -html=coverage.out` (`README.md`).
- **Frameworks:** Go standard library only (`testing`, `net/http/httptest`); no assertion framework (`CLAUDE.md`).
- **Coverage expectation:** TODO: no coverage threshold is defined in any file.
- **What must have tests:** TODO: no written rule. Observed state: test files exist for `main`, `internal/config`, `internal/handlers`, `internal/jfrog`, and `internal/templates`; `internal/models` has none (repository file listing).
- **Integration / e2e setup:** No external services are needed. `internal/jfrog` tests run the real `Client` against an `httptest.Server`; handler tests use a `mockService` implementing `jfrog.Service` and build their own `gin.New()` router; `main_test.go` covers the real `setupRouter` wiring (`CLAUDE.md`). Tests reach templates through relative paths, so they depend on the `templates/` directory being present (`CLAUDE.md`).
  - TODO: no test against a real Artifactory / Xray instance is described.

## 8. Deployment constraints

- **Environments:** TODO: no environments (dev / staging / production) are defined in any file.
- **Pipeline:** GitHub Actions. The only workflow is `release` (`.github/workflows/release.yml`), triggered by pushing a tag matching `v*`. It checks out the repository, sets up Go from `go.mod`, and runs `goreleaser/goreleaser-action@v6` with `release --clean`. There is no CI workflow that runs tests or the linter.
- **Release process:** Push a `v*` tag. GoReleaser (`.goreleaser.yaml`) runs `go mod tidy`, builds the `jfrog-manager` binary for linux / darwin / windows on amd64 / arm64, produces `tar.gz` archives (`zip` on Windows) and `checksums.txt`, and generates a changelog grouped into Features (`feat`), Fixes (`fix`), and Others, excluding `docs:`, `test:`, `chore:`, and `ci:` commits.
  - Known gap: archives include only the binary, `README.md`, and `LICENSE*`; the `templates/` directory the binary needs at runtime is not packaged (`.goreleaser.yaml`, `CLAUDE.md`).
  - Known gap: `.goreleaser.yaml` sets `-X main.version`, `main.commit`, and `main.date`, but `main.go` declares no such variables.
  - TODO: release history is unverified. A local tag `1.0.0` exists (`git tag`), which does not match the `v*` trigger; whether any release was published is unknown.
  - TODO: who may cut a release, and any approval step, is not documented.
- **Rollback:** TODO: no rollback procedure is documented.
- **Operational limits / quotas:** Upload size 500 MB; multipart memory 32 MB before spilling to disk; bulk-delete body 1 MiB and 500 paths (`internal/handlers/artifacts.go`, `main.go`). Short upstream calls time out after `TIMEOUT` seconds (default 30, must be a positive integer); uploads and downloads stream with no timeout (`internal/config/config.go`, `internal/jfrog/client.go`). Gin runs in release mode unless `GIN_MODE` is set (`main.go`).
  - TODO: no monitoring, alerting, SLA, or capacity information exists in the repository.

## 9. AI-specific instructions

Rules an AI agent must follow when working in this repo (the enforcement detail
lives in [.ai/rules.md](.ai/rules.md); repository-specific agent guidance also
lives in [CLAUDE.md](CLAUDE.md)):

- Read this file, `.ai/rules.md`, and `ARCHITECTURE.md` before editing.
- Cite file paths for every factual claim; never invent config, contracts, or owners.
- Reuse existing patterns and utilities before writing new ones.
- Keep changes atomic and update the impacted docs + CHANGELOG in the same change.
- Mark anything you cannot verify as `TODO:` rather than guessing.
- Stop and ask if a requested change conflicts with a rule here.
