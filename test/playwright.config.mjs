import os from "node:os";
import path from "node:path";

export default {
  testDir: ".",
  testMatch: "cache-update.spec.mjs",
  timeout: 300_000,
  globalTimeout: 900_000,
  workers: 1,
  reporter: "list",
  outputDir: path.join(os.tmpdir(), "spack-playwright-cache-update"),
};