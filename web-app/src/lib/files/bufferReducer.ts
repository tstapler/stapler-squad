// Pure tab/buffer state for the Files viewer (ADR-032). No React, no I/O, no file text:
// the reducer knows paths, flags and positions; text lives in the editor's EditorState.

export type ReadOnlyKind =
  | "none" | "flag_off" | "binary" | "truncated" | "too_large" | "non_utf8" | "mixed_eol"
  | "symlink" | "worktree_missing" | "hidden_session" | "git_internal";
export type FailureKind = "not_found" | "too_large" | "binary" | "encoding" | "worktree_unavailable" | "rpc_error";
export type BufferMode = "read" | "edit";

export interface ViewState { line: number; col: number; scrollTop: number }
/** One jumplist record; independent of tab order. */
export interface NavEntry { path: string; line: number; col: number }
export interface ClosedTab { path: string; viewState: ViewState }

interface BufferBase {
  path: string;
  /** Captured at open and never updated: the live session prop may change underneath a reused tab. */
  sessionId: string;
  pinned: boolean;
  viewState: ViewState;
  usedAt: number;
}
export interface LoadingBuffer extends BufferBase { status: "loading" }
export interface FailedBuffer extends BufferBase { status: "failed"; reason: FailureKind }
interface LoadedCommon extends BufferBase {
  status: "loaded";
  token: string;
  dirty: boolean;
  saving: boolean;
  source: { kind: "full" };
}
/** Edit is representable only when the file is editable (read-only reason None). */
export type LoadedBuffer =
  | (LoadedCommon & { mode: "read"; readOnly: ReadOnlyKind })
  | (LoadedCommon & { mode: "edit"; readOnly: "none" });
export type Buffer = LoadingBuffer | FailedBuffer | LoadedBuffer;

export type BufferNotice =
  | { kind: "evicted"; path: string; cap: number }
  | { kind: "refused"; path: string; cap: number };

/** Asks the editor to put the cursor at a position once the buffer's text is on screen. */
export interface PendingReveal { path: string; line: number; col: number; seq: number }

export interface BufferState {
  buffers: readonly Buffer[];
  /** -1 when no buffer is open. */
  activeIndex: number;
  cap: number;
  tick: number;
  nav: readonly NavEntry[];
  navIndex: number;
  closed: readonly ClosedTab[];
  notice: BufferNotice | null;
  pendingReveal: PendingReveal | null;
  revealSeq: number;
}

export const MOBILE_CAP = 10;
export const DESKTOP_CAP = 30;
const CLOSED_STACK_LIMIT = 10;
export const ORIGIN: ViewState = { line: 1, col: 0, scrollTop: 0 };

export type BufferAction =
  | { type: "open"; path: string; sessionId: string; pinned?: boolean; viewState?: ViewState }
  | { type: "restore"; sessionId: string; tabs: readonly { path: string; viewState: ViewState }[]; activeIndex: number }
  | { type: "close"; path: string; discardConfirmed?: boolean }
  /** `chain: "step"` moves the current jumplist entry (quickfix stepping) instead of pushing one. */
  | { type: "jump"; path: string; sessionId: string; line: number; col: number; pinned?: boolean; chain?: "start" | "step" }
  | { type: "move"; path: string; delta: -1 | 1 }
  | { type: "adoptToken"; path: string; token: string }
  | { type: "revealed"; seq: number }
  | { type: "reopenClosed" }
  | { type: "activate"; path: string }
  | { type: "pin"; path: string }
  | { type: "unpin"; path: string }
  | { type: "navBack" }
  | { type: "navForward" }
  | { type: "recordViewState"; path: string; viewState: ViewState }
  | { type: "markLoaded"; path: string; token: string; readOnly: ReadOnlyKind }
  | { type: "markFailed"; path: string; reason: FailureKind }
  | { type: "markDirty"; path: string; dirty: boolean }
  | { type: "markSaving"; path: string; saving: boolean }
  | { type: "setMode"; path: string; mode: BufferMode }
  | { type: "flagOff" }
  | { type: "setCap"; cap: number }
  | { type: "evict" }
  | { type: "clearNotice" };

export function initialBufferState(cap: number = DESKTOP_CAP): BufferState {
  return { buffers: [], activeIndex: -1, cap, tick: 0, nav: [], navIndex: -1, closed: [], notice: null, pendingReveal: null, revealSeq: 0 };
}

// ---- predicates ----

const isDirty = (b: Buffer) => b.status === "loaded" && b.dirty;
const isSaving = (b: Buffer) => b.status === "loaded" && b.saving;
const isEditing = (b: Buffer) => b.status === "loaded" && b.mode === "edit";
/** The single preview slot may be taken over by the next single-open. */
const isReplaceablePreview = (b: Buffer) => !b.pinned && !isDirty(b) && !isSaving(b);
/** Cap eviction never takes a loading buffer. */
const isEvictable = (b: Buffer) => isReplaceablePreview(b) && b.status !== "loading";

