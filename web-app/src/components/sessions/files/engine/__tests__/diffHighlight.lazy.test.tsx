import { EditorView } from "@codemirror/view";
import { installJsdomLayoutShims } from "@/lib/files/__tests__/fixtures/jsdomLayout";
import { VirtualDoc } from "@/lib/__tests__/virtualFile";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { CmEngine } from "../CmEngine";
import { markAtDocLine } from "../diffHighlight";

describe("diff marks in a lazy (windowed) document", () => {
  beforeAll(installJsdomLayoutShims);

  it("markAtDocLine_should_MapDocLinesToFileLines_AndSkipPlaceholders", async () => {
    const engine = new CmEngine();
    const host = document.createElement("div");
    document.body.appendChild(host);
    const file = new VirtualDoc(1000, 7);
    // File line 505 is the 6th line of the window 500-540; a gap precedes it, so doc line != file line.
    const marks = new Map<number, GutterMarkType>([[505, "modify"], [1, "add"]]);
    const config = { mode: "Read" as const, ariaLabel: "File: lazy.txt" };
    const buffer = await engine.createBuffer({ doc: "", lazy: { lineCount: 1000 }, gutterMarks: marks, config });
    const handle = engine.mount(host, { buffer, ...config });
    handle.provideRange({ startLine: 500, endLine: 540 }, file.fetch(499, 41).join("\n"));
    const view = EditorView.findFromDOM(host);
    if (!view) throw new Error("editor view not mounted");
    const state = view.state;

    const tinted = Array.from({ length: state.doc.lines }, (_, i) => i + 1).filter((n) => markAtDocLine(state, marks, n));
    expect(tinted).toHaveLength(1);
    expect(state.doc.line(tinted[0]).text).toBe(file.fetch(504, 1)[0]);
    // File line 1 sits in an unloaded gap: its placeholder must not be tinted.
    expect(markAtDocLine(state, marks, 1)).toBeUndefined();
    handle.destroy();
    host.remove();
  });
});
