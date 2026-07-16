# Linux release runner used by scripts/release-local.sh.
# It keeps local release reproducible on Windows hosts that cannot build Linux CGO/libvips binaries directly.
FROM docker:29.1.3-cli AS dockercli

FROM golang:1.26.5-bookworm

COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=dockercli /usr/local/libexec/docker/cli-plugins/docker-buildx /usr/local/libexec/docker/cli-plugins/docker-buildx

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        curl \
        file \
        git \
        libvips-dev \
        pkg-config \
        xz-utils \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL -o /tmp/upx.tar.xz https://github.com/upx/upx/releases/download/v5.2.0/upx-5.2.0-amd64_linux.tar.xz \
    && tar -C /tmp -xf /tmp/upx.tar.xz \
    && install /tmp/upx-5.2.0-amd64_linux/upx /usr/local/bin/upx \
    && rm -rf /tmp/upx*

