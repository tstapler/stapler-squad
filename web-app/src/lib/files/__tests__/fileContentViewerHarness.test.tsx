import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { createFilesTestHarness } from "./fixtures";
import { fakeCodeEditorProps } from "./fixtures/mockCodeEditor";
import { FileContentViewer } from "@/components/sessions/FileContentViewer";

describe("filesHarness + fake CodeEditor", () => {
  it("fileContentViewer_should_RenderTheFakeEditorInReadMode_When_TheFakeTransportServesText", async () => {
    const harness = createFilesTestHarness({
      handlers: {
        getFileContent: () => ({ content: "hello", isBinary: false, isTruncated: false, size: 5n }),
      },
    });
    render(<FileContentViewer sessionId={harness.sessionId} filePath="src/a.txt" baseUrl="http://x" />, {
      wrapper: harness.wrapper,
    });
    const editor = await screen.findByTestId("fake-code-editor");
    expect(editor.textContent).toBe("hello");
    await waitFor(() => expect(harness.calls).toContain("getFileContent"));
    expect(fakeCodeEditorProps.last).toMatchObject({ mode: "Read", path: "src/a.txt", language: "text" });
  });
});
