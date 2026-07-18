import { spawn, ChildProcess } from "child_process";
import * as fs from "fs";
import * as http from "http";
import * as os from "os";
import * as path from "path";
import { allocatePort } from "./ports";

/**
 * StackLauncher (see project_plans/isolated-dev-stacks/implementation/plan.md,
 * Epic 3.2 / Story 3.2.1). Spawns and supervises a BackendChild (the Go
 * binary) and a FrontendChild (`next dev`) sharing one STAPLER_SQUAD_INSTANCE
 * identity, then tears both down process-group-aware on request.
 *
 * IMPORTANT: this module registers ZERO process.on(...) handlers at module
 * scope or anywhere inside startDevStack()'s core. Only main() — the CLI
 * wrapper invoked exclusively via the `require.main === module` guard at the
 * bottom of this file — registers SIGINT/SIGTERM handlers. This is what lets
 * a future Playwright dev-mode harness `import { startDevStack } from
 * "./launch"` without inheriting CLI-only signal handling (Task 5.1.1a).
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Immutable value object threaded through the launcher instead of loose primitives. */
export interface DevStackConfig {
  instance: string;
  backendPort: number;
  frontendPort: number;
  apiBaseUrl: string;
}

export interface DevStackManifest {
  instance: string;
  backendPort: number;
  frontendPort: number;
  apiBaseUrl: string;
  dataDir: string;
  /** The launcher's OWN pid — informational only. NEVER used for reaping. */
  pid: number;
  backendPid: number;
  frontendPid: number;
  schemaVersion: number;
}

type KillFn = (pid: number, signal?: string | number) => void;

export interface StartDevStackOpts {
  /** Public, documented field. Defaults to false (manual-CLI fast path, Task 3.2.1i). */
  seedData?: boolean;

  // Everything below is additive test/harness-injection surface — optional,
  // defaulting to the real production behavior, so the public signature
  // documented in plan.md (`startDevStack(name?, opts?: {seedData?})`)
  // remains valid for real callers.
  maxAttempts?: number;
  intervalMs?: number;
  killFn?: KillFn;
  reconcileGraceMs?: number;
  reconcilePollIntervalMs?: number;
  teardownGraceMs?: number;
  spawnBackend?: (config: DevStackConfig) => ChildProcess;
  spawnFrontend?: (config: DevStackConfig) => ChildProcess;
  checkBackendReady?: (config: DevStackConfig) => Promise<boolean>;
  checkFrontendReady?: (config: DevStackConfig) => Promise<boolean>;
  log?: (message: string) => void;
  /**
   * Invoked synchronously with the live teardown closure as soon as it's
   * constructed — i.e. before either child has spawned. This lets a caller
   * (main()'s signal handler) reach teardown() even if a SIGINT/SIGTERM
   * arrives while startDevStack() is still mid-startup poll, so
   * already-spawned children never get orphaned by an "early" Ctrl-C.
   */
  onTeardownReady?: (stop: () => Promise<void>) => void;
}

export interface StartDevStackResult {
  backendUrl: string;
  frontendUrl: string;
  stop: () => Promise<void>;
}

// ---------------------------------------------------------------------------
// Manifest path + read/write/delete helpers
// ---------------------------------------------------------------------------

export function getManifestPath(name: string): string {
  return path.join(os.homedir(), ".stapler-squad", "instances", name, "dev-stack.json");
}

async function readManifestIfExists(manifestPath: string): Promise<DevStackManifest | null> {
  try {
    const raw = await fs.promises.readFile(manifestPath, "utf8");
    return JSON.parse(raw) as DevStackManifest;
  } catch (err) {
    if (isErrnoCode(err, "ENOENT")) return null;
    throw err;
  }
}

async function writeManifest(manifestPath: string, manifest: DevStackManifest): Promise<void> {
  await fs.promises.mkdir(path.dirname(manifestPath), { recursive: true });
  await fs.promises.writeFile(manifestPath, JSON.stringify(manifest, null, 2));
}

async function deleteManifestFile(manifestPath: string): Promise<void> {
  try {
    await fs.promises.unlink(manifestPath);
  } catch (err) {
    if (!isErrnoCode(err, "ENOENT")) throw err;
  }
}

function isErrnoCode(err: unknown, code: string): boolean {
  return Boolean(err) && typeof err === "object" && (err as NodeJS.ErrnoException).code === code;
}