const indexOf = (s: BufferState, path: string) => s.buffers.findIndex((b) => b.path === path);

// ---- helpers ----

function touched(s: BufferState, index: number): BufferState {
  const tick = s.tick + 1;
  return { ...s, tick, buffers: s.buffers.map((b, i) => (i === index ? { ...b, usedAt: tick } : b)) };
}

function mapBuffer(s: BufferState, path: string, fn: (b: Buffer) => Buffer): BufferState {
  const i = indexOf(s, path);
  if (i < 0) return s;
  const next = fn(s.buffers[i]);
  return next === s.buffers[i] ? s : { ...s, buffers: s.buffers.map((b, j) => (j === i ? next : b)) };
}

function syncCurrentNav(s: BufferState): BufferState {
  const active = s.buffers[s.activeIndex];
  const cur = s.nav[s.navIndex];
  if (!active || !cur || cur.path !== active.path) return s;
  const { line, col } = active.viewState;
  if (cur.line === line && cur.col === col) return s;
  return { ...s, nav: s.nav.map((e, i) => (i === s.navIndex ? { ...e, line, col } : e)) };
}

/** Records arrival at `path`; drops the forward half like a browser history. */
function pushNav(s: BufferState, path: string, viewState: ViewState): BufferState {
  const synced = syncCurrentNav(s);
  const head = synced.nav.slice(0, synced.navIndex + 1);
  const entry = { path, line: viewState.line, col: viewState.col };
  const nav = head[head.length - 1]?.path === path ? [...head.slice(0, -1), entry] : [...head, entry];
  return { ...synced, nav, navIndex: nav.length - 1 };
}

function pushClosed(s: BufferState, b: Buffer): BufferState {
  const closed = [...s.closed, { path: b.path, viewState: b.viewState }].slice(-CLOSED_STACK_LIMIT);
  return { ...s, closed };
}

function removeAt(s: BufferState, index: number): BufferState {
  const removed = s.buffers[index];
  const buffers = s.buffers.filter((_, i) => i !== index);
  let activeIndex = s.activeIndex;
  if (index < activeIndex) activeIndex -= 1;
  else if (index === activeIndex) {
    const mru = buffers.reduce((best, b, i) => (best < 0 || b.usedAt > buffers[best].usedAt ? i : best), -1);
    activeIndex = mru;
  }
  return pushClosed({ ...s, buffers, activeIndex }, removed);
}

function newLoading(path: string, sessionId: string, pinned: boolean, viewState: ViewState, usedAt: number): LoadingBuffer {
  return { status: "loading", path, sessionId, pinned, viewState, usedAt };
}

function lruEvictable(s: BufferState): number {
  let best = -1;
  s.buffers.forEach((b, i) => {
    if (isEvictable(b) && (best < 0 || b.usedAt < s.buffers[best].usedAt)) best = i;
  });
  return best;
}

/** Adds a new Loading buffer applying the preview slot and the one cap rule. */
function openNew(s: BufferState, path: string, sessionId: string, pinned: boolean, viewState: ViewState): BufferState {
  let next = s;
  const atCap = next.buffers.length >= next.cap;
  const previewIndex = !pinned ? next.buffers.findIndex(isReplaceablePreview) : -1;
  let victim = previewIndex;
  if (victim < 0 && atCap) victim = lruEvictable(next);
  if (victim < 0 && atCap) return { ...next, notice: { kind: "refused", path, cap: next.cap } };

  let notice = next.notice;
  if (victim >= 0) {
    const gone = next.buffers[victim];
    next = pushClosed(next, gone);
    next = { ...next, buffers: next.buffers.filter((_, i) => i !== victim), activeIndex: next.activeIndex > victim ? next.activeIndex - 1 : next.activeIndex };
    if (atCap) notice = { kind: "evicted", path: gone.path, cap: next.cap };
  }
  const tick = next.tick + 1;
  const buffers = [...next.buffers, newLoading(path, sessionId, pinned, viewState, tick)];
  return pushNav({ ...next, buffers, activeIndex: buffers.length - 1, tick, notice }, path, viewState);
}

function openAction(from: BufferState, a: Extract<BufferAction, { type: "open" }>): BufferState {
  const s = syncCurrentNav(from);
  const existing = indexOf(s, a.path);
  if (existing < 0) return openNew(s, a.path, a.sessionId, a.pinned ?? false, a.viewState ?? ORIGIN);
  let next = touched(s, existing);
  next = a.pinned ? mapBuffer(next, a.path, (b) => (b.pinned ? b : { ...b, pinned: true })) : next;
  next = { ...next, activeIndex: existing };
  return pushNav(next, a.path, next.buffers[existing].viewState);
}

