import { EditorView } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { installJsdomLayoutShims } from "@/lib/files/__tests__/fixtures/jsdomLayout";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { CmEngine } from "../CmEngine";
import { nextHunkDocLine } from "../diffHighlight";

const doc = Array.from({ length: 20 }, (_, i) => `line ${i + 1}`).join("\n");
// Hunks: lines 3-4, 10, 15-16.
const marks = new Map<number, GutterMarkType>([[3, "add"], [4, "add"], [10, "modify"], [15, "delete"], [16, "delete"]]);

describe("nextHunkDocLine", () => {
  const state = EditorState.create({ doc });
  it("advances_to_the_first_line_of_the_next_hunk", () => {
    expect(nextHunkDocLine(state, marks, 3)).toBe(10);
    expect(nextHunkDocLine(state, marks, 4)).toBe(10);
    expect(nextHunkDocLine(state, marks, 10)).toBe(15);
  });
  it("wraps_from_the_last_hunk_to_the_first", () => {
    expect(nextHunkDocLine(state, marks, 15)).toBe(3);
    expect(nextHunkDocLine(state, marks, 16)).toBe(3);
  });
  it("returns_null_without_marks", () => {
    expect(nextHunkDocLine(state, new Map(), 1)).toBeNull();
  });
});

describe("change gutter tap", () => {
  beforeAll(installJsdomLayoutShims);
  const mountView = async () => {
    const engine = new CmEngine();
    const host = document.createElement("div");
    document.body.appendChild(host);
    const config = { mode: "Read" as const, ariaLabel: "File: x.txt" };
    const buffer = await engine.createBuffer({ doc, gutterMarks: marks, config });
    engine.mount(host, { buffer, ...config });
    const view = EditorView.findFromDOM(host);
    if (!view) throw new Error("no view");
    return view;
  };
  const setPointer = (coarse: boolean) => {
    window.matchMedia = jest.fn().mockImplementation((q: string) => ({
      matches: coarse && q === "(pointer: coarse)",
      media: q,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
    })) as never;
  };
  // CM resolves a gutter click to a line from clientY against the gutter's rect, so aim at the line's block.
  const tapGutter = (view: EditorView, docLine: number) => {
    const gutter = view.dom.querySelector(".cm-changeGutter") as HTMLElement;
    const block = view.lineBlockAt(view.state.doc.line(docLine).from);
    const rect = gutter.getBoundingClientRect();
    gutter.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: rect.left + 1, clientY: rect.top + block.top + block.height / 2 }));
  };
  const cursorLine = (view: EditorView) => view.state.doc.lineAt(view.state.selection.main.head).number;

  // Each tap gets a fresh view: jsdom's layout shims lose block geometry once a tap scrolls the viewport.
  it.each([[3, 10], [4, 10], [10, 15], [15, 3], [16, 3]])("coarse_tap_on_line_%i_should_move_cursor_to_line_%i", async (tapped, expected) => {
    setPointer(true);
    const view = await mountView();
    tapGutter(view, tapped);
    expect(cursorLine(view)).toBe(expected);
  });

  it("fine_pointer_click_should_not_move_cursor", async () => {
    setPointer(false);
    const view = await mountView();
    tapGutter(view, 3);
    expect(cursorLine(view)).toBe(1);
  });
});

describe("coarse-pointer gutter sizing", () => {
  beforeAll(installJsdomLayoutShims);
  it("line_number_gutter_should_be_44px_min_inside_a_coarse_pointer_media_query_only", async () => {
    const engine = new CmEngine();
    const host = document.createElement("div");
    document.body.appendChild(host);
    const config = { mode: "Read" as const, ariaLabel: "File: x.txt" };
    engine.mount(host, { buffer: await engine.createBuffer({ doc, config }), ...config });
    const css = Array.from(document.querySelectorAll("style")).map((e) => e.textContent ?? "").join("\n");
    const block = /@media \(pointer: coarse\)\s*\{([^@]*?)\}\s*\}/.exec(css.replace(/\n/g, " "));
    expect(block?.[1]).toMatch(/\.cm-lineNumbers \.cm-gutterElement\s*\{[^}]*min-width: 44px/);
    // Outside the media query the line-number element gets no 44px minimum.
    expect(css.replace(/@media \(pointer: coarse\)[\s\S]*$/, "")).not.toMatch(/cm-lineNumbers[^{]*\{[^}]*min-width: 44px/);
  });
});
