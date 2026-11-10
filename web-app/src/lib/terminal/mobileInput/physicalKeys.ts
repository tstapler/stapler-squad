/** Bytes for physical keys the IME reducer never sees as text: Tab, Esc, navigation and Ctrl-letter. */
import { ALT_KEY_MAP, CTRL_KEY_MAP, SHIFT_KEY_MAP } from "../modifierKeys";

export interface PhysicalKey {
  key: string;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  metaKey: boolean;
}

const BASE: Readonly<Record<string, string>> = {
  Tab: "\t",
  Escape: "\x1b",
  ArrowUp: "\x1b[A",
  ArrowDown: "\x1b[B",
  ArrowRight: "\x1b[C",
  ArrowLeft: "\x1b[D",
  Home: "\x1b[H",
  End: "\x1b[F",
  PageUp: "\x1b[5~",
  PageDown: "\x1b[6~",
  Delete: "\x1b[3~",
};

/** Terminal bytes for the key, or null when the layer should leave it to the browser / `beforeinput`. */
export function physicalKeyBytes(k: PhysicalKey): string | null {
  if (k.metaKey) return null;
  const base = BASE[k.key];
  if (base !== undefined) {
    if (k.ctrlKey) return CTRL_KEY_MAP[base] ?? base;
    if (k.altKey) return ALT_KEY_MAP[base] ?? base;
    if (k.shiftKey) return SHIFT_KEY_MAP[base] ?? base;
    return base;
  }
  if (k.ctrlKey && !k.altKey && /^[a-zA-Z]$/.test(k.key)) {
    return String.fromCharCode(k.key.toUpperCase().charCodeAt(0) & 0x1f);
  }
  return null;
}
