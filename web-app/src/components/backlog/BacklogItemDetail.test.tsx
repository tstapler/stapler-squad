/**
 * Tests for BacklogItemDetail (Epic 5.4, Tasks 5.4.1b, 5.4.1e, 5.4.1h, 5.4.1i).
 *
 * Covers:
 *  - AC10: distinct badge class per status (including `duplicate`) for this
 *    component's independently-maintained STATUS_CLASS map.
 *  - AC11: the 3-state canonical-item-link resolution (loading / resolved / missing),
 *    plus the archived-canonical edge case and keyboard (Enter) activation.
 */

import React from "react";
import { render, screen, cleanup, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem, KnownBacklogStatus } from "@/lib/hooks/useBacklogService";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockGetBacklogItem = jest.fn();
const mockTransitionStatus = jest.fn();
const mockTriggerTriage = jest.fn();
const mockSpawnSessionFromItem = jest.fn();
const mockApprovePlan = jest.fn();
const mockOverrideVerdict = jest.fn();
const mockTriggerReReview = jest.fn();
const mockArchiveBacklogItem = jest.fn();
const mockUpdateBacklogItem = jest.fn();

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    getBacklogItem: mockGetBacklogItem,
    transitionStatus: mockTransitionStatus,
    triggerTriage: mockTriggerTriage,
    spawnSessionFromItem: mockSpawnSessionFromItem,
    approvePlan: mockApprovePlan,
    overrideVerdict: mockOverrideVerdict,
    triggerReReview: mockTriggerReReview,
    archiveBacklogItem: mockArchiveBacklogItem,
    updateBacklogItem: mockUpdateBacklogItem,
    lastError: null,
  }),
}));

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "item-1",
    title: "Test item",
    status: "idea",
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

beforeEach(() => {
  jest.clearAllMocks();
});

afterEach(cleanup);

// ---------------------------------------------------------------------------
// Task 5.4.1b: all-statuses distinct class test
// ---------------------------------------------------------------------------

describe("BacklogItemDetail_should_RenderDistinctDuplicateClass_When_StatusIsDuplicate", () => {
  it("renders a distinct CSS class for every known status", async () => {
    const classByStatus: Record<string, string> = {};

    for (const status of ALL_STATUSES) {
      mockGetBacklogItem.mockResolvedValueOnce(makeItem({ id: `item-${status}`, status }));
      render(<BacklogItemDetail itemId={`item-${status}`} />);
      const badge = await screen.findByLabelText(/^Status:/);
      classByStatus[status] = badge.className;
      cleanup();
    }

    const uniqueClasses = new Set(Object.values(classByStatus));
    expect(uniqueClasses.size).toBe(ALL_STATUSES.length);
    expect(classByStatus.duplicate).not.toBe(classByStatus.archived);
  });
});

// ---------------------------------------------------------------------------
// Task 5.4.1e: 3-state canonical-item-link resolution
// ---------------------------------------------------------------------------

