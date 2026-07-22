/**
 * Tests for BacklogItemCard / BacklogBoard per-card pending state (board-level action feedback).
 *
 * Covers:
 *  1. No pendingAction: button shows its normal label and is enabled
 *  2. pendingAction matches this card's action: spinner + "Running…" shown, button disabled
 *  3. BacklogBoard: a pending action on one card doesn't disable a sibling card's button
 */

import React from "react";
import { act, render, screen } from "@testing-library/react";
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

  it("BacklogItemCard_should_RenderQueuedStatusLabelIndependently_When_GetActionSpecFallsThroughToDefaultBranch", () => {
    render(
      <BacklogItemCard item={makeItem({ status: "queued" })} onAction={jest.fn()} onClick={jest.fn()} />
    );

    // getActionSpec() has no explicit "queued" case — it falls to the default
    // branch (raw status as button label) — but the status label still
    // reads the canonical "Queued" from getStatusLabel(), independently.
    expect(screen.getByTestId("backlog-item-card-status")).toHaveTextContent("Queued");
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
