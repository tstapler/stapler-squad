import { renderHook, waitFor } from "@testing-library/react";
import { createClient } from "@connectrpc/connect";
import { useBackgroundSessions } from "./useBackgroundSessions";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: () => ({}),
}));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:0",
  createAuthInterceptor: () => (next: unknown) => next,
}));

const mockListSessions = jest.fn();
(createClient as jest.Mock).mockReturnValue({ listSessions: mockListSessions });

const session = (id: string, extra: Record<string, unknown> = {}) => ({ id, title: id, status: 4, ...extra });

describe("useBackgroundSessions", () => {
  beforeEach(() => jest.clearAllMocks());

  it("useBackgroundSessions_should_mapBackgroundOutcomeAndDefaultToUnknown_When_oldServerOmitsField", async () => {
    mockListSessions.mockResolvedValue({
      sessions: [
        session("a", { backgroundOutcome: "completed" }),
        session("b", { backgroundOutcome: "failed" }),
        session("c", { backgroundOutcome: "unknown" }),
        session("d", { backgroundOutcome: "future_value" }),
        session("e"),
      ],
    });
    const { result } = renderHook(() => useBackgroundSessions(true, 3_600_000));
    await waitFor(() => expect(result.current.sessions).toHaveLength(5));
    expect(result.current.sessions.map((s) => s.backgroundOutcome)).toEqual([
      "completed",
      "failed",
      "unknown",
      "unknown",
      "unknown",
    ]);
  });

  it("useBackgroundSessions_should_defaultEverySummaryToUnknown_When_responseLacksTheFieldEntirely", async () => {
    mockListSessions.mockResolvedValue({ sessions: [session("a"), session("b")] });
    const { result } = renderHook(() => useBackgroundSessions(true, 3_600_000));
    await waitFor(() => expect(result.current.sessions).toHaveLength(2));
    expect(result.current.sessions.every((s) => s.backgroundOutcome === "unknown")).toBe(true);
    expect(result.current.failed).toBe(false);
  });
});
