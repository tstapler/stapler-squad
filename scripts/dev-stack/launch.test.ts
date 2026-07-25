import { spawn, ChildProcess } from "child_process";
import { EventEmitter } from "events";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import {
  startDevStack,
  main,
  waitForReady,
  reconcileStaleManifest,
  reapOrphanedChildren,
  getManifestPath,
  DevStackManifest,
} from "./launch";

// net's createServer getter issue (documented in ports.test.ts) doesn't apply
// to allocatePort itself, but we still want to override its return value for
// the one pure-unit test below without breaking every other (real-socket)
// integration test in this file. Wrapping the actual implementation in a
// jest.fn() lets individual tests queue mockImplementationOnce() calls while
// everything else transparently calls through to the real allocator.
jest.mock("./ports", () => {
  const actual = jest.requireActual<typeof import("./ports")>("./ports");
  return { ...actual, allocatePort: jest.fn(actual.allocatePort) };
});

jest.setTimeout(20000);

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const createdInstances: string[] = [];

function uniqueName(label: string): string {
  const name = `devstack-test-${label}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  createdInstances.push(name);
  return name;
}

function sleepMs(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/** Spawns a real long-lived detached process standing in for a Backend/FrontendChild. */
function spawnSleep(): ChildProcess {
  return spawn("sleep", ["300"], { detached: true, stdio: ["ignore", "pipe", "pipe"] });
}

/** Spawns a real short-lived process and resolves with its PID once it has exited (a confirmed-dead PID). */
function spawnAndWaitExit(): Promise<number> {
  return new Promise((resolve, reject) => {
    const child = spawn("true", [], { stdio: "ignore" });
    if (child.pid === undefined) {
      reject(new Error("spawnAndWaitExit(): child had no pid"));
      return;
    }
    const pid = child.pid;
    child.once("exit", () => resolve(pid));
    child.once("error", reject);
  });
}

/** A fake ChildProcess for pure-unit tests — no real OS process behind it. */
function createFakeChild(pid: number): ChildProcess {
  const ee = new EventEmitter();
  return Object.assign(ee, {
    pid,
    stdout: new EventEmitter(),
    stderr: new EventEmitter(),
    exitCode: null,
    signalCode: null,
  }) as unknown as ChildProcess;
}

async function writeFixtureManifest(instance: string, overrides: Partial<DevStackManifest>): Promise<void> {
  const manifestPath = getManifestPath(instance);
  await fs.promises.mkdir(path.dirname(manifestPath), { recursive: true });
  const manifest: DevStackManifest = {
    instance,
    backendPort: 0,
    frontendPort: 0,
    apiBaseUrl: "",
    dataDir: "",
    pid: 999999,
    backendPid: 0,
    frontendPid: 0,
    schemaVersion: 2,
    ...overrides,
  };
  await fs.promises.writeFile(manifestPath, JSON.stringify(manifest, null, 2));
}

function isPidAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

afterAll(async () => {
  await Promise.all(
    createdInstances.map(async (name) => {
      const dir = path.join(os.homedir(), ".stapler-squad", "instances", name);
      await fs.promises.rm(dir, { recursive: true, force: true }).catch(() => undefined);
    }),
  );
});

// ---------------------------------------------------------------------------
// REQ-5
// ---------------------------------------------------------------------------

describe("REQ-5: CLI entry point to spin up a named isolated stack", () => {
  it("startDevStack() should assemble a DevStackConfig with distinct backendPort and frontendPort when given an instance name", async () => {
    const { allocatePort } = jest.requireMock<typeof import("./ports")>("./ports");
    const allocatePortMock = allocatePort as jest.Mock;

    allocatePortMock
      .mockImplementationOnce(async () => ({ port: 45001, release: jest.fn() }))
      .mockImplementationOnce(async () => ({ port: 45002, release: jest.fn() }));

    const instance = uniqueName("config-assembly");
    const backendChild = createFakeChild(900001);
    const frontendChild = createFakeChild(900002);

    const result = await startDevStack(instance, {
      maxAttempts: 1,
      intervalMs: 1,
      spawnBackend: () => backendChild,
      spawnFrontend: () => frontendChild,
      checkBackendReady: async () => true,
      checkFrontendReady: async () => true,
    });

    expect(result.backendUrl).toBe("http://localhost:45001");
    expect(result.frontendUrl).toBe("http://localhost:45002");
    expect(result.backendUrl).not.toBe(result.frontendUrl);
  });

  it("startDevStack() should reject an instance name containing path-traversal characters", async () => {
    await expect(startDevStack("../../etc")).rejects.toThrow(/invalid/i);
    await expect(startDevStack("foo/bar")).rejects.toThrow(/invalid/i);
  });

  it("startDevStack() reconciliation sweep should never signal pid 0 or 1 even if a manifest claims one", async () => {
    const instance = uniqueName("pid-floor-guard");
    const killFn = jest.fn();

    await writeFixtureManifest(instance, { backendPid: 1, frontendPid: 0 });
    await reconcileStaleManifest(instance, { killFn });

    expect(killFn).not.toHaveBeenCalled();
  });

  it("main() should exit with a usage error when no instance name is passed as argv[2]", () => {
    const exitFn = jest.fn();
    const errorSpy = jest.spyOn(console, "error").mockImplementation(() => undefined);

    try {
      main(["node", "launch.ts"], exitFn);

      expect(exitFn).toHaveBeenCalledWith(1);
      expect(errorSpy).toHaveBeenCalled();
      expect(String(errorSpy.mock.calls[0][0])).toMatch(/usage/i);
    } finally {
      errorSpy.mockRestore();
    }
  });

  it("importing launch.ts should not register any SIGINT/SIGTERM listeners on the current process", () => {
    const sigintBefore = process.listenerCount("SIGINT");
    const sigtermBefore = process.listenerCount("SIGTERM");

    jest.resetModules();
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    require("./launch");

    expect(process.listenerCount("SIGINT")).toBe(sigintBefore);
    expect(process.listenerCount("SIGTERM")).toBe(sigtermBefore);
  });

  it("startDevStack() should terminate both children's process groups within 5s when teardown() is invoked", async () => {
    const instance = uniqueName("teardown");
    let backendChildRef: ChildProcess | undefined;
    let frontendChildRef: ChildProcess | undefined;

    const result = await startDevStack(instance, {
      maxAttempts: 5,
      intervalMs: 5,
      spawnBackend: () => {
        backendChildRef = spawnSleep();
        return backendChildRef;
      },
      spawnFrontend: () => {
        frontendChildRef = spawnSleep();
        return frontendChildRef;
      },
      checkBackendReady: async () => true,
      checkFrontendReady: async () => true,
    });

    const backendPid = backendChildRef!.pid!;
    const frontendPid = frontendChildRef!.pid!;

    const start = Date.now();
    await result.stop();
    const elapsed = Date.now() - start;

    expect(elapsed).toBeLessThan(5000);
    expect(isPidAlive(backendPid)).toBe(false);
    expect(isPidAlive(frontendPid)).toBe(false);
  });

  it("startDevStack() should expose a live teardown via onTeardownReady before either child spawns, so a signal arriving mid-startup-poll doesn't orphan the already-spawned BackendChild", async () => {
    const instance = uniqueName("early-teardown");
    let backendChildRef: ChildProcess | undefined;
    let capturedStop: (() => Promise<void>) | undefined;
    let frontendEverSpawned = false;

    const startPromise = startDevStack(instance, {
      maxAttempts: 100,
      intervalMs: 20,
      onTeardownReady: (stop) => {
        capturedStop = stop;
      },
      spawnBackend: () => {
        backendChildRef = spawnSleep();
        return backendChildRef;
      },
      spawnFrontend: () => {
        // Should never be reached in this test — teardown fires before the
        // backend readiness poll (and therefore the whole startup flow) gets
        // this far, since checkBackendReady() below never resolves true.
        frontendEverSpawned = true;
        return spawnSleep();
      },
      checkBackendReady: async () => false, // never ready — simulates a slow/hanging cold start
      checkFrontendReady: async () => true,
    });

    // Wait until onTeardownReady has fired and the BackendChild has actually spawned
    // (i.e. startDevStack() is genuinely mid-poll, not finished setting up yet).
    for (let i = 0; i < 100 && (!capturedStop || !backendChildRef?.pid); i++) {
      await sleepMs(20);
    }
    expect(capturedStop).toBeDefined();
    expect(backendChildRef?.pid).toBeDefined();

    const backendPid = backendChildRef!.pid!;
    expect(isPidAlive(backendPid)).toBe(true);

    // Simulate main()'s SIGINT handler calling the captured teardown while
    // startDevStack() is still stuck in its backend-readiness poll.
    await capturedStop!();

    expect(isPidAlive(backendPid)).toBe(false);
    expect(frontendEverSpawned).toBe(false);

    // The abandoned startDevStack() call itself will eventually reject (either
    // from waitForReady() exhausting maxAttempts, or from the internal aborted
    // check) — swallow that so it doesn't surface as an unhandled rejection.
    await startPromise.catch(() => undefined);
  });

  it("startDevStack() should write backendPid and frontendPid (not its own pid) into dev-stack.json once both children are ready", async () => {
    const instance = uniqueName("manifest");
    let backendChildRef: ChildProcess | undefined;
    let frontendChildRef: ChildProcess | undefined;

    const result = await startDevStack(instance, {
      maxAttempts: 5,
      intervalMs: 5,
      spawnBackend: () => {
        backendChildRef = spawnSleep();
        return backendChildRef;
      },
      spawnFrontend: () => {
        frontendChildRef = spawnSleep();
        return frontendChildRef;
      },
      checkBackendReady: async () => true,
      checkFrontendReady: async () => true,
    });

    try {
      const raw = await fs.promises.readFile(getManifestPath(instance), "utf8");
      const manifest = JSON.parse(raw) as DevStackManifest;

      expect(manifest.backendPid).toBe(backendChildRef!.pid);
      expect(manifest.frontendPid).toBe(frontendChildRef!.pid);
      expect(manifest.backendPid).not.toBe(manifest.pid);
      expect(manifest.frontendPid).not.toBe(manifest.pid);
      expect(manifest.schemaVersion).toBe(2);
    } finally {
      await result.stop();
    }
  });
});

// ---------------------------------------------------------------------------
// Hardest-won fix #3: bounded frontend-readiness poll kills the backend
// ---------------------------------------------------------------------------

describe("Hardest-won fix #3: bounded frontend-readiness poll", () => {
  it("startDevStack() should kill the already-started BackendChild and reject when the FrontendChild readiness poll times out", async () => {
    const instance = uniqueName("frontend-timeout");
    let backendChildRef: ChildProcess | undefined;
    let frontendChildRef: ChildProcess | undefined;

    await expect(
      startDevStack(instance, {
        maxAttempts: 3,
        intervalMs: 5,
        spawnBackend: () => {
          backendChildRef = spawnSleep();
          return backendChildRef;
        },
        spawnFrontend: () => {
          frontendChildRef = spawnSleep();
          return frontendChildRef;
        },
        checkBackendReady: async () => true,
        checkFrontendReady: async () => false,
      }),
    ).rejects.toThrow(/Frontend readiness check timed out/);

    expect(backendChildRef).toBeDefined();
    expect(frontendChildRef).toBeDefined();
    expect(isPidAlive(backendChildRef!.pid!)).toBe(false);
    expect(isPidAlive(frontendChildRef!.pid!)).toBe(false);
  });

  it("startDevStack() should resolve backendUrl and frontendUrl when both children become ready before the timeout", async () => {
    const instance = uniqueName("happy-path");

    const result = await startDevStack(instance, {
      maxAttempts: 5,
      intervalMs: 5,
      spawnBackend: () => spawnSleep(),
      spawnFrontend: () => spawnSleep(),
      checkBackendReady: async () => true,
      checkFrontendReady: async () => true,
    });

    try {
      const raw = await fs.promises.readFile(getManifestPath(instance), "utf8");
      const manifest = JSON.parse(raw) as DevStackManifest;

      expect(result.backendUrl).toBe(`http://localhost:${manifest.backendPort}`);
      expect(result.frontendUrl).toBe(`http://localhost:${manifest.frontendPort}`);
      expect(manifest.backendPort).not.toBe(manifest.frontendPort);
    } finally {
      await result.stop();
    }
  });

  it("waitForReady() should throw after maxAttempts is exhausted rather than polling forever", async () => {
    const checkFn = jest.fn().mockResolvedValue(false);
    const maxAttempts = 5;

    await expect(waitForReady(checkFn, { maxAttempts, intervalMs: 1 })).rejects.toThrow(
      /not ready after 5 attempts/i,
    );
    expect(checkFn).toHaveBeenCalledTimes(maxAttempts);
  });
});

