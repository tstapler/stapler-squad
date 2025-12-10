'use client';

import React, { useEffect, useRef, useState, useCallback } from 'react';
import dynamic from 'next/dynamic';
import {
  TestConfig,
  TEST_PRESETS,
  GeneratorFrame,
  RenderMetrics,
  createGenerator,
} from '@/lib/test-generators';

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

// Test status
type TestStatus = 'idle' | 'running' | 'completed' | 'failed';

// Preset keys
type PresetKey = keyof typeof TEST_PRESETS;

/**
 * Terminal Stress Test Page
 * Tests terminal streaming performance under high-volume data conditions
 */
export default function TerminalStressTestPage() {
  const terminalRef = useRef<HTMLDivElement>(null);
  const [terminal, setTerminal] = useState<TerminalType | null>(null);
  const [fitAddon, setFitAddon] = useState<FitAddonType | null>(null);

  // Test state
  const [status, setStatus] = useState<TestStatus>('idle');
  const [selectedPreset, setSelectedPreset] = useState<PresetKey>('ASCII_30FPS');
  const [customConfig, setCustomConfig] = useState<TestConfig | null>(null);

  // Metrics
  const [renderMetrics, setRenderMetrics] = useState<RenderMetrics>({
    framesRendered: 0,
    frameTimes: [],
    avgFrameTime: 0,
    p95FrameTime: 0,
    p99FrameTime: 0,
    maxFrameTime: 0,
    memoryUsage: [],
    peakMemory: 0,
    memoryGrowthRate: 0,
  });
  const [progress, setProgress] = useState({
    elapsed: 0,
    framesGenerated: 0,
    bytesGenerated: 0,
  });

  // Refs for test control
  const abortControllerRef = useRef<AbortController | null>(null);
  const frameTimesRef = useRef<number[]>([]);
  const lastFrameTimeRef = useRef<number>(0);
  const metricsRef = useRef(renderMetrics);

  // Update metrics ref
  useEffect(() => {
    metricsRef.current = renderMetrics;
  }, [renderMetrics]);

  // Initialize terminal
  useEffect(() => {
    if (!terminalRef.current || typeof window === 'undefined') return;
    if (!Terminal || !FitAddon) return;

    const term = new Terminal({
      cols: 80,
      rows: 24,
      scrollback: 10000,
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
      },
      allowProposedApi: true,
    });

    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(terminalRef.current);
    fit.fit();

    setTerminal(term);
    setFitAddon(fit);

    // Expose for Playwright testing
    (window as any).stressTestTerminal = term;
    (window as any).stressTestMetrics = metricsRef;

    // Handle resize
    const resizeObserver = new ResizeObserver(() => {
      fit.fit();
    });
    resizeObserver.observe(terminalRef.current);

    return () => {
      resizeObserver.disconnect();
      term.dispose();
    };
  }, []);

  // Calculate percentile
  const percentile = useCallback((arr: number[], p: number): number => {
    if (arr.length === 0) return 0;
    const sorted = [...arr].sort((a, b) => a - b);
    const index = Math.ceil((p / 100) * sorted.length) - 1;
    return sorted[Math.max(0, index)];
  }, []);

  // Update render metrics
  const updateMetrics = useCallback(() => {
    const frameTimes = frameTimesRef.current;
    if (frameTimes.length === 0) return;

    const avg = frameTimes.reduce((a, b) => a + b, 0) / frameTimes.length;
    const p95 = percentile(frameTimes, 95);
    const p99 = percentile(frameTimes, 99);
    const max = Math.max(...frameTimes);

    // Memory tracking (Chrome only)
    let memoryUsage: number[] = metricsRef.current.memoryUsage;
    let peakMemory = metricsRef.current.peakMemory;
    let memoryGrowthRate = metricsRef.current.memoryGrowthRate;

    if ((performance as any).memory) {
      const currentMemory = (performance as any).memory.usedJSHeapSize;
      memoryUsage = [...memoryUsage, currentMemory];
      peakMemory = Math.max(peakMemory, currentMemory);

      // Calculate growth rate (bytes/sec) over last 10 samples
      if (memoryUsage.length >= 10) {
        const recent = memoryUsage.slice(-10);
        const growth = recent[recent.length - 1] - recent[0];
        // Assuming ~1 sample per 100ms
        memoryGrowthRate = growth / (10 * 0.1);
      }
    }

    setRenderMetrics({
      framesRendered: frameTimes.length,
      frameTimes: [...frameTimes],
      avgFrameTime: avg,
      p95FrameTime: p95,
      p99FrameTime: p99,
      maxFrameTime: max,
      memoryUsage,
      peakMemory,
      memoryGrowthRate,
    });
  }, [percentile]);

  // Handle frame received
  const handleFrame = useCallback((frame: GeneratorFrame) => {
    if (!terminal) return;

    const now = performance.now();
    if (lastFrameTimeRef.current > 0) {
      const frameTime = now - lastFrameTimeRef.current;
      frameTimesRef.current.push(frameTime);

      // Keep last 1000 frame times
      if (frameTimesRef.current.length > 1000) {
        frameTimesRef.current = frameTimesRef.current.slice(-1000);
      }
    }
    lastFrameTimeRef.current = now;

    // Write frame to terminal
    terminal.write(frame.content);

    // Update metrics periodically
    if (frameTimesRef.current.length % 10 === 0) {
      updateMetrics();
    }
  }, [terminal, updateMetrics]);

  // Start test
  const startTest = useCallback(async () => {
    if (!terminal || status === 'running') return;

    // Reset state
    setStatus('running');
    frameTimesRef.current = [];
    lastFrameTimeRef.current = 0;
    setProgress({ elapsed: 0, framesGenerated: 0, bytesGenerated: 0 });
    setRenderMetrics({
      framesRendered: 0,
      frameTimes: [],
      avgFrameTime: 0,
      p95FrameTime: 0,
      p99FrameTime: 0,
      maxFrameTime: 0,
      memoryUsage: [],
      peakMemory: 0,
      memoryGrowthRate: 0,
    });

    // Clear terminal
    terminal.clear();

    // Create abort controller
    abortControllerRef.current = new AbortController();

    // Get config
    const config = customConfig || TEST_PRESETS[selectedPreset];

    // Create generator
    const generator = createGenerator(config);
    const startTime = performance.now();
    const durationMs = config.duration * 1000;
    const targetIntervalMs = 1000 / config.rate;

    // For log flood, use batching
    const batchSize = config.type === 'log-flood' ? Math.ceil(config.rate / 100) : 1;

    // Run test loop
    const runLoop = () => {
      if (abortControllerRef.current?.signal.aborted) {
        setStatus('completed');
        updateMetrics();
        return;
      }

      const elapsed = performance.now() - startTime;
      if (elapsed >= durationMs) {
        setStatus('completed');
        updateMetrics();
        return;
      }

      // Generate and render frame
      const frame = generator.nextFrame(batchSize);
      handleFrame(frame);

      // Update progress
      setProgress({
        elapsed,
        framesGenerated: generator.getSequence(),
        bytesGenerated: frame.content.length * generator.getSequence(),
      });

      // Schedule next frame
      const nextDelay = Math.max(0, targetIntervalMs);
      setTimeout(runLoop, nextDelay);
    };

    runLoop();
  }, [terminal, status, customConfig, selectedPreset, handleFrame, updateMetrics]);

  // Stop test
  const stopTest = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setStatus('completed');
    updateMetrics();
  }, [updateMetrics]);

  // Reset test
  const resetTest = useCallback(() => {
    stopTest();
    terminal?.clear();
    setStatus('idle');
    frameTimesRef.current = [];
    lastFrameTimeRef.current = 0;
    setProgress({ elapsed: 0, framesGenerated: 0, bytesGenerated: 0 });
    setRenderMetrics({
      framesRendered: 0,
      frameTimes: [],
      avgFrameTime: 0,
      p95FrameTime: 0,
      p99FrameTime: 0,
      maxFrameTime: 0,
      memoryUsage: [],
      peakMemory: 0,
      memoryGrowthRate: 0,
    });
  }, [terminal, stopTest]);

  // Export metrics as JSON
  const exportMetrics = useCallback(() => {
    const data = {
      config: customConfig || TEST_PRESETS[selectedPreset],
      renderMetrics,
      progress,
      timestamp: new Date().toISOString(),
    };

    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `stress-test-${Date.now()}.json`;
    a.click();
    URL.revokeObjectURL(url);
  }, [customConfig, selectedPreset, renderMetrics, progress]);

  // Format bytes
  const formatBytes = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
  };

  // Format time
  const formatTime = (ms: number): string => {
    if (ms < 1000) return `${ms.toFixed(0)}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  };

  // Get status color
  const getStatusColor = (): string => {
    switch (status) {
      case 'running': return '#f59e0b';
      case 'completed': return '#10b981';
      case 'failed': return '#ef4444';
      default: return '#6b7280';
    }
  };

  // Get frame time status
  const getFrameTimeStatus = (ms: number): { color: string; label: string } => {
    if (ms <= 16.67) return { color: '#10b981', label: 'Excellent (60fps)' };
    if (ms <= 33.33) return { color: '#f59e0b', label: 'Good (30fps)' };
    return { color: '#ef4444', label: 'Poor (<30fps)' };
  };

  return (
    <div style={{ padding: '20px', fontFamily: 'system-ui, sans-serif', maxWidth: '1400px', margin: '0 auto' }}>
      <h1 style={{ marginBottom: '10px' }}>Terminal Streaming Stress Test</h1>
      <p style={{ color: '#6b7280', marginBottom: '20px' }}>
        Test terminal performance under high-volume streaming conditions
      </p>

      {/* Controls */}
      <div style={{
        display: 'flex',
        gap: '20px',
        marginBottom: '20px',
        flexWrap: 'wrap',
        alignItems: 'flex-end',
      }}>
        {/* Preset selector */}
        <div>
          <label style={{ display: 'block', marginBottom: '5px', fontWeight: 500 }}>
            Test Preset
          </label>
          <select
            value={selectedPreset}
            onChange={(e) => setSelectedPreset(e.target.value as PresetKey)}
            disabled={status === 'running'}
            data-testid="preset-selector"
            style={{
              padding: '8px 12px',
              borderRadius: '6px',
              border: '1px solid #d1d5db',
              minWidth: '200px',
            }}
          >
            <optgroup label="ASCII Video">
              <option value="ASCII_30FPS">ASCII Video @ 30 FPS</option>
              <option value="ASCII_60FPS">ASCII Video @ 60 FPS (Matrix Rain)</option>
            </optgroup>
            <optgroup label="Log Flood">
              <option value="LOG_1K">Log Flood @ 1K lines/sec</option>
              <option value="LOG_5K">Log Flood @ 5K lines/sec</option>
              <option value="LOG_10K">Log Flood @ 10K lines/sec</option>
            </optgroup>
            <optgroup label="Color Stress">
              <option value="COLOR_RAINBOW">Rainbow Gradient</option>
            </optgroup>
            <optgroup label="Large Payload">
              <option value="PAYLOAD_100KB">Large Payload (100KB)</option>
              <option value="PAYLOAD_1MB">Large Payload (1MB)</option>
            </optgroup>
          </select>
        </div>

        {/* Action buttons */}
        <div style={{ display: 'flex', gap: '10px' }}>
          <button
            onClick={startTest}
            disabled={status === 'running'}
            data-testid="start-test"
            style={{
              padding: '8px 16px',
              borderRadius: '6px',
              border: 'none',
              background: status === 'running' ? '#9ca3af' : '#10b981',
              color: 'white',
              fontWeight: 500,
              cursor: status === 'running' ? 'not-allowed' : 'pointer',
            }}
          >
            {status === 'running' ? 'Running...' : 'Start Test'}
          </button>
          <button
            onClick={stopTest}
            disabled={status !== 'running'}
            data-testid="stop-test"
            style={{
              padding: '8px 16px',
              borderRadius: '6px',
              border: 'none',
              background: status !== 'running' ? '#9ca3af' : '#ef4444',
              color: 'white',
              fontWeight: 500,
              cursor: status !== 'running' ? 'not-allowed' : 'pointer',
            }}
          >
            Stop
          </button>
          <button
            onClick={resetTest}
            data-testid="reset-test"
            style={{
              padding: '8px 16px',
              borderRadius: '6px',
              border: '1px solid #d1d5db',
              background: 'white',
              fontWeight: 500,
              cursor: 'pointer',
            }}
          >
            Reset
          </button>
          <button
            onClick={exportMetrics}
            disabled={renderMetrics.framesRendered === 0}
            data-testid="export-metrics"
            style={{
              padding: '8px 16px',
              borderRadius: '6px',
              border: '1px solid #d1d5db',
              background: renderMetrics.framesRendered === 0 ? '#f3f4f6' : 'white',
              fontWeight: 500,
              cursor: renderMetrics.framesRendered === 0 ? 'not-allowed' : 'pointer',
            }}
          >
            Export JSON
          </button>
        </div>
      </div>

      {/* Layout: Metrics and Terminal side by side */}
      <div style={{ display: 'grid', gridTemplateColumns: '350px 1fr', gap: '20px' }}>
        {/* Metrics Panel */}
        <div
          data-testid="metrics-panel"
          style={{
            background: '#f9fafb',
            borderRadius: '8px',
            padding: '16px',
            border: '1px solid #e5e7eb',
          }}
        >
          <h3 style={{ margin: '0 0 16px 0', display: 'flex', alignItems: 'center', gap: '8px' }}>
            Performance Metrics
            <span
              data-testid="test-status"
              style={{
                fontSize: '12px',
                padding: '2px 8px',
                borderRadius: '9999px',
                background: getStatusColor(),
                color: 'white',
                textTransform: 'capitalize',
              }}
            >
              {status}
            </span>
          </h3>

          {/* Progress */}
          <div style={{ marginBottom: '16px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px' }}>
              <span>Progress</span>
              <span data-testid="progress-elapsed">{formatTime(progress.elapsed)}</span>
            </div>
            <div style={{
              height: '4px',
              background: '#e5e7eb',
              borderRadius: '2px',
              overflow: 'hidden',
            }}>
              <div
                style={{
                  height: '100%',
                  width: `${Math.min(100, (progress.elapsed / ((customConfig || TEST_PRESETS[selectedPreset]).duration * 1000)) * 100)}%`,
                  background: '#10b981',
                  transition: 'width 0.1s',
                }}
              />
            </div>
          </div>

          {/* Frame metrics */}
          <div style={{ display: 'grid', gap: '12px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>Frames Rendered</span>
              <span data-testid="frames-rendered" style={{ fontWeight: 600 }}>
                {renderMetrics.framesRendered.toLocaleString()}
              </span>
            </div>

            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>Bytes Generated</span>
              <span data-testid="bytes-generated" style={{ fontWeight: 600 }}>
                {formatBytes(progress.bytesGenerated)}
              </span>
            </div>

            <hr style={{ border: 'none', borderTop: '1px solid #e5e7eb', margin: '4px 0' }} />

            <div>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px' }}>
                <span>Avg Frame Time</span>
                <span
                  data-testid="avg-frame-time"
                  style={{
                    fontWeight: 600,
                    color: getFrameTimeStatus(renderMetrics.avgFrameTime).color,
                  }}
                >
                  {renderMetrics.avgFrameTime.toFixed(2)}ms
                </span>
              </div>
              <div style={{ fontSize: '12px', color: '#6b7280' }}>
                {getFrameTimeStatus(renderMetrics.avgFrameTime).label}
              </div>
            </div>

            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>P95 Frame Time</span>
              <span data-testid="p95-frame-time" style={{ fontWeight: 600 }}>
                {renderMetrics.p95FrameTime.toFixed(2)}ms
              </span>
            </div>

            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>P99 Frame Time</span>
              <span data-testid="p99-frame-time" style={{ fontWeight: 600 }}>
                {renderMetrics.p99FrameTime.toFixed(2)}ms
              </span>
            </div>

            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>Max Frame Time</span>
              <span
                data-testid="max-frame-time"
                style={{
                  fontWeight: 600,
                  color: renderMetrics.maxFrameTime > 100 ? '#ef4444' : 'inherit',
                }}
              >
                {renderMetrics.maxFrameTime.toFixed(2)}ms
              </span>
            </div>

            <hr style={{ border: 'none', borderTop: '1px solid #e5e7eb', margin: '4px 0' }} />

            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>Peak Memory</span>
              <span data-testid="peak-memory" style={{ fontWeight: 600 }}>
                {formatBytes(renderMetrics.peakMemory)}
              </span>
            </div>

            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span>Memory Growth</span>
              <span
                data-testid="memory-growth"
                style={{
                  fontWeight: 600,
                  color: renderMetrics.memoryGrowthRate > 1024 * 1024 ? '#ef4444' : 'inherit',
                }}
              >
                {formatBytes(Math.abs(renderMetrics.memoryGrowthRate))}/s
              </span>
            </div>
          </div>

          {/* Test info */}
          <div style={{
            marginTop: '16px',
            padding: '12px',
            background: '#fff',
            borderRadius: '6px',
            border: '1px solid #e5e7eb',
            fontSize: '12px',
          }}>
            <div style={{ fontWeight: 600, marginBottom: '8px' }}>Current Test Config</div>
            <div style={{ color: '#6b7280' }}>
              <div>Type: {(customConfig || TEST_PRESETS[selectedPreset]).type}</div>
              <div>Rate: {(customConfig || TEST_PRESETS[selectedPreset]).rate}/sec</div>
              <div>Duration: {(customConfig || TEST_PRESETS[selectedPreset]).duration}s</div>
            </div>
          </div>
        </div>

        {/* Terminal */}
        <div
          ref={terminalRef}
          data-testid="terminal-container"
          style={{
            border: '1px solid #374151',
            borderRadius: '8px',
            background: '#1e1e1e',
            padding: '8px',
            minHeight: '500px',
          }}
        />
      </div>

      {/* Test expectations */}
      <div style={{
        marginTop: '20px',
        padding: '16px',
        background: '#eff6ff',
        borderRadius: '8px',
        border: '1px solid #bfdbfe',
      }}>
        <h4 style={{ margin: '0 0 8px 0', color: '#1e40af' }}>Expected Performance Targets</h4>
        <ul style={{ margin: 0, paddingLeft: '20px', color: '#1e40af' }}>
          <li>Frame time should be &le;16.67ms (60fps) for ASCII video tests</li>
          <li>Frame time should be &le;33.33ms (30fps) for log flood tests</li>
          <li>Memory growth should be &lt;1MB/sec (no memory leaks)</li>
          <li>No dropped frames during normal operation</li>
        </ul>
      </div>
    </div>
  );
}
