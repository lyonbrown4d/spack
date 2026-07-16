#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="${REFINE_AOT_DIR:-"$ROOT_DIR/tmp/refine-aot"}"
CUSTOM_SOURCE_DIR="${REFINE_SOURCE_DIR:-}"
SOURCE_DIR="${CUSTOM_SOURCE_DIR:-"$WORK_DIR/source"}"
SOURCE_MARKER="$SOURCE_DIR/.spack-refine-example"
REPO_DIR="$WORK_DIR/refine-repo"
DIST_DIR="$WORK_DIR/dist"
EXTRA_DIR="$WORK_DIR/extra-assets"
CACHE_DIR="$WORK_DIR/cache"
BUNDLE_PATH="$WORK_DIR/app.spack"
PATHS_ENV="$WORK_DIR/paths.env"
COMPOSE_FILE="$ROOT_DIR/deploy/bench/docker-compose.refine-aot.yml"
RESULTS_DIR="$ROOT_DIR/tmp/k6/results"
BENCH_GOARCH="${BENCH_GOARCH:-amd64}"
RUNTIME_IMAGE="${SPACK_RUNTIME_BENCH_IMAGE:-spack-k6-bench:local}"
COMPILER_IMAGE="${SPACK_COMPILER_BENCH_IMAGE:-spack-compiler-bench:local}"
SPACK_RUNTIME_BENCH_BUILD="${SPACK_RUNTIME_BENCH_BUILD:-true}"
SPACK_COMPILER_BENCH_BUILD="${SPACK_COMPILER_BENCH_BUILD:-true}"
export SPACK_RUNTIME_BENCH_IMAGE="$RUNTIME_IMAGE"
REFINE_EXAMPLE_REPO="${REFINE_EXAMPLE_REPO:-https://github.com/refinedev/refine.git}"
REFINE_EXAMPLE_REF="${REFINE_EXAMPLE_REF:-@refinedev/core@5.0.12}"
REFINE_EXAMPLE_PATH="${REFINE_EXAMPLE_PATH:-examples/app-crm-minimal}"
REFINE_PACKAGE_MANAGER="${REFINE_PACKAGE_MANAGER:-auto}"
REFINE_FIXTURE_PACKAGES="${REFINE_FIXTURE_PACKAGES:-date-fns lodash-es @types/lodash-es nanoid recharts zod}"
K6_VUS="${K6_VUS:-}"
K6_DURATION="${K6_DURATION:-}"
ACCEPT_ENCODING="${ACCEPT_ENCODING:-br,gzip}"
SPACK_IMAGE_ENABLE="${SPACK_IMAGE_ENABLE:-false}"
SPACK_IMAGE_WIDTHS="${SPACK_IMAGE_WIDTHS:-320,640}"
SPACK_IMAGE_FORMATS="${SPACK_IMAGE_FORMATS:-webp}"
REFINE_BENCH_TARGETS="${REFINE_BENCH_TARGETS:-direct,aot}"
REFINE_AOT_SKIP_PREPARE="${REFINE_AOT_SKIP_PREPARE:-false}"
REFINE_AOT_RUN_TAG="${REFINE_AOT_RUN_TAG:-}"

cd "$ROOT_DIR"
if [[ -d /usr/bin ]]; then
  export PATH="/usr/bin:$PATH"
fi

usage() {
  cat <<'USAGE'
Usage: scripts/refine-aot-bench.sh <prepare|smoke|perf|stress|baseline|down>

Environment:
  REFINE_SOURCE_DIR    Existing Refine project to build instead of cloning an example.
  REFINE_EXAMPLE_PATH  Example path inside refinedev/refine. Default: examples/app-crm-minimal.
  REFINE_EXAMPLE_REF   Git ref for refinedev/refine. Default: @refinedev/core@5.0.12.
  REFINE_FIXTURE_PACKAGES
                      Extra npm packages imported by the benchmark fixture entry.
  REFINE_BUILD_MODE    Build mode: vite or package. Default: vite.
  SPACK_RUNTIME_BENCH_IMAGE
                      Runtime image used by direct and AOT containers. Default: spack-k6-bench:local.
  SPACK_COMPILER_BENCH_IMAGE
                      Compiler image used to produce the .spack bundle in Docker. Default: spack-compiler-bench:local.
  SPACK_RUNTIME_BENCH_BUILD
                      Build the local runtime image before running. Default: true.
  SPACK_COMPILER_BENCH_BUILD
                      Build the local compiler image before running. Default: true.
  K6_VUS               k6 virtual users. Default: 64, Taskfile smoke defaults to 1.
  K6_DURATION          k6 duration. Default: 30s, Taskfile smoke defaults to 5s.

  ACCEPT_ENCODING      Accept-Encoding header for k6. Default: br,gzip.
  REFINE_BENCH_TARGETS
                      Benchmark targets, comma-separated: direct,aot[,caddy,nginx].
                      Default: direct,aot.
  SPACK_IMAGE_ENABLE   Enable compiler-side image variant generation. Default: false.
  SPACK_IMAGE_WIDTHS   Compiler-side responsive image widths. Default: 320,640.
  SPACK_IMAGE_FORMATS  Compiler-side image output formats. Default: webp.
USAGE
}

