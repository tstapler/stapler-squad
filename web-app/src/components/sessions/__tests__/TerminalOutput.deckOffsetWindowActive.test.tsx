// @feature terminal-reconnect
/**
 * Regression for the keep-alive mobile bug: a background window's terminal is still mounted
 * (window_keep_alive) and its own pane is still "visible" within that window, so without a
 * windowActive gate it kept re-publishing --mobile-deck-top-offset and clobbering the
 * foreground window's anchor (TerminalOutput.tsx:133).
 *
 * Asserts directly on the usePublishDeckTopOffset call site rather than the real global CSS var:
 * the latter goes through sharedBodyMeasurer's shared body MutationObserver/rAF scheduling, which
 * re-measures every mounted subscriber (including an unrelated one) on any DOM mutation — fine in
 * production (each subscriber reads its own element), but a source of order/timing flake in a test
 * that mounts two instances in one document and swaps a single global getBoundingClientRect mock.
 */

import React from "react";
import { render } from "@testing-library/react";

// kibitzer:ignore replace-magic-literal -- jest.mock() factories run hoisted before any top-level
// const, so each one must repeat the require() path inline; matches every sibling TerminalOutput test file.
const mockXtermHandle: any = { ...require("./terminalOutputTestMocks").createMockXtermHandle(), serializeAddon: null };

jest.mock("../XtermTerminal", () =>
  require("./terminalOutputTestMocks").capturingXtermTerminalMockModule(mockXtermHandle, () => {})
);
jest.mock("@/lib/hooks/useTerminalStream", () => ({ useTerminalStream: jest.fn() }));
jest.mock("@/lib/terminal/TerminalDimensionCache", () =>
  require("./terminalOutputTestMocks").terminalDimensionCacheWithValidationMockModule()
);
jest.mock("@/lib/terminal/TerminalStreamManager", () =>
  require("./terminalOutputTestMocks").terminalStreamManagerWithSerializeAddonMockModule()
);
jest.mock("@/lib/contexts/AnalyticsContext", () =>
  require("./terminalOutputTestMocks").analyticsContextPlainMockModule()
);
jest.mock("@/lib/contexts/ApprovalsContext", () =>
  require("./terminalOutputTestMocks").approvalsContextMockModule()
);
jest.mock("@/lib/hooks/useHandedness", () => require("./terminalOutputTestMocks").handednessMockModule());
jest.mock("@/lib/hooks/useSplitContainerSize", () =>
  require("./terminalOutputTestMocks").splitContainerSizeMockModule()
);
jest.mock("@/lib/hooks/useBrowserLogStream", () =>
  require("./terminalOutputTestMocks").browserLogStreamMockModule()
);
jest.mock("@/components/providers/ViewportProvider", () =>
  require("./terminalOutputTestMocks").viewportProviderDesktopMockModule()
);

const usePublishDeckTopOffset = jest.fn();
jest.mock("@/lib/hooks/usePublishStackTopOffset", () => ({
  usePublishDeckTopOffset: (...args: unknown[]) => usePublishDeckTopOffset(...args),
  usePublishStackTopOffset: jest.fn(),
}));

// eslint-disable-next-line import/first
import { TerminalOutput } from "../TerminalOutput";
import { TerminalPoolProvider } from "@/lib/terminal/TerminalPool";
// eslint-disable-next-line import/first
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";

function makeStreamMock(overrides: Record<string, unknown> = {}) {
  return {
    isConnected: true,
    error: null,
    output: "",
    connect: jest.fn(),
    disconnect: jest.fn(),
    sendInput: jest.fn(),
    resize: jest.fn(),
    scrollbackLoaded: false,
    requestScrollback: jest.fn(),
    sendFlowControl: jest.fn(),
    startRecording: jest.fn(),
    stopRecording: jest.fn(),
    terminalState: "CONNECTED",
    isHardFailed: false,
    handleManualReconnect: jest.fn(),
    requestFullResync: jest.fn(),
    markResyncComplete: jest.fn(),
    markPaneResponseReceived: jest.fn(),
    ...overrides,
  };
}

function renderTerminal(windowActive: boolean, isVisible: boolean) {
  return render(
    <TerminalPoolProvider>
      <TerminalOutput sessionId="s1" baseUrl="/api" isVisible={isVisible} windowActive={windowActive} />
    </TerminalPoolProvider>,
  );
}

/** The `enabled` arg usePublishDeckTopOffset was last called with. */
const lastEnabled = () => usePublishDeckTopOffset.mock.calls.at(-1)?.[1];

beforeEach(() => {
  jest.clearAllMocks();
  (useTerminalStream as jest.Mock).mockReturnValue(makeStreamMock());
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
});

describe("TerminalOutput deck anchor respects windowActive", () => {
  it("anchors the deck when the window is active and the pane is visible", () => {
    renderTerminal(true, true);
    expect(lastEnabled()).toBe(true);
  });

  it("must not anchor the deck for a visible pane in a keep-alive background window", () => {
    renderTerminal(false, true);
    expect(lastEnabled()).toBe(false);
  });

  it("still does not anchor the deck for a non-visible pane in an active window", () => {
    renderTerminal(true, false);
    expect(lastEnabled()).toBe(false);
  });
});
