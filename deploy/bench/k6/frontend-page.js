import http from 'k6/http';
import { check, sleep } from 'k6';

const targetURL = trimRight(__ENV.TARGET_URL || 'http://spack:80', '/');
const benchName = __ENV.BENCH_NAME || targetURL;
const pagePath = normalizePath(__ENV.PAGE_PATH || '/');
const assetPaths = parsePaths(
  __ENV.ASSET_PATHS ||
    '/assets/style.4b21c9aa.css,/assets/vendor.6d1e3a9b.js,/assets/app.8f3a1c2d.js,/assets/hero.9d7f2a10.jpg,/assets/card.2e4c91ab.png,/assets/logo.1a2b3c4d.svg,/assets/font.7f6e5d4c.woff2,/manifest.webmanifest',
);
const vus = parsePositiveInt(__ENV.VUS, 64);
const duration = __ENV.DURATION || '30s';
const acceptEncoding = __ENV.ACCEPT_ENCODING || 'identity';

export const options = {
  discardResponseBodies: true,
  scenarios: {
    frontend_page: {
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
  const url = `${targetURL}${pagePath}`;
  for (let attempt = 0; attempt < 60; attempt += 1) {
    const response = http.get(url, requestParams(pagePath, 'ready'));
    if (response.status === 200) {
      return;
    }
    sleep(0.5);
  }
  throw new Error(`target ${benchName} was not ready at ${url}`);
}

export default function () {
  const page = http.get(`${targetURL}${pagePath}`, requestParams(pagePath, 'html'));
  check(page, {
    [`${benchName} html status is 200`]: (response) => response.status === 200,
  });

  const requests = assetPaths.map((path) => ['GET', `${targetURL}${path}`, null, requestParams(path, 'asset')]);
  const responses = http.batch(requests);
  for (const [index, response] of responses.entries()) {
    const path = assetPaths[index];
    check(response, {
      [`${benchName} ${path} status is 200`]: (res) => res.status === 200,
    });
  }
}

function requestParams(path, phase) {
  const headers = {
    Accept: '*/*',
    'User-Agent': 'spack-k6-frontendbench/1.0',
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
    .map(normalizePath);
  if (parsed.length === 0) {
    throw new Error('ASSET_PATHS must contain at least one path');
  }
  return parsed;
}

function normalizePath(path) {
  return path.startsWith('/') ? path : `/${path}`;
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