main() {
  local command="${1:-smoke}"
  case "$command" in
    prepare)
      prepare
      ;;
    smoke)
      K6_VUS="${K6_VUS:-1}"
      K6_DURATION="${K6_DURATION:-5s}"
      run_workload "smoke"
      ;;
    perf)
      K6_VUS="${K6_VUS:-64}"
      K6_DURATION="${K6_DURATION:-30s}"
      run_workload "perf"
      ;;
    stress)
      K6_VUS="${K6_VUS:-256}"
      K6_DURATION="${K6_DURATION:-2m}"
      run_workload "stress"
      ;;
    baseline)
      run_baseline
      ;;
    down)
      down
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

prepare() {
  require_command git
  require_command go
  require_command docker
  require_command curl
  require_node_package_manager

  mkdir -p "$WORK_DIR" "$RESULTS_DIR" "$ROOT_DIR/tmp/k6/linux/$BENCH_GOARCH"
  prepare_refine_source
  build_refine_dist
  enrich_dist_assets
  write_paths_env
  build_runtime_image
  build_compiler_image
  compile_aot_bundle
}

run_workload() {
  local mode="$1"
  local run_tag="${REFINE_AOT_RUN_TAG:-$mode}"
  local target
  local url
  local prefix

  if [[ "$REFINE_AOT_SKIP_PREPARE" != "true" ]]; then
    prepare
  fi
  up
  trap down EXIT
  write_startup_sample "$run_tag"

  local -a targets=()
  mapfile -t targets < <(collect_bench_targets)
  for target in "${targets[@]}"; do
    url="$(resolve_target_url "$target")"
    prefix="refine-${target}"
    run_frontend_k6 "$prefix" "$url" "${prefix}-${run_tag}.json"
    run_static_k6 "$prefix" "$url" "${prefix}-static-${run_tag}.json"
  done
}

collect_bench_targets() {
  local raw="${REFINE_BENCH_TARGETS:-direct,aot}"
  local target
  local -a targets=()
  local found=0

  IFS=',' read -r -a targets <<< "$raw"
  for target in "${targets[@]}"; do
    target="${target//[[:space:]]/}"
    case "$target" in
      direct|aot|caddy|nginx)
        printf '%s\n' "$target"
        found=1
        ;;
      "")
        ;;
      *)
        echo "unsupported target '$target' in REFINE_BENCH_TARGETS" >&2
        return 1
        ;;
    esac
  done

  if [[ $found -eq 0 ]]; then
    printf '%s\n' direct
    printf '%s\n' aot
  fi
}

resolve_target_service() {
  case "$1" in
    direct)
      echo "spack-direct"
      ;;
    aot)
      echo "spack-aot"
      ;;
    caddy)
      echo "caddy"
      ;;
    nginx)
      echo "nginx"
      ;;
    *)
      return 1
      ;;
  esac
}

resolve_target_url() {
  case "$1" in
    direct)
      echo "http://spack-direct:80"
      ;;
    aot)
      echo "http://spack-aot:80"
      ;;
    caddy)
      echo "http://caddy:80"
      ;;
    nginx)
      echo "http://nginx:8080"
      ;;
    *)
      return 1
      ;;
  esac
}

resolve_target_readiness_urls() {
  case "$1" in
    direct)
      echo "http://127.0.0.1:18082/livez"
      ;;
    aot)
      echo "http://127.0.0.1:18083/livez"
      ;;
  esac
}

collect_bench_services() {
  local target service
  while read -r target; do
    service="$(resolve_target_service "$target")"
    if [[ -z "$service" ]]; then
      return 1
    fi
    printf '%s\n' "$service"
  done < <(collect_bench_targets)
}

