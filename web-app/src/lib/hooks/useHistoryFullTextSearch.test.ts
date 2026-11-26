import { renderHook, waitFor, act } from "@testing-library/react";
import { useHistoryFullTextSearch } from "./useHistoryFullTextSearch";
import { createClient } from "@connectrpc/connect";

jest.mock("@connectrpc/connect");
jest.mock("@/gen/session/v1/session_pb", () => ({ SessionService: {} }));
jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

const mockSearchClaudeHistory = jest.fn();
const mockStreamSearchClaudeHistory = jest.fn();
let mockStreamingFlag = false;

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: () => mockStreamingFlag,
}));

(createClient as jest.Mock).mockReturnValue({
  searchClaudeHistory: mockSearchClaudeHistory,
  streamSearchClaudeHistory: mockStreamSearchClaudeHistory,
});

beforeEach(() => {
  jest.clearAllMocks();
  mockStreamingFlag = false;
  mockSearchClaudeHistory.mockResolvedValue({
    results: [],
    totalMatches: 0,
    queryTimeMs: 0n,
    hasMore: false,
  });
});

describe("useHistoryFullTextSearch", () => {
  it("useHistoryFullTextSearch_should_IncludeNewFlagsInRequest_When_OptionsSet", async () => {
    const { result } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));

    await act(async () => {
      await result.current.search({
        query: "dark mode toggle",
        project: "/repo",
        groupBySession: true,
        includeContext: true,
        excludeAutomationSessions: true,
        limit: 5,
      });
    });

    await waitFor(() => expect(mockSearchClaudeHistory).toHaveBeenCalled());
    const [payload] = mockSearchClaudeHistory.mock.calls[0];
    expect(payload.groupBySession).toBe(true);
    expect(payload.includeContext).toBe(true);
    expect(payload.excludeAutomationSessions).toBe(true);
    expect(payload.limit).toBe(5);
  });

  it("useHistoryFullTextSearch_should_OmitNewFieldsFromResult_When_OptionsNotSet", async () => {
    const { result } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));

    await act(async () => {
      await result.current.search({ query: "auth refactor" });
    });

    await waitFor(() => expect(mockSearchClaudeHistory).toHaveBeenCalled());
    const [payload] = mockSearchClaudeHistory.mock.calls[0];
    expect(payload.groupBySession).toBe(false);
    expect(payload.includeContext).toBe(false);
    expect(payload.excludeAutomationSessions).toBe(false);
  });

  it("useHistoryFullTextSearch_should_ConvertNewResultFields_When_Present", async () => {
    mockSearchClaudeHistory.mockResolvedValue({
      results: [
        {
          sessionId: "s1",
          sessionName: "Session 1",
          project: "/repo",
          messageIndex: 4,
          score: 5.5,
          snippets: [],
          metadata: undefined,
          moreMatchesInSessionCount: 2,
          contextWindow: [{ role: "user", content: "hi", timestamp: undefined, model: "" }],
          bookendFirst: [],
          bookendLast: [],
        },
      ],
      totalMatches: 3,
      queryTimeMs: 0n,
      hasMore: false,
    });
    const { result } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));

    await act(async () => {
      await result.current.search({ query: "dark mode toggle", groupBySession: true, includeContext: true });
    });

    await waitFor(() => expect(result.current.results).toHaveLength(1));
    expect(result.current.results[0].moreMatchesInSessionCount).toBe(2);
    expect(result.current.results[0].contextWindow).toHaveLength(1);
    expect(result.current.results[0].contextWindow[0].content).toBe("hi");
  });
});

