import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SessionRow } from "./SessionRow";
import { ReviveOutcome, SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({})),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
}));

jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  useSessionServiceContext: () => ({
    draftPullRequest: jest.fn(),
    createPullRequest: jest.fn(),
  }),
}));

jest.mock("@/lib/hooks/useFocusTrap", () => ({
  useFocusTrap: () => {},
}));

jest.mock("@/lib/hooks/useAvailablePrograms", () => ({
  useAvailablePrograms: () => ({ programs: [], loading: false }),
}));

jest.mock("@/lib/hooks/useSessionActions", () => ({
  useSessionActions: () => ({
    pause: jest.fn(),
    resume: jest.fn(),
    delete: jest.fn(),
    rename: jest.fn(),
    restart: jest.fn(),
    createCheckpoint: jest.fn(),
    updateTags: jest.fn(),
    update: jest.fn(),
  }),
}));

const minimalSession: Partial<Session> = {
  id: "s1",
  title: "Test Session",
  status: 1 as Session["status"],
  tags: [],
  category: "",
  path: "/tmp/session",
  branch: "",
  program: "claude",
};

describe("SessionRow — note badge", () => {
  it("SessionRow_should_RenderNoteBadge_When_SessionNoteIsNonEmpty", () => {
    const session = { ...minimalSession, note: "waiting on CI" } as unknown as Session;
    render(<SessionRow session={session} />);
    expect(screen.getByTestId("badge-has-note")).toBeInTheDocument();
  });

  it("SessionRow_should_NotRenderNoteBadge_When_SessionNoteIsWhitespaceOrEmpty", () => {
    const whitespaceSession = { ...minimalSession, note: "   \n" } as unknown as Session;
    const { rerender } = render(<SessionRow session={whitespaceSession} />);
    expect(screen.queryByTestId("badge-has-note")).toBeNull();

    const emptySession = { ...minimalSession, note: "" } as unknown as Session;
    rerender(<SessionRow session={emptySession} />);
    expect(screen.queryByTestId("badge-has-note")).toBeNull();
  });
});

// session-revive-uuid-loss UX AC7: the row folds the "context lost" signal
// into its single combined aria-label instead of adding a second,
// separately-announced landmark (ux.md's explicit accessibility rule).
describe("SessionRow — revived context badge", () => {
  it("SessionRow_should_RenderBadgeAndExtendAriaLabel_When_ReviveOutcomeIsFreshLostHistory", () => {
    const session = { ...minimalSession, reviveOutcome: ReviveOutcome.FRESH_LOST_HISTORY } as unknown as Session;
    render(<SessionRow session={session} />);
    expect(screen.getByTestId("revived-context-badge")).toBeInTheDocument();
    expect(screen.getByTestId("session-row")).toHaveAttribute(
      "aria-label",
      expect.stringContaining(", context: lost"),
    );
  });

  it("SessionRow_should_NotRenderBadgeOrExtendAriaLabel_When_ReviveOutcomeIsNotFreshLostHistory", () => {
    const session = { ...minimalSession, reviveOutcome: ReviveOutcome.RESUME_LIVE } as unknown as Session;
    render(<SessionRow session={session} />);
    expect(screen.queryByTestId("revived-context-badge")).toBeNull();
    expect(screen.getByTestId("session-row").getAttribute("aria-label")).not.toContain("context: lost");
  });
});

// Builds a Timestamp-shaped object (seconds/nanos) the number of minutes ago from now —
// matches the {seconds: bigint, nanos: number} shape session-staleness.ts and SessionRow's
// own formatElapsed helper read from lastMeaningfulOutput/lastTerminalUpdate.
function minutesAgoTimestamp(minutes: number) {
  const seconds = Math.floor(Date.now() / 1000) - minutes * 60;
  return { seconds: BigInt(seconds), nanos: 0 };
}

// Epic 5.2 (async-session-creation) parity fix: SessionRow is the view users
// actually see (SessionList.tsx defaults viewMode to "row"; SessionCard.tsx
// is unreachable), so the Failed-state rendering built in SessionCard.tsx
// must also work here. Mirrors SessionCard.test.tsx's "Failed status pill"
// / "Failed reason-specific message" / live-region describe blocks.
describe("SessionRow — Failed status dot", () => {
  it("SessionRow_should_RenderFailedLabelAndDistinctDotStatus_When_StatusIsFailed", () => {
    const session = { ...minimalSession, status: SessionStatus.FAILED } as unknown as Session;
    const { container } = render(<SessionRow session={session} />);

    // Radix Tooltip only renders its label text into a portal on
    // hover/focus, so assert directly on the dot's data-status attribute
    // (queried via the DOM, not an accessible-name query) -- distinct value
    // per plan.md's Pattern Decisions table, not a reuse of "crashed".
    const dot = container.querySelector('[data-status="failed"]');
    expect(dot).not.toBeNull();
    expect(screen.getByTestId("session-row")).toHaveAttribute(
      "aria-label",
      expect.stringContaining("status: Failed")
    );
  });
});

