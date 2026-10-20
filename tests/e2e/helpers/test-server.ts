import { exec, spawn, ChildProcess } from 'child_process';
import { promisify } from 'util';
import * as fs from 'fs';
import * as net from 'net';
import * as os from 'os';
import * as path from 'path';
import { SessionClient } from './session-client';

const execPromise = promisify(exec);

function findFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.unref();
    srv.on('error', reject);
    srv.listen(0, () => {
      const port = (srv.address() as net.AddressInfo).port;
      srv.close(() => resolve(port));
    });
  });
}

export interface TestServerConfig {
  port: number;
  testDir: string;
  buildPath: string;
  seedSessions: number;
  liveSeedSessions: number;
  /**
   * Also start the remote-access HTTPS listener (--remote-access). Passkey auth is attached only to
   * that listener, so unauthenticated-login behaviour is observed at getRemoteBaseUrl() with TLS
   * errors ignored (the listener uses a private CA).
   */
  remoteAuth: boolean;
  /** 0 means "pick a free port at start() time"; only used when remoteAuth is set. */
  remotePort: number;
}

export class TestServer {
  private process: ChildProcess | null = null;
  private config: TestServerConfig;
  // PID-scoped copy of the shared repo-root binary (see ensureBinary) — set
  // when we made one, so stop() knows to clean it up.
  private binDir: string | null = null;

  constructor(config: Partial<TestServerConfig> = {}) {
    const pid = process.pid;
    // 0 means "pick a free port at start() time"; TEST_SERVER_PORT overrides for CI
    const port = config.port ?? parseInt(process.env.TEST_SERVER_PORT || '0', 10);
    this.config = {
      port,
      testDir: config.testDir || process.env.TEST_SERVER_DIR || `/tmp/stapler-squad-test-${pid}`,
      // TEST_SERVER_BINARY: additive override for Story 2.3.2's woven-binary
      // parity leg (project_plans/go-auto-instrumentation) — e.g.
      // TEST_SERVER_BINARY=$(pwd)/../../stapler-squad-otel npm test. A no-op
      // when unset: falls through to the existing default, unchanged.
      buildPath: config.buildPath || process.env.TEST_SERVER_BINARY || path.join(__dirname, '../../../stapler-squad'),
      seedSessions: config.seedSessions ?? 6,
      liveSeedSessions: config.liveSeedSessions ?? 3,
      remoteAuth: config.remoteAuth ?? false,
      remotePort: config.remotePort ?? 0,
    };
  }

