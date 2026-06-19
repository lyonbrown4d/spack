# Digest locked from Docker Hub library/debian:stable-slim on 2026-06-20.
FROM debian@sha256:34363c20bd149e41365fc77b086da067ed13ab2dff4cd0612788e12e6d52c44c AS debian

ARG TARGETPLATFORM

WORKDIR /opt

COPY --chmod=755 ${TARGETPLATFORM}/spack /opt/spack

USER 65532:65532

ENTRYPOINT ["/opt/spack"]

EXPOSE 80
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/opt/spack", "healthcheck", "--url", "http://127.0.0.1/livez", "--timeout", "3s"]
