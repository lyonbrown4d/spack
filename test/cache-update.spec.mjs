import { spawn, spawnSync } from "node:child_process";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium, test } from "@playwright/test";

const testDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = process.env.SPACK_REPO_ROOT ?? path.resolve(testDir, "..");
const compilerImage = process.env.SPACK_CACHE_UPDATE_COMPILER_IMAGE ??
  "ghcr.io/lyonbrown4d/spack-compiler:v2.1.5";
const versions = [
  { name: "v1", hash: "a1b2c3d4" },
  { name: "v2", hash: "e5f6a7b8" },
  { name: "v3", hash: "c9d0e1f2" },
];

let workspace;
let runtimeBinary;

test.beforeAll(async () => {
  workspace = await mkdtemp(path.join(os.tmpdir(), "spack-cache-update-"));
  runtimeBinary = path.join(workspace, process.platform === "win32" ? "spack-runtime.exe" : "spack-runtime");
  runChecked("go", ["build", "-trimpath", "-o", runtimeBinary, "./cmd/spack-runtime"], {
    cwd: repoRoot,
    env: { ...process.env, CGO_ENABLED: "0" },
    timeout: 240_000,
  });
});

test.afterAll(async () => {
  if (workspace) await rm(workspace, { recursive: true, force: true });
});

for (const mode of ["direct", "aot"]) {
  test(`${mode} runtime recovers across consecutive frontend deployments`, async () => {
    const scenarioDir = path.join(workspace, mode);
    await mkdir(scenarioDir, { recursive: true });
    const deployments = [];

    for (const version of versions) {
      const distDir = path.join(scenarioDir, `dist-${version.name}`);
      await writeFixture(distDir, version);
      const root = mode === "aot"
        ? compileBundle(distDir, path.join(scenarioDir, `bundle-${version.name}`))
        : distDir;
      deployments.push({ ...version, root });
    }

    const port = await reservePort();
    let runtime = await startRuntime(deployments[0].root, port);
    const browser = await chromium.launch({
      channel: process.env.SPACK_CACHE_UPDATE_BROWSER_CHANNEL ?? "chrome",
      headless: true,
    });
    const context = await browser.newContext();
    const page = await context.newPage();
    const evidence = [];

    page.on("response", async (response) => {
      const url = new URL(response.url());
      if (response.request().resourceType() === "document" || url.pathname.includes("/assets/lazy-")) {
        const headers = await response.allHeaders();
        evidence.push({
          url: url.pathname,
          status: response.status(),
          contentType: headers["content-type"] ?? "",
          cacheControl: headers["cache-control"] ?? "",
        });
      }
    });

    try {
      await page.goto(`http://127.0.0.1:${port}/account/orders?source=v1`);
      await waitForVersion(page, "v1", evidence);

      for (let index = 1; index < deployments.length; index += 1) {
        await runtime.stop();
        runtime = await startRuntime(deployments[index].root, port);
        await page.evaluate(() => {
          globalThis.spackLoadLazy().catch((error) => {
            globalThis.spackLazyError = String(error);
          });
        });
        await waitForVersion(page, deployments[index].name, evidence);
      }
    } finally {
      await context.close();
      await browser.close();
      await runtime.stop();
    }
  });
}

async function writeFixture(distDir, version) {
  await rm(distDir, { recursive: true, force: true });
  const assetsDir = path.join(distDir, "assets");
  await mkdir(assetsDir, { recursive: true });

  const index = `<!doctype html>
<html lang="en" data-version="loading">
<head><meta charset="utf-8"><title>SPACK cache update ${version.name}</title></head>
<body><main id="version">loading</main>
<script type="module" src="/assets/app-${version.hash}.js"></script></body>
</html>
`;
  const app = `const version = "${version.name}";
document.documentElement.dataset.version = version;
document.querySelector("#version").textContent = version;
const boots = Number(sessionStorage.getItem("spack_test_boots") || "0") + 1;
sessionStorage.setItem("spack_test_boots", String(boots));
globalThis.spackLoadLazy = () => import("/assets/lazy-${version.hash}.js");
`;
  const lazy = `export const version = "${version.name}";\n`;

  await Promise.all([
    writeFile(path.join(distDir, "index.html"), index, "utf8"),
    writeFile(path.join(assetsDir, `app-${version.hash}.js`), app, "utf8"),
    writeFile(path.join(assetsDir, `lazy-${version.hash}.js`), lazy, "utf8"),
  ]);
}

