import { Terminal } from "@xterm/xterm";
import { attachAndroidImeInput, compositionDelta } from "../androidImeInput";

const flush = () => new Promise((r) => setTimeout(r, 5));

/** Replays the event sequence Android Gboard emits for a composing word (keydown 229, no per-key keys). */
function gboard(ta: HTMLTextAreaElement) {
  let composing = false;
  let start = 0;
  const keydown = (key = "Unidentified", keyCode = 229) =>
    ta.dispatchEvent(new KeyboardEvent("keydown", { key, keyCode, bubbles: true, cancelable: true } as KeyboardEventInit));
  const keyup = () => ta.dispatchEvent(new KeyboardEvent("keyup", { key: "Unidentified", keyCode: 229, bubbles: true }));
  return {
    /** Composition text is the word so far, replacing the previous composition in the textarea. */
    compose(word: string, over = 0) {
      keydown();
      if (!composing) {
        composing = true;
        start = ta.value.length - over; // `over`: chars of existing text Gboard re-composes
        ta.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
      }
      ta.dispatchEvent(new CompositionEvent("compositionupdate", { data: word, bubbles: true }));
      ta.value = ta.value.slice(0, start) + word;
      ta.dispatchEvent(new InputEvent("input", { data: word, inputType: "insertCompositionText", bubbles: true, composed: true }));
      keyup();
    },
    commit(word: string) {
      ta.value = ta.value.slice(0, start) + word;
      ta.dispatchEvent(new CompositionEvent("compositionend", { data: word, bubbles: true }));
      composing = false;
    },
    insertText(text: string, inputType = "insertText") {
      keydown();
      ta.dispatchEvent(new InputEvent("beforeinput", { data: text, inputType, bubbles: true, cancelable: true, composed: true }));
      ta.value += text;
      ta.dispatchEvent(new InputEvent("input", { data: text, inputType: "insertText", bubbles: true, composed: true }));
      keyup();
    },
    backspace() {
      keydown();
      ta.dispatchEvent(new InputEvent("beforeinput", { inputType: "deleteContentBackward", bubbles: true, cancelable: true, composed: true }));
      ta.value = ta.value.slice(0, -1);
      ta.dispatchEvent(new InputEvent("input", { inputType: "deleteContentBackward", bubbles: true, composed: true }));
      keyup();
    },
  };
}

function setup(attach: boolean, pasted: string[] = []) {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const term = new Terminal();
  term.open(host);
  const sent: string[] = [];
  term.onData((d) => sent.push(d));
  const appSent: string[] = [];
  const dispose = attach ? attachAndroidImeInput(term.element as HTMLElement, (d) => appSent.push(d), (t) => pasted.push(t)) : () => {};
  const ta = term.textarea as HTMLTextAreaElement;
  return { term, ta, sent, appSent, dispose, ime: gboard(ta), host };
}

describe("stock xterm with Gboard composition (documents the bug)", () => {
  it("emits nothing while a word is still composing", async () => {
    const { ime, sent, term } = setup(false);
    ime.compose("h");
    await flush();
    ime.compose("hi");
    await flush();
    expect(sent).toEqual([]);
    term.dispose();
  });
});