describe("useHistoryFullTextSearch streaming (flag history_search_streaming)", () => {
  const result = (id: string) => ({
    sessionId: id, sessionName: id, project: "", messageIndex: 0, score: 1,
    snippets: [], contextWindow: [], bookendFirst: [], bookendLast: [], moreMatchesInSessionCount: 0,
  });

  it("useHistoryFullTextSearch_should_RenderResultsProgressively_When_StreamingMessagesArrive", async () => {
    mockStreamingFlag = true;
    let releaseSecond!: () => void;
    const gate = new Promise<void>((r) => { releaseSecond = r; });
    mockStreamSearchClaudeHistory.mockImplementation(async function* () {
      yield { results: [result("a"), result("b")], totalMatches: 3, queryTimeMs: 1n, hasMore: false, isFinal: false, indexWarming: false };
      await gate;
      yield { results: [result("c")], totalMatches: 3, queryTimeMs: 1n, hasMore: false, isFinal: true, indexWarming: false };
    });

    const { result: hook } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));
    let done!: Promise<void>;
    act(() => { done = hook.current.search({ query: "x" }); });

    await waitFor(() => expect(hook.current.results.map((r) => r.sessionId)).toEqual(["a", "b"]));
    expect(hook.current.loading).toBe(true);

    releaseSecond();
    await act(async () => { await done; });
    expect(hook.current.results.map((r) => r.sessionId)).toEqual(["a", "b", "c"]);
    expect(hook.current.loading).toBe(false);
    expect(mockSearchClaudeHistory).not.toHaveBeenCalled();
  });

  it("useHistoryFullTextSearch_should_ShowFirstBatchWithoutAnyTimerAdvance_And_NeverClearList_When_LaterBatchesArrive", async () => {
    mockStreamingFlag = true;
    let releaseSecond!: () => void;
    const gate = new Promise<void>((r) => { releaseSecond = r; });
    mockStreamSearchClaudeHistory.mockImplementation(async function* () {
      yield { results: [result("a")], totalMatches: 2, queryTimeMs: 1n, hasMore: false, isFinal: false, indexWarming: false };
      await gate;
      yield { results: [result("b")], totalMatches: 2, queryTimeMs: 1n, hasMore: false, isFinal: true, indexWarming: false };
    });

    jest.useFakeTimers();
    const { result: hook } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));
    const seen: number[] = [];
    let done!: Promise<void>;
    act(() => { done = hook.current.search({ query: "x" }); });

    try {
      // Only microtasks are flushed; no timer is advanced, so a debounce/throttle/timeout
      // between the first yield and the render would leave results empty here.
      for (let i = 0; i < 20; i++) await act(async () => { await Promise.resolve(); });
      expect(hook.current.results).toHaveLength(1);
      seen.push(hook.current.results.length);
      // Second batch is still gated, so the first batch is visible before the stream completes.
      expect(hook.current.loading).toBe(true);

      releaseSecond();
      await act(async () => { await done; });
      seen.push(hook.current.results.length);
      expect(seen).toEqual([1, 2]);
    } finally {
      jest.useRealTimers();
    }
  });

  it("useHistoryFullTextSearch_should_ExposeIndexWarmingUntilResultsArrive_When_ColdStartStream", async () => {
    mockStreamingFlag = true;
    let release!: () => void;
    const gate = new Promise<void>((r) => { release = r; });
    mockStreamSearchClaudeHistory.mockImplementation(async function* () {
      yield { results: [], totalMatches: 0, queryTimeMs: 0n, hasMore: false, isFinal: false, indexWarming: true };
      await gate;
      yield { results: [result("a")], totalMatches: 1, queryTimeMs: 2n, hasMore: false, isFinal: true, indexWarming: false };
    });

    const { result: hook } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));
    let done!: Promise<void>;
    act(() => { done = hook.current.search({ query: "x" }); });

    await waitFor(() => expect(hook.current.indexWarming).toBe(true));
    expect(hook.current.loading).toBe(true);
    expect(hook.current.results).toEqual([]);

    release();
    await act(async () => { await done; });
    expect(hook.current.indexWarming).toBe(false);
    expect(hook.current.results.map((r) => r.sessionId)).toEqual(["a"]);
  });

  it("useHistoryFullTextSearch_should_ShowWarmingPreviewThenReplaceWithRanked_When_ColdStartStream", async () => {
    mockStreamingFlag = true;
    let release!: () => void;
    const gate = new Promise<void>((r) => { release = r; });
    mockStreamSearchClaudeHistory.mockImplementation(async function* () {
      yield { results: [result("preview")], totalMatches: 0, queryTimeMs: 0n, hasMore: false, isFinal: false, indexWarming: true };
      await gate;
      yield { results: [result("ranked")], totalMatches: 1, queryTimeMs: 2n, hasMore: false, isFinal: true, indexWarming: false };
    });

    const { result: hook } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));
    let done!: Promise<void>;
    act(() => { done = hook.current.search({ query: "x" }); });

    await waitFor(() => expect(hook.current.results.map((r) => r.sessionId)).toEqual(["preview"]));
    expect(hook.current.indexWarming).toBe(true);

    release();
    await act(async () => { await done; });
    expect(hook.current.results.map((r) => r.sessionId)).toEqual(["ranked"]);
  });

  it("useHistoryFullTextSearch_should_KeepLoadedPages_When_AppendStreamStartsWithWarmingPreview", async () => {
    mockStreamingFlag = true;
    mockStreamSearchClaudeHistory.mockImplementationOnce(async function* () {
      yield { results: [result("page1")], totalMatches: 2, queryTimeMs: 1n, hasMore: true, isFinal: true, indexWarming: false };
    });
    mockStreamSearchClaudeHistory.mockImplementationOnce(async function* () {
      yield { results: [result("preview")], totalMatches: 0, queryTimeMs: 0n, hasMore: false, isFinal: false, indexWarming: true };
      yield { results: [result("page2")], totalMatches: 2, queryTimeMs: 1n, hasMore: false, isFinal: true, indexWarming: false };
    });

    const { result: hook } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));
    await act(async () => { await hook.current.search({ query: "x" }); });
    await act(async () => { await hook.current.search({ query: "x", append: true }); });

    expect(hook.current.results.map((r) => r.sessionId)).toEqual(["page1", "preview", "page2"]);
  });

  it("useHistoryFullTextSearch_should_StopLoadingAndClearWarming_When_StreamEndsWithoutFinal", async () => {
    mockStreamingFlag = true;
    mockStreamSearchClaudeHistory.mockImplementation(async function* () {
      yield { results: [result("a")], totalMatches: 5, queryTimeMs: 1n, hasMore: true, isFinal: false, indexWarming: false };
    });

    const { result: hook } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));
    await act(async () => { await hook.current.search({ query: "x" }); });

    expect(hook.current.loading).toBe(false);
    expect(hook.current.results).toHaveLength(1);
  });

  it("useHistoryFullTextSearch_should_ClearWarming_When_StreamErrors", async () => {
    mockStreamingFlag = true;
    mockStreamSearchClaudeHistory.mockImplementation(async function* () {
      yield { results: [], totalMatches: 0, queryTimeMs: 0n, hasMore: false, isFinal: false, indexWarming: true };
      throw new Error("unavailable");
    });

    const { result: hook } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));
    await act(async () => { await hook.current.search({ query: "x" }); });

    expect(hook.current.indexWarming).toBe(false);
    expect(hook.current.error).not.toBeNull();
  });

  it("useHistoryFullTextSearch_should_UseUnaryCall_When_FlagOff", async () => {
    const { result: hook } = renderHook(() => useHistoryFullTextSearch({ autoSearch: false }));
    await act(async () => { await hook.current.search({ query: "x" }); });
    expect(mockSearchClaudeHistory).toHaveBeenCalled();
    expect(mockStreamSearchClaudeHistory).not.toHaveBeenCalled();
  });
});
