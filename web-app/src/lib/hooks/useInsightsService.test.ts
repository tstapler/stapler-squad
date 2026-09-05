/**
 * Tests for useInsightsSummary (plan.md Epic 1.5 Story 1.5.4).
 *
 * Covers the per-session live-patch branch in useInsightsService.ts's
 * WatchInsights stream handler (lines ~109-133 per plan.md) — previously
 * unreachable in any test because the backend never populated
 * `InsightsEvent.session` on an "update" event until Story 1.5.3 landed.
 */

import { renderHook, act, waitFor } from "@testing-library/react";

const mockGetInsightsSummary = jest.fn();
const mockWatchInsights = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getInsightsSummary: (...args: unknown[]) => mockGetInsightsSummary(...args),
    watchInsights: (...args: unknown[]) => mockWatchInsights(...args),
  }),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => jest.fn(),
}));

import { useInsightsSummary } from "@/lib/hooks/useInsightsService";

/** Async-iterable test double with a manually-controlled event queue,
 * mirroring useWatchBacklogItems.test.ts's makeControllableStream. */
function makeControllableStream() {
  type QueueItem = { kind: "event"; value: unknown } | { kind: "done" };
  const queue: QueueItem[] = [];
  let notify: (() => void) | null = null;

  const push = (item: QueueItem) => {
    queue.push(item);
    const n = notify;
    notify = null;
    n?.();
  };

  const stream = {
    [Symbol.asyncIterator]: () => ({
      next: async () => {
        while (queue.length === 0) {
          await new Promise<void>((r) => {
            notify = r;
          });
        }
        const item = queue.shift()!;
        if (item.kind === "done") return { done: true, value: undefined };
        return { done: false, value: item.value };
      },
    }),
  };

  return {
    emit: (value: unknown) => push({ kind: "event", value }),
    end: () => push({ kind: "done" }),
    [Symbol.for("stream")]: stream,
    stream,
  };
}

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
}

function makeSession(overrides: Record<string, unknown> = {}) {
  return {
    sessionId: "sess-abc",
    conversationId: "abc",
    projectPath: "/proj",
    primaryModel: "claude-sonnet-4",
    totalInputTokens: 1000n,
    totalOutputTokens: 500n,
    cacheCreationTokens: 0n,
    cacheReadTokens: 0n,
    estimatedCostUsd: 9.99,
    cacheHitRate: 0,
    messageCount: 3,
    isOrphan: false,
    skillActivations: [],
    topTools: [],
    unpricedModels: [],
    ...overrides,
  };
}

function makeInitialSummary() {
  return {
    sessions: [makeSession({ conversationId: "abc", estimatedCostUsd: 1.0 })],
    totalCostUsd: 1.0,
    totalInputTokens: 1000n,
    totalOutputTokens: 500n,
    totalCacheReadTokens: 0n,
    overallCacheHitRate: 0,
    daily: [],
    models: [],
    topSkills: [],
    topTools: [],
    isLoading: false,
    unpricedModels: [],
    findings: [],
    activityBreakdown: [],
  };
}

describe("useInsightsSummary", () => {
  beforeEach(() => {
    mockGetInsightsSummary.mockReset();
    mockWatchInsights.mockReset();
    mockGetInsightsSummary.mockResolvedValue(makeInitialSummary());
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it("useInsightsSummary_should_patchSessionInPlaceWithNoRefetch_when_updateEventWithPopulatedSessionReceived", async () => {
    const controllable = makeControllableStream();
    mockWatchInsights.mockReturnValue(controllable.stream);

    const { result } = renderHook(() => useInsightsSummary());

    // Initial fetchSummary() resolves via mockGetInsightsSummary.
    await waitFor(() => expect(result.current.summary).not.toBeNull());
    expect(mockGetInsightsSummary).toHaveBeenCalledTimes(1);

    const patchedSession = makeSession({ conversationId: "abc", estimatedCostUsd: 9.99 });

    await act(async () => {
      controllable.emit({ eventType: "update", session: patchedSession, allParsed: true });
      await flush();
    });

    // Patched in place, keyed on conversationId — no second fetchSummary() call.
    expect(mockGetInsightsSummary).toHaveBeenCalledTimes(1);
    expect(result.current.summary?.sessions).toHaveLength(1);
    expect(result.current.summary?.sessions[0].estimatedCostUsd).toBe(9.99);
    expect(result.current.summary?.sessions[0].conversationId).toBe("abc");
  });

  it("useInsightsSummary_should_appendNewSession_when_updateEventConversationIdNotYetPresent", async () => {
    const controllable = makeControllableStream();
    mockWatchInsights.mockReturnValue(controllable.stream);

    const { result } = renderHook(() => useInsightsSummary());

    await waitFor(() => expect(result.current.summary).not.toBeNull());
    expect(mockGetInsightsSummary).toHaveBeenCalledTimes(1);

    const newSession = makeSession({ conversationId: "new-conv", estimatedCostUsd: 4.2 });

    await act(async () => {
      controllable.emit({ eventType: "update", session: newSession, allParsed: true });
      await flush();
    });

    expect(mockGetInsightsSummary).toHaveBeenCalledTimes(1);
    expect(result.current.summary?.sessions).toHaveLength(2);
    expect(
      result.current.summary?.sessions.some((s) => s.conversationId === "new-conv")
    ).toBe(true);
  });

  it("useInsightsSummary_should_refetch_when_parseCompleteEventReceived", async () => {
    const controllable = makeControllableStream();
    mockWatchInsights.mockReturnValue(controllable.stream);

    renderHook(() => useInsightsSummary());

    await waitFor(() => expect(mockGetInsightsSummary).toHaveBeenCalledTimes(1));

    await act(async () => {
      controllable.emit({ eventType: "parse_complete", allParsed: true });
      await flush();
    });

    await waitFor(() => expect(mockGetInsightsSummary).toHaveBeenCalledTimes(2));
  });
});
