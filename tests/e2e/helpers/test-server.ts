import { exec, spawn, ChildProcess } from 'child_process';
import { promisify } from 'util';
import * as fs from 'fs';
import * as path from 'path';

const execPromise = promisify(exec);

export interface TestServerConfig {
  port: number;
  testDir: string;
  buildPath: string;
}

export class TestServer {
  private process: ChildProcess | null = null;
  private config: TestServerConfig;

  constructor(config: Partial<TestServerConfig> = {}) {
    const pid = process.pid;
    this.config = {
      port: config.port || 8544, // Use different port for tests
      testDir: config.testDir || `/tmp/claude-squad-test-${pid}`,
      buildPath: config.buildPath || path.join(__dirname, '../../../claude-squad'),
    };
  }

  /**
   * Start the test server with isolated data directory
   */
  async start(): Promise<void> {
    console.log(`Starting test server on port ${this.config.port}...`);
    console.log(`Test data directory: ${this.config.testDir}`);

    // Ensure test directory is clean
    await this.cleanupTestDir();

    // Build the Go binary if needed
    await this.ensureBinary();

    // Start the server in test mode
    this.process = spawn(this.config.buildPath, [
      '--web',
      '--test-mode',
      '--test-dir', this.config.testDir
    ], {
      env: {
        ...process.env,
        PORT: this.config.port.toString(),
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    // Wait for server to be ready
    await this.waitForServer();

    console.log(`✅ Test server started on http://localhost:${this.config.port}`);
  }

  /**
   * Stop the test server and cleanup
   */
  async stop(): Promise<void> {
    if (this.process) {
      console.log('Stopping test server...');
      this.process.kill('SIGTERM');

      // Wait for process to exit
      await new Promise<void>((resolve) => {
        if (this.process) {
          this.process.on('exit', () => resolve());
          // Force kill after 5 seconds
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

    // Cleanup test directory
    await this.cleanupTestDir();
  }

  /**
   * Get the base URL for the test server
   */
  getBaseUrl(): string {
    return `http://localhost:${this.config.port}`;
  }

  /**
   * Get the test data directory
   */
  getTestDir(): string {
    return this.config.testDir;
  }

  /**
   * Wait for the server to be ready
   */
  private async waitForServer(maxAttempts = 30): Promise<void> {
    const url = `${this.getBaseUrl()}/health`;

    for (let i = 0; i < maxAttempts; i++) {
      try {
        const response = await fetch(url);
        if (response.ok) {
          return;
        }
      } catch (error) {
        // Server not ready yet
      }

      // Wait 1 second between attempts
      await new Promise(resolve => setTimeout(resolve, 1000));
    }

    throw new Error(`Server failed to start after ${maxAttempts} seconds`);
  }

  /**
   * Ensure the Go binary is built
   */
  private async ensureBinary(): Promise<void> {
    // Check if binary exists and is recent
    const stats = await fs.promises.stat(this.config.buildPath).catch(() => null);
    if (stats && stats.isFile()) {
      // Binary exists, check if it's recent (within last hour)
      const age = Date.now() - stats.mtimeMs;
      if (age < 3600000) {
        return; // Binary is recent enough
      }
    }

    console.log('Building Go binary...');
    const projectRoot = path.join(__dirname, '../../..');
    await execPromise('go build -o claude-squad .', { cwd: projectRoot });
    console.log('✅ Binary built');
  }

  /**
   * Cleanup test directory
   */
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

// Global test server instance
let globalTestServer: TestServer | null = null;

/**
 * Get or create the global test server instance
 */
export function getGlobalTestServer(): TestServer {
  if (!globalTestServer) {
    globalTestServer = new TestServer();
  }
  return globalTestServer;
}

/**
 * Start the global test server (for use in globalSetup)
 */
export async function startGlobalTestServer(): Promise<void> {
  const server = getGlobalTestServer();
  await server.start();
}

/**
 * Stop the global test server (for use in globalTeardown)
 */
export async function stopGlobalTestServer(): Promise<void> {
  if (globalTestServer) {
    await globalTestServer.stop();
    globalTestServer = null;
  }
}
