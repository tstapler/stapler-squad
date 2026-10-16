import { renderHook, waitFor, act } from "@testing-library/react";
import { useHistoryFullTextSearch } from "./useHistoryFullTextSearch";
import { createClient } from "@connectrpc/connect";

jest.mock("@connectrpc/connect");
jest.mock("@/gen/session/v1/session_pb", () => ({ SessionService: {} }));
jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

const mockSearchClaudeHistory = jest.fn();

(createClient as jest.Mock).mockReturnValue({ searchClaudeHistory: mockSearchClaudeHistory });

beforeEach(() => {
  jest.clearAllMocks();
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
