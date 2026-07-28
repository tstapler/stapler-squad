/**
 * Tests the CodeMirror-vs-Shiki branching in FileContentViewer:
 *  - a file with no diff for its path uses the Shiki viewer (small-file default)
 *  - a file that appears in diffContent uses CodeMirror, with gutter markers
 *    built from buildGutterMarks reaching the gutter() line-marker callback
 */

import React from "react";
import { render, waitFor } from "@testing-library/react";
import { FileContentViewer } from "../FileContentViewer";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockUseGetFileContent = jest.fn();
jest.mock("@/lib/hooks/useFileService", () => ({
  useGetFileContent: (...args: unknown[]) => mockUseGetFileContent(...args),
}));

const mockGetSingletonHighlighter = jest.fn();
jest.mock("shiki", () => ({
  getSingletonHighlighter: (...args: unknown[]) => mockGetSingletonHighlighter(...args),
}));

interface CapturedGutterConfig {
  lineMarker: (view: unknown, block: { from: number }) => { toDOM: () => HTMLElement } | null;
}

let capturedGutterConfig: CapturedGutterConfig | null = null;
const mockEditorViewCtor = jest.fn();

jest.mock("@codemirror/view", () => {
  class GutterMarker {}
  class EditorView {
    static lineWrapping = {};
    constructor(opts: unknown) {
      mockEditorViewCtor(opts);
    }
    destroy() {}
  }
  function gutter(config: CapturedGutterConfig) {
    capturedGutterConfig = config;
    return { __gutter: true };
  }
  return { EditorView, gutter, GutterMarker };
});

jest.mock("@codemirror/state", () => ({
  EditorState: {
    readOnly: { of: (v: unknown) => ({ readOnly: v }) },
    create: (opts: unknown) => opts,
  },
}));

jest.mock("codemirror", () => ({ basicSetup: {} }));
jest.mock("@codemirror/theme-one-dark", () => ({ oneDark: {} }));

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
  mockGetSingletonHighlighter.mockResolvedValue({
    loadLanguage: jest.fn().mockResolvedValue(undefined),
    codeToHtml: jest.fn().mockReturnValue("<pre><code>mock</code></pre>"),
  });
});

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("FileContentViewer — viewer selection", () => {
  it("uses Shiki (not CodeMirror) for a small file with no diff", async () => {
    mockUseGetFileContent.mockReturnValue({
      data: { isBinary: false, content: SMALL_CONTENT, isTruncated: false, size: BigInt(0) },
      loading: false,
      error: null,
    });

    const { container } = render(
      <FileContentViewer sessionId="s1" filePath="src/foo.txt" baseUrl="http://x" />
    );

    // Shiki's highlighter promise is cached at module scope in FileContentViewer,
    // so across tests only the first Shiki render actually invokes the mock —
    // assert on rendered output instead of call count.
    await waitFor(() => expect(container.innerHTML).toContain("mock"));
    expect(mockEditorViewCtor).not.toHaveBeenCalled();
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
    expect(mockGetSingletonHighlighter).not.toHaveBeenCalled();

    // The gutter's lineMarker callback should mark line 1 as "modify" and
    // return null for an untouched line.
    expect(capturedGutterConfig).not.toBeNull();
    const marker = capturedGutterConfig!.lineMarker(fakeViewForLine(1), { from: 0 });
    expect(marker).not.toBeNull();
    expect(marker!.toDOM().className).toBe("gutterMarkerModify");

    const noMark = capturedGutterConfig!.lineMarker(fakeViewForLine(3), { from: 0 });
    expect(noMark).toBeNull();
  });

  it("uses Shiki when diffContent is present but doesn't cover this file", async () => {
    mockUseGetFileContent.mockReturnValue({
      data: { isBinary: false, content: SMALL_CONTENT, isTruncated: false, size: BigInt(0) },
      loading: false,
      error: null,
    });

    const diffForOtherFile = MODIFY_DIFF.replace(/foo\.txt/g, "bar.txt");
    const { container } = render(
      <FileContentViewer
        sessionId="s1"
        filePath="src/foo.txt"
        baseUrl="http://x"
        diffContent={diffForOtherFile}
      />
    );

    await waitFor(() => expect(container.innerHTML).toContain("mock"));
    expect(mockEditorViewCtor).not.toHaveBeenCalled();
  });
});
