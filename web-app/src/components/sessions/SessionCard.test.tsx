import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionCard } from "./SessionCard";
import { ReviveOutcome, SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import { staleBadge, statusCreationFailed } from "./SessionCard.css";

jest.mock("@connectrpc/connect", () => require("./__tests__/sessionCardTestFixtures").mockConnect());

jest.mock("@connectrpc/connect-web", () => require("./__tests__/sessionCardTestFixtures").mockConnectWeb());

jest.mock("@/lib/contexts/ReviewQueueContext", () => require("./__tests__/sessionCardTestFixtures").mockReviewQueueContext());

jest.mock("@/lib/contexts/SessionServiceContext", () => require("./__tests__/sessionCardTestFixtures").mockSessionServiceContext());

jest.mock("@/lib/store", () => require("./__tests__/sessionCardTestFixtures").mockStore());

jest.mock("@/lib/store/sessionsSlice", () => require("./__tests__/sessionCardTestFixtures").mockSessionsSlice());

jest.mock("@/lib/hooks/useTerminalSnapshot", () => require("./__tests__/sessionCardTestFixtures").mockUseTerminalSnapshot());

jest.mock("@/lib/hooks/useFocusTrap", () => require("./__tests__/sessionCardTestFixtures").mockUseFocusTrap());

jest.mock("@/components/ui/AppLink", () => require("./__tests__/sessionCardTestFixtures").mockAppLink());

jest.mock("@/components/ui/Modal", () => require("./__tests__/sessionCardTestFixtures").mockModal());

jest.mock("@/components/ui/Tooltip", () => require("./__tests__/sessionCardTestFixtures").mockTooltip());

jest.mock("@/lib/hooks/useSessionActions", () => require("./__tests__/sessionCardTestFixtures").mockUseSessionActions());

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

describe("SessionCard — note badge", () => {
  it("SessionCard_should_RenderNoteBadge_When_SessionNoteIsNonEmpty", () => {
    const session = { ...minimalSession, note: "waiting on CI" } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.getByTestId("badge-has-note")).toBeInTheDocument();
  });

  it("SessionCard_should_NotRenderNoteBadge_When_SessionNoteIsWhitespaceOrEmpty", () => {
    const whitespaceSession = { ...minimalSession, note: "   \n" } as unknown as Session;
    const { rerender } = render(<SessionCard session={whitespaceSession} />);
    expect(screen.queryByTestId("badge-has-note")).toBeNull();

    const emptySession = { ...minimalSession, note: "" } as unknown as Session;
    rerender(<SessionCard session={emptySession} />);
    expect(screen.queryByTestId("badge-has-note")).toBeNull();
  });

  it("SessionCard_should_TruncateNoteBadgeTooltipToOneHundredTwentyChars_When_NoteIsLong", () => {
    const longNote = "N".repeat(150);
    const session = { ...minimalSession, note: longNote } as unknown as Session;
    render(<SessionCard session={session} />);
    const badge = screen.getByTestId("badge-has-note");
    const expected = "N".repeat(119) + "…";
    expect(badge.closest('[data-testid="tooltip-mock"]')).toHaveAttribute("data-label", expected);
  });
});

describe("SessionCard — revived context badge", () => {
  it("SessionCard_should_RenderRevivedContextBadge_When_ReviveOutcomeIsFreshLostHistory", () => {
    const session = { ...minimalSession, reviveOutcome: ReviveOutcome.FRESH_LOST_HISTORY } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.getByTestId("revived-context-badge")).toBeInTheDocument();
  });

  it("SessionCard_should_NotRenderRevivedContextBadge_When_ReviveOutcomeIsNotFreshLostHistory", () => {
    const session = { ...minimalSession, reviveOutcome: ReviveOutcome.RESUME_LIVE } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.queryByTestId("revived-context-badge")).toBeNull();
  });
});

describe("SessionCard — host badge (ssh-remote-workspaces Epic 6.2, Story 6.2.1)", () => {
  it("SessionCard_should_RenderHostBadge_When_SessionRemoteNameIsSet", () => {
    const session = { ...minimalSession, remoteName: "prod-box" } as unknown as Session;
    render(<SessionCard session={session} />);

    const badge = screen.getByTestId("host-badge");
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveAttribute("role", "img");
    expect(badge).toHaveAttribute("aria-label", "Running on prod-box");
  });

  it("SessionCard_should_NotRenderHostBadge_When_SessionIsLocal", () => {
    const session = { ...minimalSession, remoteName: "" } as unknown as Session;
    render(<SessionCard session={session} />);

    expect(screen.queryByTestId("host-badge")).toBeNull();
  });
});

