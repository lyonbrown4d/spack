# Digest locked from Docker Hub library/alpine:latest on 2026-06-20.
FROM alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS alpine

ARG TARGETPLATFORM

WORKDIR /opt
COPY --chmod=755 ${TARGETPLATFORM}/spack /opt/spack

USER 65532:65532

ENTRYPOINT ["/opt/spack"]

EXPOSE 80
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/opt/spack", "healthcheck", "--url", "http://127.0.0.1/livez", "--timeout", "3s"]
