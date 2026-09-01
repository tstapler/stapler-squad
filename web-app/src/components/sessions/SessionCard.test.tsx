import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionCard } from "./SessionCard";
import { ReviveOutcome, SessionStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import { staleBadge } from "./SessionCard.css";

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
