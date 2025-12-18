/**
 * Types for ANSI Escape Code Test Harness
 * Based on production data from escape-codes.json (77 unique codes, 26,974 occurrences)
 */

/**
 * Escape code categories based on production data
 */
export type EscapeCategory =
  | 'SGR'       // Select Graphic Rendition (text styling)
  | 'Cursor'    // Cursor movement and positioning
  | 'Erase'     // Screen/line clearing
  | 'Scroll'    // Scroll region control
  | 'DECPriv'   // DEC Private modes
  | 'Charset'   // Character set designation
  | 'CSI';      // Generic CSI sequences

/**
 * Test priority based on production frequency
 */
export type TestPriority = 'critical' | 'high' | 'medium' | 'low';

/**
 * Parameter specification for parameterized codes
 */
export interface ParamSpec {
  name: string;
  type: 'number' | 'range' | 'enum';
  min?: number;
  max?: number;
  values?: (string | number)[];
  default?: string | number;
}

/**
 * Definition of a single escape code
 */
export interface EscapeCodeDefinition {
  /** Hex representation of the code (from production data) */
  code: string;
  /** Actual escape sequence to emit */
  sequence: string;
  /** Human-readable description */
  humanReadable: string;
  /** Code category */
  category: EscapeCategory;
  /** Production occurrence count */
  count: number;
  /** Test priority based on frequency */
  priority: TestPriority;
  /** Parameter specifications (for codes like cursor position) */
  params?: ParamSpec[];
  /** Example usage with rendered output description */
  example?: string;
  /** Whether this code is supported by xterm.js */
  supported: boolean;
  /** Notes about edge cases or known issues */
  notes?: string;
}

/**
 * Test scenario configuration
 */
export interface EscapeCodeTestScenario {
  id: string;
  name: string;
  description: string;
  /** Codes to test in this scenario */
  codes: EscapeCodeDefinition[];
  /** Test pattern type */
  pattern: 'isolated' | 'sequential' | 'mixed' | 'stress';
  /** Number of frames to generate */
  frameCount: number;
  /** Target frames per second */
  frameRate: number;
  /** Additional configuration */
  config?: {
    /** Terminal width for this test */
    width?: number;
    /** Terminal height for this test */
    height?: number;
    /** Whether to reset terminal between codes */
    resetBetween?: boolean;
  };
}

/**
 * Single test frame with escape codes
 */
export interface EscapeCodeTestFrame {
  /** Frame sequence number */
  sequence: number;
  /** Frame content with escape codes */
  content: string;
  /** Escape codes used in this frame */
  codesUsed: string[];
  /** Validation rules for this frame */
  validation: ValidationRules;
  /** Expected terminal state after this frame */
  expectedState?: TerminalState;
  /** Timestamp when generated */
  timestamp: number;
  /** Checksum for integrity */
  checksum: number;
}

/**
 * Validation rules for verifying correct rendering
 */
export interface ValidationRules {
  /** Expected cursor position after frame */
  cursorPosition?: { row: number; col: number };
  /** Text content that should be present */
  textContent?: string[];
  /** Text that should NOT be present (corruption check) */
  textAbsent?: string[];
  /** Color expectations at specific positions */
  colors?: ColorExpectation[];
  /** Text attribute expectations */
  attributes?: AttributeExpectation[];
  /** Screen region checksums */
  regionChecksums?: RegionChecksum[];
}

/**
 * Color expectation at a specific position
 */
export interface ColorExpectation {
  row: number;
  col: number;
  foreground?: string;
  background?: string;
}

/**
 * Text attribute expectation
 */
export interface AttributeExpectation {
  row: number;
  col: number;
  bold?: boolean;
  dim?: boolean;
  underline?: boolean;
  reverse?: boolean;
}

/**
 * Checksum for a screen region
 */
export interface RegionChecksum {
  startRow: number;
  startCol: number;
  endRow: number;
  endCol: number;
  checksum: number;
}

/**
 * Terminal state for validation
 */
export interface TerminalState {
  cursorRow: number;
  cursorCol: number;
  scrollTop?: number;
  scrollBottom?: number;
  /** Character set mode */
  charsetMode?: 'ascii' | 'graphics';
}

/**
 * Test result for escape code testing
 */
