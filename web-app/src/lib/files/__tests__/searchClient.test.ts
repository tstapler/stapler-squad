import { act, renderHook } from "@testing-library/react";
import { Code, ConnectError } from "@connectrpc/connect";
import { createFilesTestHarness } from "./fixtures";
import { useContentSearch } from "@/lib/hooks/useContentSearch";
import { classifySearchError, connectSearchStream, type SearchEvent, type SearchQuery, type SearchStream } from "../searchClient";

const q = (pattern: string, extra: Partial<SearchQuery> = {}): SearchQuery => ({
  pattern, regex: false, caseSensitive: false, wholeWord: false, include: [], exclude: [], includeIgnored: false, largeFiles: false, ...extra,
});
const hit = (path: string, line: number) => ({ path, line, text: `line ${line}`, start: 0, end: 4, byteOffset: 0 });
const stats = (over: Partial<Extract<SearchEvent, { type: "stats" }>> = {}): SearchEvent => ({
  type: "stats", filesScanned: 12, binarySkipped: 0, truncated: false, durationMs: 5, largeFilesSkipped: 0, ...over,
});

/** A stream the test drives by hand; records the pattern and signal of every request. */
function controllable() {
  const requests: { pattern: string; signal: AbortSignal; push: (e: SearchEvent) => void; end: () => void; fail: (e: unknown) => void }[] = [];
  const stream: SearchStream = (query, signal) => ({
    [Symbol.asyncIterator]() {
      const queue: SearchEvent[] = [];
      let wake: (() => void) | null = null;
      let done = false;
      let error: unknown = null;
      const poke = () => { wake?.(); wake = null; };
      requests.push({
        pattern: query.pattern, signal,
        push: (e) => { queue.push(e); poke(); },
        end: () => { done = true; poke(); },
        fail: (e) => { error = e; done = true; poke(); },
      });
      signal.addEventListener("abort", () => { done = true; poke(); });
      return {
        async next() {
          for (;;) {
            if (queue.length) return { value: queue.shift()!, done: false };
            if (error) throw error;
            if (done) return { value: undefined, done: true };
            await new Promise<void>((r) => { wake = r; });
          }
        },
      };
    },
  });
  return { stream, requests };
}

function setup(opts: { enabled?: boolean } = {}) {
  const h = createFilesTestHarness();
  const c = controllable();
  const view = renderHook((p: { enabled: boolean }) => useContentSearch({ sessionId: "s1", stream: c.stream, enabled: p.enabled }), {
    wrapper: h.wrapper, initialProps: { enabled: opts.enabled ?? true },
  });
  return { h, c, ...view };
}
const flush = () => act(async () => { await Promise.resolve(); await Promise.resolve(); });
const events = (h: ReturnType<typeof createFilesTestHarness>, name: string) => h.analytics.events.filter((e) => e.name === name);

beforeEach(() => jest.useFakeTimers());
afterEach(() => jest.useRealTimers());

