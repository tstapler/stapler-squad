import React from "react";
import { render, screen } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { SessionDetailContent } from "./SessionDetailContent";
import {
  SessionTokenSummarySchema,
  TurnTokenStatSchema,
  type SessionTokenSummary,
  type TurnTokenStat,
} from "@/gen/session/v1/insights_pb";

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

describe("SessionDetailContent", () => {
  it("SessionDetailContent_should_renderPerTurnBreakdownTable_when_turnsPresent", () => {
    const session = makeSession();
    render(
      <SessionDetailContent
        session={session}
        turns={[makeTurn({ inputTokens: 100n, outputTokens: 50n })]}
      />
    );

    expect(screen.getByText("Per-Turn Breakdown")).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("50")).toBeInTheDocument();
  });

  it("SessionDetailContent_should_showExactEmptyStateCopy_when_noTurnData", () => {
    const session = makeSession();
    render(<SessionDetailContent session={session} turns={[]} />);

    expect(
      screen.getByText("No per-turn data available for this session.")
    ).toBeInTheDocument();
  });

  it("SessionDetailContent_should_renderHighestTokenTurnFirst_when_turnsVarySize", () => {
    const session = makeSession();
    render(
      <SessionDetailContent
        session={session}
        turns={[
          makeTurn({ model: "small-turn", inputTokens: 10n, outputTokens: 5n }),
          makeTurn({ model: "large-turn", inputTokens: 1000n, outputTokens: 500n }),
        ]}
      />
    );

    const rows = document.querySelectorAll("tbody tr");
    const modelCells = Array.from(document.querySelectorAll("td")).filter((td) =>
      ["small-turn", "large-turn"].includes(td.textContent || "")
    );
    expect(modelCells[0].textContent).toBe("large-turn");
    expect(modelCells[1].textContent).toBe("small-turn");
    expect(rows.length).toBeGreaterThan(0);
  });

  it("SessionDetailContent_should_flagOutlierCell_when_turnExceedsTwiceMean", () => {
    // totals: 15, 15, 1000 -> mean ~343.3 -> threshold ~686.7 -> only the
    // 1000-token turn exceeds it.
    const session = makeSession();
    render(
      <SessionDetailContent
        session={session}
        turns={[
          makeTurn({ model: "small-a", inputTokens: 10n, outputTokens: 5n }),
          makeTurn({ model: "small-b", inputTokens: 10n, outputTokens: 5n }),
          makeTurn({ model: "huge", inputTokens: 700n, outputTokens: 300n }),
        ]}
      />
    );

    const hugeRow = screen.getByText("huge").closest("tr") as HTMLElement;
    const smallRow = screen.getByText("small-a").closest("tr") as HTMLElement;

    const hugeSpans = Array.from(hugeRow.querySelectorAll("td span"));
    const smallSpans = Array.from(smallRow.querySelectorAll("td span"));

    expect(hugeSpans).toHaveLength(2);
    expect(smallSpans).toHaveLength(2);
    hugeSpans.forEach((s) => expect(s.className).toContain("outlierCell"));
    smallSpans.forEach((s) => expect(s.className).toBe(""));
  });

  describe("Tools Breakdown cost column", () => {
    // Story 1.2.5 AC: a double-counting-eligible tool cost renders with the
    // estimated marker ("~$"), a non-eligible one renders a plain "$" figure.
    it("SessionDetailContent_should_showEstimatedMarker_when_toolCostMayDoubleCount", () => {
      const session = makeSession({
        topTools: [
          {
            $typeName: "session.v1.TopToolEntry",
            toolName: "Read",
            callCount: 3,
            mcpServer: "",
            costUsd: 2.5,
            costMayDoubleCount: true,
            costUnpriced: false,
          },
          {
            $typeName: "session.v1.TopToolEntry",
            toolName: "Bash",
            callCount: 1,
            mcpServer: "",
            costUsd: 1,
            costMayDoubleCount: false,
            costUnpriced: false,
          },
        ],
      });
      render(<SessionDetailContent session={session} turns={[]} />);

      const readRow = screen.getByText("Read").closest("tr") as HTMLElement;
      const bashRow = screen.getByText("Bash").closest("tr") as HTMLElement;
      expect(readRow.textContent).toContain("~$2.50");
      expect(bashRow.textContent).toContain("$1.00");
      expect(bashRow.textContent).not.toContain("~");
    });

    // Story 1.2.5 AC: a tool with no priced turns renders "—", never "$0.00"
    // and never the estimated marker.
    it("SessionDetailContent_should_showDashNotZeroDollar_when_toolCostUnpriced", () => {
      const session = makeSession({
        topTools: [
          {
            $typeName: "session.v1.TopToolEntry",
            toolName: "Read",
            callCount: 50,
            mcpServer: "",
            costUsd: 0,
            costMayDoubleCount: false,
            costUnpriced: true,
          },
        ],
      });
      render(<SessionDetailContent session={session} turns={[]} />);

      expect(screen.queryByText("$0.0000")).not.toBeInTheDocument();
      expect(screen.queryByText(/~/)).not.toBeInTheDocument();
      const row = screen.getByText("Read").closest("tr") as HTMLElement;
      expect(row.textContent).toContain("—");
    });
  });

  it("SessionDetailContent_should_renderBacklogItemSection_when_backlogEntryProvided", () => {
    const session = makeSession();
    render(
      <SessionDetailContent
        session={session}
        turns={[]}
        backlogEntry={{
          itemId: "item-1",
          itemTitle: "Fix the thing",
          itemStatus: "in_progress",
          sessionRole: "primary",
        }}
      />
    );

    expect(screen.getByTestId("backlog-item-section")).toBeInTheDocument();
    expect(screen.getByText("Fix the thing")).toBeInTheDocument();
  });

  it("SessionDetailContent_should_renderSameOutputForSameProps_when_calledTwice", () => {
    // Verifies the shared-rendering guarantee (Epic 1.4, Story 1.4.2): the
    // modal and the route both render this component from the same inputs,
    // so identical props must produce identical output.
    const session = makeSession();
    const turns = [makeTurn({ inputTokens: 100n, outputTokens: 50n })];

    const first = render(<SessionDetailContent session={session} turns={turns} />);
    const firstHtml = first.container.innerHTML;
    first.unmount();

    const second = render(<SessionDetailContent session={session} turns={turns} />);
    expect(second.container.innerHTML).toBe(firstHtml);
  });
});
