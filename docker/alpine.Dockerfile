FROM alpine:latest AS alpine

ARG TARGETPLATFORM

RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates curl dumb-init \
    && adduser -D -g '' appuser

WORKDIR /opt
COPY ${TARGETPLATFORM}/spack /opt/spack

RUN chmod +x /opt/spack

USER appuser

ENTRYPOINT ["/usr/bin/dumb-init", "--"]

EXPOSE 80
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1/livez || exit 1

CMD ["sh", "-c", "/opt/spack"]
