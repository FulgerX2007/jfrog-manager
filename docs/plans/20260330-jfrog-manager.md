# JFrog Manager — Go + Gin + htmx Web Application

## Overview
- Web-based admin tool for managing JFrog Artifactory artifacts
- Browse repositories, list/upload/delete artifacts, view Xray vulnerability info
- Built with Go + Gin + htmx + Bootstrap — server-rendered HTML with htmx partial swaps
- Personal/admin tool for self-hosted Artifactory with API Key authentication
- **Done when**: user can browse repos, list/upload/delete artifacts, and view Xray vulnerabilities through the web UI, with all tests passing

## Context (from discovery)
- **Project location**: `/home/ailiev/Projects/jfrog_manager/` (empty directory)
- **Patterns from**: `otarie/sso` project (Gin, .env config, Go templates, Bootstrap UI)
- **Stack**: Go 1.24, Gin, htmx (CDN), Bootstrap 5 (CDN)
- **Auth**: JFrog API Key via `X-JFrog-Art-Api` header
- **JFrog APIs used**:
  - `GET /artifactory/api/repositories` — list repos
  - `GET /artifactory/api/storage/{repo}/?list&deep=1` — list artifacts
  - `PUT /artifactory/{repo}/{path}` — upload artifact
  - `DELETE /artifactory/{repo}/{path}` — delete artifact
  - `POST /xray/api/v1/summary/artifact` — Xray vulnerability summary
- **Conventions**: Go value types (no pointers), conventional commits

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Testing Strategy
- **Unit tests**: required for every task — test JFrog client methods, handlers, config
- **Integration tests**: mock HTTP server simulating JFrog API responses
- No e2e tests for this project (admin tool, lightweight)

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with warning prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Project scaffolding and config

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `.env.example`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [x] Initialize Go module: `go mod init jfrog_manager`
- [x] Create `internal/config/config.go` — struct with `JFrogURL`, `JFrogAPIKey`, `Port`, `Timeout` fields; `Load()` returns `(Config, error)` as value types (no pointers); use godotenv + os.Getenv with defaults (port=8080, timeout=30); use `log/slog` for logging
- [x] Create `.env.example` with placeholder values
- [x] Create `main.go` — load config, init Gin, load templates, register placeholder route for `/`, start server
- [x] Write tests for config loading (defaults, env override, missing required fields)
- [x] Run tests — must pass before next task

### Task 2: JFrog HTTP client and service interface

**Files:**
- Create: `internal/jfrog/client.go`
- Create: `internal/jfrog/service.go`
- Create: `internal/jfrog/client_test.go`

- [x] Create `internal/jfrog/service.go` — define `Service` interface with methods: `ListRepos()`, `ListArtifacts(repo)`, `UploadArtifact(repo, path, reader)`, `DeleteArtifact(repo, path)`, `GetXraySummary(repo, path)`. Handlers will depend on this interface for testability
- [x] Create `internal/jfrog/client.go` — `Client` struct with `http.Client`, base URL, API key; `NewClient(config)` constructor; `Client` satisfies `Service` interface
- [x] Add `X-JFrog-Art-Api` header injection via custom `Do()` method that wraps all HTTP calls
- [x] Write tests using `httptest.NewServer` to verify auth header, base URL construction, timeout
- [x] Write tests for error scenarios (connection refused, non-2xx responses)
- [x] Run tests — must pass before next task

### Task 3: Models and repository listing

**Files:**
- Create: `internal/models/models.go`
- Create: `internal/jfrog/artifacts.go`
- Create: `internal/jfrog/artifacts_test.go`

- [x] Create `internal/models/models.go` — `Repository` struct (Key, Type, PackageType, Description), `Artifact` struct (Name, Path, Size, LastModified, Repo)
- [x] Create `internal/jfrog/artifacts.go` — `ListRepos()` method calling `GET /artifactory/api/repositories`, returning `[]models.Repository`
- [x] Write tests for `ListRepos()` with mock server returning sample JSON
- [x] Write tests for `ListRepos()` error cases (401, 500, malformed JSON)
- [x] Run tests — must pass before next task

