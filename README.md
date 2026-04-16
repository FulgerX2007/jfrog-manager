# JFrog Manager

Web-based admin tool for managing JFrog Artifactory artifacts. Browse repositories, list/upload/download/delete artifacts (individually or in bulk), and view Xray vulnerability information through a clean web UI.

Built with Go + Gin + htmx + Bootstrap 5 — server-rendered HTML with htmx partial swaps.

## Prerequisites

- Go 1.26+
- A JFrog Artifactory instance with an access token (and optionally Xray for vulnerability data)

## Setup

1. Clone the repository:

```bash
git clone <repo-url>
cd jfrog_manager
```

2. Copy the example environment file and fill in your values:

```bash
cp .env.example .env
chmod 600 .env
```

3. Edit `.env` with your JFrog credentials:

```
JFROG_URL=https://your-instance.jfrog.io
JFROG_USERNAME=your-email@example.com
JFROG_TOKEN=your-token-here
PORT=8080
TIMEOUT=30
DEFAULT_REPO=
```

| Variable         | Required | Default | Description                                                            |
|------------------|----------|---------|------------------------------------------------------------------------|
| `JFROG_URL`      | Yes      | —       | Base URL of your JFrog Artifactory instance                            |
| `JFROG_USERNAME` | Yes      | —       | Username for HTTP Basic auth                                           |
| `JFROG_TOKEN`    | Yes      | —       | Access token / password paired with `JFROG_USERNAME`                   |
| `PORT`           | No       | `8080`  | HTTP server port                                                       |
| `TIMEOUT`        | No       | `30`    | HTTP client timeout (seconds) for short API calls. Uploads/downloads stream without a timeout. |
| `DEFAULT_REPO`   | No       | —       | Repository pre-selected in the dropdown on page load                   |
| `GIN_MODE`       | No       | `release` | Set to `debug` to enable verbose Gin logging                          |

4. Install dependencies and run:

```bash
go mod tidy
go run main.go
```

5. Open http://localhost:8080 in your browser.

## Features

- **Browse repositories** — dropdown populated from Artifactory API
- **List artifacts** — deep listing with name, path, human-readable size (KiB/MiB/GiB), and last modified date
- **Upload artifacts** — multipart upload up to 500 MB with live progress bar; uploads >32 MB spill to disk instead of buffering in memory
- **Download artifacts** — server-proxied streaming download; credentials never leave the server
- **Delete artifacts** — individual delete with confirmation, or bulk delete via row checkboxes (capped at 500 paths per request)
- **Xray vulnerabilities** — toggle a per-row panel showing severity badges (Critical/High/Medium/Low) and CVE details; loading skeleton during fetch; second click collapses the panel. Graceful fallback when Xray is unavailable or an artifact is unindexed.

## Routes

| Method | Route                            | Description                                                     |
|--------|----------------------------------|-----------------------------------------------------------------|
| GET    | `/`                              | Main page                                                       |
| GET    | `/repos`                         | htmx fragment — repository `<option>` list                      |
| GET    | `/artifacts?repo=X`              | htmx fragment — artifact table                                  |
| POST   | `/artifacts/upload`              | Multipart upload (fields: `repo`, `folder`, `file`)             |
| POST   | `/artifacts/bulk-delete`         | JSON body `{repo, paths[]}`; re-renders the list                |
| GET    | `/artifacts/download?repo=X&path=Y` | Stream artifact with `Content-Disposition: attachment`       |
| DELETE | `/artifacts?repo=X&path=Y`       | Delete a single artifact                                        |
| GET    | `/xray?repo=X&path=Y`            | htmx fragment — Xray vulnerability panel                        |

## Security notes

This service does not implement application-level authentication — anyone who can reach `PORT` has full access to the configured JFrog credentials.

Deploy behind a VPN, an SSO reverse proxy, or on `127.0.0.1` with an authenticating proxy in front. The server sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, and `Referrer-Policy: no-referrer` on every response. Request bodies for bulk-delete are capped at 1 MiB; uploads at 500 MB. Keep `.env` at mode 600 so the JFrog token is not world-readable.

## Testing

```bash
go test ./...                          # Run all tests
go test -v ./...                       # Verbose
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Project structure

```
jfrog_manager/
├── main.go                          # Entry point — config, routing, middleware
├── internal/
│   ├── config/config.go             # Environment-based configuration
│   ├── handlers/
│   │   ├── artifacts.go             # List/Upload/Download/Delete/BulkDelete
│   │   └── xray.go                  # Xray vulnerability handler
│   ├── jfrog/
│   │   ├── client.go                # HTTP client with Basic auth + streaming client for up/downloads
│   │   ├── service.go               # Service interface for testability
│   │   ├── artifacts.go             # ListRepos
│   │   └── xray.go                  # Xray summary client
│   ├── models/models.go             # Repository, Artifact, Xray data structs
│   └── templates/templates.go       # Template loading + helper funcs (humanSize, cssID, etc.)
├── templates/
│   ├── layout.html                  # Base HTML + styles + JS
│   ├── index.html                   # Main page content
│   └── partials/
│       ├── artifact_list.html       # Artifact table rows
│       ├── upload_form.html         # Upload form with progress bar
│       ├── xray_panel.html          # Vulnerability display
│       └── error.html               # Error alert
├── .env.example                     # Environment variable template
└── go.mod
```
