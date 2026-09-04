import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionRow } from "./SessionRow";
import { ReviveOutcome, SessionStatus } from "@/gen/session/v1/types_pb";
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
