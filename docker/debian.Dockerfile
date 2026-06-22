# Digest locked from Docker Hub library/debian:stable-slim on 2026-06-20.
FROM debian@sha256:34363c20bd149e41365fc77b086da067ed13ab2dff4cd0612788e12e6d52c44c AS debian

ARG TARGETPLATFORM

WORKDIR /opt

COPY --chmod=755 ${TARGETPLATFORM}/spack-runtime /opt/spack-runtime

USER 65532:65532

ENV SPACK_HTTP_PORT=8080

ENTRYPOINT ["/opt/spack-runtime"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/opt/spack-runtime", "healthcheck", "--url", "http://127.0.0.1:8080/livez", "--timeout", "3s"]
