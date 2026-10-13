/**
 * Leader chord configuration for the command bar.
 *
 * Matched on KeyboardEvent.code, never .key: on macOS Option+letter produces a
 * symbol (a dead key), so .key is unusable for Alt chords. See
 * docs/reference/leader-key-chord-probe.md for how the default was chosen and
 * what has / has not been verified.
 */

export const LEADER_STORAGE_KEY = "ssq.leaderKey";
export const LEADER_CONFIG_EVENT = "ssq:leader-config-changed";

export interface LeaderChord {
  /** KeyboardEvent.code, e.g. "Semicolon". */
  code: string;
  alt?: boolean;
  ctrl?: boolean;
  shift?: boolean;
  meta?: boolean;
}

/** `enabled: false` is the "no safe chord" fallback: no global chord, bar opened from Settings. */
export type LeaderConfig = { enabled: true; chord: LeaderChord } | { enabled: false };

export const DEFAULT_LEADER_CHORD: LeaderChord = { code: "Semicolon", alt: true };

const DEFAULT_CONFIG: LeaderConfig = { enabled: true, chord: DEFAULT_LEADER_CHORD };

const MODIFIER_CODES = new Set([
  "AltLeft", "AltRight", "ControlLeft", "ControlRight", "ShiftLeft", "ShiftRight", "MetaLeft", "MetaRight",
]);

/**
 * A chord is safe only with Alt or Meta held. Ctrl-only chords are rejected:
 * Ctrl+letter is a PTY control byte (Ctrl+C, Ctrl+A is the tmux/screen prefix),
 * and Ctrl+Space is NUL. A bare key would swallow ordinary typing.
 */
export function isSafeChord(chord: LeaderChord): boolean {
  if (!chord.code || MODIFIER_CODES.has(chord.code)) return false;
  return !!(chord.alt || chord.meta);
}

function parse(raw: string | null): LeaderConfig {
  if (!raw) return DEFAULT_CONFIG;
  try {
    const v = JSON.parse(raw) as Record<string, unknown>;
    if (v && v.disabled === true) return { enabled: false };
    if (v && typeof v.code === "string") {
      const chord: LeaderChord = {
        code: v.code,
        alt: v.alt === true,
        ctrl: v.ctrl === true,
        shift: v.shift === true,
        meta: v.meta === true,
      };
      if (isSafeChord(chord)) return { enabled: true, chord };
    }
  } catch {
    // fall through to default
  }
  return DEFAULT_CONFIG;
}

export function getLeaderConfig(): LeaderConfig {
  try {
    return parse(typeof localStorage === "undefined" ? null : localStorage.getItem(LEADER_STORAGE_KEY));
  } catch {
    return DEFAULT_CONFIG;
  }
}

/** Returns false (and stores nothing) for an unsafe chord. */
export function setLeaderConfig(config: LeaderConfig): boolean {
  if (config.enabled && !isSafeChord(config.chord)) return false;
  try {
    const value = config.enabled ? config.chord : { disabled: true };
    localStorage.setItem(LEADER_STORAGE_KEY, JSON.stringify(value));
  } catch {
    // storage blocked: still notify so in-memory consumers re-read the default
  }
  if (typeof window !== "undefined") window.dispatchEvent(new Event(LEADER_CONFIG_EVENT));
  return true;
}

export function resetLeaderConfig(): void {
  try {
    localStorage.removeItem(LEADER_STORAGE_KEY);
  } catch {
    // ignore
  }
  if (typeof window !== "undefined") window.dispatchEvent(new Event(LEADER_CONFIG_EVENT));
}

export function chordMatches(e: KeyboardEvent, chord: LeaderChord): boolean {
  return (
    e.code === chord.code &&
    e.altKey === !!chord.alt &&
    e.ctrlKey === !!chord.ctrl &&
    e.shiftKey === !!chord.shift &&
    e.metaKey === !!chord.meta
  );
}

export function chordFromEvent(e: KeyboardEvent): LeaderChord {
  return { code: e.code, alt: e.altKey, ctrl: e.ctrlKey, shift: e.shiftKey, meta: e.metaKey };
}

function codeLabel(code: string): string {
  if (code.startsWith("Key")) return code.slice(3);
  if (code.startsWith("Digit")) return code.slice(5);
  const named: Record<string, string> = {
    Semicolon: ";", Comma: ",", Period: ".", Slash: "/", Backslash: "\\", Quote: "'",
    BracketLeft: "[", BracketRight: "]", Minus: "-", Equal: "=", Backquote: "`", Space: "Space",
  };
  return named[code] ?? code;
}

export function formatChord(chord: LeaderChord): string {
  return [
    chord.ctrl && "Ctrl",
    chord.alt && "Alt",
    chord.shift && "Shift",
    chord.meta && "Meta",
    codeLabel(chord.code),
  ]
    .filter(Boolean)
    .join("+");
}

export function formatLeader(config: LeaderConfig): string {
  return config.enabled ? formatChord(config.chord) : "disabled";
}