describe("useContentSearch", () => {
  it("searchClient_should_SendOneRequest_When_TypingFooWithin300ms", async () => {
    const { c, result } = setup();
    act(() => result.current.search(q("f")));
    act(() => { jest.advanceTimersByTime(100); });
    act(() => result.current.search(q("fo")));
    act(() => { jest.advanceTimersByTime(100); });
    act(() => result.current.search(q("foo")));
    act(() => { jest.advanceTimersByTime(299); });
    expect(c.requests).toHaveLength(0);
    act(() => { jest.advanceTimersByTime(1); });
    await flush();
    expect(c.requests.map((r) => r.pattern)).toEqual(["foo"]);
  });

  it("searchClient_should_SendNothing_When_QueryIsOneCharacter", async () => {
    const { c, result } = setup();
    act(() => result.current.search(q("f")));
    act(() => { jest.advanceTimersByTime(1000); });
    await flush();
    expect(c.requests).toHaveLength(0);
    expect(result.current.status).toBe("idle");
  });

  it("searchClient_should_AbortPriorStreamAndIgnoreLateBatches_When_QueryChanges", async () => {
    const { c, result } = setup();
    act(() => result.current.search(q("foo")));
    act(() => { jest.advanceTimersByTime(300); });
    await flush();
    act(() => result.current.search(q("foob")));
    act(() => { jest.advanceTimersByTime(300); });
    await flush();
    expect(c.requests[0].signal.aborted).toBe(true);
    expect(c.requests[1].signal.aborted).toBe(false);
    act(() => c.requests[0].push({ type: "batch", matches: [hit("late.go", 1)] }));
    act(() => c.requests[1].push({ type: "batch", matches: [hit("new.go", 2)] }));
    await flush();
    expect(result.current.hits.map((m) => m.path)).toEqual(["new.go"]);
  });

  it("useContentSearch_should_EnterQueuedThenSearching_When_QueuedMessageThenBatch", async () => {
    const { h, c, result } = setup();
    act(() => result.current.runNow(q("foo")));
    await flush();
    act(() => c.requests[0].push({ type: "queued", ahead: 1 }));
    await flush();
    expect(result.current.status).toBe("queued");
    expect(result.current.ahead).toBe(1);
    expect(events(h, "files.search.queued")[0].labels).toMatchObject({ ahead: "1" });
    act(() => c.requests[0].push({ type: "batch", matches: [hit("a.go", 3)] }));
    await flush();
    expect(result.current.status).toBe("searching");
    expect(result.current.hits).toHaveLength(1);
  });

  it("useContentSearch_should_AbortAndReturnToIdleKeepingPriorResults_When_CancelWhileQueued", async () => {
    const { h, c, result } = setup();
    act(() => result.current.runNow(q("foo")));
    await flush();
    act(() => c.requests[0].push({ type: "batch", matches: [hit("a.go", 1)] }));
    act(() => c.requests[0].push(stats()));
    await flush();
    act(() => result.current.runNow(q("foob")));
    await flush();
    act(() => c.requests[1].push({ type: "queued", ahead: 2 }));
    await flush();
    act(() => result.current.cancel());
    expect(c.requests[1].signal.aborted).toBe(true);
    expect(result.current.status).toBe("idle");
    expect(result.current.hits.map((m) => m.path)).toEqual(["a.go"]);
    expect(events(h, "files.search.cancel")[0].labels).toMatchObject({ phase: "queued", results: "1-9" });
  });

  it("searchClient_should_KeepPartialResultsAndMarkCancelled_When_CancelWhileSearching", async () => {
    const { c, result } = setup();
    act(() => result.current.runNow(q("foo")));
    await flush();
    act(() => c.requests[0].push({ type: "batch", matches: [hit("a.go", 1), hit("a.go", 2)] }));
    await flush();
    act(() => result.current.cancel());
    expect(result.current.status).toBe("cancelled");
    expect(result.current.hits).toHaveLength(2);
  });

  it("useContentSearch_should_KeepPriorResultsAndSetInvalid_When_RegexRefused", async () => {
    const { h, c, result } = setup();
    act(() => result.current.runNow(q("foo")));
    await flush();
    act(() => c.requests[0].push({ type: "batch", matches: [hit("a.go", 1)] }));
    act(() => c.requests[0].push(stats()));
    await flush();
    act(() => result.current.runNow(q("(a", { regex: true })));
    await flush();
    act(() => c.requests[1].fail(new ConnectError("error parsing regexp: missing closing )", Code.InvalidArgument)));
    await flush();
    expect(result.current.status).toBe("invalid");
    expect(result.current.message).toContain("missing closing )");
    expect(result.current.hits.map((m) => m.path)).toEqual(["a.go"]);
    expect(events(h, "files.search.results").at(-1)?.labels).toMatchObject({ outcome: "invalid_regex" });
  });

  it("searchClient_should_EnterBusyStateWithoutAutoRetry_When_SearchBusyRefusal", async () => {
    const { h, c, result } = setup();
    act(() => result.current.runNow(q("foo")));
    await flush();
    act(() => c.requests[0].push({ type: "batch", matches: [hit("a.go", 1)] }));
    act(() => c.requests[0].push(stats()));
    await flush();
    act(() => result.current.runNow(q("foob")));
    await flush();
    act(() => c.requests[1].fail(new ConnectError("too many searches are running", Code.ResourceExhausted)));
    await flush();
    expect(result.current.status).toBe("busy");
    expect(result.current.hits).toHaveLength(1);
    act(() => { jest.advanceTimersByTime(60_000); });
    await flush();
    expect(c.requests).toHaveLength(2);
    act(() => result.current.retry());
    await flush();
    expect(c.requests).toHaveLength(3);
    expect(c.requests[2].pattern).toBe("foob");
    expect(events(h, "files.search.results").some((e) => e.labels?.outcome === "busy")).toBe(true);
  });

  it("searchClient_should_AbortStream_When_PanelClosedUnmountedOrDocumentHiddenOver30s", async () => {
    // panel closed
    const closed = setup();
    act(() => closed.result.current.runNow(q("foo")));
    await flush();
    closed.rerender({ enabled: false });
    expect(closed.c.requests[0].signal.aborted).toBe(true);
    closed.unmount();

    // unmounted
    const gone = setup();
    act(() => gone.result.current.runNow(q("foo")));
    await flush();
    gone.unmount();
    expect(gone.c.requests[0].signal.aborted).toBe(true);

    // hidden for 30 s
    const hiddenView = setup();
    act(() => hiddenView.result.current.runNow(q("foo")));
    await flush();
    const setVisibility = (v: "hidden" | "visible") => {
      Object.defineProperty(document, "visibilityState", { value: v, configurable: true });
      document.dispatchEvent(new Event("visibilitychange"));
    };
    act(() => setVisibility("hidden"));
    act(() => { jest.advanceTimersByTime(29_999); });
    expect(hiddenView.c.requests[0].signal.aborted).toBe(false);
    act(() => setVisibility("visible"));
    act(() => { jest.advanceTimersByTime(60_000); });
    expect(hiddenView.c.requests[0].signal.aborted).toBe(false);
    act(() => setVisibility("hidden"));
    act(() => { jest.advanceTimersByTime(30_000); });
    expect(hiddenView.c.requests[0].signal.aborted).toBe(true);
    act(() => setVisibility("visible"));
    hiddenView.unmount();
  });

  it("searchClient_should_EmitRunFirstResultAndTotalOnce_And_NeverTheQueryText_When_TypingFoo", async () => {
    const { h, c, result } = setup();
    act(() => result.current.search(q("f")));
    act(() => result.current.search(q("fo")));
    act(() => result.current.search(q("foo", { regex: true, include: ["**/*.go"] })));
    act(() => { jest.advanceTimersByTime(300); });
    await flush();
    expect(events(h, "files.search.run")).toHaveLength(1);
    expect(events(h, "files.search.run")[0].labels).toMatchObject({ query_len: "2-3", opts: "r1c0w0g1i0l0" });
    act(() => { jest.advanceTimersByTime(40); });
    act(() => c.requests[0].push({ type: "batch", matches: [hit("a.go", 1)] }));
    await flush();
    act(() => c.requests[0].push(stats({ filesScanned: 1500 })));
    await flush();
    const first = events(h, "files.perf.search_first_result_ms");
    expect(first).toHaveLength(1);
    expect(first[0].labels).toMatchObject({ scale: "1k-10k", queued: "false" });
    expect(first[0].durationMs).toBeGreaterThanOrEqual(40);
    expect(events(h, "files.perf.search_total_ms")[0].labels).toMatchObject({ outcome: "ok", scale: "1k-10k" });
    expect(events(h, "files.search.results")[0].labels).toMatchObject({ outcome: "ok", results: "1-9", files: "1-9", queued: "false" });
    expect(JSON.stringify(h.analytics.events)).not.toContain('"foo"');
  });

  it("searchClient_should_EmitNothing_When_TypingIsAbortedBeforeTheDebounceFires", async () => {
    const { h, result, unmount } = setup();
    act(() => result.current.search(q("foo")));
    unmount();
    act(() => { jest.advanceTimersByTime(1000); });
    expect(h.analytics.events.filter((e) => e.name.startsWith("files.search") || e.name.startsWith("files.perf.search"))).toHaveLength(0);
  });
});