prepare_refine_source() {
  if [[ -n "$CUSTOM_SOURCE_DIR" && -f "$SOURCE_DIR/package.json" ]]; then
    return
  fi
  if [[ -f "$SOURCE_DIR/package.json" ]] && source_marker_matches; then
    return
  fi

  rm -rf "$SOURCE_DIR" "$REPO_DIR"
  mkdir -p "$WORK_DIR"
  git clone --depth=1 --filter=blob:none --sparse --branch "$REFINE_EXAMPLE_REF" "$REFINE_EXAMPLE_REPO" "$REPO_DIR"
  git -C "$REPO_DIR" sparse-checkout set "$REFINE_EXAMPLE_PATH"

  if [[ ! -f "$REPO_DIR/$REFINE_EXAMPLE_PATH/package.json" ]]; then
    echo "Refine example $REFINE_EXAMPLE_PATH does not contain package.json" >&2
    exit 1
  fi

  mkdir -p "$SOURCE_DIR"
  cp -a "$REPO_DIR/$REFINE_EXAMPLE_PATH/." "$SOURCE_DIR/"
  write_source_marker
}

source_marker_matches() {
  [[ -f "$SOURCE_MARKER" ]] || return 1
  grep -Fx "repo=$REFINE_EXAMPLE_REPO" "$SOURCE_MARKER" >/dev/null 2>&1 &&
    grep -Fx "ref=$REFINE_EXAMPLE_REF" "$SOURCE_MARKER" >/dev/null 2>&1 &&
    grep -Fx "path=$REFINE_EXAMPLE_PATH" "$SOURCE_MARKER" >/dev/null 2>&1
}

write_source_marker() {
  cat >"$SOURCE_MARKER" <<EOF
repo=$REFINE_EXAMPLE_REPO
ref=$REFINE_EXAMPLE_REF
path=$REFINE_EXAMPLE_PATH
EOF
}

build_refine_dist() {
  pushd "$SOURCE_DIR" >/dev/null
  install_refine_dependencies
  build_refine_application
  popd >/dev/null

  local built_dir=""
  if [[ -d "$SOURCE_DIR/dist" ]]; then
    built_dir="$SOURCE_DIR/dist"
  elif [[ -d "$SOURCE_DIR/build" ]]; then
    built_dir="$SOURCE_DIR/build"
  else
    echo "Refine build did not produce dist/ or build/" >&2
    exit 1
  fi

  rm -rf "$DIST_DIR"
  mkdir -p "$DIST_DIR"
  cp -a "$built_dir/." "$DIST_DIR/"
}

enrich_dist_assets() {
  rm -rf "$EXTRA_DIR"
  go run ./cmd/benchassets --out "$EXTRA_DIR" --goarch "$BENCH_GOARCH"

  rm -rf "$DIST_DIR/spack-bench"
  mkdir -p "$DIST_DIR/spack-bench"
  cp -a "$EXTRA_DIR/." "$DIST_DIR/spack-bench/"
}

write_paths_env() {
  local asset_paths
  asset_paths="$(find_relative_paths "$DIST_DIR" | grep -v '^/index.html$' | paste -sd, -)"
  if [[ -z "$asset_paths" ]]; then
    asset_paths="/index.html"
  fi

  local bench_paths="/,/index.html,/products,/categories"
  bench_paths="$bench_paths,$asset_paths"

  cat >"$PATHS_ENV" <<EOF
REFINE_K6_PAGE_PATH=/
REFINE_K6_PATHS=$bench_paths
REFINE_K6_ASSET_PATHS=$asset_paths
EOF
}

