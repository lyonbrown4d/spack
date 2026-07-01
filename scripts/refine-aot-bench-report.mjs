#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";

const resultsDir = path.resolve(process.env.REFINE_AOT_RESULTS_DIR ?? "tmp/k6/results");
const reportPrefix = process.env.REFINE_AOT_REPORT_PREFIX ?? "refine-aot-comparison-report";
const outputJson = path.join(resultsDir, `${reportPrefix}.json`);
const outputMarkdown = path.join(resultsDir, `${reportPrefix}.md`);
const targets = ["direct", "aot", "caddy", "nginx"];
const workloads = ["page", "static"];
const targetOrder = new Map(targets.map((target, index) => [target, index]));
const workloadOrder = new Map(workloads.map((workload, index) => [workload, index]));

if (!fs.existsSync(resultsDir)) {
  throw new Error(`results directory does not exist: ${resultsDir}`);
}

const files = fs.readdirSync(resultsDir).filter((file) => file.endsWith(".json"));
const samples = files.flatMap(readSample).sort(compareSamples);
const startupSamples = files.flatMap(readStartupSample).sort(compareRuns);

if (samples.length === 0) {
  throw new Error(`no Refine AOT matrix/repeat k6 summaries found in ${resultsDir}`);
}

const matrix = withDirectComparison(
  summarizeBy(samples.filter((sample) => sample.suite === "matrix"), ["concurrency", "workload", "target"]),
  ["concurrency", "workload"],
);
const repeat = withDirectComparison(
  summarizeBy(samples.filter((sample) => sample.suite === "repeat"), ["workload", "target"]),
  ["workload"],
);

const report = {
  generatedAt: new Date().toISOString(),
  resultsDir,
  sampleCount: samples.length,
  matrix,
  repeat,
  startup: {
    matrix: startupSamples.filter((sample) => sample.suite === "matrix"),
    repeat: startupSamples.filter((sample) => sample.suite === "repeat"),
  },
  samples,
};

fs.writeFileSync(outputJson, `${JSON.stringify(report, null, 2)}\n`);
fs.writeFileSync(outputMarkdown, renderMarkdown(report));

console.log(`Wrote ${outputJson}`);
console.log(`Wrote ${outputMarkdown}`);

function readSample(fileName) {
  const match = /^refine-(direct|aot|caddy|nginx)(-static)?-(vus-(\d+)|round-(\d+))\.json$/.exec(fileName);
  if (!match) {
    return [];
  }

  const body = JSON.parse(fs.readFileSync(path.join(resultsDir, fileName), "utf8"));
  const run = match[3];
  return [
    {
      suite: run.startsWith("vus-") ? "matrix" : "repeat",
      run,
      concurrency: match[4] == null ? null : Number.parseInt(match[4], 10),
      round: match[5] == null ? null : Number.parseInt(match[5], 10),
      target: match[1],
      workload: match[2] == null ? "page" : "static",
      file: fileName,
      metrics: {
        failedRate: pickNumber(body, ["metrics", "http_req_failed", "rate"], ["metrics", "http_req_failed", "value"]),
        requestsPerSecond: pickNumber(body, ["metrics", "http_reqs", "rate"]),
        requestCount: pickNumber(body, ["metrics", "http_reqs", "count"]),
        iterationsPerSecond: pickNumber(body, ["metrics", "iterations", "rate"]),
        p50Ms: pickNumber(body, ["metrics", "http_req_duration", "p(50)"]),
        p95Ms: pickNumber(body, ["metrics", "http_req_duration", "p(95)"]),
        p99Ms: pickNumber(body, ["metrics", "http_req_duration", "p(99)"]),
        maxMs: pickNumber(body, ["metrics", "http_req_duration", "max"]),
      },
    },
  ];
}

function readStartupSample(fileName) {
  const match = /^refine-aot-startup-(vus-(\d+)|round-(\d+))\.json$/.exec(fileName);
  if (!match) {
    return [];
  }

  const body = JSON.parse(fs.readFileSync(path.join(resultsDir, fileName), "utf8"));
  const run = match[1];
  return [
    {
      suite: run.startsWith("vus-") ? "matrix" : "repeat",
      run,
      concurrency: match[2] == null ? null : Number.parseInt(match[2], 10),
      round: match[3] == null ? null : Number.parseInt(match[3], 10),
      file: fileName,
      directReadyMs: finiteOrNull(body.direct_ready_ms),
      aotReadyMs: finiteOrNull(body.aot_ready_ms),
      generatedAt: body.generated_at ?? null,
    },
  ];
}

