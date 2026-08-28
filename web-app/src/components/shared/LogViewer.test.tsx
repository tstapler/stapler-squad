/**
 * Focused tests for LogViewer's new contracts (this PR adds both): the
 * onStateChange callback that lifts logs/rawEntries/error up to a page-level
 * toolbar, and the forwardRef-exposed refresh() handle. Child rendering
 * (VirtualLogList, LogViewerToolbar, etc.) is stubbed out — those have their
 * own tests — so this only exercises the wiring LogViewer itself owns.
 */

import { createRef } from "react";
import { render, waitFor } from "@testing-library/react";
import { LogViewer, type LogViewerHandle } from "./LogViewer";

const mockGetLogs = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({ getLogs: mockGetLogs })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({})),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: jest.fn(() => "http://localhost:8543"),
}));

// Live-tail defaults to enabled and fetches immediately on mount — not what
// these tests are about, so it's neutralized the same way useLogViewer's own
// tests do (mock its module rather than letting it race the assertions below).
jest.mock("@/lib/hooks/useLiveTail", () => ({
  useLiveTail: () => [{ isActive: true, isPaused: false, newLogCount: 0, lastFetch: null, error: null }, {}],
}));

jest.mock("./VirtualLogList", () => ({
  VirtualLogList: () => <div data-testid="virtual-log-list-stub" />,
}));
jest.mock("./JumpToLatestButton", () => ({
  JumpToLatestButton: () => null,
}));
jest.mock("./LogViewerToolbar", () => ({
  LogViewerToolbar: () => <div data-testid="toolbar-stub" />,
}));
jest.mock("./ShortcutHelpOverlay", () => ({
  ShortcutHelpOverlay: () => null,
}));

function protoEntry(message: string) {
  return { message, level: "INFO", timestamp: undefined };
}

describe("LogViewer", () => {
  beforeEach(() => {
    mockGetLogs.mockReset();
  });

  it("reports logs, rawEntries, and error via onStateChange after a successful fetch", async () => {
    mockGetLogs.mockResolvedValueOnce({ entries: [protoEntry("hello")], totalCount: 1 });
    const onStateChange = jest.fn();

    render(<LogViewer source="app" onStateChange={onStateChange} />);

    await waitFor(() =>
      expect(onStateChange).toHaveBeenCalledWith(
        expect.objectContaining({
          logs: expect.arrayContaining([expect.objectContaining({ message: "hello" })]),
          rawEntries: expect.arrayContaining([expect.objectContaining({ message: "hello" })]),
          totalCount: 1,
          error: null,
        }),
      ),
    );
  });

  it("reports a non-null error via onStateChange when the fetch fails", async () => {
    mockGetLogs.mockRejectedValueOnce(new Error("backend unavailable"));
    const onStateChange = jest.fn();

    render(<LogViewer source="app" onStateChange={onStateChange} />);

    await waitFor(() =>
      expect(onStateChange).toHaveBeenCalledWith(expect.objectContaining({ error: "backend unavailable" })),
    );
  });

  it("re-fetches when the exposed ref's refresh() is called", async () => {
    mockGetLogs.mockResolvedValueOnce({ entries: [protoEntry("first")], totalCount: 1 });
    const ref = createRef<LogViewerHandle>();

    render(<LogViewer source="app" ref={ref} />);
    await waitFor(() => expect(mockGetLogs).toHaveBeenCalledTimes(1));

    mockGetLogs.mockResolvedValueOnce({ entries: [protoEntry("second")], totalCount: 1 });
    ref.current?.refresh();

    await waitFor(() => expect(mockGetLogs).toHaveBeenCalledTimes(2));
  });
});
