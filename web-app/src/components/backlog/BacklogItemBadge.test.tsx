/**
 * Tests for BacklogItemBadge (Epic 5.4, Task 5.4.1a).
 *
 * Covers AC10 (distinct badge class per status, including `duplicate`) for this
 * component's independently-maintained STATUS_CLASS map.
 */

import React from "react";
import { render, screen, cleanup } from "@testing-library/react";
import { BacklogItemBadge } from "./BacklogItemBadge";
import type { KnownBacklogStatus } from "@/lib/hooks/useBacklogService";

const ALL_STATUSES: KnownBacklogStatus[] = [
  "idea",
  "refining",
  "ready",
  "in_progress",
  "review",
  "done",
  "archived",
  "duplicate",
];

describe("BacklogItemBadge_should_RenderDistinctDuplicateClass_When_StatusIsDuplicate", () => {
  afterEach(cleanup);

  it("renders a distinct CSS class for every known status", () => {
    const classByStatus: Record<string, string> = {};

    for (const status of ALL_STATUSES) {
      render(
        <BacklogItemBadge itemTitle="Test item" status={status} acTotal={0} acDone={0} />
      );
      const chip = screen.getByLabelText(new RegExp(`^Status:`, "i"));
      classByStatus[status] = chip.className;
      cleanup();
    }

    // Every status must have a distinct class string.
    const uniqueClasses = new Set(Object.values(classByStatus));
    expect(uniqueClasses.size).toBe(ALL_STATUSES.length);

    // Specifically: duplicate must not reuse archived's class (the exact
    // regression pitfall this test exists to catch).
    expect(classByStatus.duplicate).not.toBe(classByStatus.archived);
  });

  it("labels the duplicate status chip as 'Duplicate'", () => {
    render(<BacklogItemBadge itemTitle="Test item" status="duplicate" acTotal={0} acDone={0} />);
    expect(screen.getByLabelText("Status: Duplicate")).toBeInTheDocument();
  });
});