describe("classifySearchError", () => {
  it("classifySearchError_should_MapConnectCodesToStates", () => {
    expect(classifySearchError(new ConnectError("x", Code.ResourceExhausted)).kind).toBe("busy");
    expect(classifySearchError(new ConnectError("bad regexp", Code.InvalidArgument))).toEqual({ kind: "invalid", message: "bad regexp" });
    expect(classifySearchError(new ConnectError("WorktreeMissing: session worktree is not on disk", Code.FailedPrecondition)).kind).toBe("worktree");
    expect(classifySearchError(new ConnectError("x", Code.Canceled)).kind).toBe("cancelled");
    expect(classifySearchError(new ConnectError("x", Code.Unavailable)).kind).toBe("error");
    expect(classifySearchError(new Error("boom")).kind).toBe("error");
  });
});

describe("connectSearchStream", () => {
  beforeEach(() => jest.useRealTimers());

  it("connectSearchStream_should_MapRequestOptionsAndStreamEvents_When_ServerStreams", async () => {
    let seen: { clientId: string; pattern: string; regex: boolean; caseSensitive: boolean; word: boolean; include: string[]; ignored: boolean } | null = null;
    createFilesTestHarness({
      handlers: {
        async *searchContent(req: any) {
          const o = req.options;
          seen = { clientId: req.clientId, pattern: o.pattern, regex: o.matchMode === 2, caseSensitive: o.caseMode === 1, word: o.wordMode === 2, include: o.scope.includeGlobs, ignored: o.scope.includeIgnored };
          yield { message: { case: "queued", value: { ahead: 1 } } };
          yield { message: { case: "batch", value: { matches: [{ path: "pkg/a.go", line: 12, text: "func needle()", startUtf16: 5, endUtf16: 11, byteOffset: 100n }] } } };
          yield { message: { case: "stats", value: { filesScanned: 3, binarySkipped: 1, truncated: true, durationMs: 7n, largeFilesSkipped: 0 } } };
        },
      } as never,
    });
    const out: SearchEvent[] = [];
    for await (const ev of connectSearchStream("s1", "tab-1")(q("needle", { regex: true, caseSensitive: true, wholeWord: true, include: ["**/*.go"], includeIgnored: true }), new AbortController().signal)) out.push(ev);
    expect(seen).toEqual({ clientId: "tab-1", pattern: "needle", regex: true, caseSensitive: true, word: true, include: ["**/*.go"], ignored: true });
    expect(out).toEqual([
      { type: "queued", ahead: 1 },
      { type: "batch", matches: [{ path: "pkg/a.go", line: 12, text: "func needle()", start: 5, end: 11, byteOffset: 100 }] },
      { type: "stats", filesScanned: 3, binarySkipped: 1, truncated: true, durationMs: 7, largeFilesSkipped: 0 },
    ]);
  });

  it("connectSearchStream_should_RejectWithWorktreeMissing_When_ServerRefuses", async () => {
    createFilesTestHarness({
      handlers: { async *searchContent() { throw new ConnectError("WorktreeMissing: session worktree is not on disk", Code.FailedPrecondition); } } as never,
    });
    const run = async () => { for await (const ev of connectSearchStream("s1", "tab-1")(q("needle"), new AbortController().signal)) void ev; };
    const err = await run().catch((e) => e);
    expect(classifySearchError(err).kind).toBe("worktree");
  });
});
