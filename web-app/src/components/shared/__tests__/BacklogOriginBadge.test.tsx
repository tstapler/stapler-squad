// Tests for BacklogOriginBadge — see describe/it names below for coverage.

import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { BacklogOriginBadge } from "../BacklogOriginBadge";
import type { BacklogIndexEntry } from "@/lib/hooks/useBacklogService";

function makeEntry(overrides: Partial<BacklogIndexEntry> = {}): BacklogIndexEntry {
  return {
    itemId: "item-123",
    itemTitle: "Fix the thing",
    itemStatus: "in_progress",
    sessionRole: "work",
    ...overrides,
  };
}

describe("BacklogOriginBadge", () => {
  it("BacklogOriginBadge_should_renderNull_When_noEntry", () => {
    const { container } = render(<BacklogOriginBadge />);
    expect(container).toBeEmptyDOMElement();
  });

  it("BacklogOriginBadge_should_renderNull_When_entryHasNoItemId", () => {
    const { container } = render(<BacklogOriginBadge entry={makeEntry({ itemId: "" })} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("BacklogOriginBadge_should_renderRoleLabel_When_linked", () => {
    render(<BacklogOriginBadge entry={makeEntry({ sessionRole: "review" })} />);
    expect(screen.getByTestId("backlog-origin-badge")).toBeTruthy();
    expect(screen.getByText("review")).toBeTruthy();
  });

  it("BacklogOriginBadge_should_linkToBacklogItem_When_linked", () => {
    render(<BacklogOriginBadge entry={makeEntry({ itemId: "abc-999" })} />);
    const link = screen.getByTestId("backlog-origin-badge");
    expect(link.getAttribute("href")).toBe("/backlog?item=abc-999");
  });

  it("BacklogOriginBadge_should_fallBackToGenericLabel_When_sessionRoleIsUnexpected", () => {
    render(<BacklogOriginBadge entry={makeEntry({ sessionRole: "some-future-role" })} />);
    expect(screen.getByText("backlog")).toBeTruthy();
  });

  it("BacklogOriginBadge_should_stopPropagation_When_clicked", () => {
    const parentHandler = jest.fn();
    render(
      <div onClick={parentHandler}>
        <BacklogOriginBadge entry={makeEntry()} />
      </div>
    );

    fireEvent.click(screen.getByTestId("backlog-origin-badge"));

    expect(parentHandler).not.toHaveBeenCalled();
  });
});
