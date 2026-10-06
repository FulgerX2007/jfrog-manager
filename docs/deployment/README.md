# Deployment Documentation

How this project is built, configured, deployed, and operated.

> Keep in sync with the code: when pipeline, config, or infra changes, update this
> folder in the same change.

## Environments

No deployment environment is defined in this repository: there is no Dockerfile, no docker-compose file, no Kubernetes/Helm manifest and no deploy job in CI (the only workflow is `.github/workflows/release.yml`, which publishes release archives). Consequently there is no local-dev vs production split of compose/k8s files to describe.

| Environment | Purpose | URL / target | Notes |
|-------------|---------|--------------|-------|
| Local | Run the server from a checkout (`go run main.go`, see `README.md` "Setup") | `http://localhost:8080` by default (`PORT` default in `internal/config/config.go`) | Must be started from the repo root because templates load from the relative directory `templates` (`main.go`) |
| TODO: staging / production | TODO: | TODO: | TODO: no hosted environment, hosting target or URL is recorded in the repo |

Network placement constraint (`README.md` "Security notes"): the service has no application-level authentication, so anyone who can reach `PORT` has full access to the configured JFrog credentials. The README says to deploy behind a VPN, an SSO reverse proxy, or on `127.0.0.1` with an authenticating proxy in front.

## Build & release

- **CI/CD:** GitHub Actions, one workflow: `release` (`.github/workflows/release.yml`).
  - Trigger: push of a tag matching `v*`.
  - Single job `goreleaser` on `ubuntu-latest` with `contents: write` permission. Steps: checkout with full history (`actions/checkout@v4`, `fetch-depth: 0`), set up Go from the version in `go.mod` (`actions/setup-go@v5`, module cache enabled), run GoReleaser (`goreleaser/goreleaser-action@v6`, GoReleaser `~> v2`, args `release --clean`) using the built-in `GITHUB_TOKEN`.
  - There is no test, lint or deploy workflow; `go test ./...` and `golangci-lint run` are run manually (`README.md` "Testing", `CLAUDE.md`).
- **Artifacts produced:** defined in `.goreleaser.yaml`.
  - Binary `jfrog-manager`, built with `CGO_ENABLED=0` for linux, darwin and windows on amd64 and arm64.
  - Archives named `{ProjectName}_{Version}_{Os}_{Arch}`, `tar.gz` (`zip` on windows), plus `checksums.txt`.
  - Archives contain the binary, `README.md`, `LICENSE*` and the `templates/` directory (`.goreleaser.yaml`, `archives.files`). The binary loads `templates/` from disk at start-up (`templates.Load("templates")` in `main.go`) and exits if loading fails, so start it from the unpacked archive directory.
  - ldflags set `main.version`, `main.commit` and `main.date`, but `main.go` declares no such variables, so no version information is exposed by the binary.
  - A local, non-release build is `go build -o jfrog_manager .` (`CLAUDE.md`); that binary name and GoReleaser's `dist/` are gitignored (`.gitignore`).
- **Release process:** push a `v*` tag; the `release` workflow publishes a GitHub release (`draft: false`, `prerelease: auto`) with the archives and checksums. GoReleaser runs `go mod tidy` as a pre-build hook. Release notes are generated from GitHub commit data and grouped as Features (`feat`), Fixes (`fix`) and Others; commits prefixed `docs:`, `test:`, `chore:` and `ci:` are excluded (`.goreleaser.yaml`).
  - TODO: who is allowed to cut a release and what must pass beforehand — not documented in the repo.
- **Versioning/tagging:** tags must start with `v` to trigger the workflow (`.github/workflows/release.yml`). The only tag present in the local clone is `1.0.0` (`git tag -l`), which does not match `v*` and so would not have triggered this workflow.
  - TODO: confirm the versioning scheme (semantic versioning is implied by the tag shape but not stated anywhere) and whether any release has actually been published.

## Configuration

