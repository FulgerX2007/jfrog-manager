# AI Agent Rules — JFrog Manager

Operating rules for AI coding agents working in this repository. These are
**enforceable constraints**, not suggestions. If a task conflicts with a rule here,
stop and ask.

> See also: [CLAUDE.md](../CLAUDE.md) — this file does not duplicate it.

`CLAUDE.md` is the source for commands, architecture, the htmx/template contract,
input limits, and code conventions. The sections below state only the hard rules and
point to the file that proves each one.

## Operating procedure (run this loop for every task)

1. Read `PROJECT.md` — stack, standards, conventions, constraints.
2. Read this file — the hard rules below.
3. Read `ARCHITECTURE.md` — components, data flow, dependencies.
4. **Detect impacted modules** from the task and the diff; list them with paths.
5. Update code following existing conventions.
6. Update the impacted documentation (see *Documentation update requirements*).
7. Add a `CHANGELOG.md` entry under `[Unreleased]`.

## Coding conventions

- Follow the style/formatter and linter named in `PROJECT.md`. The linter is
  `golangci-lint run` with default linters; there is no `.golangci.yml` in the
  repository (`CLAUDE.md`, "Commands").
- TODO: formatter requirement (for example `gofmt` / `goimports`) is not stated in any repository file.
- Reuse existing utilities and patterns before introducing new ones — search first.
- Match the surrounding code's naming, structure, and idioms.
- Pass and return structs by value and use value receivers; pointers only for types
  the libraries hand out as pointers (`CLAUDE.md`, "Conventions"; see `Handler` in
  `internal/handlers/artifacts.go`, `Client` in `internal/jfrog/client.go`, `Config`
  in `internal/config/config.go`).
- Handlers depend only on the `jfrog.Service` interface
  (`internal/jfrog/service.go`). A new JFrog operation must be added to the
  interface, implemented on `Client`, and added to both `mockService`
  implementations (`main_test.go`, `internal/handlers/artifacts_test.go`).
- A new route must be registered in `setupRouter` (`main.go`) and in the test
  routers (`internal/handlers/artifacts_test.go`, `internal/handlers/xray_test.go`).
- New template functions must be added to `FuncMap()` in
  `internal/templates/templates.go`; new partials must live under `templates/` or
  `templates/partials/`, the only two globs `Load` reads.
- DOM ids derived from artifact paths must go through the `cssID` template function
  (`internal/templates/templates.go`).
- Use conventional commit prefixes. The GoReleaser changelog groups on `feat` and
  `fix` and excludes `docs:`, `test:`, `chore:`, `ci:` (`.goreleaser.yaml`).

## Refactoring rules

- Refactors are behavior-preserving; do not mix a refactor with a feature/fix.
- Keep changes atomic and scoped to the task.
- Do not rename/move public symbols without updating all references and docs.

## Database modification rules

- Not applicable today: the repository has no database, no migration tool, and no
  `docs/database/` directory. The application holds no persistent state of its own
  and proxies every call to Artifactory / Xray (`CLAUDE.md`, "What this is";
  `go.mod` has no direct database dependency).
- TODO: migration tool and location — decide and document before introducing any datastore.
- If a datastore is ever introduced: create `docs/database/`, never edit an
  already-released migration (add a new one), and document every schema change in
  `docs/database/` and `CHANGELOG.md`.

## Logging standards

- Logging library: the standard library `log/slog`, called through the package-level
  functions (`slog.Info`, `slog.Warn`, `slog.Error`). No third-party logging library
  is a dependency (`go.mod`). Do not introduce one.
- Message plus key/value attributes, with the error under the `"error"` key, for
  example `slog.Error("listing artifacts", "error", err)`
  (`internal/handlers/artifacts.go`).
- Levels as used in the code:
  - `Error` — a failed handler operation or a fatal startup failure
    (`internal/handlers/artifacts.go`, `internal/handlers/xray.go`, `main.go`).
  - `Warn` — a degraded but non-failing path: Xray unreachable or non-OK
    (`internal/jfrog/xray.go`).
  - `Info` — lifecycle events: server start, missing `.env` (`main.go`,
    `internal/config/config.go`).
- TODO: log handler/format (text vs JSON), minimum level, and log destination are not configured anywhere in the repository; the `slog` default is in effect.
- No secrets, credentials, tokens, or PII in logs. In particular never log
  `JFROG_TOKEN`, `JFROG_USERNAME`, or the Basic-auth header that
  `req.SetBasicAuth` sets (`internal/config/config.go`, `internal/jfrog/client.go`).
