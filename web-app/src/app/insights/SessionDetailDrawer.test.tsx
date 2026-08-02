import React from "react";
import { render, screen } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { SessionDetailDrawer } from "./SessionDetailDrawer";
import {
  SessionTokenSummarySchema,
  TurnTokenStatSchema,
  type SessionTokenSummary,
  type TurnTokenStat,
} from "@/gen/session/v1/insights_pb";

const mockUseSessionTurnTimeline = jest.fn();

jest.mock("@/lib/hooks/useInsightsService", () => ({
  useSessionTurnTimeline: (conversationId: string | undefined) =>
    mockUseSessionTurnTimeline(conversationId),
}));

function makeSession(overrides: Partial<SessionTokenSummary> = {}): SessionTokenSummary {
  return create(SessionTokenSummarySchema, {
    sessionId: "session-1",
    conversationId: "conversation-1",
    projectPath: "/test/project",
    primaryModel: "claude-sonnet-4",
    estimatedCostUsd: 0.02,
    messageCount: 3,
    topTools: [],
    skillActivations: [],
    unpricedModels: [],
    ...overrides,
  });
}

function makeTurn(overrides: Partial<TurnTokenStat> = {}): TurnTokenStat {
  return create(TurnTokenStatSchema, {
    model: "claude-sonnet-4",
    inputTokens: 100n,
    outputTokens: 50n,
    cacheCreationTokens: 0n,
    cacheReadTokens: 0n,
    toolNames: [],
    ...overrides,
  });
}

describe("SessionDetailDrawer", () => {
  beforeEach(() => {
    mockUseSessionTurnTimeline.mockReset();
    mockUseSessionTurnTimeline.mockReturnValue({ turns: [], loading: false, error: null });
  });

  describe("SessionDetailDrawer_should_fetchTurnTimelineOnce_When_drawerOpensForSession", () => {
    it("calls useSessionTurnTimeline with the session's conversationId exactly once per render", () => {
      const session = makeSession({ conversationId: "conversation-42" });
      render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

      expect(mockUseSessionTurnTimeline).toHaveBeenCalledWith("conversation-42");
      expect(mockUseSessionTurnTimeline).toHaveBeenCalledTimes(1);
    });

    it("renders the Per-Turn Breakdown table when turns are present", () => {
      mockUseSessionTurnTimeline.mockReturnValue({
        turns: [makeTurn({ inputTokens: 100n, outputTokens: 50n })],
        loading: false,
        error: null,
      });
      const session = makeSession();
      render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

      expect(screen.getByText("Per-Turn Breakdown")).toBeInTheDocument();
      expect(screen.getByText("100")).toBeInTheDocument();
      expect(screen.getByText("50")).toBeInTheDocument();
    });
  });

  it("SessionDetailDrawer_should_showExactEmptyStateCopy_When_noTurnData", () => {
    mockUseSessionTurnTimeline.mockReturnValue({ turns: [], loading: false, error: null });
    const session = makeSession();
    render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

    expect(
      screen.getByText("No per-turn data available for this session.")
    ).toBeInTheDocument();
  });

  it("SessionDetailDrawer_should_renderHighestTokenTurnFirst_When_turnsVarySize", () => {
    mockUseSessionTurnTimeline.mockReturnValue({
      turns: [
        makeTurn({ model: "small-turn", inputTokens: 10n, outputTokens: 5n }),
        makeTurn({ model: "large-turn", inputTokens: 1000n, outputTokens: 500n }),
      ],
      loading: false,
      error: null,
    });
    const session = makeSession();
    render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

    const rows = document.querySelectorAll("tbody tr");
    // First row belongs to the "Tools Breakdown" table only if there were tools;
    // scope to rows containing "large-turn"/"small-turn" model text.
    const modelCells = Array.from(document.querySelectorAll("td")).filter((td) =>
      ["small-turn", "large-turn"].includes(td.textContent || "")
    );
    expect(modelCells[0].textContent).toBe("large-turn");
    expect(modelCells[1].textContent).toBe("small-turn");
    expect(rows.length).toBeGreaterThan(0);
  });
});
