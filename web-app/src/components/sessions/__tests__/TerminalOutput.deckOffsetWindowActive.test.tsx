// @feature terminal-reconnect
/**
 * Regression for the keep-alive mobile bug: a background window's terminal is still mounted
 * (window_keep_alive) and its own pane is still "visible" within that window, so without a
 * windowActive gate it kept re-publishing --mobile-deck-top-offset and clobbering the
 * foreground window's anchor (TerminalOutput.tsx:132).
 */

import React from "react";
import { render, act } from "@testing-library/react";

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

// eslint-disable-next-line import/first
import { TerminalOutput } from "../TerminalOutput";
import { TerminalPoolProvider } from "@/lib/terminal/TerminalPool";
// eslint-disable-next-line import/first
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";

const DECK_TOP_OFFSET_VAR = "--mobile-deck-top-offset";
const FOREGROUND_TOOLBAR_HEIGHT = 56;
const deckVar = () => document.documentElement.style.getPropertyValue(DECK_TOP_OFFSET_VAR);

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

async function renderTerminal(windowActive: boolean, sessionId: string, bottom: number) {
  jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    top: 0, bottom, left: 0, right: 390, width: 390, height: bottom, x: 0, y: 0, toJSON: () => ({}),
  } as DOMRect);
  const utils = render(
    <TerminalPoolProvider>
      <TerminalOutput sessionId={sessionId} baseUrl="/api" isVisible windowActive={windowActive} />
    </TerminalPoolProvider>,
  );
  await act(async () => {});
  return utils;
}

beforeEach(() => {
  jest.clearAllMocks();
  document.documentElement.style.removeProperty(DECK_TOP_OFFSET_VAR);
  (useTerminalStream as jest.Mock).mockReturnValue(makeStreamMock());
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
  document.documentElement.style.removeProperty(DECK_TOP_OFFSET_VAR);
});

describe("TerminalOutput deck anchor respects windowActive", () => {
  it("a visible pane in an active window anchors the deck", async () => {
    await renderTerminal(true, "s1", FOREGROUND_TOOLBAR_HEIGHT);
    expect(deckVar()).toBe(`${FOREGROUND_TOOLBAR_HEIGHT}px`);
  });

  it("a visible pane in a keep-alive background window must not anchor the deck", async () => {
    await renderTerminal(true, "s1", FOREGROUND_TOOLBAR_HEIGHT); // foreground window claims the anchor first
    expect(deckVar()).toBe(`${FOREGROUND_TOOLBAR_HEIGHT}px`);

    // A second, kept-alive-but-backgrounded window's own "visible" pane, at a different height —
    // if it incorrectly publishes, the var changes; if correctly gated, it stays at the foreground's value.
    await renderTerminal(false, "s2", 999);
    expect(deckVar()).toBe(`${FOREGROUND_TOOLBAR_HEIGHT}px`);
  });
});
