/**
 * Shared jest.mock() factory bodies for the TerminalOutput.*.test.tsx suite.
 *
 * jest.mock(...) calls are hoisted above imports by babel-jest, so the
 * `jest.mock('module', () => ...)` calls themselves must stay inline in each
 * test file — only the *factory logic* (the object each mock module
 * resolves to) lives here, invoked via `require('./terminalOutputTestMocks')`
 * inside each file's factory. Using `require()` (rather than importing these
 * factories and referencing them directly) sidesteps hoisting entirely: the
 * factory body only runs when the mocked module is actually required, well
 * after this helper module has finished evaluating.
 *
 * Two unrelated shapes live here because two different clusters of
 * TerminalOutput.*.test.tsx files each duplicated their own preamble:
 *  - toolbar-analytics / upload / logstream: minimal XtermTerminal handle,
 *    bare TerminalDimensionCache/TerminalStreamManager mocks.
 *  - reconnect / (main) test.tsx: the fuller "supporting mocks" block
 *    (ApprovalsContext, useHandedness, useSplitContainerSize, etc.) with a
 *    TerminalDimensionCache/TerminalStreamManager shape that additionally
 *    covers validateCellDimensions/setSerializeAddon.
 */

// ── Shared by toolbar-analytics / upload / logstream ─────────────────────────

export function createMockXtermHandle() {
  return {
    terminal: null as null,
    fit: jest.fn(),
    write: jest.fn(),
    writeln: jest.fn(),
    clear: jest.fn(),
    focus: jest.fn(),
    search: jest.fn().mockReturnValue(false),
    searchNext: jest.fn().mockReturnValue(false),
    searchPrevious: jest.fn().mockReturnValue(false),
  };
}

export function xtermTerminalMockModule(mockXtermHandle: unknown) {
  const React = require("react");
  const XtermTerminal = React.forwardRef((_props: unknown, ref: unknown) => {
    React.useImperativeHandle(ref, () => mockXtermHandle);
    return React.createElement("div", { "data-testid": "mock-xterm" });
  });
  XtermTerminal.displayName = "XtermTerminal";
  return { XtermTerminal };
}

export function useTerminalStreamMockModule() {
  return { useTerminalStream: jest.fn() };
}

export function terminalDimensionCacheMockModule() {
  return {
    getCachedDimensions: jest.fn().mockReturnValue(null),
    saveDimensions: jest.fn(),
  };
}

export function terminalStreamManagerMockModule() {
  return {
    TerminalStreamManager: jest.fn().mockImplementation(() => ({
      setOnFirstOutput: jest.fn(),
      installDebugMonitor: jest.fn(),
      writeInitialContent: jest.fn().mockResolvedValue(undefined),
      write: jest.fn(),
      cleanup: jest.fn(),
      updateSendFlowControl: jest.fn(),
    })),
  };
}

export function analyticsContextMockModule(mockTrack: jest.Mock) {
  return { useAnalytics: () => ({ track: mockTrack }) };
}

// ── Shared by reconnect / (main) TerminalOutput.test.tsx ─────────────────────

export function terminalDimensionCacheWithValidationMockModule() {
  return {
    getCachedDimensions: jest.fn().mockReturnValue(null),
    saveDimensions: jest.fn(),
    validateCellDimensions: jest.fn().mockReturnValue(null),
  };
}

export function terminalStreamManagerWithSerializeAddonMockModule() {
  return {
    TerminalStreamManager: jest.fn().mockImplementation(() => ({
      setOnFirstOutput: jest.fn(),
      setSerializeAddon: jest.fn(),
      installDebugMonitor: jest.fn(),
      writeInitialContent: jest.fn().mockResolvedValue(undefined),
      write: jest.fn(),
      cleanup: jest.fn(),
      updateSendFlowControl: jest.fn(),
    })),
  };
}

export function analyticsContextPlainMockModule() {
  return { useAnalytics: () => ({ track: jest.fn() }) };
}

export function approvalsContextMockModule() {
  return {
    useApprovalsContext: () => ({
      clearForSession: jest.fn(),
      refresh: jest.fn(),
      pendingCount: 0,
    }),
  };
}

export function handednessMockModule() {
  return { useHandedness: () => ({ leftHanded: false, toggleHandedness: jest.fn() }) };
}

export function splitContainerSizeMockModule() {
  return { useSplitContainerSize: () => ({ width: 800, height: 600 }) };
}

// ── Shared across both clusters ───────────────────────────────────────────────

export function browserLogStreamMockModule() {
  return { useBrowserLogStream: jest.fn() };
}

export function viewportProviderDesktopMockModule() {
  return { useViewport: () => ({ isMobile: false }) };
}
