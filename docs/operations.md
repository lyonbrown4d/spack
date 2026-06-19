# Operations

## Inspect assets before deployment

Use `inspect` to scan an asset directory without starting the HTTP server:

```bash
spack inspect --assets /app
```

The report includes:

- Resource count.
- Source sidecar count.
- Total source and asset bytes.
- Compression savings from existing `.br`, `.zst`, and `.gz` sidecars.
- Image assets that can generate configured variants.
- Estimated memory-cache warm bytes.
- Potential issues such as missing entry or fallback assets.

The command emits JSON so it can be used in CI/CD admission checks.

## Recommended preflight

```bash
spack config validate --file spack.yaml
spack config print-effective --file spack.yaml --redact
spack inspect --assets /app
```

Treat warnings from `inspect` as deployment review items, especially missing `assets.entry`, missing fallback targets, and cache budgets that are smaller than the scanned asset set.
