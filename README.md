# JFrog Manager

Web-based admin tool for managing JFrog Artifactory artifacts. Browse repositories, list/upload/delete artifacts, and view Xray vulnerability information through a clean web UI.

Built with Go + Gin + htmx + Bootstrap 5 — server-rendered HTML with htmx partial swaps.

## Prerequisites

- Go 1.24+
- A JFrog Artifactory instance with API Key access

## Setup

1. Clone the repository:

```bash
git clone <repo-url>
cd jfrog_manager
```

2. Copy the example environment file and fill in your values:

```bash
cp .env.example .env
```

3. Edit `.env` with your JFrog credentials:

```
JFROG_URL=https://your-instance.jfrog.io
JFROG_API_KEY=your-api-key-here
PORT=8080
TIMEOUT=30
```

| Variable | Required | Default | Description |
|---|---|---|---|
| `JFROG_URL` | Yes | — | Base URL of your JFrog Artifactory instance |
| `JFROG_API_KEY` | Yes | — | API key for authentication (`X-JFrog-Art-Api` header) |
| `PORT` | No | `8080` | HTTP server port |
| `TIMEOUT` | No | `30` | HTTP client timeout in seconds |

4. Install dependencies and run:

```bash
go mod tidy
go run main.go
```

5. Open http://localhost:8080 in your browser.

## Features

- Browse repositories — dropdown populated from Artifactory API
- List artifacts — deep listing of all artifacts in a repository with name, path, size, and last modified date
- Upload artifacts — multipart file upload to any repository path
- Delete artifacts — delete with confirmation dialog, row removed via htmx
- Xray vulnerabilities — view vulnerability summary with severity badges (Critical/High/Medium/Low) and CVE details; graceful handling when Xray is unavailable

## Routes

| Method | Route | Description |
|---|---|---|
| GET | `/` | Main page with repo browser |
| GET | `/repos` | htmx fragment — repo dropdown options |
| GET | `/artifacts?repo=X` | htmx fragment — artifact table rows |
| POST | `/artifacts/upload` | Upload artifact (multipart form) |
| DELETE | `/artifacts?repo=X&path=Y` | Delete artifact |
| GET | `/xray?repo=X&path=Y` | htmx fragment — Xray vulnerability panel |

## Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Project Structure

```
jfrog_manager/
├── main.go                          # Entry point — config, routing, server
├── internal/
│   ├── config/config.go             # Environment-based configuration
│   ├── handlers/
│   │   ├── artifacts.go             # Artifact CRUD handlers
│   │   └── xray.go                  # Xray vulnerability handler
│   ├── jfrog/
│   │   ├── client.go                # HTTP client with API key auth
│   │   ├── service.go               # Service interface for testability
│   │   ├── artifacts.go             # ListRepos, ListArtifacts, Upload, Delete
│   │   └── xray.go                  # Xray summary client
│   ├── models/models.go             # Repository, Artifact, Xray data structs
│   └── templates/templates.go       # Template loading utility
├── templates/
│   ├── layout.html                  # Base HTML with Bootstrap + htmx CDN
│   ├── index.html                   # Main page
│   └── partials/
│       ├── artifact_list.html       # Artifact table rows
│       ├── upload_form.html         # Upload form
│       ├── xray_panel.html          # Vulnerability display
│       └── error.html               # Error alert
├── .env.example                     # Environment variable template
└── go.mod
```
