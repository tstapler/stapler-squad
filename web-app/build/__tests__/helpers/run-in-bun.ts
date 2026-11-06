import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";

const WEB_APP_DIR = path.resolve(__dirname, "../../..");
const BUN_SCRIPTS_DIR = path.resolve(__dirname, "../bun");
const TIMEOUT_MS = 60_000;

export interface BunRun {
  status: number | null;
  json: unknown;
  stderr: string;
}

function pinnedBunVersion(): string {
  return readFileSync(path.resolve(WEB_APP_DIR, "../.bun-version"), "utf8").trim();
}

function assertPinnedBun(): void {
  const probe = spawnSync("bun", ["--version"], { encoding: "utf8", timeout: TIMEOUT_MS, env: process.env });
  // Missing bun must fail the suite, never skip it: a skip would let Bun-only code go untested.
  if (probe.error || probe.status !== 0) {
    throw new Error(`bun not found on PATH (${probe.error?.message ?? probe.stderr}); run make ensure-tools`);
  }
  const want = pinnedBunVersion();
  const got = probe.stdout.trim();
  if (got !== want) {
    throw new Error(`bun ${got} != pinned ${want} (.bun-version); run make ensure-tools`);
  }
}

/**
 * Runs a Bun-only script under the pinned `bun` (cwd = web-app) and parses the single
 * JSON object it prints on stdout. A bare name resolves under build/__tests__/bun/.
 */
export function runInBun(script: string, args: string[] = []): BunRun {
  assertPinnedBun();
  const file = path.isAbsolute(script) ? script : path.join(BUN_SCRIPTS_DIR, script);
  const res = spawnSync("bun", [file, ...args], { cwd: WEB_APP_DIR, encoding: "utf8", timeout: TIMEOUT_MS, env: process.env });
  if (res.error) {
    throw new Error(`bun ${file} failed to run: ${res.error.message}`);
  }
  const out = res.stdout.trim();
  let json: unknown = null;
  if (out) {
    try {
      json = JSON.parse(out);
    } catch {
      throw new Error(`bun ${file} stdout was not one JSON object:\n${out}\nstderr:\n${res.stderr}`);
    }
  }
  return { status: res.status, json, stderr: res.stderr };
}
