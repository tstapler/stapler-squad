/**
 * Performance benchmarks for EscapeSequenceParser.processChunk()
 *
 * Verifies that the ED3 filter added for terminal-jank elimination does not
 * regress the hot write path below a meaningful throughput floor.
 *
 * The filter strips \x1b[2J\x1b[3J (ED2+ED3) down to \x1b[2J so Claude
 * Code's TUI repaints do not reset the xterm.js viewportY on every frame.
 *
 * Acceptance threshold: ≥ 10 MB/s on a 100 KB Claude TUI payload.
 * At 10 MB/s, a single 100 KB write costs 10 ms — well within a 16 ms frame.
 *
 * Design notes on Jest microbenchmarking:
 *   - V8's JIT needs O(10) iterations to reach steady state on regex-heavy paths.
 *     We use WARMUP=10 inside measureThroughput (discard first 10 samples), which
 *     is enough for stable numbers without creating GC pressure from a beforeAll.
 *   - We intentionally do NOT compare two paths (claude-tui vs clean) in a single
 *     test.  Interleaved timing is vulnerable to GC pauses from accumulated garbage
 *     between iterations, producing meaningless relative numbers.
 *   - The throughput floor (10 MB/s) is the correct assertion — it verifies the
 *     filter is fast enough, without requiring cross-path heap isolation that Jest
 *     cannot provide.
 *
 * Run with:
 *   npx jest EscapeSequenceParser.bench --verbose
 */

import { EscapeSequenceParser } from '../EscapeSequenceParser';

// ---------------------------------------------------------------------------
// Payload generators
// ---------------------------------------------------------------------------

/**
 * Generates a terminal payload that mimics Claude Code TUI output:
 * every repaint starts with \x1b[2J\x1b[3J (ED2 + ED3 erase scrollback)
 * followed by cursor-home and a screen's worth of colored text.
 */
function generateClaudeTuiOutput(sizeBytes: number): string {
  const line = '\x1b[32m[\u2713]\x1b[0m Processing step \x1b[33m42%\x1b[0m\r\n';
  const frame =
    '\x1b[2J\x1b[3J\x1b[H' + // ED2 + ED3 + cursor home (Claude TUI pattern)
    line.repeat(40);
  let output = '';
  while (output.length < sizeBytes) {
    output += frame;
  }
  return output.slice(0, sizeBytes);
}

function splitIntoChunks(data: string, chunkSize: number): string[] {
  const chunks: string[] = [];
  for (let i = 0; i < data.length; i += chunkSize) {
    chunks.push(data.slice(i, i + chunkSize));
  }
  return chunks;
}

// ---------------------------------------------------------------------------
// Measurement helper
// ---------------------------------------------------------------------------

interface BenchStats {
  bytesPerSec: number;
  msPerIteration: number;
  p50Ms: number;
  p95Ms: number;
}

/**
 * Measures processChunk throughput.
 *
 * Uses a single EscapeSequenceParser instance across all iterations so the
 * JIT sees the same object shape throughout.  warmupIterations samples are
 * discarded; only benchIterations samples are used for statistics.
 *
 * Higher warmup (default 10) is necessary for regex-heavy paths to reach
 * V8 steady-state before timing begins.
 */
function measureThroughput(
  chunks: string[],
  warmupIterations = 10,
  benchIterations = 20,
): BenchStats {
  const totalBytes = chunks.reduce((sum, c) => sum + c.length, 0);
  const parser = new EscapeSequenceParser();
  const samples: number[] = [];

  for (let i = 0; i < warmupIterations + benchIterations; i++) {
    parser.reset();
    const t0 = performance.now();
    for (const chunk of chunks) {
      parser.processChunk(chunk);
    }
    const elapsed = performance.now() - t0;
    if (i >= warmupIterations) {
      samples.push(elapsed);
    }
  }

  const sorted = [...samples].sort((a, b) => a - b);
  const mean = samples.reduce((s, v) => s + v, 0) / samples.length;
  const p50 = sorted[Math.floor(sorted.length * 0.5)];
  const p95 = sorted[Math.floor(sorted.length * 0.95)];

  return {
    bytesPerSec: mean > 0 ? (totalBytes / mean) * 1000 : 0,
    msPerIteration: mean,
    p50Ms: p50,
    p95Ms: p95,
  };
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

describe('EscapeSequenceParser throughput benchmarks', () => {
  const PAYLOAD_BYTES = 100 * 1024; // 100 KB – realistic Claude Code session burst
  const CHUNK_BYTES   = 4 * 1024;  // 4 KB chunks – typical WebSocket frame size
  const WARMUP        = 10;        // iterations to discard while JIT reaches steady state
  const ITERATIONS    = 20;

  test('claude-tui payload (ED2+ED3 pairs): throughput ≥ 10 MB/s', () => {
    const payload = generateClaudeTuiOutput(PAYLOAD_BYTES);
    const chunks  = splitIntoChunks(payload, CHUNK_BYTES);
    const stats   = measureThroughput(chunks, WARMUP, ITERATIONS);

    const mbPerSec = stats.bytesPerSec / (1024 * 1024);
    console.log(
      `\n  [claude-tui] ${(PAYLOAD_BYTES / 1024).toFixed(0)} KB ` +
      `× ${ITERATIONS} iters (${WARMUP} warmup discarded)\n` +
      `  mean: ${stats.msPerIteration.toFixed(2)} ms  ` +
      `p50: ${stats.p50Ms.toFixed(2)} ms  ` +
      `p95: ${stats.p95Ms.toFixed(2)} ms  ` +
      `→ ${mbPerSec.toFixed(0)} MB/s`,
    );

    // 100 KB at 10 MB/s = 10 ms — plenty of headroom inside a 16 ms frame
    expect(stats.bytesPerSec).toBeGreaterThan(10 * 1024 * 1024);
  });
});
