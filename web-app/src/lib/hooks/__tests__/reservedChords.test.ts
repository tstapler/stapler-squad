import { FILES_CHORDS, chordSignature, matchChord, type FilesChord } from "../filesChords";

type Spec = Pick<FilesChord, "mod" | "shift" | "alt" | "code" | "key">;

// ux.md 14.1 "Never registered".
const DENYLIST: Spec[] = [
  ...["w", "t", "n"].flatMap((k): Spec[] => [{ mod: true, key: k }, { mod: true, shift: true, key: k }]),
  { mod: true, key: "Tab" }, { mod: true, shift: true, key: "Tab" },
  { mod: true, key: "PageUp" }, { mod: true, key: "PageDown" },
  ...Array.from({ length: 9 }, (_, i): Spec => ({ mod: true, key: String(i + 1) })),
  { alt: true, key: "ArrowLeft" }, { alt: true, key: "ArrowRight" }, { alt: true, key: "Home" },
  ...["l", "k", "e", "d", "r", "q"].map((k): Spec => ({ mod: true, key: k })),
  ...["F5", "F6", "F11", "F12"].map((k): Spec => ({ key: k })),
  ...["i", "j", "c", "m", "Delete"].map((k): Spec => ({ mod: true, shift: true, key: k })),
];

const key = (init: KeyboardEventInit) => new KeyboardEvent("keydown", { cancelable: true, ...init });

describe("reservedChords", () => {
  it("reservedChords_should_NotOverlapDenylist_And_EveryChordHaveVisibleEquivalent", () => {
    const denied = new Set(DENYLIST.map(chordSignature));
    const overlaps = FILES_CHORDS.filter((c) => denied.has(chordSignature(c))).map(chordSignature);
    expect(overlaps).toEqual([]);
    const signatures = FILES_CHORDS.map(chordSignature);
    expect(new Set(signatures).size).toBe(signatures.length);
    for (const c of FILES_CHORDS) expect(c.equivalent.trim().length).toBeGreaterThan(10);
  });

  it("reservedChords_should_FailTheCheck_When_ADeniedChordIsRegistered", () => {
    const denied = new Set(DENYLIST.map(chordSignature));
    expect(denied.has(chordSignature({ mod: true, key: "w" }))).toBe(true);
    expect(denied.has(chordSignature({ alt: true, key: "ArrowLeft" }))).toBe(true);
    expect(denied.has(chordSignature({ mod: true, shift: true, key: "m" }))).toBe(true);
  });

  it("chordMatcher_should_IgnoreAltGrComposition_And_MatchAltBracketByCode", () => {
    expect(matchChord(key({ altKey: true, code: "BracketLeft", key: "[" }))?.id).toBe("back");
    // Option+[ types a curly quote on macOS; the physical key still matches.
    expect(matchChord(key({ altKey: true, code: "BracketLeft", key: "“" }))?.id).toBe("back");
    expect(matchChord(key({ altKey: true, code: "BracketRight", key: "]" }))?.id).toBe("forward");
    expect(matchChord(key({ altKey: true, code: "KeyW", key: "w" }))?.id).toBe("closeTab");
    // AltGr+8 types "[" on a German layout: Ctrl and Alt both set.
    expect(matchChord(key({ altKey: true, ctrlKey: true, code: "Digit8", key: "[" }))).toBeNull();
    expect(matchChord(key({ altKey: true, ctrlKey: true, code: "BracketLeft", key: "[" }))).toBeNull();
    const altGraph = key({ altKey: true, code: "BracketLeft", key: "[" });
    Object.defineProperty(altGraph, "getModifierState", { value: (m: string) => m === "AltGraph" });
    expect(matchChord(altGraph)).toBeNull();
  });

  it("chordMatcher_should_MatchModifierChordsOnlyWithTheirExactModifiers", () => {
    expect(matchChord(key({ ctrlKey: true, key: "p" }))?.id).toBe("quickOpen");
    expect(matchChord(key({ metaKey: true, key: "P" }))?.id).toBe("quickOpen");
    expect(matchChord(key({ ctrlKey: true, shiftKey: true, key: "F" }))?.id).toBe("search");
    expect(matchChord(key({ ctrlKey: true, key: "f" }))?.id).toBe("treeFilter");
    expect(matchChord(key({ key: "F4" }))?.id).toBe("nextHit");
    expect(matchChord(key({ shiftKey: true, key: "F4" }))?.id).toBe("prevHit");
    expect(matchChord(key({ ctrlKey: true, key: "w" }))).toBeNull();
    expect(matchChord(key({ altKey: true, key: "ArrowLeft", code: "ArrowLeft" }))).toBeNull();
    expect(matchChord(key({ key: "p" }))).toBeNull();
  });
});