### Task 4: List artifacts for a repository

**Files:**
- Modify: `internal/jfrog/artifacts.go`
- Modify: `internal/jfrog/artifacts_test.go`

- [x] Add `ListArtifacts(repo string)` method calling `GET /artifactory/api/storage/{repo}/?list&deep=1`
- [x] Parse response into `[]models.Artifact` with name, path, size, last modified
- [x] Write tests for `ListArtifacts()` with mock server (success + empty repo + error cases)
- [x] Run tests — must pass before next task

### Task 5: Upload and delete artifacts

**Files:**
- Modify: `internal/jfrog/artifacts.go`
- Modify: `internal/jfrog/artifacts_test.go`

- [x] Add `UploadArtifact(repo, path string, reader io.Reader)` method — `PUT /artifactory/{repo}/{path}` with file body
- [x] Add `DeleteArtifact(repo, path string)` method — `DELETE /artifactory/{repo}/{path}`
- [x] Write tests for upload (verify body streamed, correct path, success + error)
- [x] Write tests for delete (success, 404, 403)
- [x] Run tests — must pass before next task

### Task 6: Xray client

**Files:**
- Create: `internal/jfrog/xray.go`
- Create: `internal/jfrog/xray_test.go`
- Modify: `internal/models/models.go`

- [x] Add Xray models: `XraySummary` (artifacts array), `XrayArtifact` (general info, issues, licenses), `XrayIssue` (severity, description, CVEs, components)
- [x] Create `GetXraySummary(repo, path string)` method — `POST /xray/api/v1/summary/artifact` with `{"paths": ["default/{repo}/{path}"]}` (note: "default" is the standard JFrog service ID; document this assumption in code)
- [x] Handle graceful degradation: if Xray returns 404 or connection error, return empty summary with a "not available" flag
- [x] Write tests for Xray summary (success with vulnerabilities, empty results, Xray unavailable)
- [x] Run tests — must pass before next task

### Task 7: HTML templates — layout and main page

**Files:**
- Create: `templates/layout.html`
- Create: `templates/index.html`
- Create: `templates/partials/artifact_list.html`
- Create: `templates/partials/upload_form.html`
- Create: `templates/partials/xray_panel.html`
- Create: `templates/partials/error.html`

- [x] Create `templates/layout.html` — HTML5 base with Bootstrap 5 CSS (CDN), htmx (CDN), navbar with app title
- [x] Create `templates/index.html` — extends layout; repo dropdown (`hx-get="/repos"` on load), artifact table container, upload button
- [x] Create `templates/partials/artifact_list.html` — table rows with Name, Path, Size, Modified columns; delete button (`hx-delete`, `hx-confirm`); Xray button (`hx-get="/xray"`)
- [x] Create `templates/partials/upload_form.html` — inline form with repo dropdown, path input, file input; `hx-post="/artifacts/upload"` with `hx-encoding="multipart/form-data"`
- [x] Create `templates/partials/xray_panel.html` — expandable row showing severity badges (Critical/High/Medium/Low counts), CVE list with descriptions
- [x] Create `templates/partials/error.html` — Bootstrap `alert-danger` with error message
- [x] Note: use `template.ParseGlob` with multiple patterns or `template.ParseFS` to load nested `partials/` directory — `gin.LoadHTMLGlob` does not recurse by default
- [x] Write `TestTemplatesParse` — verify all templates parse without errors using `template.ParseGlob`
- [x] Run tests — must pass before next task

### Task 8: Gin handlers — artifacts

**Files:**
- Create: `internal/handlers/artifacts.go`
- Create: `internal/handlers/artifacts_test.go`

- [x] Create `Handler` struct holding JFrog `Service` interface (not concrete `Client`) and Gin template engine
- [x] Implement `Index()` — render full `index.html` page
- [x] Implement `ListRepos()` — call client, render repo dropdown options as fragment
- [x] Implement `ListArtifacts()` — read `repo` query param, call client, render `artifact_list.html` fragment
- [x] Implement `UploadArtifact()` — parse multipart form, call client, re-render artifact list on success or error fragment on failure
- [x] Implement `DeleteArtifact()` — read `repo` + `path` params, call client, return empty response (htmx removes row) or error fragment
- [x] Write tests for each handler using `httptest.NewRecorder` and mock JFrog client
- [x] Write tests for error cases (missing params, client failures)
- [x] Run tests — must pass before next task