describe("SessionRow — Failed reason-specific message", () => {
  it.each([
    ["GitHubResolutionError", "Failed to resolve GitHub URL."],
    ["StartupError", "Failed to start session."],
    ["Stale", "This session creation appears to have stalled."],
    ["SomeUnrecognizedReason", "Session creation failed."],
  ])("SessionRow_should_ShowReasonSpecificMessage_When_FailureReasonIs_%s", (failureReason, expected) => {
    const session = { ...minimalSession, status: SessionStatus.FAILED, failureReason } as unknown as Session;
    render(<SessionRow session={session} />);

    expect(screen.getByTestId("failure-message")).toHaveTextContent(expected);
  });

  it("SessionRow_should_FallBackToCreationProgress_When_FailureReasonIsAbsent", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.FAILED,
      failureReason: "",
      creationProgress: "Failed to resolve GitHub URL: connection timed out",
    } as unknown as Session;
    render(<SessionRow session={session} />);

    expect(screen.getByTestId("failure-message")).toHaveTextContent(
      "Failed to resolve GitHub URL: connection timed out"
    );
  });
});

describe("SessionRow — Creating/Failed live region (single node, no remount)", () => {
  it("SessionRow_should_ReuseSameLiveRegionNode_When_TransitioningCreatingToFailed", () => {
    const creatingSession = {
      ...minimalSession,
      status: SessionStatus.CREATING,
      creationProgress: "Cloning repository...",
    } as unknown as Session;
    const { rerender } = render(<SessionRow session={creatingSession} />);

    const liveRegionBefore = screen.getByTestId("creation-live-region");
    expect(liveRegionBefore).toHaveAttribute("aria-live", "polite");
    expect(liveRegionBefore.textContent).toBe("Cloning repository...");

    const failedSession = {
      ...minimalSession,
      status: SessionStatus.FAILED,
      failureReason: "GitHubResolutionError",
    } as unknown as Session;
    rerender(<SessionRow session={failedSession} />);

    const liveRegionsAfter = screen.getAllByTestId("creation-live-region");
    expect(liveRegionsAfter).toHaveLength(1);
    expect(liveRegionsAfter[0]).toBe(liveRegionBefore); // same DOM node, not a remount
    expect(liveRegionsAfter[0]).toHaveAttribute("aria-live", "assertive");
    expect(liveRegionsAfter[0].textContent).toBe("Failed to resolve GitHub URL.");
  });
});

describe("SessionRow — stale badge", () => {
  it("SessionRow_should_RenderStaleBadge_When_ActiveSessionExceedsThreshold", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.ACTIVE,
      lastMeaningfulOutput: minutesAgoTimestamp(45),
    } as unknown as Session;
    render(<SessionRow session={session} staleThresholdMinutes={30} />);

    const badge = screen.getByText("Stale", { exact: false });
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveAttribute("role", "img");
    expect(badge.getAttribute("aria-label")).toMatch(/^Stale — no output for/);
  });

  it("SessionRow_should_NotRenderStaleBadge_When_PausedSessionLastOutputWasSixHoursAgo", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.PAUSED,
      lastMeaningfulOutput: minutesAgoTimestamp(6 * 60),
    } as unknown as Session;
    render(<SessionRow session={session} staleThresholdMinutes={30} />);

    expect(screen.queryByText("Stale", { exact: false })).toBeNull();
  });
});

describe("SessionRow — IDLE substatus chip (Epic 3.2.2)", () => {
  it("SessionRow_should_RenderIdleChip_When_ActiveSessionSubStatusIsIdle", () => {
    // Flip of the pre-Epic-3.2.2 suppression: idle-reason items were removed from the
    // Review Queue entirely, so this chip is now the Sessions-list's only "ready for
    // next task" signal (SessionRow.tsx no longer excludes SubStatus.IDLE at :352).
    const session = {
      ...minimalSession,
      status: SessionStatus.ACTIVE,
      subStatus: SubStatus.IDLE,
    } as unknown as Session;
    render(<SessionRow session={session} />);

    const chip = screen.getByText("Idle", { exact: false });
    expect(chip).toBeInTheDocument();
    expect(chip).toHaveAttribute("role", "status");
    expect(chip).toHaveAttribute("aria-label", "Session is idle");
  });
});

