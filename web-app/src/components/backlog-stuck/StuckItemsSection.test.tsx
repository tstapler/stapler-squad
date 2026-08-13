import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";

const mockUseStuckBacklogItems = jest.fn();

jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: () => mockUseStuckBacklogItems(),
}));

import { StuckItemsSection } from "./StuckItemsSection";

function makeItem(overrides: Partial<StuckBacklogItem> = {}): StuckBacklogItem {
  return {
    itemId: "f9fcef32-c27e-434d-b23f-c873c18afa92",
    title: "fix: benchmark job CI",
    status: "pr_pending",
    reason: StuckReason.PR_READY_UNMERGED,
    firstDetectedAt: timestampFromDate(new Date(Date.now() - 3 * 24 * 60 * 60 * 1000)),
    lastCheckedAt: timestampFromDate(new Date(Date.now() - 30 * 1000)),
    prNumber: 148,
    prUrl: "https://github.com/tstapler/stapler-squad/pull/148",
    context: "PR #148 green & mergeable, unmerged for 3 days",
    ...overrides,
  } as StuckBacklogItem;
}

function baseHookReturn(overrides: Partial<ReturnType<typeof mockUseStuckBacklogItems>> = {}) {
  return {
    items: [],
    isLoading: false,
    error: null,
    lastFetched: new Date(),
    refetch: jest.fn(),
    snooze: jest.fn(),
    resetRemediation: jest.fn(),
    bulkResetParkedRemediation: jest.fn().mockResolvedValue(0),
    triggerRemediationNow: jest.fn(),
    ...overrides,
  };
}

