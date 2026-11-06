import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import { createFilesTestHarness, installJsdomLayoutShims } from "@/lib/files/__tests__/fixtures";
import { createVirtualRangeTransport } from "@/lib/files/__tests__/fixtures/virtualRangeTransport";
import { Rejected } from "@/lib/files/transport/fileTransport";
import type { BufferSource } from "@/lib/files/bufferReducer";
import { FileContentViewer } from "../FileContentViewer";
import type { ReadViewHandle } from "../files/readViewHandle";

const source: BufferSource = { kind: "windowed", tier: "t1", size: 12_000_000, windowStartLine: 812_345, analysis: "ready" };

function setup(over: { failures?: unknown[] } = {}) {
  const transport = createVirtualRangeTransport({ lines: 1_200_000, size: 12_000_000, failures: over.failures });
  const harness = createFilesTestHarness({
    transport,
    handlers: { getFileContent: () => ({ content: "first megabyte\nonly", isBinary: false, isTruncated: true, size: 12_000_000n }) },
  });
  let handle: ReadViewHandle | undefined;
  const view = (accessible: boolean) => (
    <FileContentViewer sessionId={harness.sessionId} filePath="logs/big.log" baseUrl="http://x" source={source} accessible={accessible} onEditorReady={(h) => { handle = h; }} />
  );
  const utils = render(view(false), { wrapper: harness.wrapper });
  return { ...utils, transport, harness, view, handle: () => handle as ReadViewHandle };
}

describe("windowed FileContentViewer", () => {
  beforeAll(installJsdomLayoutShims);

  it("windowedViewer_should_OpenAtTheAnchorLineFromOneRangeRead_And_KeepTheLineAcrossTheAccessibleViewToggle", async () => {
    const t = setup();
    await waitFor(() => expect(t.handle()?.windowed).toBeDefined());
    await waitFor(() => expect(t.handle().getCursor().line).toBe(812_345));
    expect(t.transport.reads).toHaveLength(1);
    expect(t.harness.calls).not.toContain("getFileContent"); // the truncated whole-file read is never needed

    // A jump through the bridge handle lands on the exact absolute line.
    await act(async () => { await t.handle().windowed!.goTo(1_000_000); });
    expect(t.handle().getCursor().line).toBe(1_000_000);

    // Accessible view: pages come from the same file; leaving it reopens the editor on the last line.
    t.rerender(t.view(true));
    expect(await screen.findByTestId("accessible-view")).toBeInTheDocument();
    await screen.findByText(/^Lines 1-500 of /);
    t.rerender(t.view(false));
    await waitFor(() => expect(t.handle().getCursor().line).toBe(1_000_000));
  });

  it("windowedViewer_should_FallBackToTheTruncatedRead_WithAQuietChip_When_RangeReadsAreGone", async () => {
    const t = setup({ failures: [new Rejected("unsupported")] });
    expect(await screen.findByTestId("fallback-chip")).toHaveTextContent("Using simpler file mode");
    await waitFor(() => expect(t.harness.calls).toContain("getFileContent"));
  });
});