// ---------------------------------------------------------------------------
// waitForReady — reusable bounded polling helper (Task 3.2.1b/c)
// ---------------------------------------------------------------------------

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Polls checkFn up to maxAttempts times, sleeping intervalMs between
 * attempts, resolving as soon as checkFn resolves true. Throws once
 * maxAttempts is exhausted without a true result — never polls forever.
 * maxAttempts/intervalMs are always parameter-injectable so tests never
 * have to wait anywhere near the real ~90s production ceiling.
 */
export async function waitForReady(
  checkFn: () => Promise<boolean>,
  opts: { maxAttempts: number; intervalMs: number },
): Promise<void> {
  const { maxAttempts, intervalMs } = opts;
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    if (await checkFn()) return;
    if (attempt < maxAttempts) {
      await sleep(intervalMs);
    }
  }
  throw new Error(`waitForReady(): not ready after ${maxAttempts} attempts`);
}

// ---------------------------------------------------------------------------
// Ring buffer stdout/stderr capture (Task 3.2.1b/c UX Blocker)
// ---------------------------------------------------------------------------

const RING_BUFFER_MAX_LINES = 50;
const RING_BUFFER_MAX_BYTES = 4096;

interface RingBuffer {
  tail(): string;
  discard(): void;
}

function attachRingBuffer(child: ChildProcess): RingBuffer {
  const lines: string[] = [];
  let bytes = 0;
  let discarded = false;

  const onData = (data: Buffer | string) => {
    if (discarded) return;
    const text = data.toString();
    for (const line of text.split("\n")) {
      lines.push(line);
      bytes += line.length;
    }
    while (lines.length > RING_BUFFER_MAX_LINES || bytes > RING_BUFFER_MAX_BYTES) {
      const removed = lines.shift();
      if (removed !== undefined) bytes -= removed.length;
    }
  };

  child.stdout?.on("data", onData);
  child.stderr?.on("data", onData);

  return {
    tail: () => lines.join("\n"),
    discard: () => {
      discarded = true;
      lines.length = 0;
      bytes = 0;
    },
  };
}

// ---------------------------------------------------------------------------
// Default spawn + readiness-check implementations (production behavior)
// ---------------------------------------------------------------------------

function repoRoot(): string {
  return path.resolve(__dirname, "../..");
}