describe("BacklogItemDetail — duplicate-of link resolution", () => {
  it("BacklogItemDetail_should_ShowLoadingText_When_DuplicateOfFetchInFlight", async () => {
    const item = makeItem({ id: "dup-1", status: "duplicate", duplicateOfId: "canonical-1" });
    let resolveCanonical: (v: BacklogItem | null) => void = () => {};
    const canonicalPromise = new Promise<BacklogItem | null>((resolve) => {
      resolveCanonical = resolve;
    });
    mockGetBacklogItem.mockImplementation(async (id: string) => {
      if (id === "dup-1") return item;
      if (id === "canonical-1") return canonicalPromise;
      return null;
    });

    render(<BacklogItemDetail itemId="dup-1" />);

    // Assert on the substring "Loading", not the bare ellipsis (accessibility fix).
    expect(await screen.findByText(/Duplicate of:.*Loading/)).toBeInTheDocument();

    const row = screen.getByText(/Duplicate of:.*Loading/).closest("div");
    expect(row).toHaveAttribute("aria-live", "polite");

    // Resolve so the pending promise doesn't leak into other tests / act warnings.
    await act(async () => {
      resolveCanonical(null);
      await Promise.resolve();
    });
  });

  it("BacklogItemDetail_should_ShowClickableCanonicalLink_When_DuplicateOfResolves", async () => {
    const item = makeItem({ id: "dup-1", status: "duplicate", duplicateOfId: "canonical-1" });
    const canonical = makeItem({ id: "canonical-1", status: "idea", title: "Canonical Item" });
    mockGetBacklogItem.mockImplementation(async (id: string) => {
      if (id === "dup-1") return item;
      if (id === "canonical-1") return canonical;
      return null;
    });
    const onNavigateToItem = jest.fn();
    const user = userEvent.setup();

    render(<BacklogItemDetail itemId="dup-1" onNavigateToItem={onNavigateToItem} />);

    const link = await screen.findByTestId("duplicate-of-link");
    expect(link).toHaveTextContent("Duplicate of: Canonical Item");

    await user.click(link);
    expect(onNavigateToItem).toHaveBeenCalledWith("canonical-1");
  });

  it("BacklogItemDetail_should_ShowItemNotFoundText_When_DuplicateOfIsNull", async () => {
    const item = makeItem({ id: "dup-1", status: "duplicate", duplicateOfId: "canonical-1" });
    mockGetBacklogItem.mockImplementation(async (id: string) => {
      if (id === "dup-1") return item;
      if (id === "canonical-1") return null;
      return null;
    });
    const onClose = jest.fn();

    render(<BacklogItemDetail itemId="dup-1" onClose={onClose} />);

    expect(await screen.findByText("Duplicate of: (item not found)")).toBeInTheDocument();
    expect(screen.queryByTestId("duplicate-of-link")).not.toBeInTheDocument();

    // Panel stays usable: close button and title remain in the DOM.
    expect(screen.getByTestId("backlog-detail-close")).toBeInTheDocument();
    expect(screen.getByText(item.title)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Task 5.4.1h: archived canonical still resolves (not MISSING)
// ---------------------------------------------------------------------------

describe("BacklogItemDetail_should_ResolveArchivedCanonicalItem_When_DuplicateOfIsArchived", () => {
  it("resolves an archived canonical item to the RESOLVED (clickable) state", async () => {
    const item = makeItem({ id: "dup-1", status: "duplicate", duplicateOfId: "canonical-1" });
    const archivedCanonical = makeItem({
      id: "canonical-1",
      status: "archived",
      title: "Archived Canonical",
    });
    mockGetBacklogItem.mockImplementation(async (id: string) => {
      if (id === "dup-1") return item;
      if (id === "canonical-1") return archivedCanonical;
      return null;
    });

    render(<BacklogItemDetail itemId="dup-1" />);

    const link = await screen.findByTestId("duplicate-of-link");
    expect(link).toHaveTextContent("Duplicate of: Archived Canonical");
    expect(screen.queryByText("Duplicate of: (item not found)")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Task 5.4.1i: keyboard (Enter) activation of the resolved link
// ---------------------------------------------------------------------------

describe("BacklogItemDetail_should_ActivateLinkOnEnterKey_When_Focused", () => {
  it("calls onNavigateToItem when the focused link is activated via Enter", async () => {
    const item = makeItem({ id: "dup-1", status: "duplicate", duplicateOfId: "canonical-1" });
    const canonical = makeItem({ id: "canonical-1", status: "idea", title: "Canonical Item" });
    mockGetBacklogItem.mockImplementation(async (id: string) => {
      if (id === "dup-1") return item;
      if (id === "canonical-1") return canonical;
      return null;
    });
    const onNavigateToItem = jest.fn();
    const user = userEvent.setup();

    render(<BacklogItemDetail itemId="dup-1" onNavigateToItem={onNavigateToItem} />);

    const link = await screen.findByTestId("duplicate-of-link");
    link.focus();
    expect(link).toHaveFocus();

    await user.keyboard("{Enter}");

    expect(onNavigateToItem).toHaveBeenCalledWith("canonical-1");
  });
});