describe("attachAndroidImeInput", () => {
  it("streams composing letters immediately, one delta per update", async () => {
    const { ime, sent, appSent, term } = setup(true);
    ime.compose("h");
    await flush();
    ime.compose("hi");
    await flush();
    expect(appSent.join("")).toBe("hi");
    expect(sent).toEqual([]); // xterm's own composition/diff paths are fully bypassed (no double send)
    term.dispose();
  });

  it("does not resend text at compositionend", async () => {
    const { ime, appSent, term } = setup(true);
    ime.compose("h");
    ime.compose("hi");
    ime.commit("hi");
    await flush();
    expect(appSent.join("")).toBe("hi");
    term.dispose();
  });

  it("sends the correction when autocorrect rewrites the composing word", async () => {
    const { ime, appSent, term } = setup(true);
    ime.compose("teh");
    ime.commit("the");
    await flush();
    // 'e','h' replaced by 'h','e': DEL x2 then "he"
    expect(appSent.join("")).toBe("teh\x7f\x7fhe");
    term.dispose();
  });

  it("handles consecutive words without carrying stale state", async () => {
    const { ime, appSent, term } = setup(true);
    ime.compose("ab");
    ime.commit("ab");
    ime.insertText(" ");
    ime.compose("cd");
    ime.commit("cd");
    await flush();
    expect(appSent.join("")).toBe("ab cd");
    term.dispose();
  });

  it("sends non-composing insertText and backspace", async () => {
    const { ime, ta, appSent, sent, term } = setup(true);
    ta.value = "";
    ime.insertText("x");
    ime.backspace();
    await flush();
    expect(appSent.join("")).toBe("x\x7f");
    expect(sent).toEqual([]);
    term.dispose();
  });

  it("leaves real keys (Enter/Tab, keyCode != 229) to xterm", async () => {
    const { ta, sent, appSent, term } = setup(true);
    ta.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", keyCode: 13, bubbles: true, cancelable: true } as KeyboardEventInit));
    await flush();
    expect(sent).toEqual(["\r"]);
    expect(appSent).toEqual([]);
    term.dispose();
  });

  it("stops intercepting after dispose", async () => {
    const { ime, appSent, dispose, term } = setup(true);
    dispose();
    ime.compose("h");
    await flush();
    expect(appSent).toEqual([]);
    term.dispose();
  });
});

describe("attachAndroidImeInput re-composition over committed text", () => {
  it("does not resend a committed word when a suggestion tap re-composes it", async () => {
    const { ime, appSent, term } = setup(true);
    ime.compose("wor");
    ime.commit("wor");
    ime.compose("wor", 3);
    ime.compose("word", 3);
    ime.commit("word");
    await flush();
    expect(appSent.join("")).toBe("word");
    term.dispose();
  });

  it("does not resend the remaining word after backspacing into it", async () => {
    const { ime, appSent, term } = setup(true);
    ime.compose("worx");
    ime.commit("worx");
    ime.backspace();
    ime.compose("wor", 3);
    ime.compose("worl", 3);
    ime.commit("worl");
    await flush();
    expect(appSent.join("")).toBe("worx\x7fl");
    term.dispose();
  });

  it("still types a new word after a space as a fresh composition", async () => {
    const { ime, appSent, term } = setup(true);
    ime.compose("wor");
    ime.commit("wor");
    ime.insertText(" ");
    ime.compose("wor");
    ime.commit("wor");
    await flush();
    expect(appSent.join("")).toBe("wor wor");
    term.dispose();
  });
});

describe("attachAndroidImeInput paste routing", () => {
  it("routes multi-line insertText through paste, never raw send", async () => {
    const pasted: string[] = [];
    const { ime, appSent, sent, term } = setup(true, pasted);
    ime.insertText("ls\nrm -rf x");
    await flush();
    expect(pasted).toEqual(["ls\nrm -rf x"]);
    expect(appSent).toEqual([]);
    expect(sent).toEqual([]);
    term.dispose();
  });

  it("routes multi-character insertText and insertFromPaste through paste", async () => {
    const pasted: string[] = [];
    const { ime, appSent, term } = setup(true, pasted);
    ime.insertText("hello world");
    ime.insertText("clip", "insertFromPaste");
    await flush();
    expect(pasted).toEqual(["hello world", "clip"]);
    expect(appSent).toEqual([]);
    term.dispose();
  });

  it("sends a single astral character (emoji) as typed text", async () => {
    const pasted: string[] = [];
    const { ime, appSent, term } = setup(true, pasted);
    ime.insertText("\u{1F600}");
    await flush();
    expect(appSent).toEqual(["\u{1F600}"]);
    expect(pasted).toEqual([]);
    term.dispose();
  });

  it("treats insertReplacementText as a correction of the current word", async () => {
    const { ime, appSent, term } = setup(true);
    ime.compose("teh");
    ime.commit("teh");
    ime.insertText("the", "insertReplacementText");
    await flush();
    expect(appSent.join("")).toBe("teh\x7f\x7fhe");
    term.dispose();
  });
});

describe("compositionDelta", () => {
  it("compares by code point so surrogate pairs are never split", () => {
    expect(compositionDelta("a\u{1F600}", "a\u{1F601}")).toBe("\x7f\u{1F601}");
    expect(compositionDelta("\u{1F600}", "\u{1F600}b")).toBe("b");
  });
});
