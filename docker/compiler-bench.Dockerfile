# Builds a local spack-compiler image for end-to-end AOT benchmark workflows.
FROM golang:1.26.5-bookworm AS build

ARG TARGETARCH=amd64
ARG GOPROXY=https://goproxy.cn,direct
ENV DEBIAN_FRONTEND=noninteractive
ENV GOPROXY=${GOPROXY}

WORKDIR /src

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends ca-certificates pkg-config libvips-dev \
    && rm -rf /var/lib/apt/lists/*

COPY . .

RUN CGO_ENABLED=1 GOOS=linux GOARCH=${TARGETARCH} \
    go build -tags=spack_libvips -trimpath -ldflags="-s -w -buildid=" -o /out/spack-compiler ./cmd/spack-compiler

# Digest locked from Docker Hub library/debian:stable-slim on 2026-06-20.
FROM debian@sha256:34363c20bd149e41365fc77b086da067ed13ab2dff4cd0612788e12e6d52c44c AS debian

ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /workspace

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends ca-certificates libvips42 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build --chmod=755 /out/spack-compiler /usr/local/bin/spack-compiler

USER 65532:65532

ENTRYPOINT ["spack-compiler"]