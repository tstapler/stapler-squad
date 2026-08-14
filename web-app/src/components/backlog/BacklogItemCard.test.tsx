/**
 * Tests for BacklogItemCard / BacklogBoard per-card pending state (board-level action feedback).
 *
 * Covers:
 *  1. No pendingAction: button shows its normal label and is enabled
 *  2. pendingAction matches this card's action: spinner + "Running…" shown, button disabled
 *  3. BacklogBoard: a pending action on one card doesn't disable a sibling card's button
 */

import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { BacklogItemCard } from "./BacklogItemCard";
import { BacklogBoard } from "./BacklogBoard";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { useWatchBacklogItems } from "@/lib/hooks/useWatchBacklogItems";

// BacklogBoard (Epic 5.2, backlog-event-driven-updates) now sources its items
// from the live useWatchBacklogItems stream/store directly instead of an
// `items` prop — mock the hook so the "BacklogBoard" describe block below can
// still feed it fixture items without a real Redux store/ConnectRPC client.
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: jest.fn(),
}));
const mockUseWatchBacklogItems = useWatchBacklogItems as jest.Mock;

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

function makeStuckItem(overrides: Partial<StuckBacklogItem> = {}): StuckBacklogItem {
  return {
    itemId: "item-1",
    title: "Some backlog item",
    status: "in_progress",
    reason: StuckReason.PR_READY_UNMERGED,
    firstDetectedAt: timestampFromDate(new Date(Date.now() - 2 * 24 * 60 * 60 * 1000)),
    lastCheckedAt: timestampFromDate(new Date(Date.now() - 30 * 1000)),
    prNumber: 0,
    prUrl: "",
    context: "",
    ...overrides,
  } as StuckBacklogItem;
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

describe("BacklogItemCard — last-review verdict badge", () => {
  it("shows a FAIL badge when the item's most recent review verdict is FAIL", () => {
    render(
      <BacklogItemCard
        item={makeItem({ status: "in_progress", gateVerdict: "FAIL" })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    expect(screen.getByText("✗ FAIL")).toBeInTheDocument();
  });

  it("shows no badge when the item has never been reviewed", () => {
    render(
      <BacklogItemCard
        item={makeItem({ status: "in_progress", gateVerdict: undefined })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    expect(screen.queryByText(/PASS|FAIL|PARTIAL|UNVERIFIABLE/)).not.toBeInTheDocument();
  });

  it("shows no badge for a PENDING verdict (review still running, not a card-worthy signal)", () => {
    render(
      <BacklogItemCard
        item={makeItem({ status: "review", gateVerdict: "PENDING" })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    expect(screen.queryByText(/PENDING/)).not.toBeInTheDocument();
  });
});

describe("BacklogItemCard — flash on live update (Epic 6.1)", () => {
  afterEach(() => {
    jest.useRealTimers();
  });

  function cardEl() {
    return screen.getByTestId("backlog-item-card");
  }

  it("does not flash on initial mount even when liveVersion is already set", () => {
    render(
      <BacklogItemCard item={makeItem({ liveVersion: 1 })} onAction={jest.fn()} onClick={jest.fn()} />
    );

    expect(cardEl().className).not.toMatch(/justChanged/);
  });

  it("flashes when liveVersion changes after mount, then clears after ~250ms", () => {
    jest.useFakeTimers();
    const onAction = jest.fn();
    const onClick = jest.fn();
    const { rerender } = render(
      <BacklogItemCard item={makeItem({ liveVersion: 1 })} onAction={onAction} onClick={onClick} />
    );
    expect(cardEl().className).not.toMatch(/justChanged/);

    rerender(<BacklogItemCard item={makeItem({ liveVersion: 2 })} onAction={onAction} onClick={onClick} />);
    expect(cardEl().className).toMatch(/justChanged/);

    act(() => {
      jest.advanceTimersByTime(250);
    });
    expect(cardEl().className).not.toMatch(/justChanged/);
  });

  it("does not flash when liveVersion is undefined (item came from a one-shot fetch, not the watch stream)", () => {
    const onAction = jest.fn();
    const onClick = jest.fn();
    const { rerender } = render(
      <BacklogItemCard item={makeItem({ status: "in_progress" })} onAction={onAction} onClick={onClick} />
    );

    // Content changes but liveVersion stays undefined both times — no signal
    // to flash on (this is exactly the shape of e.g. listBacklogItems results).
    rerender(<BacklogItemCard item={makeItem({ status: "review" })} onAction={onAction} onClick={onClick} />);
    expect(cardEl().className).not.toMatch(/justChanged/);
  });

  it("does not flash on a snapshot/resync-driven update (liveVersion unchanged even though the item object is new)", () => {
    const onAction = jest.fn();
    const onClick = jest.fn();
    const { rerender } = render(
      <BacklogItemCard item={makeItem({ status: "in_progress", liveVersion: 3 })} onAction={onAction} onClick={onClick} />
    );

    // Simulates a resnapshot: a brand-new item object (possibly with
    // different field values) but the SAME liveVersion, because the
    // triggering event was is_snapshot: true and never bumped it.
    rerender(
      <BacklogItemCard item={makeItem({ status: "review", liveVersion: 3 })} onAction={onAction} onClick={onClick} />
    );
    expect(cardEl().className).not.toMatch(/justChanged/);
  });

  // pre-mortem.md #3: an update to one item must not force an unrelated
  // item's card to re-render. BacklogItemCard must stay wrapped in
  // React.memo for that guarantee to have any teeth — the actual
  // reference-stability half of the guarantee (does an unrelated item's
  // mapped object keep the same identity across an unrelated live update?)
  // is exercised end-to-end in useWatchBacklogItems.test.ts, where the
  // memoized-per-item mapping cache that makes memo effective actually
  // lives.
  it("stays wrapped in React.memo so unchanged props are never enough to trigger a re-render", () => {
    expect((BacklogItemCard as unknown as { $$typeof: symbol }).$$typeof).toBe(Symbol.for("react.memo"));
  });
});

describe("BacklogItemCard — canonical status label (Story 5.1.0)", () => {
  it("BacklogItemCard_should_RenderStatusLabelFromGetStatusLabel_When_ItemStatusIsReview", () => {
    render(
      <BacklogItemCard item={makeItem({ status: "review" })} onAction={jest.fn()} onClick={jest.fn()} />
    );

    expect(screen.getByTestId("backlog-item-card-status")).toHaveTextContent("Review");
    // Action button text (getActionSpec()) stays unchanged and unaffected.
    const button = screen.getByTestId("backlog-action-view_review");
    expect(button).toHaveTextContent("View Review");
  });

  it("BacklogItemCard_should_RenderQueuedStatusLabelIndependently_When_GetPrimaryCardActionFallsThroughToDisabledFallback", () => {
    render(
      <BacklogItemCard
        item={makeItem({
          status: "queued",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: undefined,
          triageStatus: undefined,
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    // getPrimaryCardAction() has no actionable next step for a gated queued
    // item with no plan and no failed-triage evidence — it falls to the
    // disabled "Queued" fallback — but the status label still reads the
    // canonical "Queued" from getStatusLabel(), independently.
    expect(screen.getByTestId("backlog-item-card-status")).toHaveTextContent("Queued");
    const button = screen.getByTestId("backlog-action-queued");
    expect(button).toHaveTextContent("Queued");
    expect(button).toBeDisabled();
  });

  it("BacklogItemCard_should_ShowApprovePlan_When_QueuedItemIsGatedWithAPlan", () => {
    render(
      <BacklogItemCard
        item={makeItem({
          status: "queued",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: "/plans/queued-item",
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    expect(screen.getByTestId("backlog-action-approve_plan")).toHaveTextContent("Approve Plan");
    expect(screen.queryByTestId("backlog-action-queued")).not.toBeInTheDocument();
  });

  it("BacklogItemCard_should_ShowRetryTriage_When_QueuedItemIsGatedWithFailedTriageAndNoPlan", () => {
    render(
      <BacklogItemCard
        item={makeItem({
          status: "queued",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: undefined,
          triageStatus: "failed",
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    expect(screen.getByTestId("backlog-action-retry_triage")).toHaveTextContent("Retry Triage");
  });

  it("BacklogItemCard_should_ShowDisabledQueuedFallback_When_QueuedItemPlanIsApprovedOrSkipped", () => {
    // getAvailableActions never grants spawn_session for "queued" — even
    // once the plan is approved/skipped, a queued item has no button to
    // press until it's dequeued into "ready" (or another status).
    const approved = render(
      <BacklogItemCard
        item={makeItem({
          status: "queued",
          skipPlanning: false,
          planApproved: true,
          planArtifactsPath: "/plans/queued-item",
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );
    expect(approved.getByTestId("backlog-action-queued")).toHaveTextContent("Queued");
    approved.unmount();

    render(
      <BacklogItemCard
        item={makeItem({ status: "queued", skipPlanning: true, planApproved: false })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );
    expect(screen.getByTestId("backlog-action-queued")).toHaveTextContent("Queued");
  });
});

describe("BacklogItemCard — ready-status primary action gating (plan approval gate fix)", () => {
  it("BacklogItemCard_should_ShowTriggerTriage_When_ReadyWithNoPlanAndNoFailedTriage", () => {
    render(
      <BacklogItemCard
        item={makeItem({
          status: "ready",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: undefined,
          triageStatus: undefined,
          repoPath: "/repo",
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    const button = screen.getByTestId("backlog-action-trigger_triage");
    expect(button).toHaveTextContent("Trigger Triage");
    expect(button).not.toBeDisabled();
  });

  it("BacklogItemCard_should_ShowApprovePlan_When_ReadyWithPlanAwaitingApproval", () => {
    render(
      <BacklogItemCard
        item={makeItem({
          status: "ready",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: "/plans/ready-item",
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    expect(screen.getByTestId("backlog-action-approve_plan")).toHaveTextContent("Approve Plan");
    expect(screen.queryByTestId("backlog-action-trigger_triage")).not.toBeInTheDocument();
  });

  it("BacklogItemCard_should_ShowSpawnSession_When_ReadyWithApprovedPlan", () => {
    render(
      <BacklogItemCard
        item={makeItem({
          status: "ready",
          skipPlanning: false,
          planApproved: true,
          planArtifactsPath: "/plans/ready-item",
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    expect(screen.getByTestId("backlog-action-spawn_session")).toHaveTextContent("Spawn Session");
  });

  it("BacklogItemCard_should_ShowSpawnSession_When_ReadyWithPlanningSkipped", () => {
    render(
      <BacklogItemCard
        item={makeItem({ status: "ready", skipPlanning: true, planApproved: false })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    expect(screen.getByTestId("backlog-action-spawn_session")).toHaveTextContent("Spawn Session");
  });

  it("BacklogItemCard_should_ShowRetryTriage_When_ReadyWithFailedTriageAndNoPlan", () => {
    render(
      <BacklogItemCard
        item={makeItem({
          status: "ready",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: undefined,
          triageStatus: "failed",
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    const button = screen.getByTestId("backlog-action-retry_triage");
    expect(button).toHaveTextContent("Retry Triage");
    expect(screen.queryByTestId("backlog-action-trigger_triage")).not.toBeInTheDocument();
  });
});

describe("BacklogItemCard — pending/triage disabling against a derived action", () => {
  it("BacklogItemCard_should_DisableAndShowRunning_When_PendingActionMatchesTheDerivedApprovePlanAction", () => {
    render(
      <BacklogItemCard
        item={makeItem({
          status: "ready",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: "/plans/ready-item",
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
        pendingAction="approve_plan"
      />
    );

    const button = screen.getByTestId("backlog-action-approve_plan");
    expect(button).toHaveTextContent("Running…");
    expect(button).toBeDisabled();
  });

  it("BacklogItemCard_should_DisableButtonRegardlessOfDerivedAction_When_TriageIsRunning", () => {
    render(
      <BacklogItemCard
        item={makeItem({
          status: "ready",
          skipPlanning: false,
          planApproved: true,
          planArtifactsPath: "/plans/ready-item",
          triageStatus: "running",
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    expect(screen.getByTestId("backlog-action-spawn_session")).toBeDisabled();
  });

  it("BacklogItemCard_should_StopShowingRunning_When_ALiveUpdateChangesTheDerivedActionWhilePendingActionStillReflectsThePriorOne", () => {
    // Simulates the plan being approved server-side (a live update bumps
    // item.liveVersion and flips planApproved) while `pendingAction` is
    // still "approve_plan" from before that update resolved. The new
    // derived action is "spawn_session" — isActionPending must correctly
    // become false rather than falsely showing "Running…" on a button
    // whose action no longer matches the in-flight one.
    const { rerender } = render(
      <BacklogItemCard
        item={makeItem({
          status: "ready",
          skipPlanning: false,
          planApproved: false,
          planArtifactsPath: "/plans/ready-item",
          liveVersion: 1,
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
        pendingAction="approve_plan"
      />
    );
    expect(screen.getByTestId("backlog-action-approve_plan")).toHaveTextContent("Running…");

    rerender(
      <BacklogItemCard
        item={makeItem({
          status: "ready",
          skipPlanning: false,
          planApproved: true,
          planArtifactsPath: "/plans/ready-item",
          liveVersion: 2,
        })}
        onAction={jest.fn()}
        onClick={jest.fn()}
        pendingAction="approve_plan"
      />
    );

    const button = screen.getByTestId("backlog-action-spawn_session");
    expect(button).toHaveTextContent("Spawn Session");
    // Still disabled — pendingAction !== null guards the button regardless
    // of which action is currently the primary one.
    expect(button).toBeDisabled();
  });
});

describe("BacklogItemCard — compact BlockerChip (Story 5.1.1)", () => {
  // Same benign jest/vanilla-extract mock warning silencing as
  // BlockerChip.test.tsx (getStuckReasonClass resolves through the mocked
  // .css.ts exports, which triggers an "Invalid value for prop className"
  // React warning that isn't a real bug).
  beforeAll(() => {
    jest.spyOn(console, "error").mockImplementation(() => {});
  });

  afterAll(() => {
    jest.restoreAllMocks();
  });

  it("BacklogItemCard_should_RenderCompactBlockerChip_When_StuckItemPropProvided", () => {
    render(
      <BacklogItemCard
        item={makeItem()}
        onAction={jest.fn()}
        onClick={jest.fn()}
        stuckItem={makeStuckItem({ reason: StuckReason.PR_READY_UNMERGED })}
      />
    );

    expect(screen.getByTestId("blocker-chip")).toBeInTheDocument();
    expect(screen.getByText("🟢")).toBeInTheDocument();
    expect(screen.getByText("PR ready to merge")).toBeInTheDocument();
    // Compact variant — no duration text in the card footer.
    expect(screen.queryByTestId("blocker-chip-duration")).not.toBeInTheDocument();
  });

  it("BacklogItemCard_should_OmitBlockerChipAndKeepFooterUnchanged_When_StuckItemPropUndefined", () => {
    render(<BacklogItemCard item={makeItem()} onAction={jest.fn()} onClick={jest.fn()} />);

    expect(screen.queryByTestId("blocker-chip")).not.toBeInTheDocument();
    // Footer still shows the action button as before.
    expect(screen.getByTestId("backlog-action-mark_ready")).toBeInTheDocument();
  });
});

describe("BacklogItemCard — GitHub provenance badge (Epic 4.1, backlog-github-two-way-sync)", () => {
  it("BacklogItemCard_should_RenderProvenanceBadge_When_ExternalUrlPresent", () => {
    render(
      <BacklogItemCard
        item={makeItem({ externalUrl: "https://github.com/acme/widget/issues/42", externalId: "42" })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    const badge = screen.getByRole("link", { name: "Imported from GitHub issue #42" });
    expect(badge).toHaveAttribute("href", "https://github.com/acme/widget/issues/42");
    expect(badge).toHaveAttribute("target", "_blank");
    expect(badge).toHaveTextContent("#42");
  });

  it("BacklogItemCard_should_OmitProvenanceBadge_When_ExternalUrlEmpty", () => {
    render(<BacklogItemCard item={makeItem()} onAction={jest.fn()} onClick={jest.fn()} />);

    expect(screen.queryByRole("link", { name: /Imported from GitHub issue/ })).not.toBeInTheDocument();
  });

  it("BacklogItemCard_should_OmitProvenanceBadge_When_ExternalUrlPresentButExternalIdMissing", () => {
    // Guards against a literal "Imported from GitHub issue #undefined" / "#undefined"
    // badge — nothing in the type system enforces externalId always accompanying
    // a real externalUrl, so the badge must not render on externalUrl alone.
    render(
      <BacklogItemCard
        item={makeItem({ externalUrl: "https://github.com/acme/widget/issues/42", externalId: undefined })}
        onAction={jest.fn()}
        onClick={jest.fn()}
      />
    );

    expect(screen.queryByRole("link", { name: /Imported from GitHub issue/ })).not.toBeInTheDocument();
    expect(screen.queryByText(/undefined/)).not.toBeInTheDocument();
  });

  it("BacklogItemCard_should_NotTriggerOnClick_When_ProvenanceBadgeClicked", () => {
    const onClick = jest.fn();
    render(
      <BacklogItemCard
        item={makeItem({ externalUrl: "https://github.com/acme/widget/issues/42", externalId: "42" })}
        onAction={jest.fn()}
        onClick={onClick}
      />
    );

    fireEvent.click(screen.getByRole("link", { name: "Imported from GitHub issue #42" }));

    expect(onClick).not.toHaveBeenCalled();
  });

  it("BacklogItemCard_should_NotTriggerCardOnClick_When_EnterPressedOnProvenanceBadge", () => {
    // Regression test: the card's onKeyDown handler used to fire on ANY
    // bubbled Enter/Space keydown, including from this nested focusable
    // anchor — preventDefault() on the bubbled event meant a keyboard user
    // could never actually follow the link via Enter. Guarding on
    // `e.target === e.currentTarget` fixes it; this asserts the card's
    // onClick (its keyboard-activation path) is not invoked when the event
    // originates on the badge.
    const onClick = jest.fn();
    render(
      <BacklogItemCard
        item={makeItem({ externalUrl: "https://github.com/acme/widget/issues/42", externalId: "42" })}
        onAction={jest.fn()}
        onClick={onClick}
      />
    );

    const badge = screen.getByRole("link", { name: "Imported from GitHub issue #42" });
    fireEvent.keyDown(badge, { key: "Enter" });

    expect(onClick).not.toHaveBeenCalled();
  });

  it("BacklogItemCard_should_TriggerOnClick_When_EnterPressedOnCardItself", () => {
    // Companion test: the guard must not break the card's own keyboard
    // activation — Enter on the card (not a nested child) should still
    // invoke onClick.
    const onClick = jest.fn();
    render(
      <BacklogItemCard item={makeItem()} onAction={jest.fn()} onClick={onClick} />
    );

    fireEvent.keyDown(screen.getByTestId("backlog-item-card"), { key: "Enter" });

    expect(onClick).toHaveBeenCalledWith("item-1");
  });
});

describe("BacklogBoard — cross-card independence", () => {
  it("only disables the pending card's button, leaving a sibling card interactive", () => {
    const items = [
      makeItem({ id: "item-1", title: "First item" }),
      makeItem({ id: "item-2", title: "Second item" }),
    ];
    mockUseWatchBacklogItems.mockReturnValue({ items, connectionState: "live" });

    render(
      <BacklogBoard
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
