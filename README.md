# SPACK

SPACK is a container-first static asset runtime for SPA and frontend build outputs.

SPACK 是面向容器化前端应用的静态资源运行时，聚焦 SPA/static assets、压缩变体、图片变体、HTTP 缓存、可观测性和高性能分发。

Runtime note: SPACK serves source assets and compiler-produced variants. `compression.mode=warmup` is the compiler pre-generation path; `compression.mode=lazy` and `compression.queue_size` remain legacy compatibility knobs rather than the recommended runtime surface.

## Documentation / 文档

Detailed user documentation and architecture design have moved to the GitHub Wiki:

详细用户文档、架构设计和图表已迁移到 GitHub Wiki：

https://github.com/lyonbrown4d/spack/wiki

Start here:

- [Quick Start / 快速开始](https://github.com/lyonbrown4d/spack/wiki/Quick-Start)
- [Configuration / 配置参考](https://github.com/lyonbrown4d/spack/wiki/Configuration)
- [Architecture / 架构设计](https://github.com/lyonbrown4d/spack/wiki/Architecture)
- [Cache and Performance / 缓存与性能](https://github.com/lyonbrown4d/spack/wiki/Cache-and-Performance)
- [Operations / 运维检查](https://github.com/lyonbrown4d/spack/wiki/Operations)
- [Reliability and Consistency / 可靠性与一致性](https://github.com/lyonbrown4d/spack/wiki/Reliability-and-Consistency-Contract)
- [Development and Release / 开发与发布](https://github.com/lyonbrown4d/spack/wiki/Development-and-Release)

## Container

```dockerfile
FROM ghcr.io/lyonbrown4d/spack:latest

COPY ./dist /app

ENV SPACK_ASSETS_ROOT=/app
ENV SPACK_ASSETS_PATH=/
ENV SPACK_ASSETS_ENTRY=index.html
ENV SPACK_ASSETS_FALLBACK_TARGET=index.html
ENV SPACK_HTTP_PORT=80
```

Release images are published to both `ghcr.io/lyonbrown4d/spack` and `lyonbrown4d/spack`.

Release archives include `spack-runtime` for Linux amd64/arm64, macOS amd64/arm64, and Windows amd64. Linux `.deb`, `.rpm`, and `.apk` packages install `spack-runtime` to `/usr/bin`. `spack-compiler` is released for Linux amd64 through GoReleaser and supplemented by the release CI matrix for Linux arm64, macOS amd64/arm64, and Windows amd64. Compiler builds use `spack_libvips` and require the target host or image to provide compatible `libvips` runtime libraries, including the AVIF encoder plugin when AVIF output is enabled.

## Local Development

```powershell
task test
task lint
task build
```

Run the local SPA fixture:

```powershell
pnpm -C test build
$env:SPACK_ASSETS_ROOT = (Resolve-Path .\test\build\dist).Path
go run ./cmd/spack-runtime
```

Run local HTTP k6 benchmarks:

```powershell
task perf:k6:frontend
task perf:k6:split
```

Run the versioned performance contract:

```powershell
task perf:contract:pr
```

Budgets and release performance policy live in `performance/`.
