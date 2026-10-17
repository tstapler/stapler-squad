import React from "react";
import { act, render, waitFor } from "@testing-library/react";
import fs from "fs";
import path from "path";
import { installJsdomLayoutShims } from "@/lib/files/__tests__/fixtures/jsdomLayout";
import { CodeEditor, type CodeEditorHandle, type EditorMode } from "../CodeEditor";

const lines = (n: number) => Array.from({ length: n }, (_, i) => `line ${i + 1}`).join("\n");

async function mount(props: Partial<React.ComponentProps<typeof CodeEditor>> = {}) {
  let handle: CodeEditorHandle | null = null;
  const ui = (p: Partial<React.ComponentProps<typeof CodeEditor>>) => (
    <CodeEditor content="alpha" language="text" path="src/a.txt" onReady={(h) => (handle = h)} {...props} {...p} />
  );
  const utils = render(ui({}));
  await waitFor(() => expect(handle).not.toBeNull());
  return {
    ...utils,
    handle: () => handle as unknown as CodeEditorHandle,
    rerenderWith: (p: Partial<React.ComponentProps<typeof CodeEditor>>) => utils.rerender(ui(p)),
    content: () => utils.container.querySelector<HTMLElement>(".cm-content")!,
  };
}

installJsdomLayoutShims();

describe("CodeEditor", () => {
  it("codeEditor_should_ReconfigureReadOnlyWithoutRecreatingView_When_ModeSwitchesToEdit", async () => {
    const m = await mount({ mode: "Read" as EditorMode });
    const editorEl = m.container.querySelector(".cm-editor");
    expect(m.handle().insertText("x")).toBe(false);
    expect(m.handle().getText()).toBe("alpha");

    m.rerenderWith({ mode: "Edit" });
    await waitFor(() => expect(m.handle().insertText("x")).toBe(true));
    expect(m.handle().getText()).toMatch(/x/);
    expect(m.container.querySelector(".cm-editor")).toBe(editorEl);

    m.rerenderWith({ mode: "Read" });
    await waitFor(() => expect(m.handle().insertText("y")).toBe(false));
  });

  it("codeEditor_should_PutCursorAtStartOfLine_When_GotoLineCalled", async () => {
    const m = await mount({ content: lines(400) });
    act(() => m.handle().gotoLine(250));
    expect(m.handle().getCursor()).toEqual({ line: 250, col: 0 });
    act(() => m.handle().gotoLine(9999));
    expect(m.handle().getCursor().line).toBe(400);
  });

  it("codeEditor_should_KeepEditsAndExtensions_When_BufferStateSwapped", async () => {
    const m = await mount({ mode: "Edit" });
    const first = m.handle().getState();
    const second = await m.handle().createBufferState({ doc: "beta\ngamma", language: "text" });
    act(() => m.handle().setBufferState(second));
    expect(m.handle().getText()).toBe("beta\ngamma");
    expect(m.content().getAttribute("aria-label")).toBe("File: src/a.txt");
    expect(m.container.querySelector(".cm-gutters")).not.toBeNull();
    m.handle().insertText("Z");
    const editedSecond = m.handle().getState();
    act(() => m.handle().setBufferState(first));
    expect(m.handle().getText()).toBe("alpha");
    act(() => m.handle().setBufferState(editedSecond));
    expect(m.handle().getText()).toMatch(/Z/);
  });

  it("codeEditor_should_SetAccessibleNameReadonlyAndHostileInputAttributes_When_Mounted", async () => {
    const m = await mount({ mode: "Read" });
    const el = m.content();
    expect(el.getAttribute("aria-label")).toBe("File: src/a.txt");
    expect(el.getAttribute("aria-readonly")).toBe("true");
    expect(el.getAttribute("autocorrect")).toBe("off");
    expect(el.getAttribute("autocapitalize")).toBe("off");
    expect(el.getAttribute("spellcheck")).toBe("false");
    m.rerenderWith({ mode: "Edit" });
    await waitFor(() => expect(m.content().getAttribute("aria-readonly")).toBe("false"));
  });

  it("codeEditor_should_NotDispatchModeChange_When_Composing_And_ApplyItAfterCompositionEnd", async () => {
    const m = await mount({ mode: "Read" });
    act(() => void m.content().dispatchEvent(new Event("compositionstart", { bubbles: true })));
    m.rerenderWith({ mode: "Edit" });
    expect(m.handle().insertText("x")).toBe(false);
    act(() => void m.content().dispatchEvent(new Event("compositionend", { bubbles: true })));
    await waitFor(() => expect(m.handle().insertText("x")).toBe(true));
  });

  it("codeEditor_should_RebuildBuffer_When_ReadOnlyContentChanges_And_IgnoreItInEditMode", async () => {
    const m = await mount({ mode: "Read" });
    m.rerenderWith({ content: "next" });
    await waitFor(() => expect(m.handle().getText()).toBe("next"));
    m.rerenderWith({ mode: "Edit" });
    await waitFor(() => expect(m.handle().insertText("!")).toBe(true));
    m.rerenderWith({ mode: "Edit", content: "from props" });
    expect(m.handle().getText()).toMatch(/!/);
    expect(m.handle().getText()).not.toBe("from props");
  });

  it("codeEditor_should_StayUnder350Lines", () => {
    const source = fs.readFileSync(path.resolve(__dirname, "../CodeEditor.tsx"), "utf8");
    expect(source.split("\n").length).toBeLessThanOrEqual(350);
  });
});
