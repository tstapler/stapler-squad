// The Files tab's registered chords (ux.md 14.1). One table drives the matcher, the shortcut overlay and
// the reservedChords test, so a chord cannot be documented without being matched, or the reverse.

export type ChordId =
  | "quickOpen" | "search" | "gotoLine" | "treeFilter" | "back" | "forward" | "closeTab" | "save" | "nextHit" | "prevHit";

export interface FilesChord {
  id: ChordId;
  /** Overlay text. */
  label: string;
  /** Ctrl on Windows and Linux, Cmd on macOS. */
  mod?: boolean;
  shift?: boolean;
  alt?: boolean;
  /** Matched by the physical key, so Option on macOS and non-US layouts reach the same key. */
  code?: "BracketLeft" | "BracketRight" | "KeyW";
  /** Matched by the produced character (lower-cased) or key name. */
  key?: string;
  /** The visible button or menu route that always exists (U-CHORD-1). */
  equivalent: string;
}

export const FILES_CHORDS: readonly FilesChord[] = [
  { id: "quickOpen", label: "Quick open", mod: true, key: "p", equivalent: "Search files button (data-testid files-quick-open)" },
  { id: "search", label: "Search in files", mod: true, shift: true, key: "f", equivalent: "Search tab in the Explorer | Search header" },
  { id: "gotoLine", label: "Go to line", mod: true, key: "g", equivalent: "Go to line button in the navigation row" },
  { id: "treeFilter", label: "Filter the file tree", mod: true, key: "f", equivalent: "File tree filter field" },
  { id: "back", label: "Jumplist back", alt: true, code: "BracketLeft", equivalent: "Back button in the navigation row" },
  { id: "forward", label: "Jumplist forward", alt: true, code: "BracketRight", equivalent: "Forward button in the navigation row" },
  { id: "closeTab", label: "Close tab", alt: true, code: "KeyW", equivalent: "Close button on the tab" },
  { id: "save", label: "Save", mod: true, key: "s", equivalent: "Save button in the status bar" },
  { id: "nextHit", label: "Next search hit", key: "F4", equivalent: "Next hit chevron on the quickfix chip" },
  { id: "prevHit", label: "Previous search hit", shift: true, key: "F4", equivalent: "Previous hit chevron on the quickfix chip" },
];

/** AltGr is reported as Ctrl+Alt on many Windows layouts; a character typed with it is never a chord. */
const isAltGr = (e: KeyboardEvent) => (e.altKey && e.ctrlKey) || (typeof e.getModifierState === "function" && e.getModifierState("AltGraph"));

/** The chord a keydown is, or null; the caller decides whether it is allowed to act on it. */
export function matchChord(e: KeyboardEvent): FilesChord | null {
  if (isAltGr(e)) return null;
  const mod = e.ctrlKey || e.metaKey;
  const key = e.key?.toLowerCase();
  return (
    FILES_CHORDS.find((c) => {
      if (!!c.mod !== mod || !!c.alt !== e.altKey || !!c.shift !== e.shiftKey) return false;
      if (c.code) return e.code === c.code;
      return key === c.key?.toLowerCase();
    }) ?? null
  );
}

/** Canonical form used to compare a chord against the denylist, e.g. `Mod+Shift+F`. */
export function chordSignature(c: Pick<FilesChord, "mod" | "shift" | "alt" | "code" | "key">): string {
  const keyName = c.code ?? (c.key ?? "").toUpperCase();
  return [c.mod && "Mod", c.alt && "Alt", c.shift && "Shift", keyName].filter(Boolean).join("+");
}
