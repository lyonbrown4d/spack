#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCH_SCRIPT="${SCRIPT_DIR}/refine-aot-bench.sh"

usage() {
  cat <<'USAGE'
Usage: scripts/refine-aot-bench-matrix.sh <matrix|repeat>

Environment:
  REFINE_AOT_CONCURRENCIES
                        Comma-separated VUS values for matrix mode.
                        Defaults to 1,8,64,256,1024.
  SPACK_AOT_REPEAT_ROUNDS
                        Repeat rounds for repeat mode. Defaults to 3.
  K6_VUS               VUS for repeat mode. Defaults to 64.
  K6_DURATION          Duration for each run. Defaults to 30s.
  REFINE_BENCH_TARGETS  comma-separated: direct,aot,caddy,nginx. Defaults to direct,aot,caddy,nginx.
  SPACK_IMAGE_ENABLE    Enable compiler image variants. Defaults to false.
  SPACK_IMAGE_WIDTHS    Compiler width list. Defaults to 320,640.
  SPACK_IMAGE_FORMATS   Compiler format list. Defaults to webp.
USAGE
}

parse_concurrencies() {
  local raw="${REFINE_AOT_CONCURRENCIES:-1,8,64,256,1024}"
  local -a values=()
  local value

  IFS=',' read -r -a values <<< "$raw"
  for value in "${values[@]}"; do
    value="${value//[[:space:]]/}"
    if [[ -z "$value" ]]; then
      continue
    fi
    if ! [[ "$value" =~ ^[0-9]+$ ]]; then
      echo "invalid concurrency value: $value" >&2
      exit 1
    fi
    printf '%s\n' "$value"
  done
}

run_matrix() {
  local concurrency

  bash "$BENCH_SCRIPT" prepare

  while IFS= read -r concurrency; do
    REFINE_AOT_SKIP_PREPARE=true REFINE_AOT_RUN_TAG="vus-${concurrency}" K6_VUS="$concurrency" bash "$BENCH_SCRIPT" perf
  done < <(parse_concurrencies)
}

run_repeat() {
  local rounds="${SPACK_AOT_REPEAT_ROUNDS:-3}"
  local rounds_i

  if ! [[ "$rounds" =~ ^[1-9][0-9]*$ ]]; then
    echo "SPACK_AOT_REPEAT_ROUNDS must be a positive integer, got: $rounds" >&2
    exit 1
  fi

  bash "$BENCH_SCRIPT" prepare

  for ((rounds_i = 1; rounds_i <= rounds; rounds_i++)); do
    REFINE_AOT_SKIP_PREPARE=true REFINE_AOT_RUN_TAG="round-${rounds_i}" bash "$BENCH_SCRIPT" perf
  done
}

main() {
  case "${1:-matrix}" in
    matrix)
      run_matrix
      ;;
    repeat)
      run_repeat
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
}

main "$@"
