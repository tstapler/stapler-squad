import { exec, spawn, ChildProcess } from 'child_process';
import { promisify } from 'util';
import * as fs from 'fs';
import * as net from 'net';
import * as os from 'os';
import * as path from 'path';
import { randomBytes } from 'crypto';
import { SessionClient } from './session-client';
import { webBuildMode } from './web-build';

const execPromise = promisify(exec);

export function findFreePort(): Promise<number> {
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
  /** Prebuilt binary + seed tools shared by a pool; when unset start() prepares (and owns) its own. */
  assets?: SharedAssets;
  /** Port source for port 0; injectable so the lost-port retry can be exercised deterministically. */
  allocatePort?: () => Promise<number>;
}

/** Binary and seed tools built once and shared read-only by every server of a run. */
export interface SharedAssets {
  binary: string;
  seedDemo: string;
  seedHidden: string;
  /** Directory holding all of the above; removed by cleanupSharedAssets. */
  dir: string;
}

const projectRoot = path.join(__dirname, '../../..');

/**
 * Builds the server binary (unless TEST_SERVER_BINARY pins one) and the two seed tools once.
 * Building the seed tools up front replaces a per-server `go run` (compile + link on every start).
 */
export async function prepareSharedAssets(): Promise<SharedAssets> {
  const dir = path.join(os.tmpdir(), 'stapler-squad-e2e-bin', `${process.pid}-${Date.now()}`);
  await fs.promises.mkdir(dir, { recursive: true });
  let binary = process.env.TEST_SERVER_BINARY || '';
  if (!binary) {
    // `make stapler-squad` — not a raw `go build` — so its own dependency chain
    // (server/web/dist <- web-app/out <- web-app/src/**) decides whether the frontend
    // needs rebuilding; a wall-clock age check once served a stale embedded UI.
    console.log('Ensuring Go binary is up to date (make stapler-squad)...');
    // `npx playwright` leaves npm_config_user_agent=npm/..., which web-app's bun-only
    // preinstall guard rejects when make triggers `bun install`.
    const { npm_config_user_agent: _npmUserAgent, ...makeEnv } = process.env;
    // The Makefile seam reads WEB_BUILD; set it explicitly so a typo fails here, not as a silent Next build.
    makeEnv.WEB_BUILD = webBuildMode();
    await execPromise('make stapler-squad', { cwd: projectRoot, env: makeEnv });
    console.log('✅ Binary up to date');
    // The Makefile writes one repo-root path that concurrent runs would race a rebuild over;
    // run from a private copy (TEST_SERVER_BINARY points at a separately built binary: never copy it).
    binary = path.join(dir, 'stapler-squad');
    await fs.promises.copyFile(path.join(projectRoot, 'stapler-squad'), binary);
    await fs.promises.chmod(binary, 0o755);
  }
  // CI shards run in the Playwright image (no Go, no generated sources), so the binary job prebuilds
  // the seed tools and the shard points at them.
  let seedDemo = process.env.TEST_SEED_DEMO || '';
  let seedHidden = process.env.TEST_SEED_HIDDEN || '';
  if (!seedDemo || !seedHidden) {
    seedDemo = path.join(dir, 'seed-demo');
    seedHidden = path.join(dir, 'seed-hidden');
    await Promise.all([
      execPromise(`go build -o "${seedDemo}" ./tests/demo/seed`, { cwd: projectRoot }),
      execPromise(`go build -o "${seedHidden}" ./tests/e2e/seed-hidden`, { cwd: projectRoot }),
    ]);
  }
  return { binary, seedDemo, seedHidden, dir };
}

export async function cleanupSharedAssets(assets: SharedAssets): Promise<void> {
  await fs.promises.rm(assets.dir, { recursive: true, force: true }).catch((error) => {
    console.warn(`Warning: Failed to cleanup binary directory: ${error}`);
  });
}

/** Free ports are probed then released, so another process can grab one before we bind. */
const MAX_START_ATTEMPTS = 3;