function summarizeBy(rows, keys) {
  const groups = new Map();
  for (const row of rows) {
    const key = keys.map((name) => row[name] ?? "").join("\u0000");
    if (!groups.has(key)) {
      groups.set(key, { dimensions: Object.fromEntries(keys.map((name) => [name, row[name]])), rows: [] });
    }
    groups.get(key).rows.push(row);
  }

  return [...groups.values()]
    .map(({ dimensions, rows: groupedRows }) => ({
      ...dimensions,
      samples: groupedRows.length,
      files: groupedRows.map((row) => row.file).sort(),
      failedRate: summarizeMetric(groupedRows, "failedRate"),
      requestsPerSecond: summarizeMetric(groupedRows, "requestsPerSecond"),
      iterationsPerSecond: summarizeMetric(groupedRows, "iterationsPerSecond"),
      p50Ms: summarizeMetric(groupedRows, "p50Ms"),
      p95Ms: summarizeMetric(groupedRows, "p95Ms"),
      p99Ms: summarizeMetric(groupedRows, "p99Ms"),
      maxMs: summarizeMetric(groupedRows, "maxMs"),
    }))
    .sort(compareSummaries);
}

function withDirectComparison(rows, keyFields) {
  const directByKey = new Map();
  for (const row of rows) {
    if (row.target === "direct") {
      directByKey.set(comparisonKey(row, keyFields), row);
    }
  }

  return rows.map((row) => {
    const baseline = directByKey.get(comparisonKey(row, keyFields));
    return {
      ...row,
      versusDirect: baseline == null ? null : {
        requestsPerSecondRatio: ratio(row.requestsPerSecond.mean, baseline.requestsPerSecond.mean),
        requestsPerSecondDeltaPercent: percentDelta(row.requestsPerSecond.mean, baseline.requestsPerSecond.mean),
        p95DeltaPercent: percentDelta(row.p95Ms.mean, baseline.p95Ms.mean),
        p99DeltaPercent: percentDelta(row.p99Ms.mean, baseline.p99Ms.mean),
      },
    };
  });
}

function comparisonKey(row, fields) {
  return fields.map((field) => row[field] ?? "").join("\u0000");
}

function summarizeMetric(rows, metricName) {
  const values = rows.map((row) => row.metrics[metricName]).filter(Number.isFinite);
  if (values.length === 0) {
    return { samples: 0, min: null, max: null, mean: null, stdev: null };
  }

  const sum = values.reduce((acc, value) => acc + value, 0);
  const mean = sum / values.length;
  const variance = values.reduce((acc, value) => acc + (value - mean) ** 2, 0) / values.length;
  return {
    samples: values.length,
    min: round(Math.min(...values)),
    max: round(Math.max(...values)),
    mean: round(mean),
    stdev: round(Math.sqrt(variance)),
  };
}

function renderMarkdown(body) {
  const lines = [
    "# Refine AOT benchmark comparison",
    "",
    `Generated at: ${body.generatedAt}`,
    `Results dir: \`${path.relative(process.cwd(), body.resultsDir) || "."}\``,
    `Samples: ${body.sampleCount}`,
    "",
    "## Concurrency matrix",
    "",
    "| VUs | workload | target | samples | failed max | req/s mean | req/s vs direct | p95 mean | p95 vs direct | p99 mean | p99 vs direct |",
    "|---:|---|---|---:|---:|---:|---:|---:|---:|---:|---:|",
  ];

  appendSummaryRows(lines, body.matrix, ["concurrency", "workload", "target"]);
  lines.push("", "## Repeat samples", "");
  lines.push("| workload | target | samples | failed max | req/s mean | req/s vs direct | p95 mean | p95 vs direct | p99 mean | p99 vs direct |");
  lines.push("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|");
  appendSummaryRows(lines, body.repeat, ["workload", "target"]);
  lines.push("", "## Startup readiness", "");
  lines.push("| suite | run | direct ready | AOT ready | file |");
  lines.push("|---|---|---:|---:|---|");
  appendStartupRows(lines, body.startup.matrix);
  appendStartupRows(lines, body.startup.repeat);
  lines.push("", "Raw k6 summaries and the JSON report are stored in the same results directory.", "");
  return `${lines.join("\n")}\n`;
}