function compileBundle(distDir, outputDir) {
  runChecked("docker", [
    "run", "--rm",
    "-v", `${distDir}:/workspace/dist:ro`,
    "-v", `${outputDir}:/workspace/out`,
    compilerImage,
    "--assets.path=/",
    "--assets.entry=index.html",
    "--assets.fallback.on=not_found",
    "--assets.fallback.target=index.html",
    "--compression.enable=false",
    "--image.enable=false",
    "--frontend.resource_hints.enable=false",
    "compile", "/workspace/dist", "-o", "/workspace/out/app.spack",
  ], { cwd: repoRoot, timeout: 240_000 });
  return path.join(outputDir, "app.spack");
}

async function startRuntime(root, port) {
  const output = [];
  const child = spawn(runtimeBinary, [], {
    cwd: repoRoot,
    env: {
      ...process.env,
      SPACK_ASSETS_ROOT: root,
      SPACK_ASSETS_PATH: "/",
      SPACK_ASSETS_ENTRY: "index.html",
      SPACK_ASSETS_FALLBACK_ON: "not_found",
      SPACK_ASSETS_FALLBACK_TARGET: "index.html",
      SPACK_HTTP_PORT: String(port),
      SPACK_HTTP_MEMORY_CACHE_ENABLE: "false",
      SPACK_COMPRESSION_ENABLE: "false",
      SPACK_IMAGE_ENABLE: "false",
      SPACK_FRONTEND_RESOURCE_HINTS_ENABLE: "false",
      SPACK_FRONTEND_STALE_ASSET_RECOVERY_ENABLE: "true",

      SPACK_LOGGER_LEVEL: "error",
    },
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: true,
  });
  child.stdout.on("data", (chunk) => output.push(String(chunk)));
  child.stderr.on("data", (chunk) => output.push(String(chunk)));

  try {
    await waitUntilReady(port, child, output);
  } catch (error) {
    child.kill();
    throw error;
  }

  return {
    async stop() {
      if (child.exitCode !== null) return;
      child.kill();
      await Promise.race([
        new Promise((resolve) => child.once("exit", resolve)),
        new Promise((resolve) => setTimeout(resolve, 5_000)),
      ]);
    },
  };
}

async function waitUntilReady(port, child, output) {
  const deadline = Date.now() + 20_000;
  const url = `http://127.0.0.1:${port}/index.html`;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`runtime exited with code ${child.exitCode}:\n${output.join("")}`);
    }
    try {
      const response = await fetch(url, { cache: "no-store" });
      if (response.ok) return;
    } catch {
      // Runtime has not bound its socket yet.
    }
    await delay(100);
  }
  throw new Error(`runtime did not become ready at ${url}:\n${output.join("")}`);
}

async function waitForVersion(page, expected, evidence) {
  const deadline = Date.now() + 10_000;
  while (Date.now() < deadline) {
    const actual = await page.locator("html").getAttribute("data-version").catch(() => null);
    if (actual === expected) return;
    await delay(100);
  }

  const state = await page.evaluate(() => ({
    version: document.documentElement.dataset.version,
    recoveryMarker: sessionStorage.getItem("_spack_sr"),
    boots: sessionStorage.getItem("spack_test_boots"),
    lazyError: globalThis.spackLazyError ?? "",
    url: location.href,
  }));
  throw new Error(
    `expected frontend ${expected}, got ${JSON.stringify(state)}; responses=${JSON.stringify(evidence)}`,
  );
}

function runChecked(command, args, options) {
  const result = spawnSync(command, args, {
    ...options,
    encoding: "utf8",
    env: options?.env ?? process.env,
    windowsHide: true,
  });
  if (result.status !== 0) {
    throw new Error(
      `${command} ${args.join(" ")} failed with status ${result.status}\nstdout:\n${result.stdout ?? ""}\nstderr:\n${result.stderr ?? ""}`,
    );
  }
}

async function reservePort() {
  const server = createServer();
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  const port = typeof address === "object" && address ? address.port : 0;
  await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
  return port;
}

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}