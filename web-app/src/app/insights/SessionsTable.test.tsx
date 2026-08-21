import React from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { SessionTokenSummary } from "@/gen/session/v1/insights_pb";
import { SessionsTable } from "./SessionsTable";

function makeSession(overrides: Partial<SessionTokenSummary> = {}): SessionTokenSummary {
  return {
    sessionId: "session-1",
    conversationId: "conversation-1",
    projectPath: "/test/project",
    primaryModel: "claude-sonnet-4",
    totalInputTokens: BigInt(1000),
    totalOutputTokens: BigInt(500),
    cacheCreationTokens: BigInt(0),
    cacheReadTokens: BigInt(0),
    estimatedCostUsd: 0.0021,
    cacheHitRate: 0,
    messageCount: 5,
    firstMessageAt: undefined,
    lastMessageAt: undefined,
    isOrphan: false,
    skillActivations: [],
    topTools: [],
    unpricedModels: [],
    ...overrides,
  } as unknown as SessionTokenSummary;
}

describe("SessionsTable", () => {
  describe("SessionsTable_should_showUnpricedBadge_When_sessionHasUnpricedModels", () => {
    it("renders the cost value and an unpriced badge for that row", () => {
      const session = makeSession({
        estimatedCostUsd: 0,
        unpricedModels: ["claude-opus-6"],
      });
      render(<SessionsTable sessions={[session]} />);

      expect(screen.getByText("$0.0000")).toBeInTheDocument();
      expect(screen.getByText("unpriced")).toBeInTheDocument();
    });

    it("does not render a button/CTA alongside the badge", () => {
      const session = makeSession({
        estimatedCostUsd: 0,
        unpricedModels: ["claude-opus-6"],
      });
      render(<SessionsTable sessions={[session]} />);

      // The row itself may be a role="button" when onSessionClick is wired,
      // but no onSessionClick is passed here, so the row should carry no
      // button role. (The 4 sortable column headers are always role="button"
      // regardless of onSessionClick — scope this assertion to the tbody so
      // it doesn't false-fail on those.)
      const tbody = document.querySelector("tbody") as HTMLElement;
      expect(within(tbody).queryByRole("button")).toBeNull();
    });
  });

  describe("SessionsTable_should_omitUnpricedBadge_When_noUnpricedModels", () => {
    it("shows only the plain cost figure, no badge", () => {
      const session = makeSession({
        estimatedCostUsd: 0.0021,
        unpricedModels: [],
      });
      render(<SessionsTable sessions={[session]} />);

      expect(screen.getByText("$0.0021")).toBeInTheDocument();
      expect(screen.queryByText("unpriced")).toBeNull();
    });
  });

  describe("SessionsTable_should_triggerOnSessionClick_When_rowWithUnpricedBadgeClicked", () => {
    it("still calls onSessionClick when a badged row is clicked", async () => {
      const user = userEvent.setup();
      const session = makeSession({
        estimatedCostUsd: 0,
        unpricedModels: ["claude-opus-6"],
      });
      const onSessionClick = jest.fn();
      render(<SessionsTable sessions={[session]} onSessionClick={onSessionClick} />);

      // Scope to tbody: the 4 sortable column headers are also role="button".
      const tbody = document.querySelector("tbody") as HTMLElement;
      const row = within(tbody).getByRole("button");
      await user.click(row);

      expect(onSessionClick).toHaveBeenCalledTimes(1);
      expect(onSessionClick).toHaveBeenCalledWith(session);
    });
  });

  describe("SessionsTable_should_supportClickToSort_When_headerClicked", () => {
    it("sorts by cost descending on first click", async () => {
      const user = userEvent.setup();
      const cheap = makeSession({
        sessionId: "cheap",
        conversationId: "cheap",
        projectPath: "/proj/cheap",
        estimatedCostUsd: 0.01,
      });
      const expensive = makeSession({
        sessionId: "expensive",
        conversationId: "expensive",
        projectPath: "/proj/expensive",
        estimatedCostUsd: 2.5,
      });
      render(<SessionsTable sessions={[cheap, expensive]} />);

      const costHeader = screen.getByRole("columnheader", { name: /cost/i });
      await user.click(within(costHeader).getByRole("button"));

      expect(costHeader).toHaveAttribute("aria-sort", "descending");
      const firstRowCost = document.querySelectorAll("tbody tr")[0];
      expect(within(firstRowCost as HTMLElement).getByText("$2.50")).toBeInTheDocument();
    });

    it("toggles to ascending on second click of the same header", async () => {
      const user = userEvent.setup();
      const cheap = makeSession({ sessionId: "cheap", conversationId: "cheap", estimatedCostUsd: 0.01 });
      const expensive = makeSession({ sessionId: "expensive", conversationId: "expensive", estimatedCostUsd: 2.5 });
      render(<SessionsTable sessions={[cheap, expensive]} />);

      const costHeader = screen.getByRole("columnheader", { name: /cost/i });
      const sortButton = within(costHeader).getByRole("button");
      await user.click(sortButton);
      await user.click(sortButton);

      expect(costHeader).toHaveAttribute("aria-sort", "ascending");
      const firstRow = document.querySelectorAll("tbody tr")[0];
      expect(within(firstRow as HTMLElement).getByText("$0.010")).toBeInTheDocument();
    });

    it("sorts unpriced sessions last for cost regardless of direction", async () => {
      const user = userEvent.setup();
      const unpriced = makeSession({
        sessionId: "session-noprice",
        conversationId: "session-noprice",
        estimatedCostUsd: 0,
        unpricedModels: ["claude-opus-6"],
      });
      const priced = makeSession({ sessionId: "priced", conversationId: "priced", estimatedCostUsd: 1.0 });
      render(<SessionsTable sessions={[unpriced, priced]} />);

      const costHeader = screen.getByRole("columnheader", { name: /cost/i });
      const sortButton = within(costHeader).getByRole("button");
      await user.click(sortButton); // descending
      let rows = document.querySelectorAll("tbody tr");
      expect(within(rows[rows.length - 1] as HTMLElement).getByText("unpriced")).toBeInTheDocument();

      await user.click(sortButton); // ascending
      rows = document.querySelectorAll("tbody tr");
      expect(within(rows[rows.length - 1] as HTMLElement).getByText("unpriced")).toBeInTheDocument();
    });
  });
});