describe("SessionRow — backlog-origin badge", () => {
  it("SessionRow_should_RenderBacklogOriginBadge_When_BacklogEntryProvided", () => {
    const session = { ...minimalSession } as unknown as Session;
    render(
      <SessionRow
        session={session}
        backlogEntry={{ itemId: "item-1", itemTitle: "Fix the thing", itemStatus: "in_progress", sessionRole: "triage" }}
      />
    );
    expect(screen.getByTestId("backlog-origin-badge")).toBeInTheDocument();
  });

  it("SessionRow_should_NotRenderBacklogOriginBadge_When_NoBacklogEntry", () => {
    const session = { ...minimalSession } as unknown as Session;
    render(<SessionRow session={session} />);
    expect(screen.queryByTestId("backlog-origin-badge")).toBeNull();
  });
});

// session-list-density Epic 1.2 Story 1.2.1: agent/memory demoted to
// defaultVisible: false in session-columns.ts.
describe("SessionRow — agent/memory columns demoted to hidden by default", () => {
  it("SessionRow_should_NotRenderAgentOrMemorySpans_When_UsingDefaultVisibleColumns", () => {
    const session = { ...minimalSession } as unknown as Session;
    render(<SessionRow session={session} />);
    expect(
      screen.queryByRole("img", { name: `Agent: ${session.program}` })
    ).toBeNull();
    expect(screen.queryByRole("img", { name: "No memory data" })).toBeNull();
  });
});

// session-list-density Epic 1.2 Story 1.2.2: elapsed moved out of the grid
// loop onto a hardcoded second line inside nameCell.
describe("SessionRow — elapsed renders as a second line, not a grid cell", () => {
  it("SessionRow_should_RenderElapsedAsSecondLine_When_ElapsedColumnVisible", () => {
    const session = {
      ...minimalSession,
      existingDir: "/tmp/session",
    } as unknown as Session;
    const { container } = render(<SessionRow session={session} />);

    const rowEl = screen.getByTestId("session-row");
    const timeEl = container.querySelector("time");
    expect(timeEl).not.toBeNull();

    // Not a direct grid-cell child of the row's own display:grid element.
    expect(timeEl?.parentElement).not.toBe(rowEl);

    // Lives inside nameCell (same container as the path line), not as a
    // grid-cell sibling of it.
    expect(timeEl?.closest('[data-testid="session-row-name-cell"]')).toBeTruthy();
  });

  it("SessionRow_should_NotRenderElapsedSecondLine_When_ElapsedNotInVisibleColumns", () => {
    const session = { ...minimalSession } as unknown as Session;
    const { container } = render(
      <SessionRow session={session} visibleColumns={[]} />
    );
    expect(container.querySelector("time")).toBeNull();
  });
});

// session-list-density Epic 2.1 Story 2.1.1: wrap instead of ellipsis, apply
// truncateWorkspacePath at every container width.
const CANONICAL_WORKSPACE_PATH =
  "/Users/tstapler/.stapler-squad/workspaces/6eb0b580fa0331d5/worktrees/stapler-squad-wasted-space_18d807dfb97a2b28";
const WORKSPACE_HASH = "6eb0b580fa0331d5";
const WORKTREE_UUID_SUFFIX = "18d807dfb97a2b28";

describe("SessionRow — path truncation via truncateWorkspacePath (Epic 2.1 Story 2.1.1)", () => {
  it("SessionRow_should_HideOpaqueSegments_When_PathExceeds72Chars", () => {
    const session = {
      ...minimalSession,
      existingDir: CANONICAL_WORKSPACE_PATH,
    } as unknown as Session;
    render(<SessionRow session={session} />);

    const pathEl = screen.getByRole("img", {
      name: `Path: ${CANONICAL_WORKSPACE_PATH}`,
    });
    expect(pathEl.textContent).not.toContain(WORKSPACE_HASH);
    expect(pathEl.textContent).not.toContain(WORKTREE_UUID_SUFFIX);

    // Full-value companion (Tooltip label / aria-label) still carries the
    // untruncated path unchanged.
    expect(pathEl.getAttribute("aria-label")).toBe(
      `Path: ${CANONICAL_WORKSPACE_PATH}`
    );
  });

  it("SessionRow_should_KeepSamePathText_When_RenderedAtNarrowContainerWidth", () => {
    // The narrow (<200px) container-query breakpoint is CSS-only (font size,
    // chip wrapping) and never switches the truncation budget (Story 2.1.2's
    // Resolution Note) — there's no separate narrow-width render path to
    // simulate here, so this asserts there is exactly one path <span>, i.e.
    // no dual-render leftover from the dropped Task 2.1.2c approach.
    const session = {
      ...minimalSession,
      existingDir: CANONICAL_WORKSPACE_PATH,
    } as unknown as Session;
    render(<SessionRow session={session} />);

    const pathEls = screen.getAllByRole("img", {
      name: `Path: ${CANONICAL_WORKSPACE_PATH}`,
    });
    expect(pathEls).toHaveLength(1);
  });

  // Story 3.1.1 / Task 3.1.1b: the row's own aria-label must carry the full
  // path, not the visually-truncated string shown in the path <span>.
  it("SessionRow_should_ExposeFullPathInAriaLabel_When_QueriedByRole", () => {
    const session = {
      ...minimalSession,
      existingDir: CANONICAL_WORKSPACE_PATH,
    } as unknown as Session;
    render(<SessionRow session={session} />);

    const row = screen.getByTestId("session-row");
    const ariaLabel = row.getAttribute("aria-label");
    expect(ariaLabel).toContain(CANONICAL_WORKSPACE_PATH);
    expect(ariaLabel).toContain(WORKSPACE_HASH);
    expect(ariaLabel).toContain(WORKTREE_UUID_SUFFIX);
  });
});

