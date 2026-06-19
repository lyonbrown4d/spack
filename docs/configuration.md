# SPACK configuration

This repository is the authoritative source for versioned SPACK configuration. The Wiki may link to or mirror this document, but release tags and pull requests should update files under `docs/`.

## API version

SPACK runtime configuration uses an explicit API version:

```yaml
apiVersion: spack.io/v1alpha1
kind: RuntimeConfig
```

The version gives SPACK a migration path for future field renames, default changes, and deprecations.

## Minimal config

```yaml
apiVersion: spack.io/v1alpha1
kind: RuntimeConfig

assets:
  root: /app
  path: /
  entry: index.html
  fallback:
    on: not_found
    target: index.html
```

## Deployment checks

Validate a file without starting the server:

```bash
spack config validate --file spack.yaml
```

Print the effective configuration after merging defaults, config files, environment variables, and CLI flags:

```bash
spack config print-effective --file spack.yaml --redact
```

`--redact` hides local filesystem paths such as `assets.root`, `compression.cache_dir`, and `logger.file.path`.