compile_aot_bundle() {
  local compiler_dist_dir compiler_cache_dir compiler_out_dir
  compiler_dist_dir="$(native_path "$DIST_DIR")"
  compiler_cache_dir="$(native_path "$CACHE_DIR")"
  compiler_out_dir="$(native_path "$WORK_DIR")"

  rm -rf "$CACHE_DIR"
  mkdir -p "$CACHE_DIR"
  rm -f "$BUNDLE_PATH"
  chmod 0777 "$CACHE_DIR" "$WORK_DIR" 2>/dev/null || true

  local user_args=()
  if command -v id >/dev/null 2>&1; then
    user_args=(--user "$(id -u):$(id -g)")
  fi

  MSYS2_ARG_CONV_EXCL="*" docker run --rm \
    "${user_args[@]}" \
    -v "$compiler_dist_dir:/workspace/dist:ro" \
    -v "$compiler_cache_dir:/workspace/cache" \
    -v "$compiler_out_dir:/workspace/out" \
    "$COMPILER_IMAGE" \
    --assets.path=/ \
    --assets.entry=index.html \
    --assets.fallback.on=not_found \
    --assets.fallback.target=index.html \
    --compression.enable=true \
    --compression.mode=warmup \
    --compression.cache_dir=/workspace/cache \
    --image.enable="$SPACK_IMAGE_ENABLE" \
    --image.widths="$SPACK_IMAGE_WIDTHS" \
    --image.formats="$SPACK_IMAGE_FORMATS" \
    --frontend.resource_hints.enable=false \
    compile /workspace/dist -o /workspace/out/app.spack

  chmod 0644 "$BUNDLE_PATH" 2>/dev/null || true

  if [[ ! -f "$BUNDLE_PATH" ]]; then
    echo "Compiler container did not produce $BUNDLE_PATH" >&2
    exit 1
  fi
}

write_startup_sample() {
  local mode="$1"
  local target url ready_ms output
  local payload

  output="$RESULTS_DIR/refine-aot-startup-${mode}.json"
  payload='{"mode":"'$mode'"'

  while read -r target; do
    url="$(resolve_target_readiness_urls "$target")"
    if [[ -z "$url" ]]; then
      continue
    fi
    ready_ms="$(measure_ready_millis "$url")"
    payload="$payload,\"${target}_ready_ms\":$ready_ms"
  done < <(collect_bench_targets)

  payload="$payload,\"generated_at\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
  echo "$payload" > "$output"
  echo "Wrote $output"
}

measure_ready_millis() {
  local url="$1"
  local started_at finished_at
  started_at="$(node -e 'console.log(Date.now())')"
  for _ in $(seq 1 100); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      finished_at="$(node -e 'console.log(Date.now())')"
      echo $((finished_at - started_at))
      return
    fi
    sleep 0.1
  done
  echo "runtime did not become ready: $url" >&2
  exit 1
}

run_baseline() {
  local rounds="${SPACK_PERF_ROUNDS:-3}"
  if ! [[ "$rounds" =~ ^[0-9]+$ ]] || (( rounds < 1 )); then
    echo "SPACK_PERF_ROUNDS must be a positive integer, got: $rounds" >&2
    exit 1
  fi

  export K6_VUS="${K6_VUS:-256}"
  export K6_DURATION="${K6_DURATION:-2m}"
  export ACCEPT_ENCODING="${ACCEPT_ENCODING:-br,gzip}"

  local round mode
  for ((round = 1; round <= rounds; round++)); do
    mode="baseline-r${round}"
    echo "Running Refine AOT baseline sample ${round}/${rounds} with K6_VUS=$K6_VUS K6_DURATION=$K6_DURATION"
    down
    run_workload "$mode"
    down
  done

  write_baseline_summary "$rounds"
}