function jumpAction(from: BufferState, a: Extract<BufferAction, { type: "jump" }>): BufferState {
  const s = syncCurrentNav(from);
  const existing = indexOf(s, a.path);
  const target = (b?: Buffer): ViewState => ({ line: a.line, col: a.col, scrollTop: b?.viewState.scrollTop ?? 0 });
  let next: BufferState;
  if (existing < 0) {
    next = openNew(s, a.path, a.sessionId, a.pinned ?? false, target());
    if (next.notice?.kind === "refused") return next;
  } else {
    next = touched(s, existing);
    next = mapBuffer(next, a.path, (b) => ({ ...b, pinned: b.pinned || !!a.pinned, viewState: target(b) }));
    next = pushNav({ ...next, activeIndex: existing }, a.path, target(s.buffers[existing]));
  }
  if (a.chain !== "step" || s.navIndex < 0) return next;
  const nav = s.nav.map((e, i) => (i === s.navIndex ? { path: a.path, line: a.line, col: a.col } : e));
  return { ...next, nav, navIndex: s.navIndex };
}

function moveAction(s: BufferState, path: string, delta: -1 | 1): BufferState {
  const from = indexOf(s, path);
  const to = from + delta;
  if (from < 0 || to < 0 || to >= s.buffers.length) return s;
  const buffers = s.buffers.slice();
  [buffers[from], buffers[to]] = [buffers[to], buffers[from]];
  const activePath = s.buffers[s.activeIndex]?.path;
  return { ...s, buffers, activeIndex: buffers.findIndex((b) => b.path === activePath) };
}

function navigate(s: BufferState, delta: -1 | 1): BufferState {
  const synced = syncCurrentNav(s);
  const target = synced.navIndex + delta;
  const entry = synced.nav[target];
  if (!entry) return s;
  const next: BufferState = { ...synced, navIndex: target };
  const index = indexOf(next, entry.path);
  if (index >= 0) {
    const seen = mapBuffer(next, entry.path, (b) => ({ ...b, viewState: { ...b.viewState, line: entry.line, col: entry.col } }));
    return { ...touched(seen, index), activeIndex: index };
  }
  const sessionId = next.buffers[next.activeIndex]?.sessionId ?? "";
  const opened = openNew(next, entry.path, sessionId, false, { ...ORIGIN, line: entry.line, col: entry.col });
  if (opened.notice?.kind === "refused") return { ...s, notice: opened.notice };
  // openNew appended a nav entry; this jump is already in the list.
  return { ...opened, nav: next.nav, navIndex: target };
}

function setModeAction(s: BufferState, path: string, mode: BufferMode): BufferState {
  return mapBuffer(s, path, (b) => {
    if (b.status !== "loaded") return b;
    if (mode === "read") return b.mode === "read" ? b : { ...b, mode: "read" };
    if (b.readOnly !== "none" || b.mode === "edit") return b;
    return { ...b, mode: "edit", readOnly: "none", pinned: true };
  });
}

/** A notice describes the last action only; any later action replaces it. */
export function bufferReducer(state: BufferState, a: BufferAction): BufferState {
  const s = state.notice && a.type !== "clearNotice" ? { ...state, notice: null } : state;
  return withReveal(s, apply(s, a), a);
}

/** A change of active buffer (or an explicit jump) asks the editor to restore that buffer's cursor. */
function withReveal(prev: BufferState, next: BufferState, a: BufferAction): BufferState {
  if (a.type === "revealed" || a.type === "clearNotice" || next.notice?.kind === "refused") return next;
  const active = activeBuffer(next);
  if (!active || (a.type !== "jump" && activeBuffer(prev)?.path === active.path)) return next;
  const { line, col } = active.viewState;
  if (a.type !== "jump" && line <= 1 && col === 0) return next.pendingReveal ? { ...next, pendingReveal: null } : next;
  const seq = next.revealSeq + 1;
  return { ...next, revealSeq: seq, pendingReveal: { path: active.path, line, col, seq } };
}

