# Digest locked from Docker Hub library/golang:1.26.5-alpine linux/amd64 on 2026-07-15.
FROM golang:1.26.5-alpine@sha256:111d79159b2326f7e80c4a4706e1ba166acb0e2611df853955f3621828cd49e8 AS build

RUN apk add --no-cache upx

WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg

ENV CGO_ENABLED=0
ARG SPACK_UPX_FLAGS="--best --lzma --brute"
RUN go build -trimpath -ldflags="-s -w -buildid=" -o /out/spack-runtime ./cmd/spack-runtime
RUN if [ "$SPACK_UPX_FLAGS" != "none" ]; then upx $SPACK_UPX_FLAGS /out/spack-runtime; fi

# Digest locked from Docker Hub library/alpine:latest on 2026-06-20.
FROM alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS alpine

WORKDIR /opt

COPY --from=build --chmod=755 /out/spack-runtime /opt/spack-runtime

USER 65532:65532

ENV SPACK_HTTP_PORT=8080

ENTRYPOINT ["/opt/spack-runtime"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/opt/spack-runtime", "healthcheck", "--url", "http://127.0.0.1:8080/livez", "--timeout", "3s"]
