/**
 * Tests for BacklogItemCard / BacklogBoard per-card pending state (board-level action feedback).
 *
 * Covers:
 *  1. No pendingAction: button shows its normal label and is enabled
 *  2. pendingAction matches this card's action: spinner + "Running…" shown, button disabled
 *  3. BacklogBoard: a pending action on one card doesn't disable a sibling card's button
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { BacklogItemCard } from "./BacklogItemCard";
import { BacklogBoard } from "./BacklogBoard";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Some backlog item",
    status: "idea",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [{ text: "Do the thing", status: "todo" } as never],
    linkedSessions: [],
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

describe("BacklogItemCard — per-card pending state", () => {
  it("renders the normal action label and an enabled button when nothing is pending", () => {
    render(<BacklogItemCard item={makeItem()} onAction={jest.fn()} onClick={jest.fn()} />);

    const button = screen.getByTestId("backlog-action-mark_ready");
    expect(button).toHaveTextContent("Mark Ready");
    expect(button).not.toBeDisabled();
  });

  it("shows a spinner and disables the button while its own action is pending", () => {
    render(
      <BacklogItemCard
        item={makeItem()}
        onAction={jest.fn()}
        onClick={jest.fn()}
        pendingAction="mark_ready"
      />
    );

    const button = screen.getByTestId("backlog-action-mark_ready");
    expect(button).toHaveTextContent("Running…");
    expect(button).toBeDisabled();
  });

});

describe("BacklogBoard — cross-card independence", () => {
  it("only disables the pending card's button, leaving a sibling card interactive", () => {
    const items = [
      makeItem({ id: "item-1", title: "First item" }),
      makeItem({ id: "item-2", title: "Second item" }),
    ];

    render(
      <BacklogBoard
        items={items}
        onAction={jest.fn()}
        onItemClick={jest.fn()}
        pending={{ "item-1": "mark_ready" }}
      />
    );

    const cards = screen.getAllByTestId("backlog-action-mark_ready");
    expect(cards).toHaveLength(2);
    expect(cards[0]).toBeDisabled();
    expect(cards[0]).toHaveTextContent("Running…");
    expect(cards[1]).not.toBeDisabled();
    expect(cards[1]).toHaveTextContent("Mark Ready");
  });
});