export class TestServer {
  private process: ChildProcess | null = null;
  private config: TestServerConfig;
  // Set when start() had to prepare its own assets, so stop() cleans them up.
  private ownedAssets: SharedAssets | null = null;
  // Fixed per instance and independent of the port, so a retry after a lost port race can never
  // kill the tmux server of the sibling that won that port.
  private readonly socketId = randomBytes(4).toString('hex');

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
      assets: config.assets,
      allocatePort: config.allocatePort,
    };
  }

  /**
   * Start the test server with isolated data directory and seeded demo sessions.
   */
  async start(): Promise<void> {
    if (!this.config.assets) {
      this.ownedAssets = await prepareSharedAssets();
      this.config.assets = this.ownedAssets;
    }
    const autoPort = this.config.port === 0;
    const autoRemotePort = this.config.remoteAuth && this.config.remotePort === 0;
    for (let attempt = 1; ; attempt++) {
      if (autoPort) this.config.port = await (attempt === 1 ? this.config.allocatePort ?? findFreePort : findFreePort)();
      if (autoRemotePort) this.config.remotePort = 0;
      try {
        await this.startOnce();
        return;
      } catch (err) {
        await this.killProcess();
        if (!(err instanceof ServerExitedEarly) || !autoPort || attempt >= MAX_START_ATTEMPTS) {
          // stop() is not called after a failed start; don't leak the assets this server built for itself.
          if (this.ownedAssets) {
            await cleanupSharedAssets(this.ownedAssets);
            this.ownedAssets = null;
            this.config.assets = undefined;
          }
          throw err;
        }
        console.warn(`Server lost its port or exited before becoming healthy on port ${this.config.port} (attempt ${attempt}); retrying on a new port`);
      }
    }
  }

  private async startOnce(): Promise<void> {
    const assets = this.config.assets!;
    console.log(`Starting test server on port ${this.config.port}...`);
    console.log(`Test data directory: ${this.config.testDir}`);

    await this.cleanupTestDir();
    await this.seedDemoData(assets);
    await this.seedHiddenSession(assets);

    if (this.config.remoteAuth && this.config.remotePort === 0) {
      this.config.remotePort = await findFreePort();
    }
    const remoteArgs = this.config.remoteAuth ? ['--remote-access', '--remote-port', String(this.config.remotePort)] : [];

    this.process = spawn(assets.binary, [
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
    await this.waitForServer(this.process);
    if (this.config.remoteAuth) await this.waitForPort(this.config.remotePort);
    await this.seedLiveSessions();

    console.log(`✅ Test server started on http://localhost:${this.config.port}`);
  }

  private async killProcess(): Promise<void> {
    const proc = this.process;
    this.process = null;
    if (proc && proc.exitCode === null && proc.signalCode === null) {
      await new Promise<void>((resolve) => {
        proc.once('exit', () => resolve());
        proc.kill('SIGKILL');
      });
    }
    // --tmux-keep-server leaves this attempt's private tmux server behind.
    await execPromise(`tmux -L ${this.tmuxSocket()} kill-server`).catch(() => undefined);
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
    if (this.ownedAssets) {
      await cleanupSharedAssets(this.ownedAssets);
      this.ownedAssets = null;
      this.config.assets = undefined;
    }
  }

  /** The private tmux socket name of this server: unique per instance, so concurrent servers never share one. */
  tmuxSocket(): string {
    return `ssq-e2e-${this.socketId}`;
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
  private async waitForServer(proc: ChildProcess, maxAttempts = 90): Promise<void> {
    const url = `${this.getBaseUrl()}/health`;

    for (let i = 0; i < maxAttempts; i++) {
      // A dead process means the port was lost to another listener; /health on it would be someone else's.
      if (proc.exitCode !== null || proc.signalCode !== null) throw new ServerExitedEarly(proc.exitCode, proc.signalCode);
      if (await this.logReportsBindFailure()) throw new ServerExitedEarly(null, null, 'bind failed: address already in use');
      try {
        const response = await fetch(url, { signal: AbortSignal.timeout(2000) });
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

  /**
   * The server does not exit when its HTTP listener fails to bind: it logs "HTTP server stopped ...
   * address already in use" and keeps running without serving. That is how a lost findFreePort()
   * race looks, so read it from the server's own log.
   */
  private async logReportsBindFailure(): Promise<boolean> {
    const logPath = path.join(this.config.testDir, 'logs', 'staplersquad.log');
    const text = await fs.promises.readFile(logPath, 'utf8').catch(() => '');
    return /HTTP server stopped.*address already in use/.test(text);
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

  /**
   * Seed demo sessions so tests that need session cards have data to work with.
   * Same seeder as the e2e-video.yml CI workflow (`go run ./tests/demo/seed <dir> <count>`), prebuilt once per run.
   */
  private async seedDemoData(assets: SharedAssets): Promise<void> {
    if (this.config.seedSessions <= 0) return;
    console.log(`Seeding ${this.config.seedSessions} demo sessions...`);
    try {
      await execPromise(`"${assets.seedDemo}" "${this.config.testDir}" ${this.config.seedSessions}`);
      console.log('✅ Demo sessions seeded');
    } catch (err) {
      console.warn(`Warning: Failed to seed demo data: ${err}`);
    }
  }

  /**
   * Writes one hidden review session ("e2e-hidden-review") into the database so the
   * delivery gate resolves it as hidden. Hidden sessions cannot be created over the API.
   */
  private async seedHiddenSession(assets: SharedAssets): Promise<void> {
    try {
      await execPromise(`"${assets.seedHidden}" "${this.config.testDir}"`);
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

    const createdTitles: string[] = [];
    for (let i = 1; i <= this.config.liveSeedSessions; i++) {
      const title = `e2e-review-${i}`;
      const sessionPath = path.join(liveDir, `s${i}`);
      await fs.promises.mkdir(sessionPath, { recursive: true });

      try {
        await client.createIdleSession(title, sessionPath, { category: 'E2E Test', tags: ['e2e'] });
        createdTitles.push(title);
        console.log(`  ✓ Created live session: ${title}`);
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        console.warn(`  Warning: Failed to create live session ${title}: ${msg}`);
      }
    }

    if (createdTitles.length > 0) {
      // Poll for the sessions to be running instead of a fixed sleep. (The review queue itself stayed
      // empty for these bash sessions in a manual 85 s probe, so a fixed 12 s wait never guaranteed
      // queue membership; specs that need queue items wait for them via waitForReviewQueue.)
      console.log(`Waiting for ${createdTitles.length} live sessions to become active...`);
      await client.waitForSessionsActive(createdTitles);
      console.log('✅ Live sessions seeded');
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
}

class ServerExitedEarly extends Error {
  constructor(code: number | null, signal: NodeJS.Signals | null, reason = `exited (code=${code}, signal=${signal})`) {
    super(`test server ${reason} before becoming healthy`);
    this.name = 'ServerExitedEarly';
  }
}
