import http from 'k6/http';
import { check, sleep } from 'k6';

const targetURL = trimRight(__ENV.TARGET_URL || 'http://spack:80', '/');
const benchName = __ENV.BENCH_NAME || targetURL;
const paths = parsePaths(
  __ENV.BENCH_PATHS ||
    '/,/index.html,/assets/app.8f3a1c2d.js,/assets/vendor.6d1e3a9b.js,/assets/style.4b21c9aa.css,/assets/hero.9d7f2a10.jpg,/assets/card.2e4c91ab.png,/assets/logo.1a2b3c4d.svg,/assets/font.7f6e5d4c.woff2,/manifest.webmanifest',
);
const vus = parsePositiveInt(__ENV.VUS, 64);
const duration = __ENV.DURATION || '30s';
const acceptEncoding = __ENV.ACCEPT_ENCODING || 'identity';

export const options = {
  discardResponseBodies: true,
  scenarios: {
    static_assets: {
      executor: 'constant-vus',
      vus,
      duration,
      gracefulStop: '5s',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  thresholds: {
    http_req_failed: ['rate<0.001'],
  },
};

export function setup() {
  const url = `${targetURL}${paths[0]}`;
  for (let attempt = 0; attempt < 60; attempt += 1) {
    const response = http.get(url, requestParams(paths[0], 'ready'));
    if (response.status === 200) {
      return;
    }
    sleep(0.5);
  }
  throw new Error(`target ${benchName} was not ready at ${url}`);
}

export default function () {
  const path = paths[(__ITER + __VU) % paths.length];
  const response = http.get(`${targetURL}${path}`, requestParams(path, 'load'));
  check(response, {
    [`${benchName} status is 200`]: (r) => r.status === 200,
  });
}

function requestParams(path, phase) {
  const headers = {
    'Accept': '*/*',
    'User-Agent': 'spack-k6-staticbench/1.0',
  };
  if (acceptEncoding !== '') {
    headers['Accept-Encoding'] = acceptEncoding;
  }
  return {
    headers,
    tags: {
      target: benchName,
      asset: path,
      phase,
    },
  };
}

function parsePaths(raw) {
  const parsed = raw
    .split(',')
    .map((path) => path.trim())
    .filter((path) => path !== '')
    .map((path) => (path.startsWith('/') ? path : `/${path}`));
  if (parsed.length === 0) {
    throw new Error('BENCH_PATHS must contain at least one path');
  }
  return parsed;
}

function parsePositiveInt(raw, fallback) {
  const parsed = Number.parseInt(raw || '', 10);
  if (Number.isFinite(parsed) && parsed > 0) {
    return parsed;
  }
  return fallback;
}

function trimRight(value, suffix) {
  let out = value;
  while (out.endsWith(suffix)) {
    out = out.slice(0, -suffix.length);
  }
  return out;
}
