# Digest locked from Docker Hub library/golang:1.26.3-alpine linux/amd64 on 2026-06-20.
FROM golang:1.26.3-alpine@sha256:f44b851aa23dfa219d18db6eab743203245429d355cb619cf96a2ffe2a84ba7a AS build

RUN apk add --no-cache upx

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w -buildid=" -o /out/spack-runtime ./cmd/spack-runtime
RUN upx --best --lzma --brute /out/spack-runtime

# Digest locked from Docker Hub library/alpine:latest on 2026-06-20.
FROM alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS alpine

WORKDIR /opt

COPY --from=build --chmod=755 /out/spack-runtime /opt/spack-runtime

USER 65532:65532

ENTRYPOINT ["/opt/spack-runtime"]

EXPOSE 80
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/opt/spack-runtime", "healthcheck", "--url", "http://127.0.0.1/livez", "--timeout", "3s"]
