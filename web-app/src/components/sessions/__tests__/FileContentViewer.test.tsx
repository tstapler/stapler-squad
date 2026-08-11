/**
 * Tests FileContentViewer's CodeMirror rendering:
 *  - a file with no diff (or a diff that doesn't cover this file) renders
 *    CodeMirror with no gutter marks
 *  - a file that appears in diffContent renders CodeMirror with gutter
 *    markers built from buildGutterMarks reaching the gutter() line-marker
 *    callback
 */

import React from "react";
import { render, waitFor } from "@testing-library/react";
import { FileContentViewer, isDoubleGPress, DOUBLE_G_WINDOW_MS } from "../FileContentViewer";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockUseGetFileContent = jest.fn();
jest.mock("@/lib/hooks/useFileService", () => ({
  useGetFileContent: (...args: unknown[]) => mockUseGetFileContent(...args),
}));

interface CapturedGutterConfig {
  lineMarker: (view: unknown, block: { from: number }) => { toDOM: () => HTMLElement } | null;
}

let capturedGutterConfig: CapturedGutterConfig | null = null;
const mockEditorViewCtor = jest.fn();

interface CapturedKeyBinding {
  key: string;
  run: (view: unknown) => boolean;
}
let capturedKeymapBindings: CapturedKeyBinding[] | null = null;

jest.mock("@codemirror/view", () => {
  class GutterMarker {}
  class EditorView {
    static lineWrapping = {};
    static theme = jest.fn(() => ({}));
    constructor(opts: unknown) {
      mockEditorViewCtor(opts);
    }
    dispatch() {}
    destroy() {}
  }
  function gutter(config: CapturedGutterConfig) {
    capturedGutterConfig = config;
    return { __gutter: true };
  }
  const keymap = {
    of: jest.fn((bindings: CapturedKeyBinding[]) => {
      capturedKeymapBindings = bindings;
      return { __keymap: bindings };
    }),
  };
  return { EditorView, gutter, GutterMarker, keymap };
});

jest.mock("@codemirror/state", () => ({
  EditorState: {
    readOnly: { of: (v: unknown) => ({ readOnly: v }) },
    create: (opts: unknown) => opts,
  },
  // Minimal stand-in for CodeMirror's Compartment: of()/reconfigure() just
  // need to be callable and return something dispatch() (also mocked) can
  // accept; the real reconfiguration behavior is CodeMirror's own, not this
  // component's, so it isn't re-tested here.
  Compartment: class Compartment {
    of(ext: unknown) {
      return { __compartmentOf: ext };
    }
    reconfigure(ext: unknown) {
      return { __compartmentReconfigure: ext };
    }
  },
}));

jest.mock("codemirror", () => ({ basicSetup: {} }));
jest.mock("@codemirror/theme-one-dark", () => ({ oneDark: {} }));

jest.mock("@codemirror/language", () => ({
  HighlightStyle: { define: jest.fn(() => ({})) },
  syntaxHighlighting: jest.fn(() => ({})),
}));

jest.mock("@codemirror/commands", () => ({
  cursorCharLeft: jest.fn(),
  cursorCharRight: jest.fn(),
  cursorLineDown: jest.fn(),
  cursorLineUp: jest.fn(),
  selectCharLeft: jest.fn(),
  selectCharRight: jest.fn(),
  selectLineDown: jest.fn(),
  selectLineUp: jest.fn(),
  cursorLineBoundaryBackward: jest.fn(),
  cursorLineBoundaryForward: jest.fn(),
  selectLineBoundaryBackward: jest.fn(),
  selectLineBoundaryForward: jest.fn(),
  cursorDocStart: jest.fn(),
  cursorDocEnd: jest.fn(),
  cursorPageDown: jest.fn(),
  cursorPageUp: jest.fn(),
  selectPageDown: jest.fn(),
  selectPageUp: jest.fn(),
}));

jest.mock("@codemirror/search", () => ({
  openSearchPanel: jest.fn(),
  findNext: jest.fn(),
  findPrevious: jest.fn(),
}));