// Builds a Timestamp-shaped object (seconds/nanos) the number of minutes ago from now —
// matches the {seconds: bigint, nanos: number} shape session-staleness.ts and SessionCard's
// own formatTimeAgo/formatDate helpers read from lastMeaningfulOutput/lastTerminalUpdate.
function minutesAgoTimestamp(minutes: number) {
  const seconds = Math.floor(Date.now() / 1000) - minutes * 60;
  return { seconds: BigInt(seconds), nanos: 0 };
}

describe("SessionCard — stale badge", () => {
  it("SessionCard_should_RenderStaleBadge_When_ActiveSessionExceedsThreshold", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.ACTIVE,
      lastMeaningfulOutput: minutesAgoTimestamp(45),
    } as unknown as Session;
    render(<SessionCard session={session} staleThresholdMinutes={30} />);

    const badge = screen.getByText("Stale", { exact: false });
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveAttribute("role", "img");
    expect(badge.getAttribute("aria-label")).toMatch(/^Stale — no output for/);
  });

  it("SessionCard_should_NotRenderStaleBadge_When_PausedSessionLastOutputWasSixHoursAgo", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.PAUSED,
      lastMeaningfulOutput: minutesAgoTimestamp(6 * 60),
    } as unknown as Session;
    render(<SessionCard session={session} staleThresholdMinutes={30} />);

    expect(screen.queryByText("Stale", { exact: false })).toBeNull();
  });

  it("SessionCard_should_ApplyStaleBadgeWarningTokenStyle_When_StaleBadgeRenders", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.ACTIVE,
      lastMeaningfulOutput: minutesAgoTimestamp(45),
    } as unknown as Session;
    render(<SessionCard session={session} staleThresholdMinutes={30} />);

    const badge = screen.getByText("Stale", { exact: false });
    // Reuses the same warning-token-based style exported for the badge — asserts the
    // rendered class matches the `staleBadge` export (same toHaveClass(String(...))
    // pattern ReviewQueuePanel.test.tsx uses for its own vanilla-extract class assertions,
    // and the same warning tokens stuckReason.css.ts's chipStaleWork applies via
    // vars.color.warningBg/warningText/warning).
    expect(badge).toHaveClass(String(staleBadge));
  });
});

describe("SessionCard — retry badge and permanently-failed status", () => {
  it("SessionCard_should_NotRenderRetryBadge_When_RetryAttemptIsZero", () => {
    const session = { ...minimalSession, retryAttempt: 0, retryMaxAttempts: 3 } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.queryByLabelText(/Retry attempt/)).toBeNull();
  });

  it("SessionCard_should_RenderRetryBadgeAfterReviewQueueBadge_When_RetryAttemptPositive", () => {
    const session = { ...minimalSession, retryAttempt: 2, retryMaxAttempts: 3 } as unknown as Session;
    render(<SessionCard session={session} />);
    const badge = screen.getByLabelText("Retry attempt 2 of 3");
    expect(badge).toBeInTheDocument();
  });

  it("SessionCard_should_ExposeFullSentenceAriaLabel_When_StatusIsPermanentlyFailed", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.PERMANENTLY_FAILED,
      retryMaxAttempts: 3,
    } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.getByLabelText("Session status: Failed — gave up after 3 attempts")).toBeInTheDocument();
  });

  it("PermanentlyFailed with the default max_attempts=1 policy still shows both badges (attempt count is non-zero)", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.PERMANENTLY_FAILED,
      retryAttempt: 1,
      retryMaxAttempts: 1,
    } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.getByLabelText("Retry attempt 1 of 1")).toBeInTheDocument();
    expect(screen.getByLabelText("Session status: Failed — gave up after 1 attempt")).toBeInTheDocument();
  });
});

// Epic 5.2 (async-session-creation): Failed-state rendering.
describe("SessionCard — Failed status pill", () => {
  it("SessionCard_should_RenderFailedLabelAndDistinctToken_When_StatusIsFailed", () => {
    const session = { ...minimalSession, status: SessionStatus.FAILED } as unknown as Session;
    render(<SessionCard session={session} />);

    const pill = screen.getByTestId("status-pill");
    expect(pill).toHaveTextContent("Failed");
    // Distinct token per plan.md's Pattern Decisions table -- not a reuse of
    // statusCrashed's error-palette token.
    expect(pill).toHaveClass(String(statusCreationFailed));
  });

  it("SessionCard_should_RenderAWarningGlyph_When_StatusIsFailed", () => {
    const session = { ...minimalSession, status: SessionStatus.FAILED } as unknown as Session;
    render(<SessionCard session={session} />);

    const pill = screen.getByTestId("status-pill");
    expect(pill.textContent).toContain("⚠");
  });
});