- Log actionable context (ids, operation, outcome), not noise.
- Upstream error bodies go to the log only, never to the browser
  (`internal/handlers/artifacts.go`; `CLAUDE.md`, "Input limits and validation").

## Error handling standards

- Handle errors explicitly; never silently swallow them.
- Fail loudly in the right layer; surface actionable messages.
- When returning an underlying error, wrap it with `fmt.Errorf("doing thing: %w", err)`,
  as most call sites in `internal/jfrog/client.go` and `internal/jfrog/artifacts.go` do. There is no custom error type in the repository.
- Handlers report failures through `h.renderError`
  (`internal/handlers/artifacts.go`): HTTP 422 with the `error` fragment and the
  `HX-Retarget: #error-container` / `HX-Reswap: innerHTML` headers. Oversized
  uploads return 413. Keep new handlers on this path.
- The one intentional non-error: `GetXraySummary` returns
  `XraySummary{Available: false}` with a nil error when Xray is unreachable or
  returns a non-2xx status; only a malformed 2xx body is an error
  (`internal/jfrog/xray.go`). Do not turn this into a failure.

## Testing standards

- Test command: `go test ./...` (`README.md`, "Testing"; `CLAUDE.md`, "Commands").
- Single test: `go test ./internal/handlers -run TestUploadArtifact_Success -v`
  (`CLAUDE.md`, "Commands").
- Run `golangci-lint run` before proposing a change (`CLAUDE.md`, "Commands").
- New business logic must ship with tests.
- Standard library only (`testing`, `net/http/httptest`); do not add an assertion
  framework (`go.mod`; `CLAUDE.md`, "Conventions").
- `internal/jfrog` tests run the real `Client` against an `httptest.Server`
  (`internal/jfrog/client_test.go`); handler tests use a `mockService`
  implementing `jfrog.Service` (`internal/handlers/artifacts_test.go`);
  `main_test.go` covers the real route wiring.
- Tests run locally only: the sole CI workflow is the tag-triggered release
  (`.github/workflows/release.yml`).
- TODO: coverage expectation and integration-test setup. No coverage threshold is defined, and no test runs against a real Artifactory / Xray instance.

## Security restrictions

- No hardcoded secrets; read config/secrets from the approved mechanism: environment
  variables, optionally loaded from `.env` by godotenv
  (`internal/config/config.go`, `.env.example`). `.env` is gitignored
  (`.gitignore`); never commit it or print its contents.
- Validate and sanitize all external input. Every handler that takes a `repo` / `path` /
  `folder` value rejects one containing `..` (`internal/handlers/artifacts.go`,
  `internal/handlers/xray.go`; `CLAUDE.md`, "Input limits and validation").
- Do not add dependencies without justification; prefer the standard library. The
  only direct dependencies are `gin` and `godotenv` (`go.mod`).
- Do not weaken or remove the existing limits (`internal/handlers/artifacts.go`,
  `main.go`): 500 MB upload cap via `http.MaxBytesReader`, 32 MB
  `MaxMultipartMemory`, 1 MiB body and 500 paths for bulk delete.
- Keep the `securityHeaders` middleware on every response: `X-Content-Type-Options:
  nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer` (`main.go`).
- Build upstream URLs with `url.PathEscape` for the repo and `escapePathSegments`
  for artifact paths; never escape a whole artifact path in one call
  (`internal/jfrog/client.go`).
- The application has no authentication of its own; anyone who can reach `PORT`
  acts with the configured JFrog credentials (`README.md`, "Security notes"). Do
  not add code or docs that assume a caller is authenticated.
- TODO: project-specific security constraints from `PROJECT.md` §4.

## Documentation update requirements

A change is **not complete** until its docs are updated in the same change:

- Structure/component/dependency changed → `ARCHITECTURE.md` (+ diagram).
- Public API changed → `docs/api/`.
- Schema/migration changed → not applicable today (no datastore; see "Database modification rules").
- Deploy/config changed → `docs/deployment/`.
- Significant, hard-to-reverse decision → new ADR in `docs/decisions/`.
- User-facing behavior changed → `README.md`.
- Always → a `CHANGELOG.md` entry under `[Unreleased]`.

Cite file paths for every factual claim. Mark anything you cannot verify as `TODO:`.
Never invent configuration, contracts, SLAs, or owners.
