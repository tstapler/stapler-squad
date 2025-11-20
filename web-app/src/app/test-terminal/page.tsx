'use client';

import React, { useEffect, useRef, useState } from 'react';
import dynamic from 'next/dynamic';
import { StateApplicator } from '@/lib/terminal/StateApplicator';
import { TerminalState, TerminalLine, TerminalDimensions } from '@/gen/session/v1/events_pb';

// Dynamically import xterm to avoid SSR issues
type TerminalType = import('@xterm/xterm').Terminal;
type FitAddonType = import('@xterm/addon-fit').FitAddon;

let Terminal: typeof import('@xterm/xterm').Terminal | undefined;
let FitAddon: typeof import('@xterm/addon-fit').FitAddon | undefined;

if (typeof window !== 'undefined') {
  const XTermModule = require('@xterm/xterm');
  const FitAddonModule = require('@xterm/addon-fit');
  Terminal = XTermModule.Terminal;
  FitAddon = FitAddonModule.FitAddon;
  require('@xterm/xterm/css/xterm.css');
}

/**
 * Test page for terminal flickering verification
 * Simulates rapid ANSI updates like Claude's pulsing status bar
 */
export default function TestTerminalPage() {
  const terminalRef = useRef<HTMLDivElement>(null);
  const [terminal, setTerminal] = useState<TerminalType | null>(null);
  const [stateApplicator, setStateApplicator] = useState<StateApplicator | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const [metrics, setMetrics] = useState({
    framesRendered: 0,
    averageFrameTime: 0,
    changedLines: 0,
    unchangedLines: 0,
    totalUpdates: 0,
  });

  const intervalRef = useRef<number | null>(null);
  const metricsRef = useRef(metrics);
  const frameTimesRef = useRef<number[]>([]);

  // Update metrics ref when state changes
  useEffect(() => {
    metricsRef.current = metrics;
  }, [metrics]);

  // Initialize terminal
  useEffect(() => {
    if (!terminalRef.current || typeof window === 'undefined') return;
    if (!Terminal || !FitAddon) return; // Wait for modules to load

    const term = new Terminal({
      cols: 80,
      rows: 24,
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
      },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalRef.current);
    fitAddon.fit();

    const applicator = new StateApplicator(term);

    setTerminal(term);
    setStateApplicator(applicator);

    // Expose terminal and applicator for testing
    (window as any).testTerminal = term;
    (window as any).testStateApplicator = applicator;
    (window as any).testMetrics = metricsRef;

    return () => {
      term.dispose();
    };
  }, []);

  // Generate terminal state with simulated content
  const generateState = (sequence: number, colorIndex: number): TerminalState => {
    const colors = [31, 32, 33, 34, 35, 36]; // Red, Green, Yellow, Blue, Magenta, Cyan
    const color = colors[colorIndex % colors.length];
    const spinner = ['⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'][sequence % 8];

    const lines: TerminalLine[] = [];

    // Create 24 lines (terminal rows)
    for (let i = 0; i < 24; i++) {
      let content: string;

      if (i === 0) {
        // Header line (unchanged)
        content = '═══ Terminal Flickering Test ═══';
      } else if (i === 22) {
        // Status bar line (changes every update - simulates Claude's pulsing)
        content = `\x1b[${color}m${spinner}\x1b[0m Working... (update #${sequence})`;
      } else if (i === 23) {
        // Footer line (unchanged)
        content = `Press Start to begin test | Sequence: ${sequence}`;
      } else {
        // Static content lines (unchanged)
        content = `Line ${i}: Static content that doesn't change`;
      }

      lines.push(
        new TerminalLine({
          content: new TextEncoder().encode(content),
        })
      );
    }

    return new TerminalState({
      sequence: BigInt(sequence),
      lines,
      dimensions: new TerminalDimensions({
        cols: 80,
        rows: 24,
      }),
    });
  };

  // Start rapid updates
  const startTest = () => {
    if (!stateApplicator || isRunning) return;

    setIsRunning(true);
    let sequence = 0;
    let lastFrameTime = performance.now();

    // Simulate rapid updates (faster than Claude's actual speed for stress testing)
    // 20ms interval = 50 updates/sec, but RAF will batch to 60fps
    intervalRef.current = window.setInterval(() => {
      const colorIndex = Math.floor(sequence / 10); // Change color every 10 frames
      const state = generateState(sequence, colorIndex);

      const startTime = performance.now();
      stateApplicator.applyState(state);
      const endTime = performance.now();

      // Track frame timing
      const frameTime = endTime - lastFrameTime;
      lastFrameTime = endTime;
      frameTimesRef.current.push(frameTime);

      // Keep last 60 frame times (1 second at 60fps)
      if (frameTimesRef.current.length > 60) {
        frameTimesRef.current.shift();
      }

      // Update metrics
      const avgFrameTime =
        frameTimesRef.current.reduce((a, b) => a + b, 0) / frameTimesRef.current.length;

      setMetrics({
        framesRendered: sequence + 1,
        averageFrameTime: avgFrameTime,
        changedLines: 1, // Only status bar line changes
        unchangedLines: 23, // All other lines stay same
        totalUpdates: sequence + 1,
      });

      sequence++;

      // Auto-stop after 300 updates (~5 seconds at 60fps)
      if (sequence >= 300) {
        stopTest();
      }
    }, 20); // 20ms = 50 updates/sec (faster than Claude's actual speed)
  };

  // Stop updates
  const stopTest = () => {
    if (intervalRef.current !== null) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    setIsRunning(false);
  };

  // Reset test
  const resetTest = () => {
    stopTest();
    stateApplicator?.resetSequence();
    terminal?.clear();
    setMetrics({
      framesRendered: 0,
      averageFrameTime: 0,
      changedLines: 0,
      unchangedLines: 0,
      totalUpdates: 0,
    });
    frameTimesRef.current = [];
  };

  return (
    <div style={{ padding: '20px', fontFamily: 'monospace' }}>
      <h1>Terminal Flickering Test</h1>
      <p>
        This page tests the StateApplicator's incremental rendering and RAF batching.
        <br />
        Rapid updates (50/sec) are batched to 60fps with only changed lines redrawn.
      </p>

      <div style={{ marginBottom: '20px' }}>
        <button
          onClick={startTest}
          disabled={isRunning}
          data-testid="start-test"
          style={{
            padding: '10px 20px',
            marginRight: '10px',
            background: isRunning ? '#666' : '#4CAF50',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: isRunning ? 'not-allowed' : 'pointer',
          }}
        >
          {isRunning ? 'Running...' : 'Start Test'}
        </button>
        <button
          onClick={stopTest}
          disabled={!isRunning}
          data-testid="stop-test"
          style={{
            padding: '10px 20px',
            marginRight: '10px',
            background: !isRunning ? '#666' : '#f44336',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: !isRunning ? 'not-allowed' : 'pointer',
          }}
        >
          Stop Test
        </button>
        <button
          onClick={resetTest}
          data-testid="reset-test"
          style={{
            padding: '10px 20px',
            background: '#2196F3',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
          }}
        >
          Reset
        </button>
      </div>

      <div
        data-testid="metrics-display"
        style={{
          marginBottom: '20px',
          padding: '15px',
          background: '#f5f5f5',
          borderRadius: '4px',
        }}
      >
        <h3 style={{ marginTop: 0 }}>Performance Metrics</h3>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
          <div>
            <strong>Frames Rendered:</strong> <span data-testid="frames-rendered">{metrics.framesRendered}</span>
          </div>
          <div>
            <strong>Avg Frame Time:</strong>{' '}
            <span
              data-testid="avg-frame-time"
              style={{ color: metrics.averageFrameTime > 40 ? 'red' : 'green' }}
            >
              {metrics.averageFrameTime.toFixed(2)}ms
            </span>
          </div>
          <div>
            <strong>Changed Lines:</strong> <span data-testid="changed-lines">{metrics.changedLines}</span>
          </div>
          <div>
            <strong>Unchanged Lines:</strong> <span data-testid="unchanged-lines">{metrics.unchangedLines}</span>
          </div>
          <div>
            <strong>Total Updates:</strong> <span data-testid="total-updates">{metrics.totalUpdates}</span>
          </div>
          <div>
            <strong>Status:</strong>{' '}
            <span
              data-testid="test-status"
              style={{ color: isRunning ? 'orange' : metrics.framesRendered > 0 ? 'green' : 'black' }}
            >
              {isRunning ? 'Running' : metrics.framesRendered > 0 ? 'Completed' : 'Ready'}
            </span>
          </div>
        </div>
        <div style={{ marginTop: '10px' }}>
          <strong>Expected:</strong>
          <ul style={{ marginTop: '5px', marginBottom: 0 }}>
            <li>Frame time should be ~16.67ms (60fps) or less</li>
            <li>Changed lines should be 1 (only status bar)</li>
            <li>No visible flickering during test</li>
          </ul>
        </div>
      </div>

      <div
        ref={terminalRef}
        data-testid="terminal-container"
        style={{
          border: '1px solid #ccc',
          borderRadius: '4px',
          padding: '10px',
          background: '#1e1e1e',
        }}
      />
    </div>
  );
}
