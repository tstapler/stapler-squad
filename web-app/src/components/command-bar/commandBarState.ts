import type { PaletteEntry, ShortcutTree, ShortcutTreeNode } from "@/lib/shortcuts/shortcutRegistry";

/** Row height doubles as the 44px minimum touch target (WCAG 2.5.8). Plain consts: .css.ts exports are not values under jest. */
export const ROW_HEIGHT_PX = 44;
export const LIST_MAX_HEIGHT_PX = 352;

/** Above this many rows the palette renders only the visible window. */
export const VIRTUALIZE_THRESHOLD = 100;

export interface BarState {
  open: boolean;
  /** Which-key position: keys pressed since the leader. */
  path: string[];
  /** Non-empty means fuzzy mode. */
  query: string;
  helpOpen: boolean;
  activeIndex: number;
}

export const CLOSED_STATE: BarState = { open: false, path: [], query: "", helpOpen: false, activeIndex: 0 };

export type BarAction =
  | { type: "open"; path: string[] }
  | { type: "close" }
  | { type: "descend"; key: string }
  | { type: "back" }
  | { type: "setQuery"; query: string }
  | { type: "move"; delta: number; count: number }
  | { type: "setActive"; index: number }
  | { type: "toggleHelp" };

export function barReducer(state: BarState, action: BarAction): BarState {
  switch (action.type) {
    case "open":
      return { open: true, path: action.path, query: "", helpOpen: false, activeIndex: 0 };
    case "close":
      return CLOSED_STATE;
    case "descend":
      return { ...state, path: [...state.path, action.key], query: "", activeIndex: 0, helpOpen: false };
    case "back":
      return { ...state, path: state.path.slice(0, -1), query: "", activeIndex: 0, helpOpen: false };
    case "setQuery":
      return { ...state, query: action.query, activeIndex: 0, helpOpen: false };
    case "move": {
      if (action.count === 0) return state;
      const next = (state.activeIndex + action.delta + action.count) % action.count;
      return { ...state, activeIndex: next };
    }
    case "setActive":
      return { ...state, activeIndex: action.index };
    case "toggleHelp":
      return { ...state, helpOpen: !state.helpOpen };
  }
}

export function isFuzzy(state: BarState): boolean {
  return state.query.length > 0;
}

/** Children of the tree node at `path`; [] when the path no longer exists. */
export function childrenAt(tree: ShortcutTree, path: string[]): ShortcutTreeNode[] {
  let level = tree;
  for (const key of path) {
    const node = level.find((n) => n.key === key);
    if (!node) return [];
    level = node.children;
  }
  return level;
}

/** Heading for a path: the group label of its first key, e.g. "Windows". */
export function pathLabel(tree: ShortcutTree, path: string[]): string {
  if (path.length === 0) return "";
  return tree.find((n) => n.key === path[0])?.label ?? path[0];
}

export function findChild(nodes: ShortcutTreeNode[], key: string): ShortcutTreeNode | undefined {
  return nodes.find((n) => n.key === key) ?? nodes.find((n) => n.key.toLowerCase() === key.toLowerCase());
}

export interface PaletteRow {
  id: string;
  label: string;
  group: string;
  /** Display parts of the key sequence ("w", "1") or of the direct chord. */
  keys: string[];
  run: () => void;
}

function directChord(entry: PaletteEntry): string[] {
  const s = entry.shortcut;
  const m = s.modifiers ?? {};
  return [m.ctrl && "Ctrl", m.alt && "Alt", m.shift && "Shift", m.meta && "Meta", s.key]
    .filter((x): x is string => typeof x === "string" && x.length > 0);
}

export function paletteRows(entries: PaletteEntry[]): PaletteRow[] {
  return entries.map((e) => ({
    id: e.id,
    label: e.shortcut.label,
    group: e.shortcut.group ?? "",
    keys: e.shortcut.sequence ?? directChord(e),
    run: e.shortcut.action,
  }));
}