function apply(s: BufferState, a: BufferAction): BufferState {
  switch (a.type) {
    case "open":
      return openAction(s, a);
    case "restore": {
      const unique = a.tabs.filter((t, i) => a.tabs.findIndex((u) => u.path === t.path) === i);
      const buffers = unique.map((t, i) => newLoading(t.path, a.sessionId, true, t.viewState, i + 1));
      const activeIndex = buffers.length === 0 ? -1 : Math.min(Math.max(a.activeIndex, 0), buffers.length - 1);
      const active = buffers[activeIndex];
      const nav = active ? [{ path: active.path, line: active.viewState.line, col: active.viewState.col }] : [];
      return { ...s, buffers: buffers.slice(0, s.cap), activeIndex: Math.min(activeIndex, s.cap - 1), tick: buffers.length, nav, navIndex: nav.length - 1 };
    }
    case "close": {
      const i = indexOf(s, a.path);
      if (i < 0 || (isDirty(s.buffers[i]) && !a.discardConfirmed)) return s;
      return removeAt(s, i);
    }
    case "jump":
      return jumpAction(s, a);
    case "move":
      return moveAction(s, a.path, a.delta);
    case "adoptToken":
      return mapBuffer(s, a.path, (b) => (b.status !== "loaded" || b.token === a.token ? b : { ...b, token: a.token }));
    case "revealed":
      return s.pendingReveal?.seq === a.seq ? { ...s, pendingReveal: null } : s;
    case "reopenClosed": {
      const top = s.closed[s.closed.length - 1];
      if (!top) return s;
      const rest = { ...s, closed: s.closed.slice(0, -1) };
      const already = indexOf(s, top.path);
      if (already >= 0) return { ...touched(rest, already), activeIndex: already };
      const sessionId = s.buffers[0]?.sessionId ?? "";
      const opened = openNew({ ...s, closed: s.closed.slice(0, -1) }, top.path, sessionId, true, top.viewState);
      return opened.notice?.kind === "refused" ? { ...s, notice: opened.notice } : opened;
    }
    case "activate": {
      const i = indexOf(s, a.path);
      return i < 0 ? s : { ...touched(s, i), activeIndex: i };
    }
    case "pin":
      return mapBuffer(s, a.path, (b) => (b.pinned ? b : { ...b, pinned: true }));
    case "unpin":
      // A dirty or editing buffer must stay pinned or the next single-open would replace it.
      return mapBuffer(s, a.path, (b) => (!b.pinned || isDirty(b) || isEditing(b) ? b : { ...b, pinned: false }));
    case "navBack":
      return navigate(s, -1);
    case "navForward":
      return navigate(s, 1);
    case "recordViewState":
      return mapBuffer(s, a.path, (b) => ({ ...b, viewState: a.viewState }));
    case "markLoaded":
      return mapBuffer(s, a.path, (b): Buffer => {
        if (b.status === "failed") return b;
        const common = { path: b.path, sessionId: b.sessionId, pinned: b.pinned, viewState: b.viewState, usedAt: b.usedAt };
        const loaded = { ...common, status: "loaded" as const, token: a.token, dirty: false, saving: false, source: { kind: "full" as const } };
        return { ...loaded, mode: "read", readOnly: a.readOnly };
      });
    case "markFailed":
      return mapBuffer(s, a.path, (b) => (b.status === "loaded" ? b : { path: b.path, sessionId: b.sessionId, pinned: b.pinned, viewState: b.viewState, usedAt: b.usedAt, status: "failed", reason: a.reason }));
    case "markDirty":
      return mapBuffer(s, a.path, (b) => (b.status !== "loaded" || b.dirty === a.dirty ? b : { ...b, dirty: a.dirty, pinned: b.pinned || a.dirty }));
    case "markSaving":
      return mapBuffer(s, a.path, (b) => (b.status !== "loaded" || b.saving === a.saving ? b : { ...b, saving: a.saving }));
    case "setMode":
      return setModeAction(s, a.path, a.mode);
    case "flagOff":
      return { ...s, buffers: s.buffers.map((b) => (b.status === "loaded" && b.mode === "edit" ? { ...b, mode: "read" as const } : b)) };
    case "setCap":
      return { ...s, cap: a.cap };
    case "evict": {
      const i = lruEvictable(s);
      return i < 0 ? s : removeAt(s, i);
    }
    case "clearNotice":
      return s.notice ? { ...s, notice: null } : s;
  }
}

// ---- selectors ----

export const activeBuffer = (s: BufferState): Buffer | null => s.buffers[s.activeIndex] ?? null;
export const selectedPath = (s: BufferState): string | null => activeBuffer(s)?.path ?? null;
/** Most recently used first; feeds RecentFilesSection. */
export const mruPaths = (s: BufferState): string[] =>
  [...s.buffers].sort((x, y) => y.usedAt - x.usedAt).map((b) => b.path);
export const hasDirty = (s: BufferState): boolean => s.buffers.some(isDirty);
export const canGoBack = (s: BufferState): boolean => s.navIndex > 0;
export const canGoForward = (s: BufferState): boolean => s.navIndex >= 0 && s.navIndex < s.nav.length - 1;