// session-list-density Epic 3.2 Story 3.2.1 (ADR-001): agent icon / memory
// badge gain focusable Tooltip wrappers, and their data is folded into the
// row's aria-label so it's reachable even when the columns are hidden by
// default (Epic 1.2's demotion).
describe("SessionRow — agent/memory accessible disclosure (Epic 3.2 Story 3.2.1)", () => {
  it("SessionRow_should_ShowTooltip_When_AgentIconReceivesKeyboardFocus", async () => {
    const session = { ...minimalSession, program: "claude" } as unknown as Session;
    render(<SessionRow session={session} visibleColumns={["agent"]} />);

    const agentIcon = screen.getByRole("img", { name: "Agent: claude" });
    expect(agentIcon).toHaveAttribute("tabIndex", "0");

    fireEvent.focus(agentIcon);

    await waitFor(() => {
      expect(screen.getByRole("tooltip")).toHaveTextContent("claude");
    });
  });

  it("SessionRow_should_ShowProcessRssTooltip_When_MemoryBadgeReceivesKeyboardFocus", async () => {
    const session = {
      ...minimalSession,
      memoryRssMb: 512n,
    } as unknown as Session;
    render(<SessionRow session={session} visibleColumns={["memory"]} />);

    const memoryBadge = screen.getByRole("img", { name: "512 MB RAM" });
    expect(memoryBadge).toHaveAttribute("tabIndex", "0");

    fireEvent.focus(memoryBadge);

    await waitFor(() => {
      expect(screen.getByRole("tooltip")).toHaveTextContent("Process RSS: 512 MB");
    });
  });

  it("SessionRow_should_IncludeAgentAndMemoryInAriaLabel_When_ColumnsNotInVisibleColumns", () => {
    const session = {
      ...minimalSession,
      program: "claude",
      memoryRssMb: 200n,
    } as unknown as Session;
    // Default visibleColumns (agent/memory demoted, Epic 1.2) — neither
    // column renders as a grid cell, but the row aria-label still carries
    // the data (ADR-001's "zero extra interaction" guarantee).
    render(<SessionRow session={session} />);

    expect(screen.queryByRole("img", { name: "Agent: claude" })).toBeNull();
    expect(screen.queryByRole("img", { name: "200 MB RAM" })).toBeNull();

    const ariaLabel = screen.getByTestId("session-row").getAttribute("aria-label");
    expect(ariaLabel).toContain("agent: claude");
    expect(ariaLabel).toContain("memory: 200 MB");
  });

  it("SessionRow_should_OmitMemoryFragmentFromAriaLabel_When_MemMBIsZero", () => {
    const session = { ...minimalSession, program: "claude" } as unknown as Session;
    render(<SessionRow session={session} />);

    const ariaLabel = screen.getByTestId("session-row").getAttribute("aria-label");
    expect(ariaLabel).toContain("agent: claude");
    expect(ariaLabel).not.toContain("memory:");
  });

  // validation.md UX Criterion 7: missing agent/memory data renders nothing
  // (omitted), not a placeholder error string -- even when the columns are
  // explicitly made visible, proving the omission is data-driven, not just
  // the column being hidden by default (Epic 1.2's demotion, covered by
  // SessionRow_should_NotRenderAgentOrMemorySpans_When_UsingDefaultVisibleColumns above).
  it("SessionRow_should_OmitAgentAndMemoryUI_When_ProgramAndMemMBAreAbsent", () => {
    const session = {
      ...minimalSession,
      program: "",
      memoryRssMb: 0n,
    } as unknown as Session;
    render(<SessionRow session={session} visibleColumns={["agent", "memory"]} />);

    expect(screen.queryByRole("img", { name: /^Agent:/ })).toBeNull();
    expect(screen.queryByRole("img", { name: "No memory data" })).toBeNull();

    const ariaLabel = screen.getByTestId("session-row").getAttribute("aria-label");
    expect(ariaLabel).not.toContain(", agent:");
    expect(ariaLabel).not.toContain("memory:");
  });
});
