import { exec, spawn, ChildProcess } from 'child_process';
import { promisify } from 'util';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import * as readline from 'readline';

const execPromise = promisify(exec);

/**
 * TestSSHD manages a standalone, real SSH server (tests/e2e/sshd) used as the SSH target for
 * remote-workspaces.spec.ts. See that binary's doc comment for why it exists and what it does
 * (real local shell exec over a real SSH channel, fresh host key per start for a genuine TOFU
 * flow) -- this class is this repo's TestServer-shaped wrapper around spawning/stopping it,
 * mirroring TestServer's own ensureBinary/start/stop pattern in this same directory.
 */
export class TestSSHD {
  private process: ChildProcess | null = null;
  private binPath: string;
  private port = 0;
  private basePath = '';

  constructor(binPath?: string) {
    this.binPath = binPath || path.join(__dirname, '../.bin/e2e-sshd');
  }

  async start(): Promise<void> {
    await this.ensureBinary();
    this.basePath = await fs.promises.mkdtemp(path.join(os.tmpdir(), 'ssq-e2e-sshd-basepath-'));

    this.process = spawn(this.binPath, ['-port', '0'], {
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    this.process.stderr?.on('data', (chunk) => {
      console.error(`[e2e-sshd] ${chunk.toString().trimEnd()}`);
    });

    this.port = await this.waitForReady();
    console.log(`✅ Test SSH server (e2e-sshd) started on 127.0.0.1:${this.port}, basePath=${this.basePath}`);
  }

  private waitForReady(): Promise<number> {
    if (!this.process?.stdout) {
      return Promise.reject(new Error('e2e-sshd process has no stdout'));
    }
    return new Promise((resolve, reject) => {
      const rl = readline.createInterface({ input: this.process!.stdout! });
      const timeout = setTimeout(() => {
        rl.close();
        reject(new Error('e2e-sshd did not print READY within 15s'));
      }, 15000);

      rl.on('line', (line) => {
        const match = line.match(/^READY (\d+)$/);
        if (match) {
          clearTimeout(timeout);
          rl.close();
          resolve(parseInt(match[1], 10));
        }
      });

      this.process!.once('exit', (code) => {
        clearTimeout(timeout);
        reject(new Error(`e2e-sshd exited before READY (code ${code})`));
      });
    });
  }

  private async ensureBinary(): Promise<void> {
    const stats = await fs.promises.stat(this.binPath).catch(() => null);
    if (stats?.isFile()) {
      const age = Date.now() - stats.mtimeMs;
      if (age < 3600000) return;
    }

    console.log('Building e2e-sshd binary...');
    await fs.promises.mkdir(path.dirname(this.binPath), { recursive: true });
    const projectRoot = path.join(__dirname, '../../..');
    await execPromise(`go build -o "${this.binPath}" ./tests/e2e/sshd`, { cwd: projectRoot });
    console.log('✅ e2e-sshd binary built');
  }

  async stop(): Promise<void> {
    if (this.process) {
      this.process.kill('SIGTERM');
      await new Promise<void>((resolve) => {
        if (!this.process) {
          resolve();
          return;
        }
        this.process.on('exit', () => resolve());
        setTimeout(() => {
          this.process?.kill('SIGKILL');
          resolve();
        }, 5000);
      });
      this.process = null;
    }
    if (this.basePath) {
      await fs.promises.rm(this.basePath, { recursive: true, force: true }).catch(() => {});
    }
  }

  getHost(): string {
    return '127.0.0.1';
  }

  getPort(): number {
    return this.port;
  }

  getUser(): string {
    return os.userInfo().username;
  }

  /** A fresh, writable local directory this sshd's real shell can use as a remote's base_path. */
  getBasePath(): string {
    return this.basePath;
  }
}

// Deliberately no getGlobalTestSSHD()/module-level singleton here (an earlier version had one,
// wired through global-setup.ts/global-teardown.ts): a single sshd instance shared across the
// whole `npx playwright test` invocation means only the FIRST project to dial it ever sees an
// unknown host key -- KnownHostsStore trust is keyed by host:port, server-side, for the life of
// the test server, so every later project (chromium-dom after chromium) or retry sees an
// ALREADY-trusted host and skips the TOFU dialog entirely. Callers that need a fresh,
// never-before-seen host (i.e. remote-workspaces.spec.ts) must construct and own their own
// `new TestSSHD()` scoped to their own file (test.beforeAll/afterAll), so every project's
// separate run of that file gets its own port and its own fresh host key.
