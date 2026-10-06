# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

TODO: confirm that the project follows Semantic Versioning. The repository contains no versioning policy; the only evidence is the tag pattern `v*` in `.github/workflows/release.yml` and one git tag named `1.0.0` (see "Release history" below).

## [Unreleased]

### Added

- Vulnerability report: a repository-wide list of Xray vulnerabilities in the latest version of each package, sortable by component and severity, showing the impact path of each finding, with CSV export (`GET /vulnerabilities`, `GET /vulnerabilities/export`).

### Changed

- Redesigned the interface: repository picker and section tabs in the top bar, a component filter, a latest-versions / all-files switch and a text filter on the artifact list, and a vulnerability report grouped by component with severity filters. Bootstrap is no longer loaded.
- The artifact list shows the latest version of each package by default (`view=all` lists every file), and Xray findings are offered for latest versions only.
- The per-artifact Xray panel shows impact paths and Xray issue ids, and ranks unknown severities last.

### Fixed

- Release archives now include the `templates/` directory, which the binary loads at start-up (`.goreleaser.yaml`).

### Removed

### Security

## Release history

No past release is recorded in this file, because none could be confirmed from the repository.

- TODO: decide whether the git tag `1.0.0` is a published release and, if so, add a `## [1.0.0] - YYYY-MM-DD` section for it. What the repository shows: `git tag -l` lists a single lightweight tag `1.0.0` pointing at commit `41d6b08` ("Merge pull request #1 from FulgerX2007/jfrog-manager"). A lightweight tag has no tag date of its own, and the name does not match the `v*` pattern that triggers the release workflow (`.github/workflows/release.yml`), so that workflow would not have run for it.
- TODO: backfill entries for changes made before this file existed, if wanted. The commit history (`git log`) is the only source; it uses conventional commit prefixes (`feat:`, `fix:`, `fix(security):`, `docs:`, `ci:`).

## How releases are produced

- Pushing a git tag matching `v*` runs the `release` workflow, which runs GoReleaser with `release --clean` (`.github/workflows/release.yml`).
- GoReleaser generates the GitHub release notes from commit messages: `feat` commits are grouped under "Features", `fix` commits under "Fixes", everything else under "Others"; commits starting with `docs:`, `test:`, `chore:` or `ci:` are excluded (`.goreleaser.yaml`, `changelog` section).
- Those generated release notes are separate from this file. Nothing in the repository updates `CHANGELOG.md` automatically, so it is maintained by hand.

<!--
Release flow: when cutting a release, rename [Unreleased] to [X.Y.Z] - YYYY-MM-DD,
then add a fresh empty [Unreleased] block above it.
Example:

## [1.0.0] - 2025-01-01
### Added
- Initial release.
-->
