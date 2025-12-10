/**
 * Log Line Generator
 * Generates high-volume log output for terminal stress testing
 */

import { LogFloodConfig, GeneratorFrame } from './types';

// Simple seeded random number generator
class SeededRandom {
  private seed: number;

  constructor(seed: number) {
    this.seed = seed;
  }

  next(): number {
    this.seed = (this.seed * 1103515245 + 12345) & 0x7fffffff;
    return this.seed / 0x7fffffff;
  }

  nextInt(max: number): number {
    return Math.floor(this.next() * max);
  }

  choice<T>(arr: T[]): T {
    return arr[this.nextInt(arr.length)];
  }
}

// Simple checksum
function checksum(str: string): number {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash;
  }
  return Math.abs(hash);
}

// ANSI color codes for log levels
const LOG_COLORS = {
  error: '\x1b[31m', // Red
  warn: '\x1b[33m',  // Yellow
  info: '\x1b[32m',  // Green
  debug: '\x1b[36m', // Cyan
};

const LOG_PREFIXES = {
  error: '[ERROR]',
  warn: '[WARN] ',
  info: '[INFO] ',
  debug: '[DEBUG]',
};

// Sample log messages for each level
const LOG_MESSAGES = {
  error: [
    'Connection refused: ECONNREFUSED',
    'Database query timeout after 30000ms',
    'Failed to parse JSON response: Unexpected token',
    'Authentication failed: Invalid credentials',
    'Out of memory: heap allocation failed',
    'File not found: /var/log/app.log',
    'Socket hang up: ECONNRESET',
    'TLS handshake failed: certificate expired',
  ],
  warn: [
    'High memory usage: 85% of available heap',
    'Slow query detected: 2500ms execution time',
    'Rate limit approaching: 90% of quota used',
    'Deprecated API endpoint called: /v1/legacy',
    'Cache miss rate increasing: 15%',
    'Connection pool near capacity: 95/100',
    'Disk space low: 10GB remaining',
    'Request retry attempt 2 of 3',
  ],
  info: [
    'Request processed successfully: 200 OK',
    'User authenticated: user@example.com',
    'Cache hit: key=session_12345',
    'Background job completed: export_data_123',
    'WebSocket connection established',
    'Configuration reloaded from file',
    'Health check passed: all services operational',
    'Batch processing started: 1000 items',
  ],
  debug: [
    'Entering function: processRequest()',
    'Variable state: count=42, status=active',
    'HTTP request: GET /api/v2/users?page=1',
    'Query executed: SELECT * FROM users LIMIT 10',
    'Response body: {"success":true,"data":[...]}',
    'Timer started: operationTimeout=5000ms',
    'Event emitted: user.created',
    'Middleware executed: authCheck (2ms)',
  ],
};

// Sample service names
const SERVICES = [
  'api-gateway',
  'user-service',
  'auth-service',
  'payment-service',
  'notification-service',
  'cache-service',
  'worker-service',
  'scheduler',
];

// Sample request IDs
function generateRequestId(rng: SeededRandom): string {
  const chars = 'abcdef0123456789';
  let id = '';
  for (let i = 0; i < 16; i++) {
    id += chars[rng.nextInt(chars.length)];
  }
  return id;
}

/**
 * Log Line Generator class
 */
export class LogLineGenerator {
  private config: LogFloodConfig;
  private rng: SeededRandom;
  private sequence: number = 0;
  private levelWeights: { level: keyof typeof LOG_COLORS; threshold: number }[];

  constructor(config: LogFloodConfig) {
    this.config = config;
    this.rng = new SeededRandom(config.seed || Date.now());

    // Build weighted level distribution
    const { levels } = config;
    const total = levels.error + levels.warn + levels.info + levels.debug;
    let cumulative = 0;
    this.levelWeights = [];

    for (const [level, weight] of Object.entries(levels)) {
      cumulative += weight / total;
      this.levelWeights.push({
        level: level as keyof typeof LOG_COLORS,
        threshold: cumulative,
      });
    }
  }

  /**
   * Select log level based on configured distribution
   */
  private selectLevel(): keyof typeof LOG_COLORS {
    const r = this.rng.next();
    for (const { level, threshold } of this.levelWeights) {
      if (r <= threshold) {
        return level;
      }
    }
    return 'info';
  }

  /**
   * Generate a single log line
   */
  private generateLine(): string {
    const level = this.selectLevel();
    const message = this.rng.choice(LOG_MESSAGES[level]);
    const service = this.rng.choice(SERVICES);
    const requestId = generateRequestId(this.rng);

    let line = '';

    // Timestamp
    if (this.config.timestamps) {
      const now = new Date();
      const ts = now.toISOString();
      line += `\x1b[90m${ts}\x1b[0m `;
    }

    // Log level with color
    if (this.config.colors) {
      line += `${LOG_COLORS[level]}${LOG_PREFIXES[level]}\x1b[0m `;
    } else {
      line += `${LOG_PREFIXES[level]} `;
    }

    // Service and request ID
    if (this.config.colors) {
      line += `\x1b[35m[${service}]\x1b[0m `;
      line += `\x1b[90m(${requestId})\x1b[0m `;
    } else {
      line += `[${service}] (${requestId}) `;
    }

    // Message
    line += message;

    return line;
  }

  /**
   * Generate a batch of log lines
   */
  nextFrame(batchSize: number = 10): GeneratorFrame {
    const startTime = performance.now();

    const lines: string[] = [];
    for (let i = 0; i < batchSize; i++) {
      lines.push(this.generateLine());
    }

    const content = lines.join('\n') + '\n';

    const frame: GeneratorFrame = {
      sequence: this.sequence,
      content,
      timestamp: startTime,
      checksum: checksum(content),
    };

    this.sequence++;
    return frame;
  }

  /**
   * Generate continuous stream of log lines
   * Returns an async generator that yields frames at the configured rate
   */
  async *stream(abortSignal?: AbortSignal): AsyncGenerator<GeneratorFrame> {
    const startTime = performance.now();
    const targetIntervalMs = 1000 / (this.config.rate / 10); // Batch 10 lines per frame
    const durationMs = this.config.duration * 1000;

    let frameCount = 0;

    while (performance.now() - startTime < durationMs) {
      if (abortSignal?.aborted) break;

      const frameStartTime = performance.now();
      yield this.nextFrame(10);
      frameCount++;

      // Calculate sleep time to maintain rate
      const elapsed = performance.now() - frameStartTime;
      const sleepTime = Math.max(0, targetIntervalMs - elapsed);

      if (sleepTime > 0) {
        await new Promise(resolve => setTimeout(resolve, sleepTime));
      }
    }
  }

  /**
   * Reset generator state
   */
  reset(): void {
    this.sequence = 0;
    this.rng = new SeededRandom(this.config.seed || Date.now());
  }

  /**
   * Get current sequence number
   */
  getSequence(): number {
    return this.sequence;
  }
}