describe("SessionCard — Failed reason-specific message", () => {
  it.each([
    ["GitHubResolutionError", "Failed to resolve GitHub URL."],
    ["StartupError", "Failed to start session."],
    ["Stale", "This session creation appears to have stalled."],
    ["SomeUnrecognizedReason", "Session creation failed."],
  ])("SessionCard_should_ShowReasonSpecificMessage_When_FailureReasonIs_%s", (failureReason, expected) => {
    const session = { ...minimalSession, status: SessionStatus.FAILED, failureReason } as unknown as Session;
    render(<SessionCard session={session} />);

    expect(screen.getByTestId("failure-message")).toHaveTextContent(expected);
  });

  it("SessionCard_should_FallBackToCreationProgress_When_FailureReasonIsAbsent", () => {
    // Covers today's actual backend wiring: failureReason isn't yet plumbed
    // onto the wire (proto/session/v1/types.proto has no failure_reason
    // field), but creation_progress already carries a detailed message for
    // GitHubResolutionError/StartupError (session_creation_pipeline.go's
    // setPhase calls before the terminal write).
    const session = {
      ...minimalSession,
      status: SessionStatus.FAILED,
      creationProgress: "Failed to resolve GitHub URL: connection timed out",
    } as unknown as Session;
    render(<SessionCard session={session} />);

    expect(screen.getByTestId("failure-message")).toHaveTextContent(
      "Failed to resolve GitHub URL: connection timed out"
    );
  });
});

describe("SessionCard — Creating/Failed live region (single node, no remount)", () => {
  it("SessionCard_should_ReuseSameLiveRegionNode_When_TransitioningCreatingToFailed", () => {
    const creatingSession = {
      ...minimalSession,
      status: SessionStatus.CREATING,
      creationProgress: "Cloning repository...",
    } as unknown as Session;
    const { rerender } = render(<SessionCard session={creatingSession} />);

    const liveRegionBefore = screen.getByTestId("creation-live-region");
    expect(liveRegionBefore).toHaveAttribute("aria-live", "polite");
    expect(liveRegionBefore.textContent).toBe("Cloning repository...");

    const failedSession = {
      ...minimalSession,
      status: SessionStatus.FAILED,
      failureReason: "GitHubResolutionError",
    } as unknown as Session;
    rerender(<SessionCard session={failedSession} />);

    const liveRegionsAfter = screen.getAllByTestId("creation-live-region");
    expect(liveRegionsAfter).toHaveLength(1);
    expect(liveRegionsAfter[0]).toBe(liveRegionBefore); // same DOM node, not a remount
    expect(liveRegionsAfter[0]).toHaveAttribute("aria-live", "assertive");
    expect(liveRegionsAfter[0].textContent).toBe("Failed to resolve GitHub URL.");
  });

  it("SessionCard_should_UpdateProgressTextInPlace_When_CreationProgressAdvancesThroughPhases", () => {
    const session1 = {
      ...minimalSession,
      status: SessionStatus.CREATING,
      creationProgress: "Resolving GitHub URL...",
    } as unknown as Session;
    const { rerender } = render(<SessionCard session={session1} />);

    const progressTextBefore = screen.getByTestId("session-progress-text");
    expect(progressTextBefore.textContent).toBe("Resolving GitHub URL...");

    const session2 = {
      ...minimalSession,
      status: SessionStatus.CREATING,
      creationProgress: "Cloning repository...",
    } as unknown as Session;
    rerender(<SessionCard session={session2} />);

    const progressTextAfter = screen.getByTestId("session-progress-text");
    expect(progressTextAfter).toBe(progressTextBefore); // same node, updated in place
    expect(progressTextAfter.textContent).toBe("Cloning repository...");
  });
});

describe("SessionCard — sub-status chip subagentCount", () => {
  it("SessionCard_should_PassSubagentCountToSubStatusChip_When_SubStatusIsWaitingForAgent", () => {
    const session = {
      ...minimalSession,
      status: SessionStatus.ACTIVE,
      subStatus: SubStatus.WAITING_FOR_AGENT,
      subagentCount: 3,
    } as unknown as Session;
    render(<SessionCard session={session} />);
    const chip = screen.getByRole("status", { name: "Waiting for agents" });
    expect(chip.textContent).toContain("3");
  });
});
