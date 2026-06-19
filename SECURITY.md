# Security Policy

## Supported versions

SPACK follows semantic versioning for release support.

| Version line | Security support |
| --- | --- |
| Latest stable minor release | Supported |
| Previous stable minor release | Supported for 90 days after the next minor release |
| Older minor releases | Not supported |
| Pre-release builds, snapshots, and `main` | Not supported for production use |

Until SPACK reaches `v1.0.0`, minor releases may include breaking changes. Security fixes are backported only when the affected release line is still inside the support window above.

## Reporting a vulnerability

Use GitHub Private Vulnerability Reporting for this repository. Do not report suspected vulnerabilities in public issues, discussions, or pull requests.

If Private Vulnerability Reporting is unavailable, contact a maintainer privately and include enough detail to reproduce the issue. Repository administrators should enable Private Vulnerability Reporting in GitHub repository settings.

Please include:

- Affected SPACK version, commit, or container image digest.
- Deployment context, including configuration relevant to the issue.
- Reproduction steps, proof of concept, logs, or packet captures when available.
- Impact assessment, including whether the issue is remotely exploitable.
- Any known workarounds.

## Response targets

SPACK maintainers target the following response windows:

| Severity | Initial response | Triage target | Fix target |
| --- | --- | --- | --- |
| Critical | 2 business days | 5 business days | 14 calendar days |
| High | 3 business days | 7 business days | 30 calendar days |
| Medium | 5 business days | 14 calendar days | Next regular release |
| Low | 10 business days | 30 calendar days | Best effort |

These are targets, not guarantees. Maintainers may adjust timelines based on exploitability, affected versions, and dependency availability.

## Disclosure process

1. Maintainers acknowledge the report privately.
2. Maintainers reproduce and classify the issue.
3. If accepted, maintainers prepare a fix on a private branch or security advisory fork.
4. Maintainers coordinate disclosure timing with the reporter when practical.
5. A patched release is published with release notes, fixed versions, and mitigation guidance.
6. The advisory is published after the fixed release is available.

Reports that are not vulnerabilities will be closed with an explanation when possible.

## Supply chain expectations

Security-sensitive release changes should preserve:

- Pinned GitHub Actions by commit SHA.
- Pinned container base images by digest.
- Vulnerability scanning with `govulncheck` and container scanning.
- SBOM generation for release images.
- Signed images and provenance attestations for release artifacts.