// ---------------------------------------------------------------------------
// Hardest-won fix #4: orphan-reconciliation sweep
// ---------------------------------------------------------------------------

describe("Hardest-won fix #4: orphan-reconciliation sweep", () => {
  it("startDevStack() should reap only the alive PID and skip the already-dead PID when a stale manifest records one of each", async () => {
    const instance = uniqueName("reconcile-mixed");
    const aliveChild = spawnSleep();
    const alivePid = aliveChild.pid!;
    const deadPid = await spawnAndWaitExit();

    await writeFixtureManifest(instance, { pid: 888888, backendPid: alivePid, frontendPid: deadPid });

    let result: Awaited<ReturnType<typeof startDevStack>> | undefined;
    try {
      result = await startDevStack(instance, {
        maxAttempts: 5,
        intervalMs: 5,
        reconcileGraceMs: 2000,
        reconcilePollIntervalMs: 20,
        spawnBackend: () => spawnSleep(),
        spawnFrontend: () => spawnSleep(),
        checkBackendReady: async () => true,
        checkFrontendReady: async () => true,
      });

      expect(isPidAlive(alivePid)).toBe(false);
      expect(result.backendUrl).toMatch(/^http:\/\/localhost:\d+$/);
      expect(result.frontendUrl).toMatch(/^http:\/\/localhost:\d+$/);
    } finally {
      if (result) {
        await result.stop();
      }
      // Best-effort: ensure the dummy "alive" child never lingers even if an assertion above failed.
      try {
        process.kill(-alivePid, "SIGKILL");
      } catch {
        // already gone
      }
    }
  });

  it("startDevStack() reconciliation sweep should escalate to SIGKILL when the orphaned process ignores SIGTERM for 5s", async () => {
    const trapChild = spawn("sh", ["-c", 'trap "" TERM; sleep 300'], {
      detached: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    const trapPid = trapChild.pid!;
    // Give the shell a moment to install the trap before we start signaling it.
    await sleepMs(200);

    const deadFrontendPid = await spawnAndWaitExit();
    const instance = uniqueName("reconcile-sigkill");
    await writeFixtureManifest(instance, { pid: 777777, backendPid: trapPid, frontendPid: deadFrontendPid });

    try {
      const start = Date.now();
      await reconcileStaleManifest(instance, { graceMs: 5000, pollIntervalMs: 50 });
      const elapsed = Date.now() - start;

      expect(elapsed).toBeGreaterThanOrEqual(4900);
      expect(isPidAlive(trapPid)).toBe(false);
    } finally {
      try {
        process.kill(-trapPid, "SIGKILL");
      } catch {
        // already gone
      }
    }
  }, 15000);

  it("startDevStack() reconciliation sweep should signal backendPid and frontendPid independently and must never signal the manifest's own pid field", async () => {
    const calls: Array<[number, string | number | undefined]> = [];
    const killFn = (pid: number, signal?: string | number): void => {
      calls.push([pid, signal]);
      // Report "alive" for every signal-0 liveness probe so the sweep
      // proceeds to send a real termination signal.
    };

    const fixtureManifest = { pid: 11111, backendPid: 22222, frontendPid: 33333 };

    await reapOrphanedChildren(fixtureManifest, { killFn, graceMs: 20, pollIntervalMs: 5 });

    const signaledPids = calls.map(([pid]) => pid);

    expect(signaledPids).toContain(-22222);
    expect(signaledPids).toContain(-33333);
    expect(signaledPids).not.toContain(-11111);
    expect(signaledPids).not.toContain(11111);
  });

  it("startDevStack() reconciliation sweep should skip signaling entirely and just delete the manifest when both PIDs are already dead", async () => {
    const deadBackendPid = await spawnAndWaitExit();
    const deadFrontendPid = await spawnAndWaitExit();
    const instance = uniqueName("reconcile-both-dead");

    await writeFixtureManifest(instance, { pid: 555555, backendPid: deadBackendPid, frontendPid: deadFrontendPid });

    const killCalls: Array<[number, string | number | undefined]> = [];
    const killFn = (pid: number, signal?: string | number): void => {
      killCalls.push([pid, signal]);
      if (signal === 0) {
        const err = new Error("ESRCH") as NodeJS.ErrnoException;
        err.code = "ESRCH";
        throw err;
      }
    };

    await reconcileStaleManifest(instance, { killFn });

    const terminationCalls = killCalls.filter(([, signal]) => signal === "SIGTERM" || signal === "SIGKILL");
    expect(terminationCalls).toHaveLength(0);

    await expect(fs.promises.access(getManifestPath(instance))).rejects.toThrow();
  });
});
