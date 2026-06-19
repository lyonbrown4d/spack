# Release and Compatibility Policy

## Versioning

SPACK uses semantic versioning:

- Major releases may introduce incompatible behavior.
- Minor releases may add features and, before `v1.0.0`, may include breaking changes.
- Patch releases are reserved for compatible bug fixes, security fixes, and documentation corrections.

After `v1.0.0`, incompatible API, configuration, CLI, or runtime behavior changes require a major release unless explicitly documented as experimental.

## Supported release lines

SPACK supports:

- The latest stable minor release.
- The previous stable minor release for 90 days after a new minor release.

Older release lines receive fixes only at maintainer discretion.

## Compatibility scope

Compatibility applies to:

- CLI flags and environment variables documented for production use.
- Configuration file keys and documented defaults.
- Container image entrypoint and healthcheck behavior.
- HTTP behavior documented as stable.
- Prometheus metric names and label schemas.

Compatibility does not apply to:

- Debug endpoints.
- Benchmark tooling.
- Internal Go packages under `internal/`.
- Pre-release versions, snapshots, or unreleased commits.

## Deprecation policy

When possible, incompatible changes should follow this process:

1. Add a replacement behavior.
2. Mark the old behavior as deprecated in documentation and release notes.
3. Keep the old behavior for at least one minor release.
4. Remove the deprecated behavior in the next eligible major release, or in a pre-`v1.0.0` minor release with explicit release notes.

Security fixes may bypass the deprecation window when preserving old behavior would keep users exposed.

## Container images

Release images are published to:

- `ghcr.io/lyonbrown4d/spack`
- `lyonbrown4d/spack`

Mutable tags such as `latest`, `alpine`, and `debian` are convenience tags only. Production deployments should use immutable version tags or digests.

Base images used by SPACK release Dockerfiles must be pinned by digest. Updating a base image digest is a supply chain change and should be reviewed like a dependency update.

## Release security requirements

Release automation should include:

- `go test ./...`
- `go test -race ./...`
- `golangci-lint run ./...`
- `govulncheck ./...`
- CodeQL or equivalent SAST.
- Container vulnerability scanning.
- SPDX or CycloneDX SBOM generation.
- Container image signing.
- GitHub artifact attestations or equivalent SLSA provenance.