jest.mock("@lezer/highlight", () => {
  // `tags` is accessed as both a property bag (tags.keyword) and via nested
  // calls (tags.special(tags.string), tags.function(tags.variableName)) —
  // a self-returning callable Proxy satisfies both shapes without needing
  // to model the real tag semantics, which the component's own (also
  // mocked) HighlightStyle.define never inspects in these tests.
  const make = (): unknown => new Proxy(() => make(), { get: () => make() });
  return { tags: make() };
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** A fake CodeMirror BlockInfo/EditorView pair that reports a fixed line number. */
function fakeViewForLine(lineNumber: number) {
  return { state: { doc: { lineAt: () => ({ number: lineNumber }) } } };
}

const SMALL_CONTENT = "a\nb\nc\nd\ne";

const MODIFY_DIFF = `diff --git a/src/foo.txt b/src/foo.txt
--- a/src/foo.txt
+++ b/src/foo.txt
@@ -1,2 +1,2 @@
-old line
+new line
 context
`;

beforeEach(() => {
  jest.clearAllMocks();
  capturedGutterConfig = null;
  capturedKeymapBindings = null;
});

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("FileContentViewer — CodeMirror rendering", () => {
  it("renders CodeMirror with no gutter marks when there is no diff", async () => {
    mockUseGetFileContent.mockReturnValue({
      data: { isBinary: false, content: SMALL_CONTENT, isTruncated: false, size: BigInt(0) },
      loading: false,
      error: null,
    });

    render(<FileContentViewer sessionId="s1" filePath="src/foo.txt" baseUrl="http://x" />);

    await waitFor(() => expect(mockEditorViewCtor).toHaveBeenCalled());
    // The gutter is always constructed but reports no marks when there's no diff.
    expect(capturedGutterConfig).not.toBeNull();
    expect(capturedGutterConfig!.lineMarker(fakeViewForLine(1), { from: 0 })).toBeNull();
  });

  it("uses CodeMirror with a gutter marker when the file has a diff", async () => {
    mockUseGetFileContent.mockReturnValue({
      data: { isBinary: false, content: SMALL_CONTENT, isTruncated: false, size: BigInt(0) },
      loading: false,
      error: null,
    });

    render(
      <FileContentViewer
        sessionId="s1"
        filePath="src/foo.txt"
        baseUrl="http://x"
        diffContent={MODIFY_DIFF}
      />
    );

    await waitFor(() => expect(mockEditorViewCtor).toHaveBeenCalled());
    expect(capturedGutterConfig).not.toBeNull();

    // The gutter's lineMarker callback should mark line 1 as "modify" and
    // return null for an untouched line.
    const marker = capturedGutterConfig!.lineMarker(fakeViewForLine(1), { from: 0 });
    expect(marker).not.toBeNull();
    expect(marker!.toDOM().className).toBe("gutterMarkerModify");

    const noMark = capturedGutterConfig!.lineMarker(fakeViewForLine(3), { from: 0 });
    expect(noMark).toBeNull();
  });

  it("renders CodeMirror with no gutter marks when diffContent doesn't cover this file", async () => {
    mockUseGetFileContent.mockReturnValue({
      data: { isBinary: false, content: SMALL_CONTENT, isTruncated: false, size: BigInt(0) },
      loading: false,
      error: null,
    });

    const diffForOtherFile = MODIFY_DIFF.replace(/foo\.txt/g, "bar.txt");
    render(
      <FileContentViewer
        sessionId="s1"
        filePath="src/foo.txt"
        baseUrl="http://x"
        diffContent={diffForOtherFile}
      />
    );

    await waitFor(() => expect(mockEditorViewCtor).toHaveBeenCalled());
    // The gutter is always constructed but reports no marks for an uncovered file.
    expect(capturedGutterConfig).not.toBeNull();
    expect(capturedGutterConfig!.lineMarker(fakeViewForLine(1), { from: 0 })).toBeNull();
  });
});

describe("FileContentViewer — vim keybindings", () => {
  it("isDoubleGPress_should_ReturnTrue_When_SecondPressIsWithinTheWindow", () => {
    expect(isDoubleGPress(1000, 1000 + DOUBLE_G_WINDOW_MS - 1)).toBe(true);
    expect(isDoubleGPress(1000, 1000 + 1)).toBe(true);
  });

  it("isDoubleGPress_should_ReturnFalse_When_SecondPressIsAtOrAfterTheWindow", () => {
    expect(isDoubleGPress(1000, 1000 + DOUBLE_G_WINDOW_MS)).toBe(false);
    expect(isDoubleGPress(1000, 1000 + DOUBLE_G_WINDOW_MS + 500)).toBe(false);
  });

  it("registers the expected hjkl/gg/G/ctrl-d/ctrl-u/shift-select/yank/search vim keymap bindings", async () => {
    mockUseGetFileContent.mockReturnValue({
      data: { isBinary: false, content: SMALL_CONTENT, isTruncated: false, size: BigInt(0) },
      loading: false,
      error: null,
    });

    render(<FileContentViewer sessionId="s1" filePath="src/foo.txt" baseUrl="http://x" />);

    await waitFor(() => expect(mockEditorViewCtor).toHaveBeenCalled());
    expect(capturedKeymapBindings).not.toBeNull();

    const keys = capturedKeymapBindings!.map((b) => b.key);
    expect(keys).toEqual(
      expect.arrayContaining([
        "h", "l", "j", "k", "0", "$", "g", "G",
        "Ctrl-d", "Ctrl-u",
        "Shift-h", "Shift-l", "Shift-j", "Shift-k", "Shift-0", "Shift-$",
        "Shift-Ctrl-d", "Shift-Ctrl-u",
        "y", "/", "n", "Shift-n",
      ])
    );
  });

  it("gg_should_JumpToDocStartOnlyOnTheSecondPress_When_PressedTwiceQuickly", async () => {
    mockUseGetFileContent.mockReturnValue({
      data: { isBinary: false, content: SMALL_CONTENT, isTruncated: false, size: BigInt(0) },
      loading: false,
      error: null,
    });

    render(<FileContentViewer sessionId="s1" filePath="src/foo.txt" baseUrl="http://x" />);
    await waitFor(() => expect(mockEditorViewCtor).toHaveBeenCalled());

    const gBinding = capturedKeymapBindings!.find((b) => b.key === "g");
    expect(gBinding).toBeDefined();

    const { cursorDocStart } = jest.requireMock("@codemirror/commands") as { cursorDocStart: jest.Mock };
    cursorDocStart.mockReturnValue(true);

    const fakeView = {};

    // First "g" press: no prior press recorded, so this should be a no-op
    // (returns true without invoking cursorDocStart).
    const firstResult = gBinding!.run(fakeView);
    expect(firstResult).toBe(true);
    expect(cursorDocStart).not.toHaveBeenCalled();

    // Second "g" press immediately after: should be treated as "gg" and
    // jump to the document start.
    const secondResult = gBinding!.run(fakeView);
    expect(secondResult).toBe(true);
    expect(cursorDocStart).toHaveBeenCalledWith(fakeView);
  });
});