write_baseline_summary() {
  local rounds="$1"
  node - "$RESULTS_DIR" "$rounds" "$K6_VUS" "$K6_DURATION" <<'NODE'
const fs = require("fs");
const path = require("path");

const [resultsDir, roundsRaw, vusRaw, duration] = process.argv.slice(2);
const rounds = Number.parseInt(roundsRaw, 10);
const vus = Number.parseInt(vusRaw, 10);
const scenarios = [
  ["refine-direct", "direct page"],
  ["refine-aot", "AOT page"],
  ["refine-direct-static", "direct static"],
  ["refine-aot-static", "AOT static"],
];
const metrics = [
  ["failed_rate", (body) => valueAt(body, ["metrics", "http_req_failed", "rate"]) ?? valueAt(body, ["metrics", "http_req_failed", "value"])],
  ["reqs_per_sec", (body) => valueAt(body, ["metrics", "http_reqs", "rate"])],
  ["iters_per_sec", (body) => valueAt(body, ["metrics", "iterations", "rate"])],
  ["p95_ms", (body) => valueAt(body, ["metrics", "http_req_duration", "p(95)"])],
  ["p99_ms", (body) => valueAt(body, ["metrics", "http_req_duration", "p(99)"])],
];

function valueAt(body, parts) {
  let current = body;
  for (const part of parts) {
    if (current == null || typeof current !== "object" || !(part in current)) {
      return null;
    }
    current = current[part];
  }
  return Number.isFinite(current) ? current : null;
}

function summarize(values) {
  const samples = values.filter(Number.isFinite);
  if (samples.length === 0) {
    return { samples: 0, min: null, max: null, mean: null, stdev: null };
  }
  const sum = samples.reduce((acc, value) => acc + value, 0);
  const mean = sum / samples.length;
  const variance = samples.reduce((acc, value) => acc + (value - mean) ** 2, 0) / samples.length;
  return {
    samples: samples.length,
    min: Math.min(...samples),
    max: Math.max(...samples),
    mean,
    stdev: Math.sqrt(variance),
  };
}

function round(value, digits = 3) {
  if (!Number.isFinite(value)) {
    return null;
  }
  const factor = 10 ** digits;
  return Math.round(value * factor) / factor;
}

function fmt(value, digits = 2) {
  if (!Number.isFinite(value)) {
    return "n/a";
  }
  return value.toFixed(digits);
}

const summary = {
  generatedAt: new Date().toISOString(),
  rounds,
  vus: Number.isFinite(vus) ? vus : vusRaw,
  duration,
  scenarios: {},
};

for (const [prefix, label] of scenarios) {
  const scenario = { label, files: [], metrics: {} };
  const valuesByMetric = Object.fromEntries(metrics.map(([name]) => [name, []]));
  for (let round = 1; round <= rounds; round += 1) {
    const fileName = `${prefix}-baseline-r${round}.json`;
    const filePath = path.join(resultsDir, fileName);
    if (!fs.existsSync(filePath)) {
      throw new Error(`missing k6 summary: ${filePath}`);
    }
    const body = JSON.parse(fs.readFileSync(filePath, "utf8"));
    scenario.files.push(fileName);
    for (const [name, pick] of metrics) {
      valuesByMetric[name].push(pick(body));
    }
  }
  for (const [name] of metrics) {
    const stat = summarize(valuesByMetric[name]);
    scenario.metrics[name] = {
      samples: stat.samples,
      min: round(stat.min),
      max: round(stat.max),
      mean: round(stat.mean),
      stdev: round(stat.stdev),
    };
  }
  summary.scenarios[prefix] = scenario;
}

const jsonPath = path.join(resultsDir, "refine-aot-baseline-summary.json");
fs.writeFileSync(jsonPath, `${JSON.stringify(summary, null, 2)}\n`);

const lines = [
  "# Refine AOT performance baseline",
  "",
  `Generated at: ${summary.generatedAt}`,
  `Rounds: ${summary.rounds}`,
  `VUs: ${summary.vus}`,
  `Duration: ${summary.duration}`,
  "",
  "| scenario | failed max | req/s mean | req/s stdev | p95 mean | p99 mean |",
  "|---|---:|---:|---:|---:|---:|",
];
for (const [prefix] of scenarios) {
  const scenario = summary.scenarios[prefix];
  lines.push(
    `| ${scenario.label} | ${fmt(scenario.metrics.failed_rate.max, 6)} | ${fmt(scenario.metrics.reqs_per_sec.mean)} | ${fmt(scenario.metrics.reqs_per_sec.stdev)} | ${fmt(scenario.metrics.p95_ms.mean)}ms | ${fmt(scenario.metrics.p99_ms.mean)}ms |`,
  );
}
lines.push("");
lines.push("Raw k6 summaries are stored next to this file as `*-baseline-rN.json`.");

const markdownPath = path.join(resultsDir, "refine-aot-baseline-summary.md");
fs.writeFileSync(markdownPath, `${lines.join("\n")}\n`);
console.log(`Wrote ${jsonPath}`);
console.log(`Wrote ${markdownPath}`);
NODE
}
build_runtime_image() {
  if [[ "$SPACK_RUNTIME_BENCH_BUILD" != "true" ]]; then
    return
  fi

  CGO_ENABLED=0 GOOS=linux GOARCH="$BENCH_GOARCH" \
    go build -trimpath -ldflags="-s -w -buildid=" -o "$ROOT_DIR/tmp/k6/linux/$BENCH_GOARCH/spack-runtime" ./cmd/spack-runtime

  docker build \
    --build-arg "TARGETPLATFORM=linux/$BENCH_GOARCH" \
    -t "$RUNTIME_IMAGE" \
    -f "$ROOT_DIR/docker/debian.Dockerfile" \
    "$ROOT_DIR/tmp/k6"
}

