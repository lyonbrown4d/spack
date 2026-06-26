# Digest locked from Docker Hub library/debian:stable-slim on 2026-06-20.
FROM debian@sha256:34363c20bd149e41365fc77b086da067ed13ab2dff4cd0612788e12e6d52c44c AS debian

ARG TARGETPLATFORM

WORKDIR /opt

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libvips42 \
    && rm -rf /var/lib/apt/lists/*

COPY --chmod=755 ${TARGETPLATFORM}/spack-compiler /opt/spack-compiler

USER 65532:65532

ENTRYPOINT ["/opt/spack-compiler"]