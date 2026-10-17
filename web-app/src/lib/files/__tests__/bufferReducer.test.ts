import {
  bufferReducer, initialBufferState, mruPaths, selectedPath, hasDirty, canGoBack, canGoForward,
  type BufferAction, type BufferState, type Buffer, type ViewState,
} from "../bufferReducer";

const S1 = "s1";
const vs = (line: number): ViewState => ({ line, col: 0, scrollTop: line * 10 });
const run = (s: BufferState, ...actions: BufferAction[]) => actions.reduce(bufferReducer, s);
const single = (path: string, sessionId = S1): BufferAction => ({ type: "open", path, sessionId });
const pinned = (path: string): Extract<BufferAction, { type: "open" }> => ({ type: "open", path, sessionId: S1, pinned: true });
const loaded = (path: string, readOnly: "none" | "binary" = "none"): BufferAction => ({ type: "markLoaded", path, token: "v1:1:1:x", readOnly });
const paths = (s: BufferState) => s.buffers.map((b) => b.path);
const get = (s: BufferState, path: string) => s.buffers.find((b) => b.path === path) as Buffer;

function mulberry32(seed: number) {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const POOL = ["a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go"];

function randomAction(rand: () => number): BufferAction {
  const pick = <T,>(xs: readonly T[]) => xs[Math.floor(rand() * xs.length)];
  const path = pick(POOL);
  const makers: (() => BufferAction)[] = [
    () => single(path), () => pinned(path), () => loaded(path), () => ({ type: "markDirty", path, dirty: rand() < 0.7 }),
    () => ({ type: "close", path, discardConfirmed: rand() < 0.5 }), () => ({ type: "setMode", path, mode: pick(["read", "edit"] as const) }),
    () => ({ type: "markSaving", path, saving: rand() < 0.5 }), () => ({ type: "activate", path }), () => ({ type: "pin", path }),
    () => ({ type: "unpin", path }), () => ({ type: "navBack" }), () => ({ type: "navForward" }), () => ({ type: "flagOff" }),
    () => ({ type: "reopenClosed" }), () => ({ type: "evict" }), () => ({ type: "markFailed", path, reason: "not_found" }),
  ];
  return pick(makers)();
}

/** Runs seeded random sequences; the failing seed and step are in the thrown message. */
function property(check: (before: BufferState, action: BufferAction, after: BufferState) => void, cap = 4) {
  for (let seed = 1; seed <= 60; seed++) {
    const rand = mulberry32(seed);
    let s = initialBufferState(cap);
    for (let step = 0; step < 80; step++) {
      const action = randomAction(rand);
      const next = bufferReducer(s, action);
      try {
        check(s, action, next);
      } catch (e) {
        throw new Error(`seed=${seed} step=${step} action=${JSON.stringify(action)}: ${(e as Error).message}`);
      }
      s = next;
    }
  }
}

const replaceable = (b: Buffer) => !b.pinned && !(b.status === "loaded" && (b.dirty || b.saving));

describe("bufferReducer", () => {
  it("bufferReducer_should_ReplacePreviewAndPinOnDoubleOpen_When_SingleOpensAlternate", () => {
    let s = run(initialBufferState(), single("a.go"), loaded("a.go"), single("b.go"));
    expect(paths(s)).toEqual(["b.go"]);
    s = run(s, pinned("b.go"), single("c.go"));
    expect(paths(s)).toEqual(["b.go", "c.go"]);
    expect(get(s, "b.go").pinned).toBe(true);
    expect(get(s, "c.go").pinned).toBe(false);
    expect(selectedPath(s)).toBe("c.go");
  });

  it("bufferReducer_should_NeverReplaceOrDropDirtyBuffer_When_AnyActionSequence", () => {
    property((before, action, after) => {
      for (const b of before.buffers) {
        if (after.buffers.some((x) => x.path === b.path)) continue;
        const dirty = b.status === "loaded" && b.dirty;
        const explicitClose = action.type === "close" && action.path === b.path && (!dirty || action.discardConfirmed);
        if (!(explicitClose || (replaceable(b) && action.type !== "close"))) throw new Error(`${b.path} vanished`);
      }
      for (const b of after.buffers) if (b.status === "loaded" && b.dirty && !b.pinned) throw new Error(`${b.path} dirty preview`);
      expect(after.buffers.length).toBeLessThanOrEqual(after.cap);
    });
  });

  it("bufferReducer_should_EvictLeastRecentCleanBuffer_When_MobileCapReachedWithDirtyBuffer", () => {
    const cap = 10;
    let s = initialBufferState(cap);
    const names = Array.from({ length: cap - 1 }, (_, i) => `p${i}.go`);
    for (const n of names) s = run(s, pinned(n), loaded(n));
    s = run(s, { type: "markDirty", path: "p0.go", dirty: true }, single("c.go"), loaded("c.go"), { type: "recordViewState", path: "c.go", viewState: vs(42) });
    expect(s.buffers).toHaveLength(cap);

    s = run(s, single("n.go"));
    expect(paths(s)).not.toContain("c.go");
    expect(paths(s)).toEqual(expect.arrayContaining(["p0.go", "p1.go", "n.go"]));
    expect(s.buffers).toHaveLength(cap);
    expect(s.notice).toEqual({ kind: "evicted", path: "c.go", cap });

    s = run(s, { type: "reopenClosed" });
    expect(s.notice?.kind).toBe("refused");
    expect(paths(s)).not.toContain("c.go");
    s = run(s, loaded("n.go"), { type: "reopenClosed" });
    expect(get(s, "c.go").viewState).toEqual(vs(42));
    expect(paths(s)).not.toContain("n.go");
  });

  it("bufferReducer_should_EvictOnlyCleanPreview_And_Refuse_When_NoneEligible", () => {
    let s = initialBufferState(2);
    s = run(s, pinned("a.go"), pinned("b.go"), single("c.go"));
    expect(paths(s)).toEqual(["a.go", "b.go"]);
    expect(s.notice).toEqual({ kind: "refused", path: "c.go", cap: 2 });

    property((before, action, after) => {
      expect(new Set(after.buffers.map((b) => b.path)).size).toBe(after.buffers.length);
      if (action.type !== "open" || before.buffers.length < before.cap) return;
      const gone = before.buffers.filter((b) => !after.buffers.some((x) => x.path === b.path));
      if (before.buffers.some((b) => b.path === action.path)) return expect(gone).toEqual([]);
      if (gone.length === 0) return expect(after.notice).toMatchObject({ kind: "refused" });
      expect(gone).toHaveLength(1);
      expect(replaceable(gone[0])).toBe(true);
      expect(after.notice).toMatchObject({ kind: "evicted" });
    });
  });

  it("navHistory_should_RestoreViewStatePerBuffer_When_AltLeftTwiceAltRightOnce", () => {
    let s = run(initialBufferState(), pinned("a.go"), { type: "recordViewState", path: "a.go", viewState: vs(10) });
    s = run(s, pinned("b.go"), { type: "recordViewState", path: "b.go", viewState: vs(40) }, pinned("c.go"), { type: "recordViewState", path: "c.go", viewState: vs(7) });
    s = run(s, { type: "navBack" }, { type: "navBack" }, { type: "navForward" });
    expect(selectedPath(s)).toBe("b.go");
    expect(get(s, "b.go").viewState).toEqual(vs(40));
    expect(s.nav.map((e) => [e.path, e.line])).toEqual([["a.go", 10], ["b.go", 40], ["c.go", 7]]);
    s = run(s, { type: "navForward" }, { type: "navForward" });
    expect(selectedPath(s)).toBe("c.go");
  });

  it("bufferReducer_should_KeepOpenSessionId_When_ActionsDispatchedUnderAnotherSession", () => {
    let s = run(initialBufferState(), { ...pinned("a.go"), sessionId: "s1" }, loaded("a.go"));
    s = run(s, single("b.go", "s2"), loaded("b.go"), { type: "markDirty", path: "a.go", dirty: true }, { type: "recordViewState", path: "a.go", viewState: vs(3) });
    expect(get(s, "a.go").sessionId).toBe("s1");
    expect(get(s, "b.go").sessionId).toBe("s2");
    s = run(s, single("a.go", "s2"), { type: "setMode", path: "a.go", mode: "edit" });
    expect(get(s, "a.go").sessionId).toBe("s1");
  });

  it("bufferReducer_should_KeepModePerBuffer_And_RejectEditForReadOnlyReason_And_ResetOnFlagOff", () => {
    let s = run(initialBufferState(), pinned("a.go"), loaded("a.go"), pinned("b.go"), loaded("b.go"), pinned("bin.png"), loaded("bin.png", "binary"));
    s = run(s, { type: "setMode", path: "a.go", mode: "edit" }, { type: "setMode", path: "bin.png", mode: "edit" });
    expect(get(s, "a.go")).toMatchObject({ mode: "edit" });
    expect(get(s, "b.go")).toMatchObject({ mode: "read" });
    expect(get(s, "bin.png")).toMatchObject({ mode: "read" });
    s = run(s, { type: "markDirty", path: "a.go", dirty: true }, { type: "setMode", path: "a.go", mode: "read" }, { type: "setMode", path: "b.go", mode: "edit" });
    expect(get(s, "a.go")).toMatchObject({ mode: "read", dirty: true });
    expect(get(s, "b.go")).toMatchObject({ mode: "edit" });
    s = run(s, { type: "flagOff" });
    expect(s.buffers.every((b) => b.status !== "loaded" || b.mode === "read")).toBe(true);
    expect(get(s, "a.go")).toMatchObject({ dirty: true });
    expect(hasDirty(s)).toBe(true);
    s = run(s, { type: "markLoaded", path: "b.go", token: "v1:2:2:y", readOnly: "none" });
    expect(get(s, "b.go")).toMatchObject({ mode: "read", dirty: false });
  });

  it("bufferReducer_should_PinPreviewOnSetModeEdit_And_NeverEvictCleanBufferInEdit", () => {
    let s = run(initialBufferState(), single("a.go"), loaded("a.go"), { type: "setMode", path: "a.go", mode: "edit" });
    expect(get(s, "a.go").pinned).toBe(true);
    s = run(s, single("b.go"));
    expect(paths(s)).toEqual(["a.go", "b.go"]);
    s = run(s, { type: "unpin", path: "a.go" });
    expect(get(s, "a.go").pinned).toBe(true);
    s = run(s, { type: "evict" }, loaded("b.go"), { type: "evict" });
    expect(paths(s)).toEqual(["a.go"]);
    property((_, __, after) => {
      for (const b of after.buffers) if (b.status === "loaded" && b.mode === "edit" && !b.pinned) throw new Error(`${b.path} editing preview`);
    });
  });

  it("bufferReducer_should_DeriveMruAndRestoreTabsPinned", () => {
    let s = run(initialBufferState(), pinned("a.go"), pinned("b.go"), pinned("c.go"), { type: "activate", path: "a.go" });
    expect(mruPaths(s)).toEqual(["a.go", "c.go", "b.go"]);
    s = run(initialBufferState(), { type: "restore", sessionId: S1, tabs: [{ path: "x.go", viewState: vs(5) }, { path: "y.go", viewState: vs(9) }], activeIndex: 1 });
    expect(selectedPath(s)).toBe("y.go");
    expect(s.buffers.every((b) => b.pinned && b.status === "loading" && b.sessionId === S1)).toBe(true);
    expect(get(s, "x.go").viewState).toEqual(vs(5));
  });

  it("navHistory_should_RecordOriginOnlyForFirstHit_And_NotPushPane_When_QuickfixStepping", () => {
    const hit = (path: string, line: number, chain: "start" | "step"): BufferAction => ({ type: "jump", path, sessionId: S1, line, col: 0, chain });
    let s = run(initialBufferState(), pinned("x.go"), { type: "recordViewState", path: "x.go", viewState: vs(12) });
    s = run(s, hit("a.go", 3, "start"), hit("b.go", 8, "step"), hit("c.go", 9, "step"));
    expect(s.nav.map((e) => [e.path, e.line])).toEqual([["x.go", 12], ["c.go", 9]]);
    expect(selectedPath(s)).toBe("c.go");
    expect(canGoBack(s)).toBe(true);
    expect(canGoForward(s)).toBe(false);
    s = run(s, { type: "navBack" });
    expect(selectedPath(s)).toBe("x.go");
    expect(get(s, "x.go").viewState.line).toBe(12);
    expect(canGoBack(s)).toBe(false);
  });

  it("bufferReducer_should_RequestOneReveal_When_JumpNavOrActivateChangesTheCursorTarget", () => {
    let s = run(initialBufferState(), pinned("a.go"), { type: "recordViewState", path: "a.go", viewState: vs(10) }, pinned("b.go"));
    s = run(s, { type: "jump", path: "a.go", sessionId: S1, line: 42, col: 3 });
    expect(s.pendingReveal).toMatchObject({ path: "a.go", line: 42, col: 3 });
    expect(get(s, "a.go").viewState).toMatchObject({ line: 42, col: 3 });
    const seq = s.pendingReveal!.seq;
    s = run(s, { type: "revealed", seq });
    expect(s.pendingReveal).toBeNull();
    s = run(s, { type: "revealed", seq: seq + 99 }, { type: "activate", path: "b.go" });
    expect(s.pendingReveal).toBeNull();
    s = run(s, { type: "activate", path: "a.go" });
    expect(s.pendingReveal).toMatchObject({ path: "a.go", line: 42 });
    expect(s.pendingReveal!.seq).toBeGreaterThan(seq);
  });

  it("bufferReducer_should_OpenPinnedAtLine_When_JumpTargetsANewPath", () => {
    const s = run(initialBufferState(), { type: "jump", path: "src/main.go", sessionId: S1, line: 42, col: 0, pinned: true });
    expect(get(s, "src/main.go")).toMatchObject({ pinned: true, status: "loading", viewState: { line: 42 } });
    expect(s.pendingReveal).toMatchObject({ path: "src/main.go", line: 42 });
  });

  it("bufferReducer_should_MoveTabAndKeepActive_When_MoveLeftOrRight", () => {
    let s = run(initialBufferState(), pinned("a.go"), pinned("b.go"), pinned("c.go"), { type: "activate", path: "b.go" });
    s = run(s, { type: "move", path: "b.go", delta: 1 });
    expect(paths(s)).toEqual(["a.go", "c.go", "b.go"]);
    expect(selectedPath(s)).toBe("b.go");
    s = run(s, { type: "move", path: "b.go", delta: 1 });
    expect(paths(s)).toEqual(["a.go", "c.go", "b.go"]);
    s = run(s, { type: "move", path: "a.go", delta: -1 });
    expect(paths(s)).toEqual(["a.go", "c.go", "b.go"]);
    expect(selectedPath(s)).toBe("b.go");
  });

  it("bufferReducer_should_AdoptTokenSilently_When_OnlyMtimeChangedAndLeaveEverythingElse", () => {
    let s = run(initialBufferState(), pinned("a.go"), loaded("a.go"), { type: "markDirty", path: "a.go", dirty: true });
    const before = get(s, "a.go");
    s = run(s, { type: "adoptToken", path: "a.go", token: "v1:5:2:h" });
    expect(get(s, "a.go")).toEqual({ ...before, token: "v1:5:2:h" });
    expect(run(s, { type: "adoptToken", path: "missing.go", token: "t" })).toBe(s);
  });
});
