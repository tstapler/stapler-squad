import React, { useEffect } from "react";
import { act, render, waitFor } from "@testing-library/react";
import { createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import { createVirtualRangeTransport } from "@/lib/files/__tests__/fixtures/virtualRangeTransport";
import { FileContentViewer } from "../FileContentViewer";

jest.mock("@/lib/hooks/useFileService", () => ({
  useGetFileContent: () => ({ data: { content: "package main\n", isBinary: false, isTruncated: false, contentType: "text/plain" }, loading: false, error: null }),
}));

/** A stand-in editor that reports ready after mount, as the real one does once its chunk loads. */
jest.mock("@/components/sessions/CodeEditor", () => {
  const { useEffect: useFx } = jest.requireActual<typeof import("react")>("react");
  const handle = {
    createBufferState: async () => ({}), setBufferState: () => undefined, onNeedRange: () => undefined, provideRange: () => undefined,
    gotoLine: () => undefined, getCursor: () => ({ line: 1, col: 0 }), getViewport: () => ({ startLine: 1, endLine: 40 }),
    highlightRange: () => undefined, getText: () => "", scrollSnapshot: () => ({}), restoreScroll: () => undefined, focus: () => undefined,
  };
  return { CodeEditor: ({ onReady }: { onReady?: (h: unknown) => void }) => { useFx(() => onReady?.(handle), []); return null; } };
});

const frame = () => act(async () => { await new Promise<void>((r) => requestAnimationFrame(() => r())); });
const opens = (h: ReturnType<typeof createFilesTestHarness>) => h.analytics.events.filter((e) => e.name === "files.perf.open_ms");

describe("files.perf.open_ms", () => {
  it("openLatency_should_EmitOnceWithClosedLabelsAndNoPath_When_FullFileFirstPaints", async () => {
    const h = createFilesTestHarness({ sessionId: "s1" });
    render(<FileContentViewer sessionId="s1" filePath="secret/dir/main.go" baseUrl="" />, { wrapper: h.wrapper });
    await frame();

    await waitFor(() => expect(opens(h)).toHaveLength(1));
    const [ev] = opens(h);
    expect(ev.category).toBe("performance");
    expect(typeof ev.durationMs).toBe("number");
    expect(ev.labels).toMatchObject({ size: "lt10k", degraded: "false", tier: "t0" });
    expect(Object.keys(ev.labels ?? {}).length).toBeLessThanOrEqual(8);
    expect(JSON.stringify(ev)).not.toContain("secret/dir");
  });

  it("openLatency_should_CarryTheTier_When_WindowedFileFirstPaints", async () => {
    const t = createVirtualRangeTransport({ lines: 200_000, size: 5 * 1024 * 1024 });
    const h = createFilesTestHarness({ sessionId: "s1", transport: t });
    render(
      <FileContentViewer sessionId="s1" filePath="big.log" baseUrl="" source={{ kind: "windowed", tier: "t1", size: t.size, windowStartLine: 1, analysis: "ready" }} />,
      { wrapper: h.wrapper },
    );

    await waitFor(() => expect(opens(h)).toHaveLength(1), { timeout: 3000 });
    expect(opens(h)[0].labels).toMatchObject({ size: "gt1m", degraded: "false", tier: "t1" });
  });
  it("openLatency_should_NotEmitForTheNewFile_When_PathChangesOnAMountedWindowedView", async () => {
    const base = createVirtualRangeTransport({ lines: 200_000, size: 5 * 1024 * 1024 });
    let releaseB: () => void = () => undefined;
    const gateB = new Promise<void>((r) => { releaseB = r; });
    const t = { ...base, size: base.size, read: async (ref: { path: string }, ...rest: Parameters<typeof base.read> extends [unknown, ...infer R] ? R : never) => { if (ref.path === "b.log") await gateB; return base.read(ref as never, ...(rest as never)); } };
    const h = createFilesTestHarness({ sessionId: "s1", transport: t as never });
    const source = (size: number) => ({ kind: "windowed" as const, tier: "t1" as const, size, windowStartLine: 1, analysis: "ready" as const });
    const view = render(<FileContentViewer sessionId="s1" filePath="a.log" baseUrl="" source={source(base.size)} />, { wrapper: h.wrapper });
    await waitFor(() => expect(opens(h)).toHaveLength(1), { timeout: 3000 });

    view.rerender(<FileContentViewer sessionId="s1" filePath="b.log" baseUrl="" source={source(base.size + 1)} />);
    await frame();
    await frame();

    expect(opens(h)).toHaveLength(1); // b.log has not painted yet: no event, certainly not a ~0 ms one
    releaseB();
    await waitFor(() => expect(opens(h)).toHaveLength(2), { timeout: 3000 });
    expect(opens(h)[1].durationMs).toBeGreaterThan(0);
  });
});
