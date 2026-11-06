/**
 * Story 0.1.5 / Task 0.1.5c: detector contract over recorded key-event fixtures (ADR-033 S1-S4).
 *
 * `detectHardwareKeyboard` does not exist yet (it lands in Task 2.4.1d), so the rule is encoded
 * once here as `referenceDetect`, straight from ADR-033, and the fixtures are checked against it.
 * Task 2.4.1d replaces `referenceDetect` with the real import.
 *
 * FIXTURES ARE SYNTHETIC PLACEHOLDERS. Shapes follow what Gboard, iOS and a Bluetooth keyboard
 * are expected to send, not what the owner's phone sent. OWNER-GATED: replace them with the JSON
 * exported from the spike page's capture panel (manual-verification-checklist.md, Task 0.1.5b).
 */

interface KeyFixture {
  key: string;
  code: string;
  keyCode: number;
  isComposing: boolean;
  isTrusted: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
}

interface DetectEnv {
  primaryPointerFineHover: boolean;
  /** S4: focus tap on a coarse device where visualViewport.height did not shrink by >= 100 px in 500 ms. */
  coarseFocusWithoutKeyboardShrink?: boolean;
}

const k = (key: string, code: string, keyCode: number, extra: Partial<KeyFixture> = {}): KeyFixture => ({
  key, code, keyCode, isComposing: false, isTrusted: true, ...extra,
});
const letter = (l: string): KeyFixture => k(l, `Key${l.toUpperCase()}`, l.toUpperCase().charCodeAt(0));
const soft = (key: string): KeyFixture => k(key, "", 229);

const STRONG_CODES = /^(Escape|F([1-9]|1[0-2])|Insert|Home|End|PageUp|PageDown|Arrow(Up|Down|Left|Right))$/;
const MODIFIERS = new Set(["Control", "Meta", "Shift", "Alt"]);

function isStrongLetter(e: KeyFixture): boolean {
  return /^Key[A-Z]$/.test(e.code) && e.key.toUpperCase() === e.code.slice(3) && e.keyCode !== 229 && !e.isComposing;
}

/** Returns the index of the keydown that flips to HardwareKeyboard, or -1 for Touch. */
function referenceDetect(events: KeyFixture[], env: DetectEnv): number {
  if (env.primaryPointerFineHover) return 0;
  let letters = 0;
  for (let i = 0; i < events.length; i++) {
    const e = events[i];
    if (!e.isTrusted) continue;
    if ((e.ctrlKey || e.metaKey) && !MODIFIERS.has(e.key)) return i;
    if (e.code !== "" && (STRONG_CODES.test(e.code) || STRONG_CODES.test(e.key))) return i;
    if (isStrongLetter(e)) {
      letters++;
      if (letters >= 3 || (env.coarseFocusWithoutKeyboardShrink && letters >= 1)) return i;
    }
  }
  return -1;
}

const COARSE: DetectEnv = { primaryPointerFineHover: false };

const SOFT_KEYBOARD_FIXTURES: Record<string, KeyFixture[]> = {
  "gboard sentence with Enter, Backspace, Space, digits, punctuation": [
    soft("h"), soft("i"), k(" ", "", 229), soft("1"), soft("2"), soft("."), soft(","),
    k("Backspace", "", 229), k("Enter", "", 229), k("Tab", "", 229),
  ],
  "ios safari: letters report key with empty code and keyCode 0": [
    k("h", "", 0), k("e", "", 0), k("y", "", 0), k(" ", "", 0), k("Backspace", "", 0), k("Enter", "", 0),
  ],
  "ime composition": [
    { key: "Process", code: "", keyCode: 229, isComposing: true, isTrusted: true },
    { key: "Process", code: "", keyCode: 229, isComposing: true, isTrusted: true },
  ],
  "emoji picker": [soft("\u{1F600}")],
  "synthetic key-bar events are untrusted": [
    k("Escape", "Escape", 27, { isTrusted: false }),
    k("ArrowLeft", "ArrowLeft", 37, { isTrusted: false }),
    k("a", "KeyA", 65, { isTrusted: false }), k("b", "KeyB", 66, { isTrusted: false }), k("c", "KeyC", 67, { isTrusted: false }),
  ],
};

const BLUETOOTH_FIXTURES: Record<string, { events: KeyFixture[]; flipAt: number }> = {
  "escape first": { events: [k("Escape", "Escape", 27), letter("a")], flipAt: 0 },
  "arrow first": { events: [k("ArrowDown", "ArrowDown", 40)], flipAt: 0 },
  "function key": { events: [k("F5", "F5", 116)], flipAt: 0 },
  "ctrl chord": { events: [k("Control", "ControlLeft", 17), k("s", "KeyS", 83, { ctrlKey: true })], flipAt: 1 },
  "three matching letters": { events: [letter("h"), letter("e"), letter("l"), letter("l")], flipAt: 2 },
};

describe("input profile detector contract (synthetic fixtures, OWNER-GATED real captures)", () => {
  it("inputProfile_should_StayTouchWithZeroFlips_When_SoftKeyboardFixturesReplayed", () => {
    for (const [name, events] of Object.entries(SOFT_KEYBOARD_FIXTURES)) {
      expect({ name, flip: referenceDetect(events, COARSE) }).toEqual({ name, flip: -1 });
    }
  });

  it("inputProfile_should_BeHardwareKeyboard_When_PrimaryFinePointerChordOrStrongKeyOrThreeMatchingLetters", () => {
    expect(referenceDetect([], { primaryPointerFineHover: true })).toBe(0);
    for (const [name, { events, flipAt }] of Object.entries(BLUETOOTH_FIXTURES)) {
      expect({ name, flip: referenceDetect(events, COARSE) }).toEqual({ name, flip: flipAt });
    }
    // S4 alone never flips; with one strong letter it does.
    const s4: DetectEnv = { primaryPointerFineHover: false, coarseFocusWithoutKeyboardShrink: true };
    expect(referenceDetect([], s4)).toBe(-1);
    expect(referenceDetect([letter("q")], s4)).toBe(0);
  });

  it.skip("inputProfile_should_ReplayOwnerCapturedFixtures_When_ExportedFromPhone [OWNER-GATED: needs phone export]", () => {
    // OWNER-GATED: paste the capture-panel JSON from the soft-keyboard and Bluetooth runs here.
    expect(Object.keys(SOFT_KEYBOARD_FIXTURES).length).toBeGreaterThan(0);
  });
});
