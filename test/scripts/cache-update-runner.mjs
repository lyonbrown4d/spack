import { copyFile, mkdir, readFile, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const playwrightVersion = "1.64.1";
const scriptsDir = path.dirname(fileURLToPath(import.meta.url));
const testDir = path.resolve(scriptsDir, "..");
const repoRoot = path.resolve(testDir, "..");
const runnerDir = path.join(os.tmpdir(), "spack-playwright-" + playwrightVersion);
const packagePath = path.join(runnerDir, "package.json");
const installedPackage = path.join(
  runnerDir,
  "node_modules",
  "@playwright",
  "test",
  "package.json",
);

await mkdir(runnerDir, { recursive: true });
const packageBody = JSON.stringify({
  private: true,
  type: "module",
  dependencies: { "@playwright/test": playwrightVersion },
}, null, 2) + "\n";

let installedVersion = "";
try {
  installedVersion = JSON.parse(await readFile(installedPackage, "utf8")).version;
} catch {
  // The isolated runner has not been installed yet.
}

if (installedVersion !== playwrightVersion) {
  await writeFile(packagePath, packageBody, "utf8");
  const npmExecPath = process.env.npm_execpath;
  if (!npmExecPath) throw new Error("npm_execpath is required");
  runChecked(process.execPath, [npmExecPath,
    "install",
    "--ignore-scripts",
    "--no-audit",
    "--no-fund",
  ], runnerDir);
}

await Promise.all([
  copyFile(
    path.join(testDir, "cache-update.spec.mjs"),
    path.join(runnerDir, "cache-update.spec.mjs"),
  ),
  copyFile(
    path.join(testDir, "playwright.config.mjs"),
    path.join(runnerDir, "playwright.config.mjs"),
  ),
]);

const executable = path.join(
  runnerDir,
  "node_modules",
  "@playwright",
  "test",
  "cli.js",
);
const result = spawnSync(
  process.execPath,
  [executable, "test", "cache-update.spec.mjs", "--config", "playwright.config.mjs"],
  {
    cwd: runnerDir,
    env: { ...process.env, SPACK_REPO_ROOT: repoRoot },
    stdio: "inherit",
    windowsHide: true,
  },
);
process.exitCode = result.status ?? 1;

function runChecked(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    env: process.env,
    windowsHide: true,
  });
  if (result.status !== 0) {
    throw new Error(
      command + " " + args.join(" ") + " failed with status " + result.status +
      "\nstdout:\n" + (result.stdout ?? "") +
      "\nstderr:\n" + (result.stderr ?? ""),
    );
  }
}
