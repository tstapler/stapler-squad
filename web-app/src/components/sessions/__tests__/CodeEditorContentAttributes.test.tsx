/**
 * Story 0.1.5 (pre-mortem #10): the editor content element must carry autocorrect-hostile
 * attributes explicitly. CodeEditor does not exist yet (Task 1.2.1a applies this for real), so
 * the assertion runs against the spike's `hostileAttributes` extension and also records
 * whether stock CodeMirror 6 sets them (the UNVERIFIED claim).
 */
import { EditorState, type Extension } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { hostileAttributes } from "@/app/spike-files/cm";

function contentDom(extensions: Extension[]): HTMLElement {
  const parent = document.createElement("div");
  document.body.appendChild(parent);
  const view = new EditorView({ state: EditorState.create({ doc: "x", extensions }), parent });
  return view.contentDOM;
}

describe("CodeEditor content attributes (Story 0.1.5)", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("codeEditor_should_SetAutocorrectAutocapitalizeSpellcheckOff_When_Mounted", () => {
    const el = contentDom([hostileAttributes]);
    expect(el.getAttribute("autocorrect")).toBe("off");
    expect(el.getAttribute("autocapitalize")).toBe("off");
    expect(el.getAttribute("spellcheck")).toBe("false");
  });

  // Resolves the plan's UNVERIFIED claim: @codemirror/view 6.41 sets all three itself
  // (dist/index.js, contentAttributes defaults). The explicit extension stays as a guard against
  // a future default change.
  it("codeMirror6_should_SetAutocorrectAutocapitalizeSpellcheckOff_By_Default", () => {
    const el = contentDom([]);
    expect(el.getAttribute("autocorrect")).toBe("off");
    expect(el.getAttribute("autocapitalize")).toBe("off");
    expect(el.getAttribute("spellcheck")).toBe("false");
  });
});