export interface EscapeCodeTestResult {
  scenario: EscapeCodeTestScenario;
  /** Total frames tested */
  framesProcessed: number;
  /** Frames with validation failures */
  failedFrames: number;
  /** Detailed failures */
  failures: TestFailure[];
  /** Coverage by category */
  coverage: CategoryCoverage[];
  /** Performance metrics */
  performance: EscapeCodePerformanceMetrics;
  /** Overall pass/fail */
  passed: boolean;
  /** Timestamp */
  timestamp: number;
}

/**
 * Individual test failure
 */
export interface TestFailure {
  frameSequence: number;
  code: string;
  expected: string;
  actual: string;
  failureType: 'rendering' | 'position' | 'color' | 'attribute' | 'corruption';
  screenshot?: string;
}

/**
 * Coverage by category
 */
export interface CategoryCoverage {
  category: EscapeCategory;
  totalCodes: number;
  testedCodes: number;
  coveragePercent: number;
  productionFrequency: number;
}

/**
 * Performance metrics for escape code testing
 */
export interface EscapeCodePerformanceMetrics {
  /** Average time to process single code (ms) */
  avgCodeProcessingTime: number;
  /** Codes processed per second */
  codesPerSecond: number;
  /** Memory usage samples */
  memoryUsage: number[];
  /** Peak memory during test */
  peakMemory: number;
  /** Any dropped codes */
  droppedCodes: number;
}

/**
 * Configuration for escape code test harness
 */
export interface EscapeCodeTestConfig {
  type: 'escape-code-test';
  /** Codes to test (empty = all) */
  codes?: string[];
  /** Categories to test (empty = all) */
  categories?: EscapeCategory[];
  /** Minimum priority to include */
  minPriority?: TestPriority;
  /** Test pattern */
  pattern: 'isolated' | 'sequential' | 'mixed' | 'stress';
  /** Frames per second */
  frameRate: number;
  /** Test duration in seconds */
  duration: number;
  /** Terminal dimensions */
  width: number;
  height: number;
  /** Reset between codes */
  resetBetween: boolean;
  /** Random seed for reproducibility */
  seed?: number;
}

/**
 * Priority thresholds based on production data
 */
export const PRIORITY_THRESHOLDS = {
  critical: 1000,  // >1000 occurrences (5 codes = 53% of usage)
  high: 100,       // >100 occurrences
  medium: 10,      // >10 occurrences
  low: 0           // <=10 occurrences
} as const;

/**
 * Test presets for escape code testing
 */
export const ESCAPE_CODE_TEST_PRESETS = {
  /** Critical codes only - fastest test, highest impact */
  CRITICAL_ONLY: {
    type: 'escape-code-test' as const,
    minPriority: 'critical' as TestPriority,
    pattern: 'isolated' as const,
    frameRate: 30,
    duration: 5,
    width: 80,
    height: 24,
    resetBetween: true,
  },
  /** All SGR codes */
  SGR_COMPLETE: {
    type: 'escape-code-test' as const,
    categories: ['SGR'] as EscapeCategory[],
    pattern: 'sequential' as const,
    frameRate: 30,
    duration: 10,
    width: 80,
    height: 24,
    resetBetween: true,
  },
  /** All cursor codes */
  CURSOR_COMPLETE: {
    type: 'escape-code-test' as const,
    categories: ['Cursor'] as EscapeCategory[],
    pattern: 'sequential' as const,
    frameRate: 30,
    duration: 10,
    width: 80,
    height: 24,
    resetBetween: false,
  },
  /** Full coverage - all 77 codes */
  FULL_COVERAGE: {
    type: 'escape-code-test' as const,
    pattern: 'sequential' as const,
    frameRate: 30,
    duration: 30,
    width: 80,
    height: 24,
    resetBetween: true,
  },
  /** Stress test - rapid fire all codes */
  STRESS_TEST: {
    type: 'escape-code-test' as const,
    pattern: 'stress' as const,
    frameRate: 120,
    duration: 10,
    width: 80,
    height: 24,
    resetBetween: false,
  },
  /** Mixed realistic usage */
  REALISTIC_MIX: {
    type: 'escape-code-test' as const,
    pattern: 'mixed' as const,
    frameRate: 60,
    duration: 15,
    width: 120,
    height: 40,
    resetBetween: false,
  },
} as const;
