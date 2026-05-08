FROM debian:stable-slim AS debian

ARG TARGETPLATFORM

WORKDIR /opt

COPY ${TARGETPLATFORM}/spack /opt/spack

RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install -y --no-install-recommends ca-certificates curl dumb-init \
    && rm -rf /var/lib/apt/lists/*

RUN chmod +x /opt/spack

ENTRYPOINT ["/usr/bin/dumb-init", "--"]

EXPOSE 80
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1/livez || exit 1

CMD ["sh", "-c", "/opt/spack"]