function appendSummaryRows(lines, rows, dimensions) {
  if (rows.length === 0) {
    const emptyDimensions = dimensions.map(() => "n/a").join(" | ");
    lines.push(`| ${emptyDimensions} | 0 | n/a | n/a | n/a | n/a | n/a | n/a | n/a |`);
    return;
  }

  for (const row of rows) {
    const cells = dimensions.map((dimension) => formatDimension(row[dimension]));
    lines.push(
      `| ${cells.join(" | ")} | ${row.samples} | ${formatNumber(row.failedRate.max, 6)} | ${formatNumber(row.requestsPerSecond.mean)} | ${formatPercent(row.versusDirect?.requestsPerSecondDeltaPercent)} | ${formatMs(row.p95Ms.mean)} | ${formatPercent(row.versusDirect?.p95DeltaPercent)} | ${formatMs(row.p99Ms.mean)} | ${formatPercent(row.versusDirect?.p99DeltaPercent)} |`,
    );
  }
}

function appendStartupRows(lines, rows) {
  for (const row of rows) {
    lines.push(`| ${row.suite} | ${row.run} | ${formatMs(row.directReadyMs)} | ${formatMs(row.aotReadyMs)} | ${row.file} |`);
  }
}

function pickNumber(body, ...paths) {
  for (const parts of paths) {
    const value = valueAt(body, parts);
    if (Number.isFinite(value)) {
      return value;
    }
  }
  return null;
}

function valueAt(body, parts) {
  let current = body;
  for (const part of parts) {
    if (current == null || typeof current !== "object" || !(part in current)) {
      return null;
    }
    current = current[part];
  }
  return finiteOrNull(current);
}

function finiteOrNull(value) {
  return Number.isFinite(value) ? value : null;
}

function compareSamples(left, right) {
  return (
    compareNullableNumber(left.concurrency, right.concurrency) ||
    compareNullableNumber(left.round, right.round) ||
    compareByOrder(workloadOrder, left.workload, right.workload) ||
    compareByOrder(targetOrder, left.target, right.target) ||
    left.file.localeCompare(right.file)
  );
}

function compareRuns(left, right) {
  return compareNullableNumber(left.concurrency, right.concurrency) || compareNullableNumber(left.round, right.round) || left.file.localeCompare(right.file);
}

function compareSummaries(left, right) {
  return (
    compareNullableNumber(left.concurrency, right.concurrency) ||
    compareByOrder(workloadOrder, left.workload, right.workload) ||
    compareByOrder(targetOrder, left.target, right.target)
  );
}

function compareNullableNumber(left, right) {
  if (left == null && right == null) {
    return 0;
  }
  if (left == null) {
    return 1;
  }
  if (right == null) {
    return -1;
  }
  return left - right;
}

function compareByOrder(order, left, right) {
  return (order.get(left) ?? Number.MAX_SAFE_INTEGER) - (order.get(right) ?? Number.MAX_SAFE_INTEGER);
}

function ratio(value, baseline) {
  if (!Number.isFinite(value) || !Number.isFinite(baseline) || baseline === 0) {
    return null;
  }
  return round(value / baseline, 4);
}

function percentDelta(value, baseline) {
  if (!Number.isFinite(value) || !Number.isFinite(baseline) || baseline === 0) {
    return null;
  }
  return round(((value - baseline) / baseline) * 100, 3);
}

function formatDimension(value) {
  return value == null || value === "" ? "n/a" : String(value);
}

function formatNumber(value, digits = 2) {
  return Number.isFinite(value) ? value.toFixed(digits) : "n/a";
}

function formatMs(value) {
  return Number.isFinite(value) ? `${value.toFixed(2)}ms` : "n/a";
}

function formatPercent(value) {
  return Number.isFinite(value) ? `${value >= 0 ? "+" : ""}${value.toFixed(2)}%` : "n/a";
}

function round(value, digits = 3) {
  if (!Number.isFinite(value)) {
    return null;
  }
  const factor = 10 ** digits;
  return Math.round(value * factor) / factor;
}