/** Case-insensitive substring match; label-prefix and word-start hits rank first. Stable otherwise. */
export function filterRows(rows: PaletteRow[], query: string): PaletteRow[] {
  const q = query.trim().toLowerCase();
  if (!q) return rows;
  const scored: Array<{ row: PaletteRow; score: number; i: number }> = [];
  rows.forEach((row, i) => {
    const label = row.label.toLowerCase();
    const hay = `${label} ${row.group.toLowerCase()} ${row.keys.join(" ").toLowerCase()}`;
    if (!hay.includes(q)) return;
    let score = 3;
    if (label.startsWith(q)) score = 0;
    else if (label.split(/[\s:/-]+/).some((w) => w.startsWith(q))) score = 1;
    else if (label.includes(q)) score = 2;
    scored.push({ row, score, i });
  });
  scored.sort((a, b) => a.score - b.score || a.i - b.i);
  return scored.map((s) => s.row);
}

export interface VirtualWindow {
  start: number;
  end: number;
  padTop: number;
  padBottom: number;
}

/** Rows to render for a fixed-height list; everything when under the threshold. */
export function virtualWindow(
  total: number,
  scrollTop: number,
  viewportHeight: number,
  rowHeight: number,
  activeIndex = 0,
  overscan = 6,
): VirtualWindow {
  if (total <= VIRTUALIZE_THRESHOLD) return { start: 0, end: total, padTop: 0, padBottom: 0 };
  // The active row is always rendered so aria-activedescendant never dangles while scroll state catches up.
  const first = Math.max(0, Math.min(Math.floor(scrollTop / rowHeight) - overscan, activeIndex));
  const last = Math.min(total, Math.max(Math.ceil((scrollTop + viewportHeight) / rowHeight) + overscan, activeIndex + 1));
  return { start: first, end: last, padTop: first * rowHeight, padBottom: (total - last) * rowHeight };
}

/** One rendered row of the bar (which-key level entry or palette hit). */
export interface Row {
  id: string;
  label: string;
  group: string;
  keys: string[];
  /** Which-key branch: pressing it descends instead of running. */
  branchKey?: string;
  run?: () => void;
}

export type KeyOutcome =
  | { type: "action"; action: BarAction }
  | { type: "execute"; row: Row }
  /** Leave which-key for fuzzy: caller resolves lazy providers, then sets the query to `char`. */
  | { type: "seedQuery"; char: string }
  | { type: "none" };

export interface KeyInput {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
}

const NONE: KeyOutcome = { type: "none" };
const act = (action: BarAction): KeyOutcome => ({ type: "action", action });

/**
 * Pure key policy for an open bar. Every key is consumed by the caller; this
 * only decides what it means. j/k navigate in which-key mode but are text in
 * fuzzy mode (arrows always navigate).
 */
export function interpretKey(e: KeyInput, s: BarState, rows: Row[], tree: ShortcutTree): KeyOutcome {
  const { key } = e;
  const plain = !e.ctrlKey && !e.metaKey && !e.altKey;
  const fuzzy = isFuzzy(s);
  const count = rows.length;

  if (key === "Escape") {
    if (s.helpOpen) return act({ type: "toggleHelp" });
    if (!fuzzy && s.path.length > 0) return act({ type: "back" });
    return act({ type: "close" });
  }
  if (key === "ArrowDown" || (plain && !fuzzy && key === "j")) return act({ type: "move", delta: 1, count });
  if (key === "ArrowUp" || (plain && !fuzzy && key === "k")) return act({ type: "move", delta: -1, count });
  if (key === "Home") return act({ type: "setActive", index: 0 });
  if (key === "End") return act({ type: "setActive", index: Math.max(0, count - 1) });
  if (key === "Enter") return rows[s.activeIndex] ? { type: "execute", row: rows[s.activeIndex] } : NONE;
  if (key === "Backspace") {
    if (fuzzy) return act({ type: "setQuery", query: s.query.slice(0, -1) });
    return s.path.length > 0 ? act({ type: "back" }) : NONE;
  }
  const printable = key.length === 1 && plain;
  if (!printable) return NONE; // Tab, modifiers, function keys: swallowed, never forwarded
  if (fuzzy) return act({ type: "setQuery", query: s.query + key });

  if (key === "?") return act({ type: "toggleHelp" });
  const node = findChild(childrenAt(tree, s.path), key);
  const row = node && rows.find((r) => r.keys[0] === node.key);
  if (row) return { type: "execute", row };
  return { type: "seedQuery", char: key };
}
