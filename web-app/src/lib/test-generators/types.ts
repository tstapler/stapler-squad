/**
 * Types for terminal stress test data generators
 */

export interface GeneratorConfig {
  /** Target frames/lines per second */
  rate: number;
  /** Test duration in seconds */
  duration: number;
  /** Random seed for reproducibility */
  seed?: number;
}

export interface AsciiFrameConfig extends GeneratorConfig {
  type: 'ascii-video';
  /** Frame width in characters */
  width: number;
  /** Frame height in lines */
  height: number;
  /** Animation pattern */
  pattern: 'bouncing-ball' | 'scrolling-text' | 'progress-bar' | 'matrix-rain';
}

export interface LogFloodConfig extends GeneratorConfig {
  type: 'log-flood';
  /** Log level distribution (error, warn, info, debug) */
  levels: { error: number; warn: number; info: number; debug: number };
  /** Include timestamps */
  timestamps: boolean;
  /** Include colors */
  colors: boolean;
}

export interface ColorStressConfig extends GeneratorConfig {
  type: 'color-stress';
  /** Color mode */
  mode: '16-color' | '256-color' | 'true-color' | 'gradient';
}

export interface LargePayloadConfig extends GeneratorConfig {
  type: 'large-payload';
  /** Payload size in bytes */
  size: number;
  /** Content pattern */
  pattern: 'random' | 'repeated' | 'structured';
}

export type TestConfig = AsciiFrameConfig | LogFloodConfig | ColorStressConfig | LargePayloadConfig;

export interface GeneratorFrame {
  /** Frame sequence number */
  sequence: number;
  /** Frame content (terminal output) */
  content: string;
  /** Timestamp when frame was generated */
  timestamp: number;
  /** Checksum for integrity validation */
  checksum: number;
}

export interface GeneratorMetrics {
  /** Total frames generated */
  framesGenerated: number;
  /** Total bytes generated */
  bytesGenerated: number;
  /** Average frame generation time (ms) */
  avgGenerationTime: number;
  /** Target rate achieved (%) */
  rateAchievement: number;
  /** Dropped frames due to timing */
  droppedFrames: number;
}

export interface TestResult {
  /** Test configuration used */
  config: TestConfig;
  /** Generator metrics */
  generatorMetrics: GeneratorMetrics;
  /** Render metrics from terminal */
  renderMetrics: RenderMetrics;
  /** Test passed */
  passed: boolean;
  /** Failure reasons if any */
  failures: string[];
}

export interface RenderMetrics {
  /** Total frames rendered */
  framesRendered: number;
  /** Frame times (ms) */
  frameTimes: number[];
  /** Average frame time (ms) */
  avgFrameTime: number;
  /** P95 frame time (ms) */
  p95FrameTime: number;
  /** P99 frame time (ms) */
  p99FrameTime: number;
  /** Max frame time (ms) */
  maxFrameTime: number;
  /** Memory usage samples (bytes) */
  memoryUsage: number[];
  /** Peak memory (bytes) */
  peakMemory: number;
  /** Memory growth rate (bytes/sec) */
  memoryGrowthRate: number;
}

/**
 * Test presets for common scenarios
 */
export const TEST_PRESETS = {
  // ASCII video at 30 FPS
  ASCII_30FPS: {
    type: 'ascii-video' as const,
    rate: 30,
    duration: 10,
    width: 80,
    height: 24,
    pattern: 'bouncing-ball' as const,
  },
  // ASCII video at 60 FPS
  ASCII_60FPS: {
    type: 'ascii-video' as const,
    rate: 60,
    duration: 10,
    width: 80,
    height: 24,
    pattern: 'matrix-rain' as const,
  },
  // Log flood at 1K lines/sec
  LOG_1K: {
    type: 'log-flood' as const,
    rate: 1000,
    duration: 5,
    levels: { error: 5, warn: 15, info: 60, debug: 20 },
    timestamps: true,
    colors: true,
  },
  // Log flood at 5K lines/sec
  LOG_5K: {
    type: 'log-flood' as const,
    rate: 5000,
    duration: 5,
    levels: { error: 5, warn: 15, info: 60, debug: 20 },
    timestamps: true,
    colors: true,
  },
  // Log flood at 10K lines/sec
  LOG_10K: {
    type: 'log-flood' as const,
    rate: 10000,
    duration: 3,
    levels: { error: 5, warn: 15, info: 60, debug: 20 },
    timestamps: true,
    colors: true,
  },
  // Color stress test
  COLOR_RAINBOW: {
    type: 'color-stress' as const,
    rate: 60,
    duration: 10,
    mode: 'gradient' as const,
  },
  // Large payload 100KB
  PAYLOAD_100KB: {
    type: 'large-payload' as const,
    rate: 1,
    duration: 5,
    size: 100 * 1024,
    pattern: 'structured' as const,
  },
  // Large payload 1MB
  PAYLOAD_1MB: {
    type: 'large-payload' as const,
    rate: 1,
    duration: 3,
    size: 1024 * 1024,
    pattern: 'structured' as const,
  },
} as const;
