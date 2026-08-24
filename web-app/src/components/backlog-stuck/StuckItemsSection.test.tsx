import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";

const mockUseStuckBacklogItems = jest.fn();
const mockGetBacklogItem = jest.fn();

jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: () => mockUseStuckBacklogItems(),
}));

// Only getBacklogItem is exercised by this file's tests (the fetch-on-expand
// mechanism that resolves currentReworkCapOverride) — the other methods are
// stubbed no-ops since no existing test path invokes them.
jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    getBacklogItem: mockGetBacklogItem,
    updateBacklogItem: jest.fn(),
    approvePlan: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    transitionStatus: jest.fn(),
  }),
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

  // backlog-bounce-escalation Story 2.1.3b: GROUP_ORDER omission-class
  // regression guard (see the file's own doc comment above GROUP_ORDER) for
  // the two new escalation reasons specifically.
  describe("StuckItemsSection_should_renderMultipleReasonsGroup_When_ItemEscalated", () => {
    it("renders the multiple_reasons group with a card for the escalated item", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem({ reason: StuckReason.MULTIPLE_REASONS, prNumber: 0, prUrl: "" })],
        })
      );
      render(<StuckItemsSection />);
      expect(screen.getByTestId(`stuck-group-${StuckReason.MULTIPLE_REASONS}`)).toBeInTheDocument();
      expect(
        screen.getByRole("heading", { level: 3, name: "Multiple reasons stuck (1)" })
      ).toBeInTheDocument();
    });
  });

  // backlog-bounce-escalation Story 2.1.3c / pre-mortem.md Failure #3: an
  // item's own multiple_reasons escalation row must not inflate its "other
  // reasons" badge — the badge should reflect only the genuinely independent
  // reasons (bouncing, push_failed), not the escalation row summarizing them.
  describe("StuckItemsSection_should_ExcludeEscalationReasonFromOtherReasonsCount_When_ItemHasMultipleReasonsRow", () => {
    it('shows "+1" (not "+2") on the bouncing card when the item also has push_failed and multiple_reasons rows', () => {
      const shared = "e5ca1a70-0000-0000-0000-000000000000";
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [
            makeItem({ itemId: shared, reason: StuckReason.BOUNCING, prNumber: 0, prUrl: "" }),
            makeItem({ itemId: shared, reason: StuckReason.PUSH_FAILED, prNumber: 0, prUrl: "" }),
            makeItem({ itemId: shared, reason: StuckReason.MULTIPLE_REASONS, prNumber: 0, prUrl: "" }),
          ],
        })
      );
      render(<StuckItemsSection />);
      const badges = screen.getAllByTestId("stuck-item-other-reasons-badge");
      const bouncingCard = screen
        .getAllByTestId("stuck-item")
        .find((c) => c.getAttribute("data-reason") === String(StuckReason.BOUNCING));
      expect(bouncingCard).toBeDefined();
      const bouncingBadge = bouncingCard!.querySelector('[data-testid="stuck-item-other-reasons-badge"]');
      expect(bouncingBadge).not.toBeNull();
      expect(bouncingBadge!.textContent).toMatch(/also stuck for 1 other reason/);
      // Sanity: there are badges on both the bouncing and push_failed cards
      // (the multiple_reasons card itself summarizes both, so it also gets a
      // badge — only the count on the non-escalation cards is under test here).
      expect(badges.length).toBeGreaterThanOrEqual(2);
    });
  });

  // backlog-bounce-escalation Story 2.1.4: de-escalation reuses the existing
  // justResolved ghost-card mechanism, but with copy that doesn't claim the
  // whole item resolved — only this card (the multiple_reasons escalation
  // row) is going away; the item remains open under its other reason(s).
  describe("StuckItemsSection_should_ShowDeescalationBanner_When_MultipleReasonsRowResolvesButItemRemainsOpen", () => {
    it("shows a de-escalation-flavored resolved banner instead of the card silently vanishing", () => {
      const shared = "de5ca1a7-0000-0000-0000-000000000000";
      const bouncingItem = makeItem({ itemId: shared, reason: StuckReason.BOUNCING, prNumber: 0, prUrl: "" });
      const escalationItem = makeItem({
        itemId: shared,
        reason: StuckReason.MULTIPLE_REASONS,
        prNumber: 0,
        prUrl: "",
      });

      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({ items: [bouncingItem, escalationItem] })
      );
      const { rerender } = render(<StuckItemsSection />);

      const escalationCard = screen
        .getAllByTestId("stuck-item")
        .find((c) => c.getAttribute("data-reason") === String(StuckReason.MULTIPLE_REASONS));
      expect(escalationCard).toBeDefined();
      fireEvent.click(escalationCard!);

      // Next poll: the multiple_reasons row has resolved, but bouncing is still open.
      mockUseStuckBacklogItems.mockReturnValue(baseHookReturn({ items: [bouncingItem] }));
      rerender(<StuckItemsSection />);

      const banner = screen.getByTestId("stuck-item-resolved-banner");
      expect(banner.textContent).toMatch(/No longer critical/);
      expect(banner.textContent).toMatch(/down to 1 open reason/);
      expect(banner.textContent).toMatch(/still open elsewhere in the list/);
      expect(banner.textContent).not.toMatch(/removed from this list shortly\./);
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

  describe("StuckItemsSection_should_offerPerReasonBulkReset_When_AGroupHasParkedItems", () => {
    it("hides a group's reset-parked button when none of its items have hit the attempt cap", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({ items: [makeItem({ remediationAttempts: 1 })] })
      );
      render(<StuckItemsSection />);
      expect(
        screen.queryByTestId(`stuck-group-reset-parked-${StuckReason.PR_READY_UNMERGED}`)
      ).not.toBeInTheDocument();
    });

    it("shows a group's reset-parked button with only that group's parked count", () => {
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [
            makeItem({ remediationAttempts: 5, reason: StuckReason.PR_READY_UNMERGED }),
            makeItem({
              itemId: "second",
              remediationAttempts: 5,
              reason: StuckReason.STALE_WORK,
            }),
            makeItem({
              itemId: "third",
              remediationAttempts: 1,
              reason: StuckReason.STALE_WORK,
            }),
          ],
        })
      );
      render(<StuckItemsSection />);
      expect(
        screen.getByTestId(`stuck-group-reset-parked-${StuckReason.PR_READY_UNMERGED}`).textContent
      ).toMatch(/Reset parked \(1\)/);
      expect(
        screen.getByTestId(`stuck-group-reset-parked-${StuckReason.STALE_WORK}`).textContent
      ).toMatch(/Reset parked \(1\)/);
    });

    it("calls bulkResetParkedRemediation scoped to just that reason and reports the count", async () => {
      const bulkResetParkedRemediation = jest.fn().mockResolvedValue(2);
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [
            makeItem({ remediationAttempts: 5, reason: StuckReason.PR_READY_UNMERGED }),
            makeItem({
              itemId: "second",
              remediationAttempts: 5,
              reason: StuckReason.STALE_WORK,
            }),
          ],
          bulkResetParkedRemediation,
        })
      );
      render(<StuckItemsSection />);
      fireEvent.click(screen.getByTestId(`stuck-group-reset-parked-${StuckReason.STALE_WORK}`));
      expect(bulkResetParkedRemediation).toHaveBeenCalledTimes(1);
      expect(bulkResetParkedRemediation).toHaveBeenCalledWith(StuckReason.STALE_WORK);
      await screen.findByText(/Reset 2 parked items in/);
    });

    it("leaves the global 'Reset all parked' button working unchanged alongside per-reason buttons", async () => {
      const bulkResetParkedRemediation = jest.fn().mockResolvedValue(3);
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [
            makeItem({ remediationAttempts: 5, reason: StuckReason.PR_READY_UNMERGED }),
            makeItem({
              itemId: "second",
              remediationAttempts: 5,
              reason: StuckReason.STALE_WORK,
            }),
          ],
          bulkResetParkedRemediation,
        })
      );
      render(<StuckItemsSection />);
      expect(screen.getByTestId("stuck-items-reset-parked").textContent).toMatch(/Reset all parked \(2\)/);
      fireEvent.click(screen.getByTestId("stuck-items-reset-parked"));
      expect(bulkResetParkedRemediation).toHaveBeenCalledWith(undefined);
      await screen.findByText(/Reset 3 parked items\./);
    });
  });

  // reworkCapOverride-current-value-display: StuckBacklogItem (this file's
  // makeItem()) doesn't carry reworkCapOverride, so on expand the section
  // fetches the full BacklogItem via getBacklogItem() and threads the result
  // down as StuckItemDetail's currentReworkCapOverride prop.
  describe("StuckItemsSection_should_FetchAndDisplayCurrentReworkCapOverride_When_ReworkCapItemIsExpanded", () => {
    beforeEach(() => {
      mockGetBacklogItem.mockReset();
    });

    it("fetches the item and displays its current override once expanded", async () => {
      mockGetBacklogItem.mockResolvedValue({ id: "item-1", reworkCapOverride: 7 });
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem({ itemId: "item-1", reason: StuckReason.REWORK_CAP, prNumber: 0, prUrl: "" })],
        })
      );
      render(<StuckItemsSection />);

      const card = screen
        .getAllByTestId("stuck-item")
        .find((c) => c.getAttribute("data-reason") === String(StuckReason.REWORK_CAP));
      expect(card).toBeDefined();
      fireEvent.click(card!);

      expect(mockGetBacklogItem).toHaveBeenCalledWith("item-1");
      const current = await screen.findByTestId("stuck-item-rework-cap-current");
      expect(current.textContent).toBe("7 rounds");
    });

    it("shows 'No override set' when the fetched item has no override", async () => {
      mockGetBacklogItem.mockResolvedValue({ id: "item-2", reworkCapOverride: undefined });
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem({ itemId: "item-2", reason: StuckReason.REWORK_CAP, prNumber: 0, prUrl: "" })],
        })
      );
      render(<StuckItemsSection />);

      const card = screen
        .getAllByTestId("stuck-item")
        .find((c) => c.getAttribute("data-reason") === String(StuckReason.REWORK_CAP));
      fireEvent.click(card!);

      expect(mockGetBacklogItem).toHaveBeenCalledWith("item-2");
      const current = await screen.findByTestId("stuck-item-rework-cap-current");
      expect(current.textContent).toBe("No override set (using global default)");
    });

    it("shows 'Unlimited' when the fetched item has an explicit override of 0", async () => {
      mockGetBacklogItem.mockResolvedValue({ id: "item-2", reworkCapOverride: 0 });
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem({ itemId: "item-2", reason: StuckReason.REWORK_CAP, prNumber: 0, prUrl: "" })],
        })
      );
      render(<StuckItemsSection />);

      const card = screen
        .getAllByTestId("stuck-item")
        .find((c) => c.getAttribute("data-reason") === String(StuckReason.REWORK_CAP));
      fireEvent.click(card!);

      expect(mockGetBacklogItem).toHaveBeenCalledWith("item-2");
      const current = await screen.findByTestId("stuck-item-rework-cap-current");
      await waitFor(() => expect(current.textContent).toBe("Unlimited"));
    });

    // Regression test for issue 2: getBacklogItem swallows RPC errors and
    // resolves to null (useBacklogService.ts's getBacklogItem) — a failed
    // fetch must not be permanently cached as "confirmed no override", or a
    // transient network blip would forever render the confident-but-false
    // "No override set" line. Asserting the retry (not a distinct loading
    // string, since StuckItemDetail renders the same fallback copy for
    // "not yet fetched" as for "confirmed unset") is what actually proves
    // the failure wasn't cached as a resolved value.
    it("does not permanently cache a failed fetch as 'no override' — retries on re-expand", async () => {
      mockGetBacklogItem.mockResolvedValueOnce(null);
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem({ itemId: "item-3", reason: StuckReason.REWORK_CAP, prNumber: 0, prUrl: "" })],
        })
      );
      render(<StuckItemsSection />);

      const card = screen
        .getAllByTestId("stuck-item")
        .find((c) => c.getAttribute("data-reason") === String(StuckReason.REWORK_CAP));
      expect(card).toBeDefined();

      // Expand: fetch fails (resolves null). A failed fetch must not be
      // permanently cached as a resolved value — the retry-count assertions
      // below are what actually prove that (the loading-state copy shown
      // meanwhile is identical to the pre-fetch state, so asserting it here
      // would be tautological).
      fireEvent.click(card!);
      await waitFor(() => expect(mockGetBacklogItem).toHaveBeenCalledTimes(1));

      // Collapse and re-expand: a genuinely-cached "no override" would never
      // call getBacklogItem again (see the caching test above) — a failed
      // fetch must retry instead.
      mockGetBacklogItem.mockResolvedValueOnce({ id: "item-3", reworkCapOverride: 9 });
      fireEvent.click(card!);
      fireEvent.click(card!);

      await waitFor(() => expect(mockGetBacklogItem).toHaveBeenCalledTimes(2));
      const retried = await screen.findByTestId("stuck-item-rework-cap-current");
      expect(retried.textContent).toBe("9 rounds");
    });
  });

  // Regression test for issue 4: StuckItemsSection.tsx:207-210 documents that
  // an already-fetched item must not be re-fetched on collapse/re-expand.
  describe("StuckItemsSection_should_NotRefetchReworkCapOverride_When_ItemIsCollapsedThenReExpanded", () => {
    beforeEach(() => {
      mockGetBacklogItem.mockReset();
    });

    it("fetches getBacklogItem exactly once across collapse + re-expand", async () => {
      mockGetBacklogItem.mockResolvedValue({ id: "item-4", reworkCapOverride: 4 });
      mockUseStuckBacklogItems.mockReturnValue(
        baseHookReturn({
          items: [makeItem({ itemId: "item-4", reason: StuckReason.REWORK_CAP, prNumber: 0, prUrl: "" })],
        })
      );
      render(<StuckItemsSection />);

      const card = screen
        .getAllByTestId("stuck-item")
        .find((c) => c.getAttribute("data-reason") === String(StuckReason.REWORK_CAP));
      expect(card).toBeDefined();

      fireEvent.click(card!); // expand — fetches
      await screen.findByTestId("stuck-item-rework-cap-current");
      fireEvent.click(card!); // collapse
      fireEvent.click(card!); // re-expand — must not re-fetch

      await screen.findByTestId("stuck-item-rework-cap-current");
      expect(mockGetBacklogItem).toHaveBeenCalledTimes(1);
    });
  });

  // Regression guard for a bug caught by adversarial review of PR #510:
  // useStuckBacklogItems() hands back a new `items` array reference on every
  // ~60s poll tick regardless of whether the data changed, and `focusItemId`
  // never clears itself from the URL. An effect keyed on `items` (rather than
  // gated by an "already applied for this focusItemId" ref) would re-run on
  // every poll and silently re-expand a card the user had just manually
  // collapsed.
  describe("focusItemId deep link (apply once per id)", () => {
    const originalScrollIntoView = Element.prototype.scrollIntoView;

    beforeEach(() => {
      // jsdom doesn't implement scrollIntoView; the auto-expand-on-focus effect calls it.
      Element.prototype.scrollIntoView = jest.fn();
    });

    afterEach(() => {
      Element.prototype.scrollIntoView = originalScrollIntoView;
    });

    it("expands the matching card once, and does not re-expand it after the user collapses it and a poll tick returns a new items array", async () => {
      const target = makeItem({ itemId: "target-item", reason: StuckReason.STALE_WORK });
      const firstItems = [target];
      mockUseStuckBacklogItems.mockReturnValue(baseHookReturn({ items: firstItems }));

      const { rerender } = render(<StuckItemsSection focusItemId="target-item" />);

      const findTargetCard = () =>
        screen.getAllByTestId("stuck-item").find((c) => c.getAttribute("data-item-id") === "target-item")!;

      await waitFor(() => expect(findTargetCard()).toHaveAttribute("aria-expanded", "true"));

      // User manually collapses the deep-linked card.
      fireEvent.click(findTargetCard());
      expect(findTargetCard()).toHaveAttribute("aria-expanded", "false");

      // Simulate a poll tick: same data, but a fresh array reference and a
      // fresh item object, and focusItemId is still on the URL.
      const polledItems = [makeItem({ itemId: "target-item", reason: StuckReason.STALE_WORK })];
      mockUseStuckBacklogItems.mockReturnValue(baseHookReturn({ items: polledItems }));
      rerender(<StuckItemsSection focusItemId="target-item" />);

      expect(findTargetCard()).toHaveAttribute("aria-expanded", "false");
    });
  });
});
