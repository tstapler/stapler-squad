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
    cacheRoiUsd: 0,
    wasteScore: undefined,
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

      // Both the Cost and Cost/Msg cells show "$0.0000" for a zero-cost
      // unpriced session, so assert at least one is present rather than a
      // single unique match.
      expect(screen.getAllByText("$0.0000").length).toBeGreaterThanOrEqual(1);
      // Two "unpriced" badges now render for an unpriced session: the Cost
      // cell and the Cache ROI cell (Story 1.3.1/1.3.3).
      expect(screen.getAllByText("unpriced").length).toBeGreaterThanOrEqual(1);
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

  describe("SessionsTable_should_stillOpenModalNotNavigate_When_rowClicked", () => {
    // Regression guard for Epic 1.4, Story 1.4.4: row click behavior is
    // unchanged by the new "Open full page" link/route — it must keep
    // opening the quick-peek modal via onSessionClick, never navigate.
    it("calls onSessionClick exactly once and performs no navigation", async () => {
      const user = userEvent.setup();
      const session = makeSession();
      const onSessionClick = jest.fn();
      const originalLocation = window.location.href;
      render(<SessionsTable sessions={[session]} onSessionClick={onSessionClick} />);

      const tbody = document.querySelector("tbody") as HTMLElement;
      const row = within(tbody).getByRole("button");
      await user.click(row);

      expect(onSessionClick).toHaveBeenCalledTimes(1);
      expect(onSessionClick).toHaveBeenCalledWith(session);
      expect(window.location.href).toBe(originalLocation);
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

      const costHeader = screen.getByRole("columnheader", { name: /^cost\s/i });
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

      const costHeader = screen.getByRole("columnheader", { name: /^cost\s/i });
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

      const costHeader = screen.getByRole("columnheader", { name: /^cost\s/i });
      const sortButton = within(costHeader).getByRole("button");
      await user.click(sortButton); // descending
      let rows = document.querySelectorAll("tbody tr");
      expect(within(rows[rows.length - 1] as HTMLElement).getAllByText("unpriced").length).toBeGreaterThanOrEqual(1);

      await user.click(sortButton); // ascending
      rows = document.querySelectorAll("tbody tr");
      expect(within(rows[rows.length - 1] as HTMLElement).getAllByText("unpriced").length).toBeGreaterThanOrEqual(1);
    });
  });

  describe("SessionsTable_should_setAriaSort_when_newColumnHeaderClicked", () => {
    it.each([
      ["Duration", "duration"],
      ["Cost/Msg", "cost/msg"],
      ["Cache ROI", "cache roi"],
      ["Waste Score", "waste score"],
    ])("sets aria-sort=descending on first click of %s", async (label, namePattern) => {
      const user = userEvent.setup();
      const a = makeSession({ sessionId: "a", conversationId: "a" });
      const b = makeSession({ sessionId: "b", conversationId: "b" });
      render(<SessionsTable sessions={[a, b]} />);

      const header = screen.getByRole("columnheader", { name: new RegExp(namePattern, "i") });
      await user.click(within(header).getByRole("button"));

      expect(header).toHaveAttribute("aria-sort", "descending");
    });
  });

  describe("SessionsTable_should_sortZeroMessageSessionLast_when_sortedByCostPerMessageEitherDirection", () => {
    it("keeps the 0-message session last on both ascending and descending clicks", async () => {
      const user = userEvent.setup();
      const zeroMessages = makeSession({
        sessionId: "zero-msg",
        conversationId: "zero-msg",
        messageCount: 0,
        estimatedCostUsd: 5,
      });
      const tenMessages = makeSession({
        sessionId: "ten-msg",
        conversationId: "ten-msg",
        messageCount: 10,
        estimatedCostUsd: 5,
      });
      render(<SessionsTable sessions={[zeroMessages, tenMessages]} />);

      const header = screen.getByRole("columnheader", { name: /cost\/msg/i });
      const sortButton = within(header).getByRole("button");

      const lastRowSessionId = () => {
        const rows = document.querySelectorAll("tbody tr");
        return rows[rows.length - 1].querySelector("td")?.getAttribute("title");
      };

      await user.click(sortButton); // descending
      expect(lastRowSessionId()).toBe("zero-msg");

      await user.click(sortButton); // ascending
      expect(lastRowSessionId()).toBe("zero-msg");
    });
  });

  describe("SessionsTable_should_renderSignedTextVsUnpricedBadge_when_cacheRoiRendered", () => {
    it("shows signed plain text for a negative ROI and the unpriced badge for an unpriced session", () => {
      const negativeRoi = makeSession({
        sessionId: "negative-roi",
        conversationId: "negative-roi",
        cacheRoiUsd: -0.42,
      });
      const unpriced = makeSession({
        sessionId: "unpriced-roi",
        conversationId: "unpriced-roi",
        unpricedModels: ["claude-opus-6"],
      });
      render(<SessionsTable sessions={[negativeRoi, unpriced]} />);

      expect(screen.getByText("-$0.42")).toBeInTheDocument();
      // Two unpriced badges now render for the unpriced row: one in Cost, one in Cache ROI.
      expect(screen.getAllByText("unpriced").length).toBeGreaterThanOrEqual(1);
    });
  });

  describe("SessionsTable_should_renderThreeDistinctWasteScoreStates_when_allThreeCasesPresent", () => {
    it("distinguishes not-evaluated, unpriced, and a real score", () => {
      const notEvaluated = makeSession({
        sessionId: "not-evaluated",
        conversationId: "not-evaluated",
        wasteScore: undefined,
      });
      const unpriced = makeSession({
        sessionId: "unpriced-waste",
        conversationId: "unpriced-waste",
        unpricedModels: ["claude-opus-6"],
        wasteScore: undefined,
      });
      const evaluated = makeSession({
        sessionId: "evaluated",
        conversationId: "evaluated",
        wasteScore: 62,
      });
      render(<SessionsTable sessions={[notEvaluated, unpriced, evaluated]} />);

      expect(screen.getByText("Not evaluated")).toBeInTheDocument();
      expect(screen.getByText("62")).toBeInTheDocument();
      // "—" also appears elsewhere (e.g. missing model/path cells are not
      // present here), so scope to the unpriced row's Waste Score cell.
      const unpricedRow = screen.getByTitle("unpriced-waste").closest("tr") as HTMLElement;
      const wasteCell = unpricedRow.querySelectorAll("td")[unpricedRow.querySelectorAll("td").length - 1];
      expect(wasteCell.textContent).toBe("—");
    });
  });

  describe("SessionsTable_should_preserve3SearchMatchedRows_when_wasteScoreHeaderClickedAfterSearch", () => {
    it("keeps the same search-matched row set after sorting by waste score", async () => {
      const user = userEvent.setup();
      const sessions = Array.from({ length: 600 }, (_, i) =>
        makeSession({
          sessionId: `session-${i}`,
          conversationId: `session-${i}`,
          projectPath: i < 3 ? `/needlezyx/${i}` : `/control-path/${i}`,
          wasteScore: i < 3 ? (i + 1) * 10 : undefined,
        })
      );
      render(<SessionsTable sessions={sessions} />);

      const searchInput = screen.getByLabelText("Search sessions by project path");
      await user.type(searchInput, "needlezyx");

      const matchedIdsBeforeSort = Array.from(document.querySelectorAll("tbody tr")).map(
        (tr) => tr.querySelector("td")?.getAttribute("title")
      );
      expect(matchedIdsBeforeSort).toHaveLength(3);

      const header = screen.getByRole("columnheader", { name: /waste score/i });
      await user.click(within(header).getByRole("button"));

      const matchedIdsAfterSort = Array.from(document.querySelectorAll("tbody tr")).map(
        (tr) => tr.querySelector("td")?.getAttribute("title")
      );
      expect(matchedIdsAfterSort).toHaveLength(3);
      expect(new Set(matchedIdsAfterSort)).toEqual(new Set(matchedIdsBeforeSort));

      // Descending waste-score sort: highest score (session-2, score 30) first.
      const firstRow = document.querySelectorAll("tbody tr")[0];
      expect(firstRow.querySelector("td")?.getAttribute("title")).toBe("session-2");
    });
  });
});