function defaultSpawnBackend(config: DevStackConfig): ChildProcess {
  const buildPath = path.join(repoRoot(), "stapler-squad");
  return spawn(buildPath, [], {
    detached: true,
    env: {
      ...process.env,
      STAPLER_SQUAD_INSTANCE: config.instance,
      PORT: String(config.backendPort),
      STAPLER_SQUAD_EXTRA_ORIGINS: `http://localhost:${config.frontendPort}`,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function defaultSpawnFrontend(config: DevStackConfig): ChildProcess {
  const webAppDir = path.join(repoRoot(), "web-app");
  const nextBin = path.join(webAppDir, "node_modules", ".bin", "next");
  return spawn(nextBin, ["dev", "--port", String(config.frontendPort), "--hostname", "localhost"], {
    cwd: webAppDir,
    detached: true,
    env: {
      ...process.env,
      NEXT_PUBLIC_API_URL: config.apiBaseUrl,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function httpProbe(url: string, isOk: (statusCode: number) => boolean): Promise<boolean> {
  return new Promise((resolve) => {
    const req = http.get(url, (res) => {
      const ok = isOk(res.statusCode ?? 0);
      res.resume();
      resolve(ok);
    });
    req.on("error", () => resolve(false));
    req.setTimeout(2000, () => {
      req.destroy();
      resolve(false);
    });
  });
}

function defaultCheckBackendReady(config: DevStackConfig): Promise<boolean> {
  return httpProbe(`http://localhost:${config.backendPort}/health`, (status) => status === 200);
}

function defaultCheckFrontendReady(config: DevStackConfig): Promise<boolean> {
  // Any HTTP response counts as "responds" — next dev may 404 briefly during
  // compile, but that still means it's listening on the port (Task 3.2.1c).
  return httpProbe(`http://localhost:${config.frontendPort}`, () => true);
}

// ---------------------------------------------------------------------------
// Process-group-aware teardown (Task 3.2.1d)
// ---------------------------------------------------------------------------

function isAlivePid(pid: number, killFn: KillFn): boolean {
  try {
    killFn(pid, 0);
    return true;
  } catch (err) {
    if (isErrnoCode(err, "ESRCH")) return false;
    throw err;
  }
}

function waitForChildExit(child: ChildProcess, timeoutMs: number): Promise<boolean> {
  return new Promise((resolve) => {
    if (child.exitCode !== null || child.signalCode !== null) {
      resolve(true);
      return;
    }
    const timer = setTimeout(() => resolve(false), timeoutMs);
    child.once("exit", () => {
      clearTimeout(timer);
      resolve(true);
    });
  });
}

/** SIGTERM the child's process group, wait up to graceMs, escalate to SIGKILL. */
async function killChildProcessGroup(
  child: ChildProcess | undefined,
  killFn: KillFn,
  graceMs: number,
): Promise<void> {
  if (!child || child.pid === undefined) return;
  const pid = child.pid;

  try {
    killFn(-pid, "SIGTERM");
  } catch (err) {
    if (isErrnoCode(err, "ESRCH")) return;
    throw err;
  }

  const exited = await waitForChildExit(child, graceMs);
  if (exited) return;

  try {
    killFn(-pid, "SIGKILL");
  } catch (err) {
    if (!isErrnoCode(err, "ESRCH")) throw err;
  }
  await waitForChildExit(child, graceMs);
}

// ---------------------------------------------------------------------------
// Startup orphan-reconciliation sweep (Task 3.2.1g)
// ---------------------------------------------------------------------------

/**
 * Pure reaping logic over a manifest-shaped object's backendPid/frontendPid
 * — deliberately NEVER reads or signals `manifest.pid` (the launcher's own,
 * informational-only PID from a prior run). Exported directly so unit tests
 * can assert exact process.kill call arguments against a fixture manifest
 * without any real spawn/filesystem I/O (Adversarial Blocker 1 regression
 * guard).
 */
export async function reapOrphanedChildren(
  manifest: Pick<DevStackManifest, "backendPid" | "frontendPid">,
  opts: {
    killFn?: KillFn;
    graceMs?: number;
    pollIntervalMs?: number;
    log?: (message: string) => void;
  } = {},
): Promise<void> {
  const killFn = opts.killFn ?? (process.kill.bind(process) as KillFn);
  const graceMs = opts.graceMs ?? 5000;
  const pollIntervalMs = opts.pollIntervalMs ?? 50;
  const log = opts.log ?? ((message: string) => console.warn(message));

  const targets: Array<{ label: "backend" | "frontend"; pid: number | undefined }> = [
    { label: "backend", pid: manifest.backendPid },
    { label: "frontend", pid: manifest.frontendPid },
  ];

  for (const { label, pid } of targets) {
    if (pid === undefined || pid === null) continue;
    // Defense in depth: a manifest is a plain JSON file on disk with no
    // integrity check. `kill(-1, sig)` broadcasts to every process the
    // caller can signal, and pid 0 targets the caller's own process group —
    // never signal either, regardless of what a (corrupted or malicious)
    // manifest claims.
    if (pid <= 1) {
      log(`Skipping reap of ${label} child: manifest pid ${pid} is not a valid process id`);
      continue;
    }
    if (!isAlivePid(pid, killFn)) continue;

    log(`Reaping orphaned ${label} child (pid ${pid})`);

    try {
      killFn(-pid, "SIGTERM");
    } catch (err) {
      if (isErrnoCode(err, "ESRCH")) continue;
      throw err;
    }

    const deadline = Date.now() + graceMs;
    let stillAlive = isAlivePid(pid, killFn);
    while (stillAlive && Date.now() < deadline) {
      await sleep(pollIntervalMs);
      stillAlive = isAlivePid(pid, killFn);
    }

    if (stillAlive) {
      try {
        killFn(-pid, "SIGKILL");
      } catch (err) {
        if (!isErrnoCode(err, "ESRCH")) throw err;
      }
      const killDeadline = Date.now() + graceMs;
      while (isAlivePid(pid, killFn) && Date.now() < killDeadline) {
        await sleep(pollIntervalMs);
      }
    }
  }
}

/**
 * If a stale dev-stack.json exists for `name` (from a previously
 * hard-killed launcher), reap any of its backendPid/frontendPid that are
 * still alive (process-group SIGTERM -> grace window -> SIGKILL), then
 * delete the stale manifest. No-op if no manifest exists. Must run BEFORE
 * any allocatePort() call in startDevStack() (Task 3.2.1g).
 */
export async function reconcileStaleManifest(
  name: string,
  opts: {
    killFn?: KillFn;
    graceMs?: number;
    pollIntervalMs?: number;
    log?: (message: string) => void;
  } = {},
): Promise<void> {
  const manifestPath = getManifestPath(name);
  const manifest = await readManifestIfExists(manifestPath);
  if (!manifest) return;

  await reapOrphanedChildren(manifest, opts);
  await deleteManifestFile(manifestPath);
}

// ---------------------------------------------------------------------------
// Startup banner (Task 3.2.1e)
// ---------------------------------------------------------------------------

function printBanner(name: string, backendUrl: string, frontendUrl: string, log: (msg: string) => void): void {
  log("");
  log(`=== DevStack "${name}" ready — NOT the systemd instance ===`);
  log(`  backend:  ${backendUrl}`);
  log(`  frontend: ${frontendUrl}`);
  log("");
}

// ---------------------------------------------------------------------------
// startDevStack — the pure, importable core (Task 3.2.1a)
// ---------------------------------------------------------------------------

/**
 * Public documented signature: startDevStack(name?, opts?: { seedData?: boolean }).
 * All other StartDevStackOpts fields are additive test/harness-injection
 * seams with production-safe defaults — real callers only ever need to pass
 * `{ seedData }`, or nothing at all.
 *
 * ZERO process.on(...) registrations happen anywhere in this function or
 * anything it calls — only main() (below) registers signal handlers.
 */
export async function startDevStack(
  name?: string,
  opts: StartDevStackOpts = {},
): Promise<StartDevStackResult> {
  if (!name) {
    throw new Error("startDevStack(): an instance name is required");
  }
  if (!/^[A-Za-z0-9_-]+$/.test(name)) {
    // Defense in depth: name is joined into a filesystem path (getManifestPath)
    // with no other sanitization, so reject anything that could traverse
    // outside ~/.stapler-squad/instances/ (e.g. "../", "/").
    throw new Error(
      `startDevStack(): instance name ${JSON.stringify(name)} is invalid — only letters, digits, "-", and "_" are allowed`,
    );
  }

  const maxAttempts = opts.maxAttempts ?? 90;
  const intervalMs = opts.intervalMs ?? 1000;
  const killFn = opts.killFn ?? (process.kill.bind(process) as KillFn);
  const teardownGraceMs = opts.teardownGraceMs ?? 5000;
  const log = opts.log ?? ((message: string) => console.log(message));

  // Task 3.2.1g: reconcile any stale manifest from a hard-killed prior
  // launcher BEFORE allocating any new ports.
  await reconcileStaleManifest(name, {
    killFn,
    graceMs: opts.reconcileGraceMs,
    pollIntervalMs: opts.reconcilePollIntervalMs,
    log,
  });

  const backendAlloc = await allocatePort();
  const frontendAlloc = await allocatePort();

  const config: DevStackConfig = {
    instance: name,
    backendPort: backendAlloc.port,
    frontendPort: frontendAlloc.port,
    apiBaseUrl: `http://localhost:${backendAlloc.port}/api`,
  };

  const manifestPath = getManifestPath(name);

  let backendChild: ChildProcess | undefined;
  let frontendChild: ChildProcess | undefined;
  let aborted = false;

  const teardown = async (): Promise<void> => {
    aborted = true;
    await killChildProcessGroup(backendChild, killFn, teardownGraceMs);
    await killChildProcessGroup(frontendChild, killFn, teardownGraceMs);
    await deleteManifestFile(manifestPath);
  };

  // Expose the live teardown closure BEFORE either child spawns, so a
  // SIGINT/SIGTERM arriving mid-startup-poll can still reach it (BLOCKER fix:
  // main()'s signal handler no longer has to wait for startDevStack() to
  // resolve before it has something to call).
  opts.onTeardownReady?.(teardown);

  /** Races a spawned child's async 'error' event against a readiness promise, so a
   * spawn failure (e.g. ENOENT) surfaces immediately instead of hanging until
   * waitForReady()'s maxAttempts ceiling with no indication of why. */
  function raceSpawnError<T>(child: ChildProcess, readyPromise: Promise<T>): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const onError = (err: Error) => reject(err);
      child.once("error", onError);
      readyPromise.then(
        (value) => {
          child.removeListener("error", onError);
          resolve(value);
        },
        (err) => {
          child.removeListener("error", onError);
          reject(err);
        },
      );
    });
  }

  try {
    // --- BackendChild (Task 3.2.1b) ---
    backendAlloc.release();
    const spawnBackend = opts.spawnBackend ?? defaultSpawnBackend;
    backendChild = spawnBackend(config);
    const backendBuffer = attachRingBuffer(backendChild);
    const checkBackendReady = opts.checkBackendReady ?? defaultCheckBackendReady;

    try {
      await raceSpawnError(
        backendChild,
        waitForReady(() => checkBackendReady(config), { maxAttempts, intervalMs }),
      );
    } catch (err) {
      throw new Error(
        `Backend health check timed out after ${maxAttempts} attempts. Last output from backend process:\n${backendBuffer.tail()}`,
        { cause: err },
      );
    }
    backendBuffer.discard();

    if (aborted) {
      throw new Error("startDevStack(): teardown requested during startup, aborting");
    }

    // --- FrontendChild (Task 3.2.1c) ---
    frontendAlloc.release();
    const spawnFrontend = opts.spawnFrontend ?? defaultSpawnFrontend;
    frontendChild = spawnFrontend(config);
    const frontendBuffer = attachRingBuffer(frontendChild);
    const checkFrontendReady = opts.checkFrontendReady ?? defaultCheckFrontendReady;

    try {
      await raceSpawnError(
        frontendChild,
        waitForReady(() => checkFrontendReady(config), { maxAttempts, intervalMs }),
      );
    } catch (err) {
      throw new Error(
        `Frontend readiness check timed out after ${maxAttempts} attempts. Last output from next dev:\n${frontendBuffer.tail()}`,
        { cause: err },
      );
    }
    frontendBuffer.discard();

    if (aborted) {
      throw new Error("startDevStack(): teardown requested during startup, aborting");
    }
  } catch (err) {
    // Adversarial Blocker 2: a FrontendChild timeout must kill the
    // already-started BackendChild rather than leaking it. teardown() is
    // idempotent-safe to call again if a concurrent signal handler already
    // invoked it via onTeardownReady (killChildProcessGroup/deleteManifestFile
    // both no-op cleanly against already-gone processes/files).
    await teardown();
    throw err;
  }

  // Task 3.2.1i: manual-CLI fast path skips seeding entirely (seedData
  // defaults to false). This is a stub branch — no seeding logic exists yet;
  // a future epic (5.1) is expected to pass { seedData: true } explicitly.
  if (opts.seedData) {
    // TODO: seedDemoData()/seedLiveSessions()-equivalent logic for the
    // Playwright dev-mode harness (Epic 5.1) — not implemented in Epic 3.2.
  }

  const backendUrl = `http://localhost:${config.backendPort}`;
  const frontendUrl = `http://localhost:${config.frontendPort}`;

  // Task 3.2.1e: manifest + banner only once BOTH children are ready.
  await writeManifest(manifestPath, {
    instance: name,
    backendPort: config.backendPort,
    frontendPort: config.frontendPort,
    apiBaseUrl: config.apiBaseUrl,
    dataDir: path.join(os.homedir(), ".stapler-squad", "instances", name, "data"),
    pid: process.pid,
    backendPid: backendChild.pid as number,
    frontendPid: frontendChild.pid as number,
    schemaVersion: 2,
  });

  printBanner(name, backendUrl, frontendUrl, log);

  return { backendUrl, frontendUrl, stop: teardown };
}

// ---------------------------------------------------------------------------
// main() — the ONLY place signal handlers get registered (Task 3.2.1d)
// ---------------------------------------------------------------------------

export function main(
  argv: string[] = process.argv,
  exitFn: (code: number) => void = process.exit.bind(process),
): void {
  const name = argv[2];
  if (!name) {
    console.error("Usage: launch.ts <instance-name>");
    exitFn(1);
    return;
  }

  let stopFn: (() => Promise<void>) | undefined;
  let shuttingDown = false;

  const shutdown = async (exitCode: number): Promise<void> => {
    if (shuttingDown) return;
    shuttingDown = true;
    try {
      if (stopFn) {
        await stopFn();
      }
    } catch (err) {
      // A signal handler's rejected promise only ever surfaces as an
      // unhandledRejection — log and still exit rather than hanging silently.
      console.error("Error during shutdown:", err);
    } finally {
      exitFn(exitCode);
    }
  };

  process.on("SIGINT", () => {
    void shutdown(0);
  });
  process.on("SIGTERM", () => {
    void shutdown(0);
  });

  startDevStack(name, {
    // Captured as soon as the live teardown closure exists — before either
    // child spawns — so a signal arriving mid-startup-poll can still tear
    // down whatever has been spawned so far (see StartDevStackOpts docs).
    onTeardownReady: (stop) => {
      stopFn = stop;
    },
  })
    .then(() => {
      // stopFn is already set via onTeardownReady; nothing further to do.
    })
    .catch((err) => {
      console.error(err instanceof Error ? err.message : String(err));
      void shutdown(1);
    });
}

if (require.main === module) {
  main();
}
