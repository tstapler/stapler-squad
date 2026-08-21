import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { StuckItem } from "./StuckItem";

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

describe("StuckItem", () => {
  describe("StuckItem_should_exposeButtonRoleAndAriaExpanded_When_Toggled", () => {
    it("renders role=button with aria-expanded=false when collapsed", () => {
      render(<StuckItem item={makeItem()} isExpanded={false} onToggleExpand={jest.fn()} />);
      const card = screen.getByTestId("stuck-item");
      expect(card).toHaveAttribute("role", "button");
      expect(card).toHaveAttribute("aria-expanded", "false");
    });

    it("calls onToggleExpand on Enter and Space", () => {
      const onToggleExpand = jest.fn();
      render(<StuckItem item={makeItem()} isExpanded={false} onToggleExpand={onToggleExpand} />);
      const card = screen.getByTestId("stuck-item");
      fireEvent.keyDown(card, { key: "Enter" });
      fireEvent.keyDown(card, { key: " " });
      expect(onToggleExpand).toHaveBeenCalledTimes(2);
    });

    it("calls onToggleExpand on Escape only while expanded", () => {
      const onToggleExpand = jest.fn();
      const { rerender } = render(
        <StuckItem item={makeItem()} isExpanded={false} onToggleExpand={onToggleExpand} />
      );
      fireEvent.keyDown(screen.getByTestId("stuck-item"), { key: "Escape" });
      expect(onToggleExpand).not.toHaveBeenCalled();

      rerender(<StuckItem item={makeItem()} isExpanded={true} onToggleExpand={onToggleExpand} />);
      fireEvent.keyDown(screen.getByTestId("stuck-item"), { key: "Escape" });
      expect(onToggleExpand).toHaveBeenCalledTimes(1);
    });

    it("mounts StuckItemDetail when expanded", () => {
      render(<StuckItem item={makeItem()} isExpanded={true} onToggleExpand={jest.fn()} />);
      expect(screen.getByTestId("stuck-item-detail")).toBeInTheDocument();
    });
  });

  describe("StuckItem_should_renderCouldntCheckChipNotGreen_When_LastCheckedOlderThan5Min", () => {
    it("shows the 'Couldn't check PR status' chip, never the healthy PR-ready chip, once stale", () => {
      const staleItem = makeItem({
        lastCheckedAt: timestampFromDate(new Date(Date.now() - 47 * 60 * 1000)),
      });
      render(<StuckItem item={staleItem} isExpanded={false} onToggleExpand={jest.fn()} />);
      const chip = screen.getByTestId("stuck-item-chip");
      expect(chip).toHaveAttribute("aria-label", "Couldn't check PR status");
      expect(chip.textContent).not.toMatch(/PR ready to merge/);
      expect(screen.getByTestId("stuck-item-last-checked").textContent).toMatch(/47m ago/);
    });

    it("shows the healthy 'PR ready to merge' chip when last_checked_at is fresh", () => {
      render(<StuckItem item={makeItem()} isExpanded={false} onToggleExpand={jest.fn()} />);
      expect(screen.getByTestId("stuck-item-chip")).toHaveAttribute(
        "aria-label",
        "PR ready to merge"
      );
    });
  });

  describe("StuckItem_should_truncateWithTitleTooltip_When_TitleIsLong", () => {
    it("keeps the full title as a title= tooltip even though the CSS truncates it visually", () => {
      const longTitle =
        "fix: an extremely long backlog item title that should truncate on narrow layouts";
      render(
        <StuckItem
          item={makeItem({ title: longTitle })}
          isExpanded={false}
          onToggleExpand={jest.fn()}
        />
      );
      expect(screen.getByText(longTitle)).toHaveAttribute("title", longTitle);
    });
  });

  describe("cross-reference badge", () => {
    it("shows 'also stuck for N other reason(s)' when otherReasonsCount > 0", () => {
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={false}
          onToggleExpand={jest.fn()}
          otherReasonsCount={1}
          otherReasonLabels={["Rework cap hit"]}
        />
      );
      expect(screen.getByTestId("stuck-item-other-reasons-badge").textContent).toMatch(
        /also stuck for 1 other reason/
      );
    });

    it("suppresses the badge when otherReasonsCount is 0", () => {
      render(<StuckItem item={makeItem()} isExpanded={false} onToggleExpand={jest.fn()} />);
      expect(screen.queryByTestId("stuck-item-other-reasons-badge")).not.toBeInTheDocument();
    });
  });

  describe("StuckItem_should_showResolvedConfirmationThenFade_When_ResolvesWhileExpanded", () => {
    it("renders an in-place resolved confirmation instead of the detail panel", () => {
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={true}
          onToggleExpand={jest.fn()}
          justResolved={true}
          resolvedMessage="PR #148 was merged."
        />
      );
      const banner = screen.getByTestId("stuck-item-resolved-banner");
      expect(banner.textContent).toMatch(/PR #148 was merged/);
      expect(screen.queryByTestId("stuck-item-detail")).not.toBeInTheDocument();
    });
  });

  describe("focus management (AC 29)", () => {
    it("returns focus to the card's toggle when it collapses", () => {
      const onToggleExpand = jest.fn();
      const { rerender } = render(
        <StuckItem item={makeItem()} isExpanded={true} onToggleExpand={onToggleExpand} />
      );
      const card = screen.getByTestId("stuck-item");
      rerender(<StuckItem item={makeItem()} isExpanded={false} onToggleExpand={onToggleExpand} />);
      expect(document.activeElement).toBe(card);
    });
  });

  describe("StuckItem_should_keepPickerOpenWithRetry_When_SnoozeStuckItemFails", () => {
    it("keeps the picker open with an error message and a retry action, and does not remove the item", async () => {
      const onSnooze = jest.fn().mockResolvedValue(false);
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={false}
          onToggleExpand={jest.fn()}
          onSnooze={onSnooze}
        />
      );

      fireEvent.click(screen.getByTestId("stuck-item-snooze-trigger"));
      expect(screen.getByTestId("stuck-item-snooze-picker")).toBeInTheDocument();

      fireEvent.click(screen.getByTestId("stuck-item-snooze-confirm"));

      await screen.findByTestId("stuck-item-snooze-error");
      expect(screen.getByTestId("stuck-item-snooze-error").textContent).toMatch(
        /Couldn.t snooze — try again/
      );
      // The confirm button doubles as Retry once in the error state.
      expect(screen.getByTestId("stuck-item-snooze-confirm").textContent).toMatch(/Retry/);
      // Picker stays open — item is still rendered, not removed.
      expect(screen.getByTestId("stuck-item")).toBeInTheDocument();
      expect(onSnooze).toHaveBeenCalledWith(
        "f9fcef32-c27e-434d-b23f-c873c18afa92",
        StuckReason.PR_READY_UNMERGED,
        expect.any(Date)
      );
    });

    it("calls onSnooze again when Retry is clicked after a failure", async () => {
      const onSnooze = jest.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={false}
          onToggleExpand={jest.fn()}
          onSnooze={onSnooze}
        />
      );

      fireEvent.click(screen.getByTestId("stuck-item-snooze-trigger"));
      fireEvent.click(screen.getByTestId("stuck-item-snooze-confirm"));
      await screen.findByTestId("stuck-item-snooze-error");

      fireEvent.click(screen.getByTestId("stuck-item-snooze-confirm"));
      await waitFor(() => expect(onSnooze).toHaveBeenCalledTimes(2));
    });
  });

  describe("StuckItem_should_showAlwaysOnSnoozeAffordance_When_HoverUnavailable", () => {
    const originalMatchMedia = window.matchMedia;

    afterEach(() => {
      window.matchMedia = originalMatchMedia;
    });

    function mockHoverUnavailable() {
      window.matchMedia = jest.fn().mockImplementation((query: string) => ({
        matches: true,
        media: query,
        onchange: null,
        addListener: jest.fn(),
        removeListener: jest.fn(),
        addEventListener: jest.fn(),
        removeEventListener: jest.fn(),
        dispatchEvent: jest.fn(),
      }));
    }

    it("renders an always-visible kebab affordance with a >=44x44px tap target when hover is unavailable", () => {
      mockHoverUnavailable();
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={false}
          onToggleExpand={jest.fn()}
          onSnooze={jest.fn()}
        />
      );

      const trigger = screen.getByTestId("stuck-item-snooze-trigger");
      expect(trigger).toBeInTheDocument();
      expect(trigger).toHaveAttribute("aria-label", "Snooze options");
      expect(trigger.textContent).toBe("⋮");
      expect(trigger.className).toMatch(/snoozeBtnAlwaysOn/);
    });

    it("renders the standard hover-reveal text button when hover is available", () => {
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={false}
          onToggleExpand={jest.fn()}
          onSnooze={jest.fn()}
        />
      );

      const trigger = screen.getByTestId("stuck-item-snooze-trigger");
      expect(trigger).toHaveAttribute("aria-label", "Snooze this item");
      expect(trigger.textContent).toBe("Snooze");
      expect(trigger.className).not.toMatch(/snoozeBtnAlwaysOn/);
    });

    it("omits the snooze control entirely when onSnooze is not provided", () => {
      render(<StuckItem item={makeItem()} isExpanded={false} onToggleExpand={jest.fn()} />);
      expect(screen.queryByTestId("stuck-item-snooze-trigger")).not.toBeInTheDocument();
    });
  });

  describe("StuckItem_should_snoozeInTwoClicks_When_ConfirmedWithDefaultDuration", () => {
    it("calls onSnooze once the picker is opened and confirmed", async () => {
      const onSnooze = jest.fn().mockResolvedValue(true);
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={false}
          onToggleExpand={jest.fn()}
          onSnooze={onSnooze}
        />
      );

      // Click 1: open the picker.
      fireEvent.click(screen.getByTestId("stuck-item-snooze-trigger"));
      expect(screen.getByTestId("stuck-item-snooze-picker")).toBeInTheDocument();

      // Click 2: confirm (default duration pre-selected — 1 day).
      fireEvent.click(screen.getByTestId("stuck-item-snooze-confirm"));

      await waitFor(() => expect(onSnooze).toHaveBeenCalledTimes(1));
      const [, reason] = onSnooze.mock.calls[0];
      expect(reason).toBe(StuckReason.PR_READY_UNMERGED);
    });

    it("closes the picker with no request sent when Cancel is clicked", () => {
      const onSnooze = jest.fn();
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={false}
          onToggleExpand={jest.fn()}
          onSnooze={onSnooze}
        />
      );

      fireEvent.click(screen.getByTestId("stuck-item-snooze-trigger"));
      fireEvent.click(screen.getByTestId("stuck-item-snooze-cancel"));

      expect(screen.queryByTestId("stuck-item-snooze-picker")).not.toBeInTheDocument();
      expect(onSnooze).not.toHaveBeenCalled();
    });

    it("does not toggle the card's own expand state when the snooze trigger is clicked", () => {
      const onToggleExpand = jest.fn();
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={false}
          onToggleExpand={onToggleExpand}
          onSnooze={jest.fn()}
        />
      );

      fireEvent.click(screen.getByTestId("stuck-item-snooze-trigger"));
      expect(onToggleExpand).not.toHaveBeenCalled();
    });
  });

  describe("StuckItem_should_exposeRetryNow_When_OnTriggerRemediationNowProvided", () => {
    it("does not render the retry control when onTriggerRemediationNow is omitted", () => {
      render(<StuckItem item={makeItem()} isExpanded={false} onToggleExpand={jest.fn()} />);
      expect(screen.queryByTestId("stuck-item-retry-now")).not.toBeInTheDocument();
    });

    it("calls onTriggerRemediationNow with itemId/reason and does not toggle expand", async () => {
      const onToggleExpand = jest.fn();
      const onTriggerRemediationNow = jest.fn().mockResolvedValue(undefined);
      render(
        <StuckItem
          item={makeItem({ reason: StuckReason.BOUNCING, remediationAttempts: 1 })}
          isExpanded={false}
          onToggleExpand={onToggleExpand}
          onTriggerRemediationNow={onTriggerRemediationNow}
        />
      );

      fireEvent.click(screen.getByTestId("stuck-item-retry-now"));
      await waitFor(() =>
        expect(onTriggerRemediationNow).toHaveBeenCalledWith(
          "f9fcef32-c27e-434d-b23f-c873c18afa92",
          StuckReason.BOUNCING
        )
      );
      expect(onToggleExpand).not.toHaveBeenCalled();
    });

    it("shows inline error text when the retry rejects", async () => {
      const onTriggerRemediationNow = jest.fn().mockRejectedValue(new Error("already parked"));
      render(
        <StuckItem
          item={makeItem({ reason: StuckReason.BOUNCING, remediationAttempts: 1 })}
          isExpanded={false}
          onToggleExpand={jest.fn()}
          onTriggerRemediationNow={onTriggerRemediationNow}
        />
      );

      fireEvent.click(screen.getByTestId("stuck-item-retry-now"));
      await waitFor(() =>
        expect(screen.getByTestId("stuck-item-retry-error").textContent).toMatch(/already parked/)
      );
    });

    it("disables the retry control once remediation_attempts reaches the cap", () => {
      render(
        <StuckItem
          item={makeItem({ reason: StuckReason.BOUNCING, remediationAttempts: 5 })}
          isExpanded={false}
          onToggleExpand={jest.fn()}
          onTriggerRemediationNow={jest.fn()}
        />
      );
      expect(screen.getByTestId("stuck-item-retry-now")).toBeDisabled();
    });
  });

  describe("StuckItem_should_scrollIntoView_When_FocusItemIdMatchesAndExpanded", () => {
    const originalScrollIntoView = Element.prototype.scrollIntoView;
    let scrollIntoView: jest.Mock;

    beforeEach(() => {
      scrollIntoView = jest.fn();
      Element.prototype.scrollIntoView = scrollIntoView;
    });

    afterEach(() => {
      Element.prototype.scrollIntoView = originalScrollIntoView;
    });

    it("scrolls the card into view when focusItemId matches and the card is expanded", () => {
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={true}
          onToggleExpand={jest.fn()}
          focusItemId="f9fcef32-c27e-434d-b23f-c873c18afa92"
        />
      );
      expect(scrollIntoView).toHaveBeenCalledWith({ block: "center" });
    });

    it("does not scroll when focusItemId does not match this card's itemId", () => {
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={true}
          onToggleExpand={jest.fn()}
          focusItemId="some-other-item-id"
        />
      );
      expect(scrollIntoView).not.toHaveBeenCalled();
    });

    it("does not scroll when focusItemId matches but the card is not expanded", () => {
      render(
        <StuckItem
          item={makeItem()}
          isExpanded={false}
          onToggleExpand={jest.fn()}
          focusItemId="f9fcef32-c27e-434d-b23f-c873c18afa92"
        />
      );
      expect(scrollIntoView).not.toHaveBeenCalled();
    });

    it("does not scroll when focusItemId is not provided", () => {
      render(<StuckItem item={makeItem()} isExpanded={true} onToggleExpand={jest.fn()} />);
      expect(scrollIntoView).not.toHaveBeenCalled();
    });
  });
});
