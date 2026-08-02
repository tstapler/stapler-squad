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

// The generic .css.ts jest mock (src/__mocks__/styleMock.js) returns
// Proxy-wrapped functions, which React's dev-mode className validation
// silently refuses to set as a DOM attribute either way (confirmed:
// "Invalid value for prop `className`" warning, attribute never appears) —
// making the conditional outlierCell className unobservable via the DOM in
// tests. Override with real strings for this one module so the outlier test
// below can assert on the actual class attribute.
jest.mock("./SessionDetailDrawer.css", () => ({
  overlay: "overlay",
  drawer: "drawer",
  drawerHeader: "drawerHeader",
  drawerTitle: "drawerTitle",
  sessionIdChip: "sessionIdChip",
  closeButton: "closeButton",
  section: "section",
  sectionTitle: "sectionTitle",
  metaGrid: "metaGrid",
  metaLabel: "metaLabel",
  metaValue: "metaValue",
  toolsTable: "toolsTable",
  toolsTh: "toolsTh",
  toolsThRight: "toolsThRight",
  toolsTd: "toolsTd",
  toolsTdRight: "toolsTdRight",
  outlierCell: "outlierCell",
  skillList: "skillList",
  skillBadge: "skillBadge",
  emptyState: "emptyState",
  srOnly: "srOnly",
  backlogLink: "backlogLink",
}));

function makeSession(
  overrides: Partial<Omit<SessionTokenSummary, "$typeName" | "$unknown">> = {}
): SessionTokenSummary {
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

function makeTurn(
  overrides: Partial<Omit<TurnTokenStat, "$typeName" | "$unknown">> = {}
): TurnTokenStat {
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
    it("calls useSessionTurnTimeline with the session's conversationId", () => {
      const session = makeSession({ conversationId: "conversation-42" });
      render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

      expect(mockUseSessionTurnTimeline).toHaveBeenCalledWith("conversation-42");
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

  it("SessionDetailDrawer_should_flagOutlierCell_When_turnExceedsTwiceMean", () => {
    // totals: 15, 15, 1000 -> mean ~343.3 -> threshold ~686.7 -> only the
    // 1000-token turn exceeds it.
    mockUseSessionTurnTimeline.mockReturnValue({
      turns: [
        makeTurn({ model: "small-a", inputTokens: 10n, outputTokens: 5n }),
        makeTurn({ model: "small-b", inputTokens: 10n, outputTokens: 5n }),
        makeTurn({ model: "huge", inputTokens: 700n, outputTokens: 300n }),
      ],
      loading: false,
      error: null,
    });
    const session = makeSession();
    render(<SessionDetailDrawer session={session} onClose={jest.fn()} />);

    const hugeRow = screen.getByText("huge").closest("tr") as HTMLElement;
    const smallRow = screen.getByText("small-a").closest("tr") as HTMLElement;

    const hugeSpans = Array.from(hugeRow.querySelectorAll("td span"));
    const smallSpans = Array.from(smallRow.querySelectorAll("td span"));

    // Input + output cells: the outlier row's spans carry the outlierCell
    // class, the non-outlier row's spans (same markup, className={undefined})
    // don't.
    expect(hugeSpans).toHaveLength(2);
    expect(smallSpans).toHaveLength(2);
    hugeSpans.forEach((s) => expect(s.className).toContain("outlierCell"));
    smallSpans.forEach((s) => expect(s.className).toBe(""));
  });
});
