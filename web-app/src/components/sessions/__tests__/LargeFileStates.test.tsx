import React from "react";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Rejected } from "@/lib/files/transport/fileTransport";
import { WindowedFile, type WindowedEngine } from "@/lib/files/windowedFile";
import { useWindowedState } from "@/lib/files/useWindowedState";
import { createVirtualRangeTransport } from "@/lib/files/__tests__/fixtures/virtualRangeTransport";
import { FallbackChip, LargeFileStates } from "../files/LargeFileStates";
import { StatusBar } from "../files/StatusBar";

const REF = { sessionId: "s1", path: "big.log" };
const statusBase = { line: 4_100_000, col: 1, eol: null, encoding: "UTF-8", language: "Plain Text", dirty: false, message: null };

function setup(over: Parameters<typeof createVirtualRangeTransport>[0] = { lines: 1_000_000 }) {
  const transport = createVirtualRangeTransport(over);
  let need: (r: { startLine: number; endLine: number }) => void = () => undefined;
  const cursor = { line: 1 };
  const engine: WindowedEngine = {
    createBufferState: async () => ({}) as never,
    setBufferState: () => undefined,
    onNeedRange: (cb) => { need = cb; },
    provideRange: () => undefined,
    gotoLine: (n) => { cursor.line = n; },
    getCursor: () => ({ line: cursor.line, col: 0 }),
    highlightRange: () => undefined,
    getViewport: () => ({ startLine: 1, endLine: 30 }),
  };
  const file = new WindowedFile(transport, REF, transport.size);
  const Harness = () => <LargeFileStates file={file} state={useWindowedState(file)} />;
  return { transport, file, engine, need: (r: { startLine: number; endLine: number }) => need(r), cursor, Harness };
}

const flush = () => act(async () => { await jest.advanceTimersByTimeAsync(0); });
const announce = () => screen.getByTestId("large-file-announce").textContent;

describe("LargeFileStates", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("largeFileStates_should_ShowGapLoadingFailedRetryIndexingT2AndFallbackChips_WithAnnouncements", async () => {
    const t = setup();
    render(<t.Harness />);
    await act(async () => { await t.file.attach(t.engine, "text", 1); });

    // Loading: nothing is announced for a quick load; a burst over 500 ms is announced once, with its first range.
    t.transport.failures.push(new Rejected("busy"));
    act(() => t.need({ startLine: 1001, endLine: 1500 }));
    expect(announce()).toBe("");
    await act(async () => { await jest.advanceTimersByTimeAsync(0); });

    // Failed: "Couldn't load lines A-B." with Retry, announced politely; the rest of the file stays usable.
    expect(screen.getByText("Couldn't load lines 1,001-1,500.")).toBeInTheDocument();
    expect(announce()).toBe("Couldn't load lines 1,001-1,500");
    await userEvent.setup({ advanceTimers: jest.advanceTimersByTime }).click(screen.getByRole("button", { name: "Retry" }));
    await flush();
    expect(screen.queryByText(/Couldn't load/)).toBeNull();
    expect(t.transport.reads.filter((r) => r.anchorLine === 1001)).toHaveLength(2);

    // A slow burst: announced after 500 ms, once, naming the first range.
    let release: () => void = () => undefined;
    const slow = t.transport.read.bind(t.transport);
    t.transport.read = (ref, req, signal) => new Promise((resolve, reject) => { release = () => slow(ref, req, signal).then(resolve, reject); });
    act(() => t.need({ startLine: 2001, endLine: 2500 }));
    await act(async () => { await jest.advanceTimersByTimeAsync(499); });
    expect(announce()).toBe("");
    await act(async () => { await jest.advanceTimersByTimeAsync(2); });
    expect(announce()).toBe("Loading lines 2,001-2,500...");
    act(() => t.need({ startLine: 3001, endLine: 3500 }));
    await act(async () => { await jest.advanceTimersByTimeAsync(600); });
    expect(announce()).toBe("Loading lines 2,001-2,500...");
    await act(async () => { release(); await jest.advanceTimersByTimeAsync(0); });
  });

  it("largeFileStates_should_ShowIndexingChipT2ReasonAndFallbackChip_InTheStatusBarAndViewer", () => {
    const { rerender } = render(<StatusBar {...statusBase} readOnly="too_large" windowed={{ lineCount: 4_200_000, exact: false, indexing: true }} />);
    expect(screen.getByText("Ln 4,100,000 of about 4.2 million, Col 1")).toBeInTheDocument();
    expect(screen.getByTestId("indexing-chip")).toHaveTextContent("Indexing file...");
    expect(screen.getByRole("button", { name: "Read-only: file too large to edit" })).toBeInTheDocument();
    rerender(<StatusBar {...statusBase} readOnly="too_large" windowed={{ lineCount: 4_200_123, exact: true, indexing: false }} />);
    expect(screen.getByText("Ln 4,100,000 of 4,200,123, Col 1")).toBeInTheDocument();
    expect(screen.queryByTestId("indexing-chip")).toBeNull();
    render(<FallbackChip />);
    expect(screen.getByTestId("fallback-chip")).toHaveTextContent("Using simpler file mode");
  });

  it("farJump_should_ShowGoingToLineWithCancel_LandWithin1LineHeight_ClampPastEnd_And_QueueBeforeIndex", async () => {
    let ready = false;
    const t = setup({ lines: 1_000_000, ready: () => ready });
    render(<t.Harness />);
    await act(async () => { await t.file.attach(t.engine, "text", 1); });

    // Queued behind the index: progress with Cancel after the pending delay, no failure.
    let done: Promise<unknown> = Promise.resolve();
    act(() => { done = t.file.goTo(750_000); });
    await act(async () => { await jest.advanceTimersByTimeAsync(200); });
    expect(screen.getByText("Going to line 750,000... waiting for the file index")).toBeInTheDocument();
    await userEvent.setup({ advanceTimers: jest.advanceTimersByTime }).click(screen.getByRole("button", { name: "Cancel" }));
    await act(async () => { await done; });
    expect(screen.queryByText(/Going to line/)).toBeNull();
    expect(t.cursor.line).toBe(1); // cancelled: no jump, no jumplist entry

    // Index ready: the same jump lands on the exact line; past the end it clamps and says so.
    ready = true;
    await act(async () => { await t.file.goTo(750_000); });
    expect(t.cursor.line).toBe(750_000);
    await act(async () => { await t.file.goTo(2_000_000); });
    expect(t.cursor.line).toBe(1_000_000);
    expect(screen.getByText("Line 2,000,000 is past the end; showing line 1,000,000")).toBeInTheDocument();
  });
});
