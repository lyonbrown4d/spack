#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

WORK_DIR="${SPACK_CONTAINER_SMOKE_DIR:-tmp/container-smoke}"
IMAGE="${SPACK_CONTAINER_SMOKE_IMAGE:-spack-container-smoke:local}"
CONTAINER="${SPACK_CONTAINER_SMOKE_CONTAINER:-spack-container-smoke-${GITHUB_RUN_ID:-local}-$$}"
PORT="${SPACK_CONTAINER_SMOKE_PORT:-18080}"
UPX_FLAGS="${SPACK_CONTAINER_SMOKE_UPX_FLAGS:---best --lzma}"

mkdir -p "$WORK_DIR/assets"
ASSETS_MOUNT="$ROOT/$WORK_DIR/assets"
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*)
    ASSETS_MOUNT="$(cygpath -w "$ASSETS_MOUNT")"
    export MSYS_NO_PATHCONV=1
    ;;
esac

cat > "$WORK_DIR/assets/index.html" <<'HTML'
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>spack smoke</title></head>
<body><script src="/app.js"></script><h1>spack smoke</h1></body>
</html>
HTML
cat > "$WORK_DIR/assets/app.js" <<'JS'
console.log("spack smoke");
JS

docker build --build-arg TARGETPLATFORM=linux/amd64 --build-arg SPACK_UPX_FLAGS="$UPX_FLAGS" -t "$IMAGE" -f docker/alpine.Dockerfile .

cleanup() {
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run -d \
  --name "$CONTAINER" \
  --read-only \
  --cap-drop=ALL \
  --security-opt no-new-privileges \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=64m \
  -p "127.0.0.1:${PORT}:8080" \
  -v "$ASSETS_MOUNT:/app:ro" \
  -e SPACK_ASSETS_ROOT=/app \
  -e SPACK_ASSETS_PATH=/ \
  -e SPACK_ASSETS_ENTRY=index.html \
  -e SPACK_ASSETS_FALLBACK_TARGET=index.html \
  -e SPACK_HTTP_PORT=8080 \
  -e SPACK_LOGGER_CONSOLE_ENABLED=true \
  "$IMAGE" >/dev/null

for attempt in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${PORT}/livez" >/dev/null; then
    break
  fi
  if [ "$attempt" -eq 30 ]; then
    docker logs "$CONTAINER" >&2 || true
    exit 1
  fi
  sleep 1
done

index_body="$(curl -fsS "http://127.0.0.1:${PORT}/")"
case "$index_body" in
  *"spack smoke"*) ;;
  *) echo "index response did not contain smoke marker" >&2; exit 1 ;;
esac

app_body="$(curl -fsS "http://127.0.0.1:${PORT}/app.js")"
case "$app_body" in
  *"spack smoke"*) ;;
  *) echo "app.js response did not contain smoke marker" >&2; exit 1 ;;
esac

RANGE_BODY="${WORK_DIR}/range-body.tmp"
range_status="$(curl -fsS -o "$RANGE_BODY" -w "%{http_code}" -H "Range: bytes=0-3" "http://127.0.0.1:${PORT}/app.js")"
rm -f "$RANGE_BODY"
if [ "$range_status" != "206" ]; then
  echo "range request expected 206, got ${range_status}" >&2
  exit 1
fi

readonly_rootfs="$(docker inspect "$CONTAINER" --format '{{.HostConfig.ReadonlyRootfs}}')"
if [ "$readonly_rootfs" != "true" ]; then
  echo "container is not running with read-only root filesystem" >&2
  exit 1
fi

cap_drop="$(docker inspect "$CONTAINER" --format '{{json .HostConfig.CapDrop}}')"
case "$cap_drop" in
  *"ALL"*) ;;
  *) echo "container did not drop all capabilities: ${cap_drop}" >&2; exit 1 ;;
esac
