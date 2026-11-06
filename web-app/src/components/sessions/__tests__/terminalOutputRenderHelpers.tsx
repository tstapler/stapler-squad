import React from "react";
import { render } from "@testing-library/react";
import { TerminalOutput } from "../TerminalOutput";
import { TerminalPoolProvider } from "@/lib/terminal/TerminalPool";

export function makeStreamMock(overrides = {}) {
  return {
    isConnected: false,
    error: null,
    connect: jest.fn(),
    disconnect: jest.fn(),
    sendInput: jest.fn(),
    resize: jest.fn(),
    scrollbackLoaded: false,
    requestScrollback: jest.fn(),
    sendFlowControl: jest.fn(),
    startRecording: jest.fn(),
    stopRecording: jest.fn(),
    // useVisibilityResync is wired unconditionally in TerminalOutput.tsx and
    // calls these on every unmount/session-id change.
    requestFullResync: jest.fn(),
    markResyncComplete: jest.fn(),
    markPaneResponseReceived: jest.fn(),
    ...overrides,
  };
}

// TerminalOutput sources its xterm.js instance from TerminalPoolProvider, so
// every render must be wrapped in one, matching production's app-level provider.
export function renderTerminal(sessionId = "session-abc", baseUrl = "/api") {
  return render(
    <TerminalPoolProvider>
      <TerminalOutput sessionId={sessionId} baseUrl={baseUrl} isVisible={false} />
    </TerminalPoolProvider>
  );
}
