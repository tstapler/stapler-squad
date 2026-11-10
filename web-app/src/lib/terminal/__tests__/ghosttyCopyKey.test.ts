import { createCopyKeyHandler } from "../ghosttyCopyKey";

function setup(selection: string, copyOk = true) {
  const calls = { copied: [] as string[], cleared: 0, results: [] as boolean[], prevented: 0 };
  const handle = createCopyKeyHandler({
    getSelection: () => selection,
    clearSelection: () => void calls.cleared++,
    copy: async (t) => (calls.copied.push(t), copyOk),
    onCopied: (ok) => calls.results.push(ok),
  });
  const key = (over: Partial<Parameters<typeof handle>[0]> = {}) =>
    handle({ type: "keydown", key: "c", ctrlKey: true, metaKey: false, altKey: false, preventDefault: () => void calls.prevented++, ...over });
  return { calls, key };
}

const flush = () => new Promise<void>((r) => setTimeout(r, 0));

describe("ghostty copy key", () => {
  it("copies and consumes Ctrl+C when text is selected, so ^C never reaches the app", async () => {
    const { calls, key } = setup("hello");
    expect(key()).toBe(true);
    await flush();
    expect(calls).toMatchObject({ copied: ["hello"], cleared: 1, results: [true], prevented: 1 });
  });

  it.each([
    ["Ctrl+Shift+C", { key: "C" }],
    ["Cmd+C", { ctrlKey: false, metaKey: true }],
  ])("also copies on %s", (_name, over) => {
    const { calls, key } = setup("x");
    expect(key(over)).toBe(true);
    expect(calls.copied).toEqual(["x"]);
  });

  it("lets Ctrl+C through to interrupt the app when nothing is selected", () => {
    const { calls, key } = setup("");
    expect(key()).toBe(false);
    expect(calls).toMatchObject({ copied: [], cleared: 0, prevented: 0 });
  });

  it("ignores other keys, key-up events and Alt combos", () => {
    const { calls, key } = setup("hello");
    expect(key({ key: "v" })).toBe(false);
    expect(key({ type: "keyup" })).toBe(false);
    expect(key({ altKey: true })).toBe(false);
    expect(calls.copied).toEqual([]);
  });

  it("reports a failed copy", async () => {
    const { calls, key } = setup("hello", false);
    key();
    await flush();
    expect(calls.results).toEqual([false]);
  });
});
