#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TAG="${SPACK_RELEASE_TAG:-${VERSION:-}}"
if [[ -z "$TAG" ]]; then
  TAG="$(git describe --tags --abbrev=0 --match 'v[0-9]*')"
fi
case "$TAG" in
  v*) ;;
  *) TAG="v$TAG" ;;
esac

WORK_DIR="${SPACK_RELEASE_VERIFY_DIR:-tmp/release-verify}"
RUNTIME_IMAGE="${SPACK_VERIFY_RUNTIME_IMAGE:-lyonbrown4d/spack:$TAG}"
COMPILER_IMAGE="${SPACK_VERIFY_COMPILER_IMAGE:-lyonbrown4d/spack-compiler:$TAG}"
ALPINE_IMAGE="${SPACK_VERIFY_ALPINE_IMAGE:-lyonbrown4d/spack:alpine-$TAG}"
DIRECT_CONTAINER="${SPACK_VERIFY_DIRECT_CONTAINER:-spack-release-direct-$$}"
AOT_CONTAINER="${SPACK_VERIFY_AOT_CONTAINER:-spack-release-aot-$$}"
DIRECT_PORT="${SPACK_VERIFY_DIRECT_PORT:-18110}"
AOT_PORT="${SPACK_VERIFY_AOT_PORT:-18111}"
VERIFY_IMAGE_ENABLE="${SPACK_IMAGE_ENABLE:-false}"
VERIFY_IMAGE_WIDTHS="${SPACK_IMAGE_WIDTHS:-320,640}"
VERIFY_IMAGE_FORMATS="${SPACK_IMAGE_FORMATS:-webp}"

native_path() {
  local value="$1"
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$value"
    return
  fi
  printf '%s\n' "$value"
}

cleanup() {
  docker rm -f "$DIRECT_CONTAINER" "$AOT_CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "$name is required" >&2
    exit 1
  fi
}

require_command base64
require_command curl
require_command docker

prepare_assets() {
  rm -rf "$WORK_DIR/assets" "$WORK_DIR/out"
  mkdir -p "$WORK_DIR/assets/assets" "$WORK_DIR/out"
  cat > "$WORK_DIR/assets/index.html" <<'HTML'
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>spack release verify</title></head>
<body>
  <img src="/assets/hero.png" alt="hero">
  <script src="/app.js"></script>
  <h1>spack release verify</h1>
</body>
</html>
HTML
  cat > "$WORK_DIR/assets/app.js" <<'JS'
window.__SPACK_RELEASE_VERIFY__ = true;
console.log("spack release verify");
JS
  printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAANSURBVBhXY/jPwPAfAAUAAf+mXJtdAAAAAElFTkSuQmCC' | base64 -d > "$WORK_DIR/assets/assets/hero.png"
  printf '\x78\xda\x01\x02' > "$WORK_DIR/assets/assets/hero.png.gz"
}

inspect_published_images() {
  docker buildx imagetools inspect "$RUNTIME_IMAGE" >/dev/null
  docker buildx imagetools inspect "$COMPILER_IMAGE" >/dev/null
  docker buildx imagetools inspect "$ALPINE_IMAGE" >/dev/null
}

compile_bundle() {
  local assets_mount out_mount report_args=() image_args=()
  image_args=(
    "--image.enable=$VERIFY_IMAGE_ENABLE"
    "--image.widths=$VERIFY_IMAGE_WIDTHS"
    "--image.formats=$VERIFY_IMAGE_FORMATS"
  )
  assets_mount="$(native_path "$ROOT_DIR/$WORK_DIR/assets")"
  out_mount="$(native_path "$ROOT_DIR/$WORK_DIR/out")"
  if compiler_supports_report; then
    report_args=(--report /workspace/out/compile-report.json)
  fi
  MSYS_NO_PATHCONV=1 docker run --rm \
    -v "$assets_mount:/workspace/dist:ro" \
    -v "$out_mount:/workspace/out" \
    "$COMPILER_IMAGE" \
    --assets.path=/ \
    --assets.entry=index.html \
    --assets.fallback.on=not_found \
    --assets.fallback.target=index.html \
    --compression.enable=true \
    --compression.mode=warmup \
    "${image_args[@]}" \
    --frontend.resource_hints.enable=false \
    compile /workspace/dist -o /workspace/out/app.spack "${report_args[@]}"

  MSYS_NO_PATHCONV=1 docker run --rm \
    -v "$out_mount:/workspace/out:ro" \
    "$COMPILER_IMAGE" verify /workspace/out/app.spack

  if [[ "${#report_args[@]}" -gt 0 && ! -s "$WORK_DIR/out/compile-report.json" ]]; then
    echo "compiler did not write compile-report.json" >&2
    exit 1
  fi
}

