import React from "react";
import { act, render, waitFor } from "@testing-library/react";
import fs from "fs";
import path from "path";
import { installJsdomLayoutShims } from "@/lib/files/__tests__/fixtures/jsdomLayout";
import { CodeEditor, type CodeEditorHandle, type EditorMode } from "../CodeEditor";
import { COARSE_MIN_PX, stepFontPx } from "@/lib/files/fontSize";

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

describe("CodeEditor on a phone", () => {
  const originalMM = window.matchMedia;
  const coarse = (on: boolean) => {
    window.matchMedia = jest.fn().mockImplementation((q: string) => ({
      matches: on && q === "(pointer: coarse)", media: q, addEventListener: jest.fn(), removeEventListener: jest.fn(),
    })) as never;
  };
  const KB = 1000;
  const bigDoc = (chars: number) => ("x".repeat(79) + "\n").repeat(Math.ceil(chars / 80));
  const fontOf = (el: HTMLElement) => getComputedStyle(el.querySelector(".cm-editor") as HTMLElement).fontSize;

  beforeEach(() => window.localStorage.clear());
  afterEach(() => { window.matchMedia = originalMM; });

  it("codeEditor_should_NotFocusOrResizeViewport_When_TapInReadModeOnCoarse", async () => {
    coarse(true);
    const m = await mount({ mode: "Read" });
    expect(m.content().getAttribute("inputmode")).toBe("none");
    expect(m.content().getAttribute("aria-readonly")).toBe("true");
    // The text stays selectable and copyable: the content is still the focusable surface, just keyboard-less.
    expect(m.content().getAttribute("contenteditable")).toBe("true");
    m.rerenderWith({ mode: "Edit" });
    await waitFor(() => expect(m.content().hasAttribute("inputmode")).toBe(false));
    m.rerenderWith({ mode: "Read" });
    await waitFor(() => expect(m.content().getAttribute("inputmode")).toBe("none"));
  });

  it("codeEditor_should_KeepInputModeOff_When_FinePointer", async () => {
    coarse(false);
    const m = await mount({ mode: "Read" });
    expect(m.content().hasAttribute("inputmode")).toBe(false);
  });

  it("largeFile_should_DegradeWithNotice_When_Coarse600Kb", async () => {
    coarse(true);
    const m = await mount({ content: bigDoc(600 * KB), language: "go" });
    expect(m.getByTestId("degrade-notice")).toHaveTextContent(/^Large file: syntax highlighting, bracket matching and folding are off/);
    expect(m.content().getAttribute("data-degraded")).toBe("true");
    expect(m.container.querySelector(".cm-foldGutter")).toBeNull();
    expect(m.container.querySelector(".cm-gutters")).not.toBeNull(); // line numbers stay
  });

  it("largeFile_should_KeepFullFeatures_When_FinePointerOrSmallFile", async () => {
    coarse(false);
    const desktop = await mount({ content: bigDoc(600 * KB), language: "go" });
    expect(desktop.queryByTestId("degrade-notice")).toBeNull();
    expect(desktop.container.querySelector(".cm-foldGutter")).not.toBeNull();
    desktop.unmount();

    coarse(true);
    const small = await mount({ content: bigDoc(20 * KB), language: "go" });
    expect(small.queryByTestId("degrade-notice")).toBeNull();
    expect(small.container.querySelector(".cm-foldGutter")).not.toBeNull();
  });

  it("largeFile_should_ScrollLongLineSideways_When_4000CharLineOnCoarse", async () => {
    coarse(true);
    const m = await mount({ content: "y".repeat(4000), wrapLines: true });
    expect(m.container.querySelector(".cm-lineWrapping")).toBeNull();
    m.unmount();

    const short = await mount({ content: "y".repeat(3999), wrapLines: true });
    expect(short.container.querySelector(".cm-lineWrapping")).not.toBeNull();
  });

  it("fontStepper_should_StepOnePxMin16Persist_When_CoarsePointer", async () => {
    coarse(true);
    const m = await mount();
    expect(fontOf(m.container)).toBe(`${COARSE_MIN_PX}px`);

    act(() => void stepFontPx(1, true));
    await waitFor(() => expect(fontOf(m.container)).toBe("17px"));
    act(() => void stepFontPx(1, true));
    await waitFor(() => expect(fontOf(m.container)).toBe("18px"));
    expect(window.localStorage.getItem("ssq.files.fontPx")).toBe("18");

    act(() => void stepFontPx(-1, true));
    act(() => void stepFontPx(-1, true));
    act(() => void stepFontPx(-1, true));
    await waitFor(() => expect(fontOf(m.container)).toBe(`${COARSE_MIN_PX}px`));
    expect(window.localStorage.getItem("ssq.files.fontPx")).toBe("16");

    // A new editor on the same device starts from the persisted size.
    act(() => void stepFontPx(1, true));
    m.unmount();
    const again = await mount();
    expect(fontOf(again.container)).toBe("17px");
  });
});
