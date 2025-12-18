'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import {
  EscapeCodeDefinition,
  EscapeCategory,
  TestPriority,
  EscapeCodeTestFrame,
  ESCAPE_CODE_TEST_PRESETS,
} from '@/lib/test-generators/escape-codes/types';
import {
  ESCAPE_CODE_LIBRARY,
  getCodesByCategory,
  getCodesByPriority,
  getCodesAtOrAbovePriority,
  getCategoryStats,
  getLibraryStats,
} from '@/lib/test-generators/escape-codes/library';
import styles from './page.module.css';

type TestPattern = 'isolated' | 'sequential' | 'mixed' | 'stress';

interface TestState {
  isRunning: boolean;
  framesGenerated: number;
  codesTested: Set<string>;
  currentCode: string | null;
  errors: string[];
  startTime: number | null;
}

export default function EscapeCodesTestPage() {
  // Test configuration
  const [selectedCodes, setSelectedCodes] = useState<Set<string>>(new Set());
  const [frameRate, setFrameRate] = useState<number>(30);
  const [testPattern, setTestPattern] = useState<TestPattern>('isolated');
  const [duration, setDuration] = useState<number>(10);
  const [selectedCategory, setSelectedCategory] = useState<EscapeCategory | 'all'>('all');
  const [selectedPriority, setSelectedPriority] = useState<TestPriority | 'all'>('all');

  // Test state
  const [testState, setTestState] = useState<TestState>({
    isRunning: false,
    framesGenerated: 0,
    codesTested: new Set(),
    currentCode: null,
    errors: [],
    startTime: null,
  });

  // Terminal display
  const [terminalContent, setTerminalContent] = useState<string[]>([]);
  const terminalRef = useRef<HTMLDivElement>(null);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const frameCountRef = useRef<number>(0);

  // Library stats
  const libraryStats = getLibraryStats();
  const categoryStats = getCategoryStats();

  // Get filtered codes based on selections
  const getFilteredCodes = useCallback(() => {
    let codes = [...ESCAPE_CODE_LIBRARY];

    if (selectedCategory !== 'all') {
      codes = codes.filter(c => c.category === selectedCategory);
    }

    if (selectedPriority !== 'all') {
      codes = codes.filter(c => c.priority === selectedPriority);
    }

    if (selectedCodes.size > 0) {
      codes = codes.filter(c => selectedCodes.has(c.code));
    }

    return codes;
  }, [selectedCategory, selectedPriority, selectedCodes]);

  // Generate test frame
  const generateTestFrame = useCallback((codes: EscapeCodeDefinition[], frameNum: number): EscapeCodeTestFrame => {
    const selectedCode = codes[frameNum % codes.length];
    const content = `Frame ${frameNum}: Testing ${selectedCode.humanReadable}\n${selectedCode.sequence}Test content for ${selectedCode.category} code`;

    return {
      sequence: frameNum,
      content,
      codesUsed: [selectedCode.code],
      validation: {},
      timestamp: Date.now(),
      checksum: 0,
    };
  }, []);

  // Start test
  const startTest = useCallback(() => {
    const codes = getFilteredCodes();

    if (codes.length === 0) {
      alert('Please select at least one escape code to test');
      return;
    }

    setTestState({
      isRunning: true,
      framesGenerated: 0,
      codesTested: new Set(),
      currentCode: null,
      errors: [],
      startTime: Date.now(),
    });

    setTerminalContent([]);
    frameCountRef.current = 0;

    // Generate frames at specified rate
    intervalRef.current = setInterval(() => {
      const frame = generateTestFrame(codes, frameCountRef.current);
      const currentCode = codes[frameCountRef.current % codes.length];

      setTerminalContent(prev => [...prev.slice(-100), frame.content]);

      setTestState(prev => {
        const newTestedSet = new Set(prev.codesTested);
        newTestedSet.add(currentCode.code);
        return {
          ...prev,
          framesGenerated: prev.framesGenerated + 1,
          codesTested: newTestedSet,
          currentCode: currentCode.humanReadable,
        };
      });

      frameCountRef.current++;

      // Stop after duration
      if (frameCountRef.current >= frameRate * duration) {
        stopTest();
      }
    }, 1000 / frameRate);
  }, [getFilteredCodes, frameRate, duration, generateTestFrame]);

  // Stop test
  const stopTest = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }

    setTestState(prev => ({
      ...prev,
      isRunning: false,
      currentCode: null,
    }));
  }, []);

  // Apply preset
  const applyPreset = (presetName: keyof typeof ESCAPE_CODE_TEST_PRESETS) => {
    const preset = ESCAPE_CODE_TEST_PRESETS[presetName];
    setFrameRate(preset.frameRate);
    setTestPattern(preset.pattern);
    setDuration(preset.duration);

    // Set filters based on preset
    if ('minPriority' in preset && preset.minPriority) {
      setSelectedPriority(preset.minPriority);
      const codes = getCodesAtOrAbovePriority(preset.minPriority);
      setSelectedCodes(new Set(codes.map(c => c.code)));
    } else if ('categories' in preset && preset.categories) {
      setSelectedCategory(preset.categories[0]);
    } else {
      setSelectedCategory('all');
      setSelectedPriority('all');
      setSelectedCodes(new Set());
    }
  };

  // Toggle code selection
  const toggleCodeSelection = (code: string) => {
    setSelectedCodes(prev => {
      const newSet = new Set(prev);
      if (newSet.has(code)) {
        newSet.delete(code);
      } else {
        newSet.add(code);
      }
      return newSet;
    });
  };

  // Select all in category
  const selectAllInCategory = (category: EscapeCategory) => {
    const codes = getCodesByCategory(category);
    setSelectedCodes(new Set(codes.map(c => c.code)));
  };

  // Calculate coverage
  const calculateCoverage = () => {
    const totalCodes = ESCAPE_CODE_LIBRARY.length;
    const testedCodes = testState.codesTested.size;
    return totalCodes > 0 ? Math.round((testedCodes / totalCodes) * 100) : 0;
  };

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, []);

  // Auto-scroll terminal
  useEffect(() => {
    if (terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
    }
  }, [terminalContent]);

  return (
    <div className={styles.container}>
      <header className={styles.header}>
        <h1>ANSI Escape Code Test Harness</h1>
        <div className={styles.stats}>
          <span>{libraryStats.totalCodes} codes</span>
          <span>{libraryStats.totalOccurrences.toLocaleString()} production occurrences</span>
        </div>
      </header>

      <div className={styles.mainContent}>
        {/* Left Panel - Code Selector */}
        <div className={styles.leftPanel}>
          <div className={styles.panelHeader}>
            <h2>Code Selector</h2>
            <div className={styles.filters}>
              <select
                value={selectedCategory}
                onChange={(e) => setSelectedCategory(e.target.value as EscapeCategory | 'all')}
                className={styles.select}
              >
                <option value="all">All Categories</option>
                {categoryStats.map(stat => (
                  <option key={stat.category} value={stat.category}>
                    {stat.category} ({stat.count})
                  </option>
                ))}
              </select>

              <select
                value={selectedPriority}
                onChange={(e) => setSelectedPriority(e.target.value as TestPriority | 'all')}
                className={styles.select}
              >
                <option value="all">All Priorities</option>
                <option value="critical">Critical</option>
                <option value="high">High</option>
                <option value="medium">Medium</option>
                <option value="low">Low</option>
              </select>
            </div>
          </div>

          <div className={styles.presets}>
            <h3>Quick Presets</h3>
            <div className={styles.presetButtons}>
              <button onClick={() => applyPreset('CRITICAL_ONLY')} className={styles.presetButton}>
                Critical Only
              </button>
              <button onClick={() => applyPreset('SGR_COMPLETE')} className={styles.presetButton}>
                All SGR
              </button>
              <button onClick={() => applyPreset('CURSOR_COMPLETE')} className={styles.presetButton}>
                All Cursor
              </button>
              <button onClick={() => applyPreset('FULL_COVERAGE')} className={styles.presetButton}>
                Full Coverage
              </button>
              <button onClick={() => applyPreset('STRESS_TEST')} className={styles.presetButton}>
                Stress Test
              </button>
            </div>
          </div>

          <div className={styles.codeList}>
            {getFilteredCodes().map(code => (
              <div
                key={code.code}
                className={`${styles.codeItem} ${selectedCodes.has(code.code) ? styles.selected : ''}`}
                onClick={() => toggleCodeSelection(code.code)}
              >
                <div className={styles.codeHeader}>
                  <input
                    type="checkbox"
                    checked={selectedCodes.has(code.code)}
                    onChange={() => toggleCodeSelection(code.code)}
                    onClick={(e) => e.stopPropagation()}
                  />
                  <span className={styles.codeTitle}>{code.humanReadable}</span>
                  <span className={`${styles.priority} ${styles[code.priority]}`}>
                    {code.priority}
                  </span>
                </div>
                <div className={styles.codeDetails}>
                  <span className={styles.category}>{code.category}</span>
                  <span className={styles.count}>{code.count} occurrences</span>
                  <code className={styles.sequence}>{code.code}</code>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Middle Panel - Terminal Preview */}
        <div className={styles.middlePanel}>
          <div className={styles.terminalHeader}>
            <h2>Terminal Preview</h2>
            {testState.currentCode && (
              <span className={styles.currentCode}>Testing: {testState.currentCode}</span>
            )}
          </div>

          <div className={styles.terminal} ref={terminalRef}>
            {terminalContent.length === 0 ? (
              <div className={styles.placeholder}>
                Terminal output will appear here when test starts...
              </div>
            ) : (
              terminalContent.map((line, idx) => (
                <div key={idx} className={styles.terminalLine}>
                  {line}
                </div>
              ))
            )}
          </div>
        </div>

        {/* Right Panel - Controls & Metrics */}
        <div className={styles.rightPanel}>
          <div className={styles.controls}>
            <h2>Test Controls</h2>

            <div className={styles.controlGroup}>
              <label>Frame Rate (FPS)</label>
              <select
                value={frameRate}
                onChange={(e) => setFrameRate(Number(e.target.value))}
                disabled={testState.isRunning}
                className={styles.select}
              >
                <option value={30}>30 FPS</option>
                <option value={60}>60 FPS</option>
                <option value={120}>120 FPS</option>
              </select>
            </div>

            <div className={styles.controlGroup}>
              <label>Test Pattern</label>
              <select
                value={testPattern}
                onChange={(e) => setTestPattern(e.target.value as TestPattern)}
                disabled={testState.isRunning}
                className={styles.select}
              >
                <option value="isolated">Isolated</option>
                <option value="sequential">Sequential</option>
                <option value="mixed">Mixed</option>
                <option value="stress">Stress</option>
              </select>
            </div>

            <div className={styles.controlGroup}>
              <label>Duration (seconds)</label>
              <input
                type="number"
                value={duration}
                onChange={(e) => setDuration(Number(e.target.value))}
                min={1}
                max={60}
                disabled={testState.isRunning}
                className={styles.input}
              />
            </div>

            <div className={styles.actionButtons}>
              {testState.isRunning ? (
                <button onClick={stopTest} className={styles.stopButton}>
                  Stop Test
                </button>
              ) : (
                <button onClick={startTest} className={styles.startButton}>
                  Start Test
                </button>
              )}
            </div>
          </div>

          <div className={styles.metrics}>
            <h2>Metrics</h2>

            <div className={styles.metricItem}>
              <label>Frames Generated</label>
              <span className={styles.metricValue}>
                {testState.framesGenerated}
              </span>
            </div>

            <div className={styles.metricItem}>
              <label>Codes Tested</label>
              <span className={styles.metricValue}>
                {testState.codesTested.size} / {getFilteredCodes().length}
              </span>
            </div>

            <div className={styles.metricItem}>
              <label>Coverage</label>
              <span className={styles.metricValue}>
                {calculateCoverage()}%
              </span>
              <div className={styles.progressBar}>
                <div
                  className={styles.progressFill}
                  style={{ width: `${calculateCoverage()}%` }}
                />
              </div>
            </div>

            {testState.errors.length > 0 && (
              <div className={styles.errors}>
                <h3>Errors</h3>
                {testState.errors.map((error, idx) => (
                  <div key={idx} className={styles.error}>
                    {error}
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className={styles.categoryStats}>
            <h3>Category Coverage</h3>
            {categoryStats.map(stat => {
              const categoryTested = Array.from(testState.codesTested).filter(
                code => ESCAPE_CODE_LIBRARY.find(c => c.code === code)?.category === stat.category
              ).length;
              const coverage = stat.count > 0 ? Math.round((categoryTested / stat.count) * 100) : 0;

              return (
                <div key={stat.category} className={styles.categoryStat}>
                  <div className={styles.categoryName}>
                    {stat.category}
                    <span className={styles.categoryCount}>
                      {categoryTested}/{stat.count}
                    </span>
                  </div>
                  <div className={styles.progressBar}>
                    <div
                      className={styles.progressFill}
                      style={{ width: `${coverage}%` }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}