build_compiler_image() {
  if [[ "$SPACK_COMPILER_BENCH_BUILD" != "true" ]]; then
    return
  fi

  docker build \
    --platform "linux/$BENCH_GOARCH" \
    -t "$COMPILER_IMAGE" \
    -f "$ROOT_DIR/docker/compiler-bench.Dockerfile" \
    "$ROOT_DIR"
}

up() {
  local compose_file
  local -a services=()
  local service
  compose_file="$(native_path "$COMPOSE_FILE")"

  while read -r service; do
    services+=("$service")
  done < <(collect_bench_services)

  if [[ ${#services[@]} -eq 0 ]]; then
    echo "no valid benchmark services configured" >&2
    exit 1
  fi

  MSYS2_ARG_CONV_EXCL="*" docker compose -f "$compose_file" up -d "${services[@]}"
}

down() {
  local compose_file
  compose_file="$(native_path "$COMPOSE_FILE")"
  MSYS2_ARG_CONV_EXCL="*" docker compose -f "$compose_file" down -v --remove-orphans
}

run_frontend_k6() {
  local bench_name="$1"
  local target_url="$2"
  local summary="$3"
  run_k6 "$bench_name" "$target_url" "$summary" "/scripts/frontend-page.js" frontend
}

run_static_k6() {
  local bench_name="$1"
  local target_url="$2"
  local summary="$3"
  run_k6 "$bench_name" "$target_url" "$summary" "/scripts/static-assets.js" static
}

run_k6() {
  local bench_name="$1"
  local target_url="$2"
  local summary="$3"
  local script="$4"
  local mode="$5"
  local page_path asset_paths bench_paths

  page_path="$(read_env_value REFINE_K6_PAGE_PATH "$PATHS_ENV")"
  asset_paths="$(read_env_value REFINE_K6_ASSET_PATHS "$PATHS_ENV")"
  bench_paths="$(read_env_value REFINE_K6_PATHS "$PATHS_ENV")"
  if [[ "$mode" == "static" ]]; then
    bench_paths="$asset_paths"
  fi

  local compose_file
  compose_file="$(native_path "$COMPOSE_FILE")"

  MSYS2_ARG_CONV_EXCL="*" docker compose -f "$compose_file" run --rm \
    -e "BENCH_NAME=$bench_name" \
    -e "TARGET_URL=$target_url" \
    -e "PAGE_PATH=$page_path" \
    -e "ASSET_PATHS=$asset_paths" \
    -e "BENCH_PATHS=$bench_paths" \
    -e "VUS=$K6_VUS" \
    -e "DURATION=$K6_DURATION" \
    -e "ACCEPT_ENCODING=$ACCEPT_ENCODING" \
    k6 run --summary-export "/results/$summary" "$script"
}

find_relative_paths() {
  local root="$1"
  (
    cd "$root"
    find . -type f \
    ! -name '*.map' \
    ! -name '*.br' \
    ! -name '*.gz' \
    ! -name '*.zst' \
    | sed 's#^\./##' \
    | sed 's#\\#/#g' \
    | sed 's#^#/#' \
    | sort
  )
}

read_env_value() {
  local key="$1"
  local file="$2"
  grep "^$key=" "$file" | head -n1 | cut -d= -f2-
}

native_path() {
  local path="$1"
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$path"
    return
  fi
  printf '%s\n' "$path"
}

require_node_package_manager() {
  local manager
  manager="$(package_manager)"
  if [[ -n "$manager" ]] && command -v "$manager" >/dev/null 2>&1; then
    return
  fi
  echo "pnpm, npm, or npm.cmd is required to build the Refine fixture" >&2
  exit 1
}

install_refine_dependencies() {
  local manager
  manager="$(package_manager)"
  case "$manager" in
    npm)
      npm install
      ;;
    npm.cmd)
      npm.cmd install
      ;;
    pnpm)
      pnpm install --frozen-lockfile=false
      ;;
    *)
      "$manager" install
      ;;
  esac
}

