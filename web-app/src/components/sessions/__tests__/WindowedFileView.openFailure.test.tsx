import React from "react";
import { act, render } from "@testing-library/react";
import { TransportFailure } from "@/lib/files/transport/fileTransport";
import { createVirtualRangeTransport } from "@/lib/files/__tests__/fixtures/virtualRangeTransport";
import type { WindowedEngine } from "@/lib/files/windowedFile";
import { WindowedFileView } from "../files/WindowedFileView";

const transport = createVirtualRangeTransport({ lines: 500_000, failures: [new TransportFailure("unavailable")] });
jest.mock("@/lib/hooks/useFileTransport", () => ({ useFileTransport: () => transport }));
jest.mock("../CodeEditor", () => ({
  CodeEditor: ({ onReady }: { onReady: (e: unknown) => void }) => {
    const engine: WindowedEngine = {
      createBufferState: async () => ({}) as never, setBufferState: () => undefined, onNeedRange: () => undefined, provideRange: () => undefined,
      gotoLine: () => undefined, getCursor: () => ({ line: 1, col: 0 }), highlightRange: () => undefined, getViewport: () => ({ startLine: 1, endLine: 30 }),
    };
    React.useEffect(() => { onReady(engine); }, []); // eslint-disable-line react-hooks/exhaustive-deps
    return null;
  },
}));

describe("WindowedFileView", () => {
  it("windowedFileView_should_FallBackToTheFullRead_When_TheFirstWindowFailsToOpen", async () => {
    const onDemoted = jest.fn();
    const source = { kind: "windowed" as const, size: 5_000_000, windowStartLine: 1 } as React.ComponentProps<typeof WindowedFileView>["source"];
    render(<WindowedFileView sessionId="s1" path="big.log" source={source} language="text" onDemoted={onDemoted} />);
    await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
    expect(onDemoted).toHaveBeenCalled(); // otherwise the pane stays a blank editor with no state or way out
  });
});
