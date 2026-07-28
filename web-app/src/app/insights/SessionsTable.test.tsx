import React from "react";
import { render, screen } from "@testing-library/react";
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
      // but no onSessionClick is passed here, so there should be no button role at all.
      expect(screen.queryByRole("button")).toBeNull();
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

      const row = screen.getByRole("button");
      await user.click(row);

      expect(onSessionClick).toHaveBeenCalledTimes(1);
      expect(onSessionClick).toHaveBeenCalledWith(session);
    });
  });
});