  /**
   * Start the test server with isolated data directory and seeded demo sessions.
   */
  async start(): Promise<void> {
    if (this.config.port === 0) {
      this.config.port = await findFreePort();
    }
    console.log(`Starting test server on port ${this.config.port}...`);
    console.log(`Test data directory: ${this.config.testDir}`);

    await this.cleanupTestDir();
    await this.ensureBinary();
    await this.seedDemoData();
    await this.seedHiddenSession();

    if (this.config.remoteAuth && this.config.remotePort === 0) {
      this.config.remotePort = await findFreePort();
    }
    const remoteArgs = this.config.remoteAuth ? ['--remote-access', '--remote-port', String(this.config.remotePort)] : [];

    this.process = spawn(this.config.buildPath, [
      '--test-mode',
      '--test-dir', this.config.testDir,
      '--tmux-keep-server',
      ...remoteArgs,
    ], {
      env: {
        ...process.env,
        PORT: this.config.port.toString(),
        // Registers the /api/debug/backlog/{seed,mutate}-* handlers
        // (server/server.go) that BacklogMutations.ts and the seed helpers
        // rely on. --test-dir already fully isolates this instance's data
        // dir (STAPLER_SQUAD_TEST_DIR takes priority over this in
        // config.GetConfigDirForDir), so this only affects the debug-handler
        // gate and log prefix, not data isolation.
        STAPLER_SQUAD_INSTANCE: 'e2e-local',
        // A private tmux server, so this run's sessions never share the live service's
        // default server (where its leaked-control-mode sweeper could see them).
        STAPLER_SQUAD_TMUX_SOCKET: this.tmuxSocket(),
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    await this.waitForServer();
    if (this.config.remoteAuth) await this.waitForPort(this.config.remotePort);
    await this.seedLiveSessions();

    console.log(`✅ Test server started on http://localhost:${this.config.port}`);
  }

  /**
   * Stop the test server and cleanup
   */
  async stop(): Promise<void> {
    if (this.process) {
      console.log('Stopping test server...');
      this.process.kill('SIGTERM');

      await new Promise<void>((resolve) => {
        if (this.process) {
          this.process.on('exit', () => resolve());
          setTimeout(() => {
            if (this.process) {
              this.process.kill('SIGKILL');
              resolve();
            }
          }, 5000);
        } else {
          resolve();
        }
      });

      this.process = null;
      console.log('✅ Test server stopped');
    }
    // --tmux-keep-server leaves the private tmux server behind; it holds only this run's sessions.
    await execPromise(`tmux -L ${this.tmuxSocket()} kill-server`).catch(() => undefined);

    await this.cleanupTestDir();
    await this.cleanupBinDir();
  }

  /** The private tmux socket name of this run: port-scoped, so concurrent runs never share one. */
  tmuxSocket(): string {
    return `ssq-e2e-${this.config.port}`;
  }

  getBaseUrl(): string {
    return `http://localhost:${this.config.port}`;
  }

  /** HTTPS origin of the remote-access listener; only meaningful when remoteAuth is set. */
  getRemoteBaseUrl(): string {
    return `https://localhost:${this.config.remotePort}`;
  }

  getTestDir(): string {
    return this.config.testDir;
  }

  // 90s: on this machine a cold test-mode boot (DB init + demo seeding) has been
  // observed taking ~30-45s before /health responds, right at the old 30s cap.
  private async waitForServer(maxAttempts = 90): Promise<void> {
    const url = `${this.getBaseUrl()}/health`;

    for (let i = 0; i < maxAttempts; i++) {
      try {
        const response = await fetch(url);
        if (response.ok) {
          return;
        }
      } catch {
        // Server not ready yet
      }

      await new Promise(resolve => setTimeout(resolve, 1000));
    }

    throw new Error(`Server failed to start after ${maxAttempts} seconds`);
  }

  /** The remote listener comes up after the local one; wait until its port accepts connections. */
  private async waitForPort(port: number, maxAttempts = 60): Promise<void> {
    for (let i = 0; i < maxAttempts; i++) {
      const open = await new Promise<boolean>((resolve) => {
        const socket = net.connect(port, '127.0.0.1');
        socket.once('connect', () => { socket.destroy(); resolve(true); });
        socket.once('error', () => resolve(false));
      });
      if (open) return;
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
    throw new Error(`Remote listener on port ${port} did not open after ${maxAttempts} seconds`);
  }

  private async ensureBinary(): Promise<void> {
    // TEST_SERVER_BINARY (woven-binary parity leg) points at a binary built
    // by a separate pipeline — never rebuild or copy it here.
    if (process.env.TEST_SERVER_BINARY) {
      return;
    }

    // `make stapler-squad` — not a raw `go build` — so its own dependency
    // chain (server/web/dist <- web-app/out <- web-app/src/**) decides
    // whether the frontend needs rebuilding. A wall-clock age check here
    // previously let a binary up to 1h old skip rebuilding even when
    // web-app/src had just changed, silently serving a stale embedded UI to
    // Playwright (see 2026-09-04 insights-cost-intelligence e2e debugging).
    console.log('Ensuring Go binary is up to date (make stapler-squad)...');
    const projectRoot = path.join(__dirname, '../../..');
    // `npx playwright` leaves npm_config_user_agent=npm/..., which web-app's bun-only
    // preinstall guard rejects when make triggers `bun install`.
    const { npm_config_user_agent: _npmUserAgent, ...makeEnv } = process.env;
    await execPromise('make stapler-squad', { cwd: projectRoot, env: makeEnv });
    console.log('✅ Binary up to date');

    // The Makefile target always writes to the single repo-root path, which
    // every concurrent e2e run / manual instance in this (shared, non-worktree
    // -isolated) checkout would otherwise spawn from directly. testDir is
    // already PID-scoped (see the constructor) — mirror that here with a
    // PID-scoped copy, so concurrent runs against this same checkout can't
    // race a rebuild out from under one another's already-spawned process.
    this.binDir = path.join(os.tmpdir(), 'stapler-squad-e2e-bin', String(process.pid));
    await fs.promises.mkdir(this.binDir, { recursive: true });
    const isolatedPath = path.join(this.binDir, 'stapler-squad');
    await fs.promises.copyFile(path.join(projectRoot, 'stapler-squad'), isolatedPath);
    await fs.promises.chmod(isolatedPath, 0o755);
    this.config.buildPath = isolatedPath;
  }

  /**
   * Seed demo sessions so tests that need session cards have data to work with.
   * Mirrors the e2e-video.yml CI workflow: `go run ./tests/demo/seed <dir> <count>`
   */
  private async seedDemoData(): Promise<void> {
    if (this.config.seedSessions <= 0) return;

    const projectRoot = path.join(__dirname, '../../..');
    const seedCmd = `go run ./tests/demo/seed "${this.config.testDir}" ${this.config.seedSessions}`;
    console.log(`Seeding ${this.config.seedSessions} demo sessions...`);
    try {
      await execPromise(seedCmd, { cwd: projectRoot });
      console.log('✅ Demo sessions seeded');
    } catch (err) {
      console.warn(`Warning: Failed to seed demo data: ${err}`);
    }
  }

  /**
   * Writes one hidden review session ("e2e-hidden-review") into the database so the
   * delivery gate resolves it as hidden. Hidden sessions cannot be created over the API.
   */
  private async seedHiddenSession(): Promise<void> {
    const projectRoot = path.join(__dirname, '../../..');
    try {
      await execPromise(`go run ./tests/e2e/seed-hidden "${this.config.testDir}"`, { cwd: projectRoot });
      console.log('✅ Hidden review session seeded');
    } catch (err) {
      console.warn(`Warning: Failed to seed hidden session: ${err}`);
    }
  }

  /**
   * Create real tmux-backed bash sessions via the live API so the review queue
   * has items for tests that assert on queue presence and acknowledge behaviour.
   * Sessions run `bash` in isolated temp directories; the review queue poller
   * flags them as idle (ReasonIdle) within ~5 seconds of inactivity.
   */
  private async seedLiveSessions(): Promise<void> {
    if (this.config.liveSeedSessions <= 0) return;

    const client = new SessionClient(this.getBaseUrl());
    const liveDir = path.join(this.config.testDir, 'live');
    await fs.promises.mkdir(liveDir, { recursive: true });

    let created = 0;
    for (let i = 1; i <= this.config.liveSeedSessions; i++) {
      const title = `e2e-review-${i}`;
      const sessionPath = path.join(liveDir, `s${i}`);
      await fs.promises.mkdir(sessionPath, { recursive: true });

      try {
        await client.createIdleSession(title, sessionPath, { category: 'E2E Test', tags: ['e2e'] });
        created++;
        console.log(`  ✓ Created live session: ${title}`);
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        console.warn(`  Warning: Failed to create live session ${title}: ${msg}`);
      }
    }

    if (created > 0) {
      // Review queue poller fires every 2s; idle threshold is 5s. Wait 12s to be safe.
      console.log(`Waiting for review queue poller to detect ${created} idle sessions...`);
      await new Promise(resolve => setTimeout(resolve, 12000));
      console.log('✅ Live sessions seeded for review queue tests');
    }
  }

  private async cleanupTestDir(): Promise<void> {
    try {
      if (fs.existsSync(this.config.testDir)) {
        await fs.promises.rm(this.config.testDir, { recursive: true, force: true });
        console.log(`✅ Cleaned up test directory: ${this.config.testDir}`);
      }
    } catch (error) {
      console.warn(`Warning: Failed to cleanup test directory: ${error}`);
    }
  }

  private async cleanupBinDir(): Promise<void> {
    if (!this.binDir) return;
    try {
      await fs.promises.rm(this.binDir, { recursive: true, force: true });
    } catch (error) {
      console.warn(`Warning: Failed to cleanup binary directory: ${error}`);
    } finally {
      this.binDir = null;
    }
  }
}

let globalTestServer: TestServer | null = null;

export function getGlobalTestServer(): TestServer {
  if (!globalTestServer) {
    globalTestServer = new TestServer();
  }
  return globalTestServer;
}

export async function startGlobalTestServer(): Promise<void> {
  const server = getGlobalTestServer();
  await server.start();
}

export async function stopGlobalTestServer(): Promise<void> {
  if (globalTestServer) {
    await globalTestServer.stop();
    globalTestServer = null;
  }
}
