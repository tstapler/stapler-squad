/**
 * Tests for BacklogItemCard (Epic 5.4, Tasks 5.4.1f, 5.4.1g).
 *
 * `getActionSpec` is not exported from BacklogItemCard.tsx, so these tests
 * assert its behavior through the rendered action button (data-testid and
 * aria-label), which is a faithful proxy for the same logic.
 *
 * Covers AC10: the `duplicate` status gets the "Duplicate" label (not the raw
 * status string) and its action button is a no-op on click (isDone: true
 * semantics, matching `done`/`archived`).
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { BacklogItemCard } from "./BacklogItemCard";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Test item",
    status: "duplicate",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions: [],
    statusEvents: [],
    ...overrides,
  };
}

describe("getActionSpec_should_ReturnDuplicateLabel_When_StatusIsDuplicate", () => {
  it("renders the 'Duplicate' label (not the raw status string) for a duplicate item", () => {
    render(
      <BacklogItemCard item={makeItem()} onAction={jest.fn()} onClick={jest.fn()} />
    );

    const button = screen.getByTestId("backlog-action-duplicate");
    expect(button).toHaveAccessibleName("Duplicate");
    expect(button).toHaveTextContent("Duplicate");
  });
});

describe("BacklogItemCard_should_NotInvokeOnAction_When_DuplicateButtonClicked", () => {
  it("does not invoke onAction when the duplicate action button is clicked", () => {
    const onAction = jest.fn();
    render(
      <BacklogItemCard item={makeItem()} onAction={onAction} onClick={jest.fn()} />
    );

    const button = screen.getByTestId("backlog-action-duplicate");
    fireEvent.click(button);

    expect(onAction).not.toHaveBeenCalled();
  });

  it("does not open the item detail either, since the click is contained to the action button", () => {
    const onAction = jest.fn();
    const onClick = jest.fn();
    render(
      <BacklogItemCard item={makeItem()} onAction={onAction} onClick={onClick} />
    );

    fireEvent.click(screen.getByTestId("backlog-action-duplicate"));

    expect(onAction).not.toHaveBeenCalled();
    expect(onClick).not.toHaveBeenCalled();
  });
});