compiler_supports_report() {
  MSYS_NO_PATHCONV=1 docker run --rm "$COMPILER_IMAGE" compile --help 2>/dev/null | grep -q -- '--report'
}

run_runtime_container() {
  local name="$1"
  local port="$2"
  local root_mount="$3"
  local assets_root="$4"
  local mount_path
  mount_path="$(native_path "$root_mount")"
  MSYS_NO_PATHCONV=1 docker run -d \
    --name "$name" \
    --read-only \
    --cap-drop=ALL \
    --security-opt no-new-privileges \
    --tmpfs /tmp:rw,nosuid,nodev,noexec,size=64m \
    -p "127.0.0.1:${port}:8080" \
    -v "$mount_path:$assets_root:ro" \
    -e "SPACK_ASSETS_ROOT=$assets_root" \
    -e SPACK_ASSETS_PATH=/ \
    -e SPACK_ASSETS_ENTRY=index.html \
    -e SPACK_ASSETS_FALLBACK_ON=not_found \
    -e SPACK_ASSETS_FALLBACK_TARGET=index.html \
    -e SPACK_HTTP_PORT=8080 \
    -e SPACK_COMPRESSION_ENABLE=true \
    -e SPACK_COMPRESSION_MODE=off \
    -e SPACK_IMAGE_ENABLE=false \
    -e SPACK_LOGGER_LEVEL=error \
    "$RUNTIME_IMAGE" >/dev/null
}

start_runtimes() {
  cleanup
  run_runtime_container "$DIRECT_CONTAINER" "$DIRECT_PORT" "$ROOT_DIR/$WORK_DIR/assets" /app
  run_runtime_container "$AOT_CONTAINER" "$AOT_PORT" "$ROOT_DIR/$WORK_DIR/out/app.spack" /app/app.spack
  wait_ready "$DIRECT_CONTAINER" "$DIRECT_PORT"
  wait_ready "$AOT_CONTAINER" "$AOT_PORT"
}

wait_ready() {
  local name="$1"
  local port="$2"
  for attempt in $(seq 1 30); do
    if curl -fsS "http://127.0.0.1:${port}/livez" >/dev/null; then
      return
    fi
    if [[ "$attempt" -eq 30 ]]; then
      docker logs "$name" >&2 || true
      exit 1
    fi
    sleep 1
  done
}

assert_text_response() {
  local url="$1"
  local marker="$2"
  local body
  body="$(curl -fsS "$url")"
  case "$body" in
    *"$marker"*) ;;
    *) echo "response $url did not contain $marker" >&2; exit 1 ;;
  esac
}

assert_range_response() {
  local url="$1"
  local out="$WORK_DIR/range.tmp"
  local status
  status="$(curl -fsS -o "$out" -w '%{http_code}' -H 'Range: bytes=0-3' "$url")"
  rm -f "$out"
  if [[ "$status" != "206" ]]; then
    echo "range request expected 206, got $status for $url" >&2
    exit 1
  fi
}

assert_png_response() {
  local url="$1"
  local headers="$WORK_DIR/headers.tmp"
  local body="$WORK_DIR/body.tmp"
  curl -fsS \
    -D "$headers" \
    -o "$body" \
    -H 'accept: image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8' \
    -H 'accept-language: zh-CN,zh;q=0.9,en;q=0.8' \
    -H 'sec-fetch-dest: image' \
    -H 'sec-fetch-mode: no-cors' \
    -H 'sec-fetch-site: same-origin' \
    -H 'user-agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36' \
    -H 'Accept-Encoding: gzip' \
    "$url"
  if grep -qi '^content-encoding:[[:space:]]*gzip' "$headers"; then
    echo "invalid external gzip sidecar was trusted for $url" >&2
    exit 1
  fi
  local magic
  magic="$(od -An -tx1 -N8 "$body" | tr -d ' \n')"
  rm -f "$headers" "$body"
  if [[ "$magic" != "89504e470d0a1a0a" ]]; then
    echo "expected PNG magic for $url, got $magic" >&2
    exit 1
  fi
}

assert_runtime() {
  local base="$1"
  assert_text_response "$base/" "spack release verify"
  assert_text_response "$base/app.js" "spack release verify"
  assert_range_response "$base/app.js"
  assert_png_response "$base/assets/hero.png"
}

prepare_assets
inspect_published_images
compile_bundle
start_runtimes
assert_runtime "http://127.0.0.1:$DIRECT_PORT"
assert_runtime "http://127.0.0.1:$AOT_PORT"

echo "Verified SPACK release $TAG with $RUNTIME_IMAGE and $COMPILER_IMAGE"