### Task 9: Gin handlers — Xray

**Files:**
- Create: `internal/handlers/xray.go`
- Create: `internal/handlers/xray_test.go`

- [x] Implement `GetXray()` — read `repo` + `path` params, call client `GetXraySummary()`, render `xray_panel.html` fragment
- [x] Handle Xray unavailable: render panel with "Xray not configured or artifact not indexed" message
- [x] Write tests for Xray handler (with vulnerabilities, empty, unavailable)
- [x] Run tests — must pass before next task

### Task 10: Wire everything together in main.go

**Files:**
- Modify: `main.go`
- Create: `main_test.go`

- [x] Load config, create JFrog client, create handler
- [x] Register all routes: `GET /`, `GET /repos`, `GET /artifacts`, `POST /artifacts/upload`, `DELETE /artifacts`, `GET /xray`
- [x] Load all templates using `template.ParseGlob` with multiple patterns for nested directories
- [x] Add `go mod tidy` to pull all dependencies
- [x] Write integration test: create Gin engine with mock `Service`, hit each route, verify 200 status codes
- [x] Run tests — must pass before next task
- [x] Manual smoke test: start server, verify page loads

### Task 11: Verify acceptance criteria

- [ ] Verify repo browsing works (dropdown populates, artifact list loads)
- [ ] Verify artifact upload works (form submits, list refreshes)
- [ ] Verify artifact delete works (confirmation dialog, row removed)
- [ ] Verify Xray panel works (shows vulnerabilities or graceful "not available")
- [ ] Verify error handling (invalid API key shows clear error, unreachable server shows alert)
- [ ] Run full test suite: `go test ./...`
- [ ] Verify test coverage: `go test -coverprofile=coverage.out ./...`

### Task 12: [Final] Documentation

- [ ] Create README.md with setup instructions, .env configuration, usage
- [ ] Move this plan to `docs/plans/completed/`

## Technical Details

### Config struct
```go
type Config struct {
    JFrogURL    string // required
    JFrogAPIKey string // required
    Port        string // default "8080"
    Timeout     int    // default 30 (seconds)
}
```

### Routes
| Method | Route | Handler | Returns |
|--------|-------|---------|---------|
| GET | `/` | Index | Full page |
| GET | `/repos` | ListRepos | Fragment — option elements |
| GET | `/artifacts?repo=X` | ListArtifacts | Fragment — table rows |
| POST | `/artifacts/upload` | UploadArtifact | Fragment — refreshed list or error |
| DELETE | `/artifacts?repo=X&path=Y` | DeleteArtifact | Empty or error fragment |
| GET | `/xray?repo=X&path=Y` | GetXray | Fragment — xray panel |

### JFrog API mapping
| Client method | JFrog endpoint | Auth header |
|---|---|---|
| ListRepos() | GET /artifactory/api/repositories | X-JFrog-Art-Api |
| ListArtifacts(repo) | GET /artifactory/api/storage/{repo}/?list&deep=1 | X-JFrog-Art-Api |
| UploadArtifact(repo, path, reader) | PUT /artifactory/{repo}/{path} | X-JFrog-Art-Api |
| DeleteArtifact(repo, path) | DELETE /artifactory/{repo}/{path} | X-JFrog-Art-Api |
| GetXraySummary(repo, path) | POST /xray/api/v1/summary/artifact | X-JFrog-Art-Api |

## Post-Completion

**Manual verification**:
- Test against real JFrog Artifactory instance
- Verify Xray integration with an indexed repository
- Test with large repositories (pagination may be needed in future)
- Test upload with various file sizes

**Future enhancements** (out of scope for now):
- Artifact property management
- Build info browsing
- Pagination for large repos
- Access token auth support
