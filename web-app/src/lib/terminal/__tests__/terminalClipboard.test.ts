import { bufferPlainText, createClipboardActions, type TerminalClipboardSurface } from "../terminalClipboard";

const flush = () => new Promise<void>((r) => setTimeout(r, 0));

const surfaceStub = (): TerminalClipboardSurface => ({
  getSelection: () => "",
  clearSelection: () => undefined,
  selectAll: () => undefined,
  paste: () => undefined,
  plainText: () => "",
  hasMouseTracking: () => false,
});

function setup(over: Partial<TerminalClipboardSurface> = {}, env: { copyOk?: boolean; clip?: string } = {}) {
  const log = { copied: [] as string[], pasted: [] as string[], cleared: 0, reports: [] as boolean[], prevented: 0 };
  const surface: TerminalClipboardSurface = {
    getSelection: () => "selected",
    clearSelection: () => void log.cleared++,
    selectAll: () => undefined,
    paste: (t) => void log.pasted.push(t),
    plainText: () => "whole buffer",
    hasMouseTracking: () => false,
    ...over,
  };
  const actions = createClipboardActions(() => surface, {
    copy: async (t) => (log.copied.push(t), env.copyOk ?? true),
    readText: async () => env.clip ?? "clipboard text",
    report: (ok) => void log.reports.push(ok),
  });
  const key = (e: Partial<Parameters<typeof actions.interceptCopyKey>[0]> = {}) =>
    actions.interceptCopyKey({ type: "keydown", key: "c", ctrlKey: true, metaKey: false, altKey: false, preventDefault: () => void log.prevented++, ...e });
  return { actions, log, key };
}

// Every renderer adapts to this one contract; these are the rules both must obey.
describe("terminal clipboard contract", () => {
  it("copies the selection and reports the outcome", async () => {
    const { actions, log } = setup();
    expect(actions.copySelection()).toBe(true);
    await flush();
    expect(log).toMatchObject({ copied: ["selected"], reports: [true] });
  });

  it("reports a failed copy", async () => {
    const { actions, log } = setup({}, { copyOk: false });
    actions.copySelection();
    await flush();
    expect(log.reports).toEqual([false]);
  });

  it("copies nothing, and says so, when nothing is selected", () => {
    const { actions, log } = setup({ getSelection: () => "" });
    expect(actions.copySelection()).toBe(false);
    expect(log.copied).toEqual([]);
  });

  it("copies the whole buffer for Copy all", async () => {
    const { actions, log } = setup();
    expect(actions.copyAll()).toBe(true);
    await flush();
    expect(log.copied).toEqual(["whole buffer"]);
  });

  describe("Ctrl/Cmd+C", () => {
    it("with a selection: copies, clears and is consumed so the app never sees ^C", async () => {
      const { key, log } = setup();
      expect(key()).toBe(true);
      await flush();
      expect(log).toMatchObject({ copied: ["selected"], cleared: 1, prevented: 1, reports: [true] });
    });

    it.each([
      ["Ctrl+Shift+C", { key: "C" }],
      ["Cmd+C", { ctrlKey: false, metaKey: true }],
    ])("also copies on %s", (_n, over) => {
      const { key, log } = setup();
      expect(key(over)).toBe(true);
      expect(log.copied).toEqual(["selected"]);
    });

    it("without a selection: passes through so Ctrl+C still interrupts", () => {
      const { key, log } = setup({ getSelection: () => "" });
      expect(key()).toBe(false);
      expect(log).toMatchObject({ copied: [], cleared: 0, prevented: 0 });
    });

    it("ignores other keys, key-up and Alt combos", () => {
      const { key, log } = setup();
      expect(key({ key: "v" })).toBe(false);
      expect(key({ type: "keyup" })).toBe(false);
      expect(key({ altKey: true })).toBe(false);
      expect(log.copied).toEqual([]);
    });
  });

  it("pastes through the renderer's bracketed-aware paste, never raw input", async () => {
    const { actions, log } = setup({}, { clip: "line1\nline2" });
    await expect(actions.paste()).resolves.toBe("pasted");
    expect(log.pasted).toEqual(["line1\nline2"]);
  });

  it("reports an empty clipboard and a blocked one differently, pasting neither", async () => {
    const empty = setup({}, { clip: "" });
    await expect(empty.actions.paste()).resolves.toBe("empty");
    expect(empty.log.pasted).toEqual([]);

    const log = { pasted: [] as unknown[] };
    const denied = createClipboardActions(() => ({ ...surfaceStub(), paste: () => log.pasted.push("never") }), {
      copy: async () => true,
      readText: async () => Promise.reject(new Error("denied")),
      report: () => undefined,
    });
    await expect(denied.paste()).resolves.toBe("blocked");
    expect(log.pasted).toEqual([]);
  });

  it("offers the menu only while the app is not tracking the mouse", () => {
    expect(setup({ hasMouseTracking: () => false }).actions.canOpenMenu()).toBe(true);
    expect(setup({ hasMouseTracking: () => true }).actions.canOpenMenu()).toBe(false);
  });

  it("is inert when the terminal is not ready", async () => {
    const actions = createClipboardActions(() => null, { copy: async () => true, readText: async () => "x", report: () => undefined });
    expect(actions.copySelection()).toBe(false);
    expect(actions.copyAll()).toBe(false);
    expect(actions.canOpenMenu()).toBe(true);
    await expect(actions.paste()).resolves.toBe("pasted");
  });
});

describe("bufferPlainText", () => {
  const rows = ["one  ", "two", undefined, "three   ", "", "  "];
  const term = {
    buffer: {
      active: {
        length: rows.length,
        getLine: (y: number) => (rows[y] === undefined ? undefined : { translateToString: (trim: boolean) => (trim ? (rows[y] as string).trimEnd() : (rows[y] as string)) }),
      },
    },
  };

  it("joins rows, skips missing ones and trims trailing blank rows", () => {
    expect(bufferPlainText(term)).toBe("one\ntwo\nthree");
  });
});
