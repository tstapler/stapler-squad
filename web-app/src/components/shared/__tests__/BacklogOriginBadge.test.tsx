/**
 * Tests for BacklogOriginBadge component.
 *
 * Covers:
 *  - Renders nothing when there is no backlog-index entry (AC3: no badge, no layout shift)
 *  - Renders the role label + link when linked
 *  - Falls back to a generic label for an unexpected/free-form sessionRole value
 *  - Links to /backlog?item=<id>
 */

import React from "react";
import { render, screen } from "@testing-library/react";
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
});
