/**
 * Tests for ConfigPageContent — the Settings > Config Files tab.
 *
 * Covers the fix for the "hangs on Loading... with no error or timeout" UX bug:
 * when listClaudeConfigs() never settles (a hung/unresponsive server), the file
 * list must surface a visible error + retry option instead of spinning forever.
 */

import React from "react";
import { render, screen, waitFor, act, fireEvent } from "@testing-library/react";
import { ConfigPageContent } from "./ConfigPageContent";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost" }));

jest.mock("@/lib/contexts/AuthContext", () => ({
  useAuth: () => ({
    authEnabled: false,
    authenticated: false,
    hasCredentials: false,
    refresh: jest.fn(),
  }),
}));

jest.mock("@/lib/auth/passkey", () => ({
  registerPasskey: jest.fn(),
  logout: jest.fn(),
}));

// Monaco is heavy and browser-API-dependent; stub it to a plain textarea so
// these tests can focus on the file-list loading/error states.
jest.mock("@monaco-editor/react", () => ({
  __esModule: true,
  default: () => <div data-testid="mock-editor" />,
}));

// Mock vanilla-extract CSS module to return the class-name keys as strings.
jest.mock("./config.css", () => {
  return new Proxy({}, { get: (_target, prop) => (typeof prop === "string" ? prop : "") });
});

const mockListClaudeConfigs = jest.fn();
const mockGetClaudeConfig = jest.fn();
const mockUpdateClaudeConfig = jest.fn();

function setClient() {
  (createClient as jest.Mock).mockReturnValue({
    listClaudeConfigs: mockListClaudeConfigs,
    getClaudeConfig: mockGetClaudeConfig,
    updateClaudeConfig: mockUpdateClaudeConfig,
  });
}

beforeEach(() => {
  jest.clearAllMocks();
  jest.useFakeTimers();
  setClient();
  (createConnectTransport as jest.Mock).mockReturnValue({});
  // Config tab also fetches /api/server-info on mount — stub fetch so it
  // resolves quickly and doesn't interfere with the configs-loading assertions.
  global.fetch = jest.fn().mockResolvedValue({
    json: () => Promise.resolve({ ca_pem_path: "", https_url: "", tls_enabled: false, hostnames: [], programs: [] }),
  }) as unknown as typeof fetch;
});

afterEach(() => {
  jest.useRealTimers();
});

test("ConfigPageContent_should_renderFileList_When_LoadSucceeds", async () => {
  mockListClaudeConfigs.mockResolvedValue({ configs: [{ name: "CLAUDE.md", content: "" }] });

  render(<ConfigPageContent />);

  await waitFor(() => expect(screen.getByText("CLAUDE.md")).toBeInTheDocument());
  expect(screen.queryByText(/Loading/)).not.toBeInTheDocument();
});

test("ConfigPageContent_should_showErrorAndRetry_When_LoadHangsPastTimeout", async () => {
  // Never resolves — simulates a hung/unresponsive server. Without the fix,
  // this leaves the file list stuck on "Loading..." forever.
  mockListClaudeConfigs.mockReturnValue(new Promise(() => {}));

  render(<ConfigPageContent />);

  expect(screen.getByText("Loading...")).toBeInTheDocument();

  await act(async () => {
    jest.advanceTimersByTime(15000);
    // Let the rejected timeout promise's microtask queue flush.
    await Promise.resolve();
    await Promise.resolve();
  });

  await waitFor(() => expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument());
  expect(screen.queryByText("Loading...")).not.toBeInTheDocument();
  expect(screen.getAllByText(/Timed out waiting for the server to respond/).length).toBeGreaterThan(0);
});

test("ConfigPageContent_should_retryLoad_When_RetryClicked", async () => {
  mockListClaudeConfigs.mockReturnValueOnce(new Promise(() => {}));

  render(<ConfigPageContent />);

  await act(async () => {
    jest.advanceTimersByTime(15000);
    await Promise.resolve();
    await Promise.resolve();
  });

  const retryButton = await screen.findByRole("button", { name: "Retry" });

  mockListClaudeConfigs.mockResolvedValueOnce({ configs: [{ name: "settings.json", content: "{}" }] });

  await act(async () => {
    fireEvent.click(retryButton);
    await Promise.resolve();
  });

  await waitFor(() => expect(screen.getByText("settings.json")).toBeInTheDocument());
});
