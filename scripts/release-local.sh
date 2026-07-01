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

GORELEASER_VERSION="${GORELEASER_VERSION:-v2.15.4}"
RELEASE_RUNNER_IMAGE="${SPACK_RELEASE_RUNNER_IMAGE:-spack-release-runner:local}"
GOPROXY_VALUE="${GOPROXY:-https://goproxy.cn,direct}"
RUNTIME_PLATFORMS="${SPACK_RUNTIME_PLATFORMS:-linux/amd64,linux/arm64}"
COMPILER_PLATFORMS="${SPACK_COMPILER_PLATFORMS:-linux/amd64}"
ALPINE_PLATFORMS="${SPACK_ALPINE_PLATFORMS:-linux/amd64}"
PUSH_GHCR="${SPACK_PUSH_GHCR:-true}"
PUSH_DOCKER_HUB="${SPACK_PUSH_DOCKER_HUB:-true}"
SKIP_GORELEASER="${SPACK_RELEASE_SKIP_GORELEASER:-false}"

native_path() {
  local value="$1"
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$value"
    return
  fi
  printf '%s\n' "$value"
}

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "$name is required" >&2
    exit 1
  fi
}

require_command docker
require_command git
require_command gh

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  GITHUB_TOKEN="$(gh auth token)"
  export GITHUB_TOKEN
fi
if [[ -z "$GITHUB_TOKEN" ]]; then
  echo "GITHUB_TOKEN is required" >&2
  exit 1
fi

build_release_runner() {
  docker build -t "$RELEASE_RUNNER_IMAGE" -f docker/release-runner.Dockerfile .
}

run_goreleaser() {
  if [[ "$SKIP_GORELEASER" == "true" ]]; then
    echo "Skipping GoReleaser because SPACK_RELEASE_SKIP_GORELEASER=true"
    return
  fi

  local repo_mount
  repo_mount="$(native_path "$ROOT_DIR")"
  MSYS_NO_PATHCONV=1 docker run --rm \
    -e GITHUB_TOKEN \
    -e "GOPROXY=$GOPROXY_VALUE" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$repo_mount:/src" \
    -w /src \
    "$RELEASE_RUNNER_IMAGE" \
    bash -c "set -euo pipefail; git config --global --add safe.directory /src; go run github.com/goreleaser/goreleaser/v2@${GORELEASER_VERSION} release --clean --skip=docker"
}

prepare_docker_context() {
  mkdir -p dist/linux/amd64 dist/linux/arm64
  cp -f dist/spack-runtime-linux_linux_amd64_v1/spack-runtime dist/linux/amd64/spack-runtime
  if [[ -f dist/spack-runtime-linux_linux_arm64_v8.0/spack-runtime ]]; then
    cp -f dist/spack-runtime-linux_linux_arm64_v8.0/spack-runtime dist/linux/arm64/spack-runtime
  fi
  cp -f dist/spack-compiler-linux_linux_amd64_v1/spack-compiler dist/linux/amd64/spack-compiler
  cp -f docker/debian.Dockerfile dist/Dockerfile.debian
  cp -f docker/compiler-debian.Dockerfile dist/Dockerfile.compiler-debian
}

registry_tags() {
  local image="$1"
  local suffixes="$2"
  local tags=()
  IFS=',' read -ra parts <<< "$suffixes"
  for part in "${parts[@]}"; do
    tags+=("-t" "$image:$part")
  done
  printf '%s\0' "${tags[@]}"
}

build_tags() {
  local ghcr_image="$1"
  local docker_image="$2"
  local suffixes="$3"
  local tags=()
  if [[ "$PUSH_GHCR" == "true" ]]; then
    while IFS= read -r -d '' item; do tags+=("$item"); done < <(registry_tags "$ghcr_image" "$suffixes")
  fi
  if [[ "$PUSH_DOCKER_HUB" == "true" ]]; then
    while IFS= read -r -d '' item; do tags+=("$item"); done < <(registry_tags "$docker_image" "$suffixes")
  fi
  printf '%s\0' "${tags[@]}"
}

run_buildx() {
  local platforms="$1"
  local dockerfile="$2"
  local context="$3"
  shift 3
  docker buildx build --platform "$platforms" -f "$dockerfile" --push "$@" "$context"
}

push_runtime_debian() {
  local tags=()
  while IFS= read -r -d '' item; do tags+=("$item"); done < <(build_tags "ghcr.io/lyonbrown4d/spack" "lyonbrown4d/spack" "$TAG,latest,debian,debian-$TAG")
  run_buildx "$RUNTIME_PLATFORMS" dist/Dockerfile.debian dist "${tags[@]}"
}

push_compiler_debian() {
  local tags=()
  while IFS= read -r -d '' item; do tags+=("$item"); done < <(build_tags "ghcr.io/lyonbrown4d/spack-compiler" "lyonbrown4d/spack-compiler" "$TAG,latest,debian,debian-$TAG")
  run_buildx "$COMPILER_PLATFORMS" dist/Dockerfile.compiler-debian dist "${tags[@]}"
}

push_runtime_alpine() {
  local tags=()
  while IFS= read -r -d '' item; do tags+=("$item"); done < <(build_tags "ghcr.io/lyonbrown4d/spack" "lyonbrown4d/spack" "alpine,alpine-$TAG")
  run_buildx "$ALPINE_PLATFORMS" docker/alpine.Dockerfile . "${tags[@]}"
}

verify_manifests() {
  docker buildx imagetools inspect "ghcr.io/lyonbrown4d/spack:$TAG" >/dev/null
  docker buildx imagetools inspect "lyonbrown4d/spack:$TAG" >/dev/null
  docker buildx imagetools inspect "ghcr.io/lyonbrown4d/spack-compiler:$TAG" >/dev/null
  docker buildx imagetools inspect "lyonbrown4d/spack-compiler:$TAG" >/dev/null
  gh api /user/packages/container/spack --jq '{name: .name, visibility: .visibility}'
  gh api /user/packages/container/spack-compiler --jq '{name: .name, visibility: .visibility}'
}

build_release_runner
run_goreleaser
prepare_docker_context
push_runtime_debian
push_compiler_debian
push_runtime_alpine
verify_manifests

echo "Published SPACK release $TAG"

