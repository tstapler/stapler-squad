"use client";

import { useEffect, useState } from "react";
import type { FilesChord } from "./filesChords";

export type LayoutMap = { get(code: string): string | undefined };

const US_KEY: Record<string, string> = { BracketLeft: "[", BracketRight: "]", KeyW: "W" };
const PUNCTUATION = /^[^A-Za-z0-9]$/;

export interface ChordLabel {
  text: string;
  /** Shown beside the text when the layout is unknown and the key is not the same symbol everywhere. */
  note?: string;
}

const modName = (isMac: boolean) => (isMac ? "Cmd" : "Ctrl");

/**
 * Overlay text for a chord. A physical-key chord is labelled with what the user's layout puts on that key
 * (Chromium's getLayoutMap), else the US label plus a note, so the text is never a chord the keyboard lacks.
 */
export function chordLabel(chord: FilesChord, layout: LayoutMap | null, isMac = false): ChordLabel {
  const parts: string[] = [];
  if (chord.mod) parts.push(modName(isMac));
  if (chord.alt) parts.push(isMac ? "Option" : "Alt");
  if (chord.shift) parts.push("Shift");
  if (!chord.code) {
    parts.push(chord.key && chord.key.length === 1 ? chord.key.toUpperCase() : (chord.key ?? ""));
    return { text: parts.join("+") };
  }
  const us = US_KEY[chord.code];
  const resolved = layout?.get(chord.code);
  if (resolved) {
    parts.push(resolved.length === 1 ? resolved.toUpperCase() : resolved);
    return { text: parts.join("+") };
  }
  parts.push(us);
  const note = !layout && PUNCTUATION.test(us) ? `(the key at the US ${us} position)` : undefined;
  return { text: parts.join("+"), note };
}

interface KeyboardWithLayout { getLayoutMap?: () => Promise<LayoutMap> }

/** The layout map once it resolves; null where the API is absent (Firefox, Safari). */
export function useLayoutMap(): LayoutMap | null {
  const [layout, setLayout] = useState<LayoutMap | null>(null);
  useEffect(() => {
    const kb = typeof navigator !== "undefined" ? ((navigator as unknown as { keyboard?: KeyboardWithLayout }).keyboard) : undefined;
    if (!kb?.getLayoutMap) return;
    let live = true;
    kb.getLayoutMap().then((m) => { if (live) setLayout(m); }, () => undefined);
    return () => { live = false; };
  }, []);
  return layout;
}