describe("StuckItemsSection", () => {
  beforeEach(() => {
    mockUseStuckBacklogItems.mockReset();
  });

  describe("empty vs filtered-empty vs loading vs error", () => {
    it("shows the reassuring empty state when there are zero stuck items", () => {
      mockUseStuckBacklogItems.mockReturnValue(baseHookReturn({ items: [] }));
      render(<StuckItemsSection />);
      expect(screen.getByTestId("stuck-items-empty").textContent).toMatch(
        /Nothing stuck — all backlog items are progressing/
      );
    });

    it("shows a loading affordance before the first successful fetch", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({ items: [], isLoading: true, lastFetched: null })
      );
      render(<StuckItemsSection />);
      expect(screen.getByTestId("stuck-items-loading")).toBeInTheDocument();
    });
  });

  describe("StuckItemsSection_should_showStaleBannerAndRetainList_When_RefreshPollFails", () => {
    it("shows a stale banner + Retry while keeping the last-good list rendered", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem()],
          error: new Error("network error"),
          lastFetched: new Date(Date.now() - 6 * 60 * 1000),
        })
      );
      render(<StuckItemsSection />);
      const banner = screen.getByTestId("stuck-items-stale-banner");
      expect(banner.textContent).toMatch(/Couldn't refresh stuck items/);
      expect(banner.textContent).toMatch(/6m ago/);
      expect(screen.getByTestId("stuck-items-retry")).toBeInTheDocument();
      // Stale list still rendered — never blanked.
      expect(screen.getByTestId("stuck-group-1")).toBeInTheDocument();
    });

    it("shows the full-body error state on a first-load failure (no prior data)", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({ items: [], error: new Error("boom"), lastFetched: null })
      );
      render(<StuckItemsSection />);
      expect(screen.getByTestId("stuck-items-error-full").textContent).toMatch(
        /Couldn't check for stuck items right now/
      );
    });
  });

  describe("StuckItemsSection_should_alwaysOfferRecoveryAction_When_InAnyDegradedState", () => {
    it("offers Retry from the first-load error state", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({ items: [], error: new Error("boom"), lastFetched: null })
      );
      render(<StuckItemsSection />);
      expect(screen.getByTestId("stuck-items-error-full").querySelector("button")).toBeInTheDocument();
    });

    it("offers Retry from the stale-banner state", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem()],
          error: new Error("boom"),
          lastFetched: new Date(),
        })
      );
      render(<StuckItemsSection />);
      expect(screen.getByTestId("stuck-items-retry")).toBeInTheDocument();
    });

    it("offers Clear filter from the filtered-empty state", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({ items: [makeItem({ reason: StuckReason.REWORK_CAP })] })
      );
      render(<StuckItemsSection />);
      fireEvent.click(screen.getByTestId(`stuck-filter-chip-${StuckReason.PR_READY_UNMERGED}`));
      expect(screen.getByTestId("stuck-items-clear-filter")).toBeInTheDocument();
    });
  });

  describe("filtered-empty → Clear filter", () => {
    it("names the active filter and clears back to All on click", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({ items: [makeItem({ reason: StuckReason.REWORK_CAP })] })
      );
      render(<StuckItemsSection />);
      fireEvent.click(screen.getByTestId(`stuck-filter-chip-${StuckReason.PR_READY_UNMERGED}`));
      expect(screen.getByTestId("stuck-items-filtered-empty").textContent).toMatch(
        /No stuck items match "PR ready to merge"/
      );
      fireEvent.click(screen.getByTestId("stuck-items-clear-filter"));
      expect(screen.getByTestId(`stuck-filter-chip-all`)).toHaveAttribute("aria-pressed", "true");
      expect(screen.queryByTestId("stuck-items-filtered-empty")).not.toBeInTheDocument();
    });
  });

  describe("StuckItemsSection_should_wrapChipsInRoleGroupWithAriaPressed_When_Rendered", () => {
    it("wraps filter chips in role=group with aria-pressed per chip", () => {
      mockUseStuckBacklogItems.mockReturnValue(baseHookReturn({ items: [makeItem()] }));
      render(<StuckItemsSection />);
      const group = screen.getByRole("group", { name: "Filter stuck items by reason" });
      expect(group).toBeInTheDocument();
      const allChip = screen.getByTestId("stuck-filter-chip-all");
      expect(allChip).toHaveAttribute("aria-pressed", "true");
      const prChip = screen.getByTestId(`stuck-filter-chip-${StuckReason.PR_READY_UNMERGED}`);
      expect(prChip).toHaveAttribute("aria-pressed", "false");
    });
  });

  describe("StuckItemsSection_should_useAriaLivePoliteForCount_When_CountChanges", () => {
    it("uses aria-live=polite on the count region, never role=alert", () => {
      mockUseStuckBacklogItems.mockReturnValue(baseHookReturn({ items: [makeItem()] }));
      render(<StuckItemsSection />);
      const count = screen.getByTestId("stuck-items-count");
      expect(count).toHaveAttribute("aria-live", "polite");
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });
  });

  describe("StuckItemsSection_should_showOtherReasonsBadge_When_SameItemInMultipleGroups", () => {
    it("shows the cross-reference badge on both cards when one item appears in two reason groups", () => {
      const shared = "96cc9eaa-0000-0000-0000-000000000000";
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [
            makeItem({ itemId: shared, reason: StuckReason.ABANDONED_REVIEW, prNumber: 0, prUrl: "" }),
            makeItem({ itemId: shared, reason: StuckReason.REWORK_CAP, prNumber: 0, prUrl: "" }),
          ],
        })
      );
      render(<StuckItemsSection />);
      const badges = screen.getAllByTestId("stuck-item-other-reasons-badge");
      expect(badges).toHaveLength(2);
      expect(badges[0].textContent).toMatch(/also stuck for 1 other reason/);
    });

    it("suppresses the badge once a reason filter narrows the item to a single visible card", () => {
      const shared = "96cc9eaa-0000-0000-0000-000000000000";
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [
            makeItem({ itemId: shared, reason: StuckReason.ABANDONED_REVIEW, prNumber: 0, prUrl: "" }),
            makeItem({ itemId: shared, reason: StuckReason.REWORK_CAP, prNumber: 0, prUrl: "" }),
          ],
        })
      );
      render(<StuckItemsSection />);
      fireEvent.click(screen.getByTestId(`stuck-filter-chip-${StuckReason.REWORK_CAP}`));
      expect(screen.queryByTestId("stuck-item-other-reasons-badge")).not.toBeInTheDocument();
    });
  });

  describe("grouping and ordering", () => {
    it("groups items under a heading with the reason label and count", () => {
      mockUseStuckBacklogItems.mockReturnValue(baseHookReturn({ items: [makeItem()] }));
      render(<StuckItemsSection />);
      expect(screen.getByRole("heading", { level: 3, name: "PR ready to merge (1)" })).toBeInTheDocument();
    });

    it("sorts items within a group stuck-longest-first", () => {
      const older = makeItem({
        itemId: "older",
        firstDetectedAt: timestampFromDate(new Date(Date.now() - 10 * 24 * 60 * 60 * 1000)),
        title: "older item",
      });
      const newer = makeItem({
        itemId: "newer",
        firstDetectedAt: timestampFromDate(new Date(Date.now() - 1 * 24 * 60 * 60 * 1000)),
        title: "newer item",
      });
      mockUseStuckBacklogItems.mockReturnValue(baseHookReturn({ items: [newer, older] }));
      render(<StuckItemsSection />);
      const titles = screen.getAllByText(/older item|newer item/).map((el) => el.textContent);
      expect(titles).toEqual(["older item", "newer item"]);
    });

    // Regression guard for backlog/plan-approval-flicker: GROUP_ORDER is a
    // manually-maintained array, not compile-checked exhaustive over
    // StuckReason like stuckReason.ts's label/icon/class maps are — a reason
    // present in the item list but absent from GROUP_ORDER is silently never
    // rendered even though it still counts toward the header total, which is
    // exactly the "count says N, list shows fewer" mismatch this feature
    // exists to avoid. plan_not_approved, spawn_failed, pr_pending_no_pr,
    // rework_blocked_stale, pr_needs_fix, and respawn_blocked_active were all
    // found missing here (discovered via this fix's e2e test never finding a
    // seeded plan_not_approved card despite "1 stuck" showing in the header).
    it.each(
      Object.values(StuckReason)
        .filter((v): v is StuckReason => typeof v === "number" && v !== StuckReason.UNSPECIFIED)
    )("renders a group heading for every non-UNSPECIFIED StuckReason value (%i)", (reason) => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({ items: [makeItem({ reason, itemId: `item-${reason}` })] })
      );
      render(<StuckItemsSection />);
      expect(screen.getByTestId(`stuck-group-${reason}`)).toBeInTheDocument();
    });
  });

  describe("StuckItemsSection_should_offerBulkResetParked_When_AnyItemHasHitTheAttemptCap", () => {
    it("hides the reset-parked button when no item has hit the attempt cap", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({ items: [makeItem({ remediationAttempts: 1 })] })
      );
      render(<StuckItemsSection />);
      expect(screen.queryByTestId("stuck-items-reset-parked")).not.toBeInTheDocument();
    });

    it("shows the reset-parked button with a count when items have parked", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem({ remediationAttempts: 5 }), makeItem({ itemId: "second", remediationAttempts: 5 })],
        })
      );
      render(<StuckItemsSection />);
      expect(screen.getByTestId("stuck-items-reset-parked").textContent).toMatch(/Reset all parked \(2\)/);
    });

    it("calls bulkResetParkedRemediation and shows the resulting count on click", async () => {
      const bulkResetParkedRemediation = jest.fn().mockResolvedValue(2);
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem({ remediationAttempts: 5 })],
          bulkResetParkedRemediation,
        })
      );
      render(<StuckItemsSection />);
      fireEvent.click(screen.getByTestId("stuck-items-reset-parked"));
      expect(bulkResetParkedRemediation).toHaveBeenCalledTimes(1);
      await screen.findByText(/Reset 2 parked items\./);
    });

    it("shows an error message when the bulk reset call rejects", async () => {
      const bulkResetParkedRemediation = jest.fn().mockRejectedValue(new Error("network down"));
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem({ remediationAttempts: 5 })],
          bulkResetParkedRemediation,
        })
      );
      render(<StuckItemsSection />);
      fireEvent.click(screen.getByTestId("stuck-items-reset-parked"));
      const message = await screen.findByTestId("stuck-items-reset-parked-message");
      expect(message.textContent).toMatch(/network down/);
    });
  });
});
