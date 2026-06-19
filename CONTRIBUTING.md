# Contributing to SPACK

Thank you for contributing to SPACK. This project favors small, reviewable changes with clear tests.

## Development setup

Requirements:

- Go version from `go.mod`.
- Task from https://taskfile.dev.
- Docker, when changing container images or benchmark tooling.

Common commands:

```powershell
task test
task lint
task check
task build
```

Security and release checks:

```powershell
task test:race
task security:govulncheck
task release:goreleaser:check
```

## Pull request expectations

Before opening a pull request:

- Add or update tests for behavior changes.
- Run `task test` and `task lint`.
- Run `task test:race` for concurrency-sensitive changes.
- Run `task security:govulncheck` for dependency, network, parser, filesystem, or release changes.
- Keep unrelated refactors out of bug fix pull requests.
- Document configuration, security, and compatibility changes.

## Code style

- Prefer simple Go code with explicit error handling.
- Keep HTTP, filesystem, and release behavior covered by tests.
- Avoid high-cardinality metric labels, especially raw URL paths, filenames, hashes, or user-controlled values.
- Treat local filesystem paths, symlinks, encoded URLs, and archive paths as security-sensitive inputs.

## Security reports

Do not report vulnerabilities in public issues or pull requests. Follow `SECURITY.md`.

## Releases

Release behavior is governed by `RELEASE.md`. Changes that affect image tags, SBOMs, signatures, provenance, supported platforms, or compatibility guarantees must update that policy.
