import { createVirtualRangeTransport } from "./fixtures/virtualRangeTransport";
import { WindowedFile, type WindowedEngine } from "../windowedFile";
import type { GutterMarkType } from "@/lib/utils/parseDiff";

type Init = Parameters<WindowedEngine["createBufferState"]>[0];

function recordingEngine() {
  const inits: Init[] = [];
  const engine: WindowedEngine = {
    createBufferState: async (init) => { inits.push(init); return {} as never; },
    setBufferState: () => undefined,
    onNeedRange: () => undefined,
    provideRange: () => undefined,
    gotoLine: () => undefined,
    getCursor: () => ({ line: 1, col: 0 }),
    highlightRange: () => undefined,
    getViewport: () => ({ startLine: 1, endLine: 40 }),
  };
  return { engine, inits };
}

describe("WindowedFile diff marks", () => {
  const marks = new Map<number, GutterMarkType>([[3, "add"]]);
  const fileFor = () => {
    const t = createVirtualRangeTransport({ lines: 1000 });
    return new WindowedFile(t, { sessionId: "s1", path: "big.log" }, t.size);
  };

  it("windowedFile_should_BuildTheLazyBufferWithMarks_When_MarksWereSetBeforeAttach", async () => {
    const { engine, inits } = recordingEngine();
    const file = fileFor();
    await file.setGutterMarks(marks);
    await file.attach(engine, "text", 1);
    expect(inits[0].gutterMarks).toBe(marks);
  });

  it("windowedFile_should_RebuildTheBufferWithNewMarks_When_MarksChangeAfterOpen", async () => {
    const { engine, inits } = recordingEngine();
    const file = fileFor();
    await file.attach(engine, "text", 1);
    expect(inits[0].gutterMarks).toBeUndefined();
    await file.setGutterMarks(marks);
    expect(inits[inits.length - 1].gutterMarks).toBe(marks);
  });
});