install_fixture_packages() {
  local manager marker
  manager="$(package_manager)"
  marker="$SOURCE_DIR/.spack-realistic-packages"

  if [[ -f "$marker" ]] && [[ "$(cat "$marker")" == "$REFINE_FIXTURE_PACKAGES" ]]; then
    echo "Refine fixture packages already installed."
    return
  fi

  echo "Installing additional frontend packages: $REFINE_FIXTURE_PACKAGES"
  case "$manager" in
    npm)
      npm install $REFINE_FIXTURE_PACKAGES
      ;;
    npm.cmd)
      npm.cmd install $REFINE_FIXTURE_PACKAGES
      ;;
    pnpm)
      pnpm add $REFINE_FIXTURE_PACKAGES
      ;;
    *)
      "$manager" add $REFINE_FIXTURE_PACKAGES
      ;;
  esac

  printf '%s' "$REFINE_FIXTURE_PACKAGES" >"$marker"
}

inject_fixture_entry() {
  local fixture_path index_path
  fixture_path="$SOURCE_DIR/src/spack-bench-fixture.ts"
  index_path="$SOURCE_DIR/index.html"

  mkdir -p "$SOURCE_DIR/src"
  cat >"$fixture_path" <<'EOF'
import { format, subDays } from "date-fns";
import groupBy from "lodash-es/groupBy";
import meanBy from "lodash-es/meanBy";
import { nanoid } from "nanoid";
import { AreaChart, BarChart, Line, LineChart, ResponsiveContainer } from "recharts";
import { z } from "zod";

const segments = ["enterprise", "startup", "agency", "consumer"] as const;

const metricSchema = z.object({
  id: z.string(),
  day: z.string(),
  segment: z.enum(segments),
  impressions: z.number(),
  conversions: z.number(),
});

type Metric = z.infer<typeof metricSchema>;
type Segment = Metric["segment"];

const baseDay = new Date("2026-01-01T00:00:00Z");
const rows: Metric[] = Array.from({ length: 240 }, (_, index) => {
  return metricSchema.parse({
    id: nanoid(),
    day: format(subDays(baseDay, index), "yyyy-MM-dd"),
    segment: segments[index % segments.length],
    impressions: 10_000 + index * 137,
    conversions: 320 + (index % 17) * 23,
  });
});

const grouped: Partial<Record<Segment, Metric[]>> = groupBy(rows, "segment");
const summary = segments.map((segment) => {
  const values = grouped[segment] ?? [];
  return {
  segment,
  conversionRate: meanBy(values, (item) => item.conversions / item.impressions),
  samples: values.length,
  };
});

window.dispatchEvent(
  new CustomEvent("spack:bench-fixture-ready", {
    detail: {
      generatedAt: new Date().toISOString(),
      rows: rows.length,
      summary,
      chartModules: [AreaChart, BarChart, Line, LineChart, ResponsiveContainer].map(
        (component) => String(component),
      ),
    },
  }),
);
EOF

  if [[ -f "$index_path" ]] && ! grep -q "spack-bench-fixture.ts" "$index_path"; then
    if grep -q "</body>" "$index_path"; then
      perl -0pi -e 's#</body>#  <script type="module" src="/src/spack-bench-fixture.ts"></script>\n</body>#' "$index_path"
    else
      printf '\n<script type="module" src="/src/spack-bench-fixture.ts"></script>\n' >>"$index_path"
    fi
  fi
}

build_refine_application() {
  local manager
  install_fixture_packages
  inject_fixture_entry

  manager="$(package_manager)"
  if [[ "${REFINE_BUILD_MODE:-vite}" == "vite" ]]; then
    case "$manager" in
      npm)
        npx vite build
        ;;
      npm.cmd)
        npx.cmd vite build
        ;;
      pnpm)
        pnpm exec vite build
        ;;
      *)
        "$manager" exec vite build
        ;;
    esac
    return
  fi

  case "$manager" in
    npm)
      npm run build || npx vite build
      ;;
    npm.cmd)
      npm.cmd run build || npx.cmd vite build
      ;;
    pnpm)
      pnpm build || pnpm exec vite build
      ;;
    *)
      "$manager" run build
      ;;
  esac
}

package_manager() {
  if [[ "$REFINE_PACKAGE_MANAGER" != "auto" ]]; then
    printf '%s\n' "$REFINE_PACKAGE_MANAGER"
    return
  fi
  if command -v pnpm >/dev/null 2>&1; then
    printf '%s\n' "pnpm"
    return
  fi
  if command -v npm >/dev/null 2>&1; then
    printf '%s\n' "npm"
    return
  fi
  if command -v npm.cmd >/dev/null 2>&1; then
    printf '%s\n' "npm.cmd"
  fi
}

require_command() {
  local command="$1"
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "$command is required" >&2
    exit 1
  fi
}

main "$@"






