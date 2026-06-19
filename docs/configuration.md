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

## Image pipeline

The default image backend is `builtin`. It is pure Go, supports JPEG/PNG input and output, and does not add CGO dependencies to the default binary.

```yaml
image:
  enable: true
  engine: builtin
  widths: "640,1280,1920"
  formats: ""
  jpeg_quality: 78
  max_source_bytes: 10485760
  max_source_pixels: 25000000
  max_width: 10000
  max_height: 10000
  max_output_variants: 12
  max_concurrent_sources: 2
  max_memory_bytes: 134217728
  min_saving_ratio: 0.05
  min_saving_bytes: 1024
```

Image generation is batched per source asset: SPACK decodes a source image once, builds a width pyramid, and encodes the requested width/format variants from that batch. Variants that do not meet the saving thresholds are not written to the artifact cache.

Future heavy backends such as libvips should ship as separate build and image flavors rather than as dependencies of the default `builtin` runtime.

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