- **Mechanism:** environment variables only. `internal/config/config.go` loads a `.env` file from the working directory with godotenv and, if none is found, logs `no .env file found, using environment variables` and reads the process environment. There are no config files and no secrets-manager integration in the repo.
- **Required settings:** `JFROG_URL`, `JFROG_USERNAME`, `JFROG_TOKEN`; start-up fails with `<NAME> is required` if any is empty (`internal/config/config.go`, `main.go`). Copy `.env.example` to `.env` and fill it in (`README.md` "Setup"). `.env` is gitignored (`.gitignore`); the README advises keeping it at mode 600 so the JFrog token is not world-readable.

| Variable | Purpose | Required | Default |
|----------|---------|----------|---------|
| `JFROG_URL` | Base URL of the JFrog instance (`.env.example`: `https://your-instance.jfrog.io`) | Yes | none |
| `JFROG_USERNAME` | Username for HTTP Basic auth against JFrog | Yes | none |
| `JFROG_TOKEN` | Token used as the Basic auth password | Yes | none |
| `PORT` | Port the HTTP server listens on (`r.Run(":" + cfg.Port)` in `main.go`, all interfaces) | No | `8080` |
| `TIMEOUT` | Timeout in seconds for short upstream calls; must be a positive integer or start-up fails | No | `30` |
| `DEFAULT_REPO` | Repository preselected in the UI | No | empty |
| `GIN_MODE` | Gin run mode; read directly in `main.go`, not in `internal/config` | No | release mode when unset |

Sources: `internal/config/config.go`, `.env.example`, `main.go`, `internal/jfrog/client.go` (Basic auth), `internal/handlers/artifacts.go` and `templates/index.html` (`DEFAULT_REPO`).

## Runbook

- **Deploy:** no deployment procedure or automation exists in the repo. What the code requires of any deployment:
  1. Provide the binary (`go build -o jfrog_manager .`, or a release archive).
  2. Place the `templates/` directory next to the process working directory; the path is relative (`main.go`).
  3. Supply the required variables via `.env` in the working directory or the process environment (`internal/config/config.go`).
  4. Start the binary from that working directory; it logs `starting server` with the port (`main.go`).
  5. Restrict network access as described in `README.md` "Security notes".
  - TODO: target host/platform, process supervisor, reverse proxy configuration and the person who performs the deploy.
- **Rollback:** TODO: no rollback procedure is documented. The service has no database and keeps no state of its own, so there is no schema or data migration to reverse.
- **Health checks / readiness:** TODO: no dedicated health or readiness endpoint exists; the registered routes are `/`, `/repos`, `/artifacts`, `/artifacts/upload`, `/artifacts/bulk-delete`, `/artifacts/download` and `/xray` (`main.go`). Whether `GET /` is acceptable as a liveness probe is undecided.
- **Common operational issues & fixes:** only the failures visible in code are listed; each logs through `log/slog` and exits with status 1 (`main.go`).
  - `failed to load config` with `JFROG_URL is required` (or `JFROG_USERNAME` / `JFROG_TOKEN`): set the missing variable (`internal/config/config.go`).
  - `failed to load config` with `TIMEOUT must be a valid integer` or `TIMEOUT must be a positive integer`: fix or unset `TIMEOUT` (`internal/config/config.go`).
  - `failed to load templates`: the process was not started from a directory containing `templates/`, for example a release binary moved out of its unpacked archive directory (`main.go`, `.goreleaser.yaml`).
  - `server failed`: the listener could not start on `PORT` (`main.go`).
  - TODO: operational issues observed in real use, monitoring and alerting, log collection, on-call contact.
- **Quotas / limits:** application-enforced limits only.
  - Upload size capped at 500 MB; larger uploads return HTTP 413 (`internal/handlers/artifacts.go`).
  - Multipart uploads above 32 MB spill to disk instead of memory (`r.MaxMultipartMemory` in `main.go`), so the host needs temporary disk space for large uploads.
  - Bulk-delete request body capped at 1 MiB and 500 paths (`internal/handlers/artifacts.go`).
  - Short upstream calls time out after `TIMEOUT` seconds; upload and download streams use a client with no timeout (`internal/jfrog/client.go`).
  - TODO: JFrog-side quotas or rate limits, and CPU/memory/disk sizing for the host — not recorded in the repo.
