import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SessionsSection } from "./SessionsSection";
import type { SessionsSectionProps } from "./SessionsSection";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

jest.mock("../SessionMonitor", () => ({ SessionMonitor: () => null }));

beforeEach(() => {
  localStorage.clear();
});

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: `entity-${Math.random()}`,
    sessionId: `session-${Math.random()}`,
    role: "work",
    estimatedCostUsd: 0,
    ...overrides,
  };
}

function makeItem(linkedSessions: LinkedSession[], overrides: Partial<BacklogItem> = {}): BacklogItem {
  return {
    id: "itm_df0d5872",
    title: "Chronically stuck item",
    status: "in_progress",
    priority: 3,
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions,
    notes: "",
    statusEvents: [],
    progressNotes: [],
    totalEstimatedCostUsd: 0,
    ...overrides,
  };
}

/** Builds the df0d5872-shaped fixture from research/features.md finding #5:
 * 6 triage + 3 review + 2 work sessions = 11 total linked sessions. */
function makeElevenSessions(): LinkedSession[] {
  const sessions: LinkedSession[] = [];
  for (let i = 0; i < 6; i++) {
    sessions.push(makeSession({ sessionId: `headless-triage-${i}`, role: "triage" }));
  }
  for (let i = 0; i < 3; i++) {
    sessions.push(makeSession({ sessionId: `headless-re-review-${i}`, role: "review" }));
  }
  for (let i = 0; i < 2; i++) {
    sessions.push(makeSession({ sessionId: `work-session-${i}`, role: "work" }));
  }
  return sessions;
}

describe("SessionsSection", () => {
  it("SessionsSection_should_RenderShowMoreButtonAndRevealRemainingSessions_When_ElevenSessionsLinkedLikeDf0d5872Case", () => {
    const item = makeItem(makeElevenSessions());
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn()}
        steeringSessionId={null}
      />
    );

    // 5 most recent shown by default (cap), plus "Show 6 more".
    expect(screen.getAllByRole("listitem")).toHaveLength(5);
    const showMore = screen.getByTestId("sessions-show-more");
    expect(showMore).toHaveTextContent("Show 6 more");

    // Identity check (regression for the "shows oldest, not most recent" bug):
    // linkedSessions is ascending by createdAt, so the visible 5 must be the
    // TAIL of the list (the 3 most recent triage/review + both work sessions),
    // never the oldest triage sessions from the head of the list.
    expect(screen.getByText("work-session-1")).toBeInTheDocument();
    expect(screen.getByText("work-session-0")).toBeInTheDocument();
    expect(screen.getByText("headless-re-review-2")).toBeInTheDocument();
    expect(screen.queryByText("headless-triage-0")).not.toBeInTheDocument();
    expect(screen.queryByText("headless-triage-5")).not.toBeInTheDocument();

    fireEvent.click(showMore);

    // All 11 shown inline, in the same list, no route change.
    expect(screen.getAllByRole("listitem")).toHaveLength(11);
    expect(screen.queryByTestId("sessions-show-more")).not.toBeInTheDocument();

    // Once expanded, the previously-hidden oldest sessions must now be present.
    expect(screen.getByText("headless-triage-0")).toBeInTheDocument();
    expect(screen.getByText("headless-triage-5")).toBeInTheDocument();
  });

  it("renders no Show More button when session count is at or below the cap", () => {
    const item = makeItem([makeSession(), makeSession(), makeSession()]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn()}
        steeringSessionId={null}
      />
    );

    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    expect(screen.queryByTestId("sessions-show-more")).not.toBeInTheDocument();
  });

  it("renders nothing when there are no linked sessions", () => {
    const item = makeItem([]);
    const { container } = render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn()}
        steeringSessionId={null}
      />
    );

    expect(container).toBeEmptyDOMElement();
  });

  it("SessionsSection_should_RenderAnchorLinkUnchanged_When_SessionKindIsWork", () => {
    const item = makeItem([makeSession({ sessionId: "a1b2c3d4", role: "work" })]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn()}
        steeringSessionId={null}
      />
    );

    expect(screen.getByRole("link", { name: /a1b2c3d4/ })).toHaveAttribute("href", "/?session=a1b2c3d4");
  });

  it("SessionsSection_should_RenderCollapsibleNotDeadAnchor_When_ClassifySessionKindReturnsBlockedGuardrail", () => {
    const item = makeItem([
      makeSession({
        entityId: "e-diff-error",
        sessionId: "diff-error-9c1d4a",
        role: "review",
        reviewVerdict: { overallOutcome: "FAIL", summary: "Review blocked: could not compute a diff." },
      }),
    ]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn()}
        steeringSessionId={null}
      />
    );

    expect(screen.queryByRole("link", { name: /diff-error-9c1d4a/ })).not.toBeInTheDocument();
    expect(screen.getByTestId("collapsible-header-session-e-diff-error")).toBeInTheDocument();
  });

  it("SessionsSection_should_RenderCollapsibleDiagnosticInsteadOfDeadAnchor_When_SessionKindIsManualReviewMarker", () => {
    // Previously a dead <a href="/?session=manual-review-..."> per the
    // Story 1.1.3 bug — now a Collapsible header expanding to BlockedNotice.
    const item = makeItem([
      makeSession({
        entityId: "e-manual-review",
        sessionId: "manual-review-a1b2c3d4-1721577600000000000",
        role: "review",
        reviewVerdict: { overallOutcome: "PASS", summary: "Manual review: verified fix locally" },
      }),
    ]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn()}
        steeringSessionId={null}
      />
    );

    expect(
      screen.queryByRole("link", { name: /manual-review-a1b2c3d4/ })
    ).not.toBeInTheDocument();
    const header = screen.getByTestId("collapsible-header-session-e-manual-review");
    expect(header.tagName).toBe("BUTTON");
    expect(header).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(header);

    expect(screen.getByTestId("blocked-notice")).toBeInTheDocument();
    expect(screen.getByText("Manual review: verified fix locally")).toBeInTheDocument();
  });

  it("SessionsSection_should_ExpandInlineDiagnosticPanelForAllFiveKinds_When_UserClicksEachSyntheticSessionRow", () => {
    const item = makeItem([
      makeSession({ sessionId: "work-uuid-1", role: "work", entityId: "e-work" }),
      makeSession({
        sessionId: "real-review-uuid-1",
        role: "review",
        entityId: "e-review",
        endedAt: new Date().toISOString(),
      }),
      makeSession({
        sessionId: "headless-triage-uuid-1",
        role: "triage",
        entityId: "e-headless",
        triageResult: { summary: "Looks good.", suggestions: [], clarifyingQuestions: [] },
      }),
      makeSession({
        sessionId: "review-blocked-uuid-1",
        role: "review",
        entityId: "e-blocked",
        reviewVerdict: { overallOutcome: "FAIL", summary: "Blocked by security check." },
      }),
      makeSession({
        sessionId: "manual-review-uuid-1",
        role: "review",
        entityId: "e-manual",
        reviewVerdict: { overallOutcome: "PASS", summary: "Verified manually." },
      }),
    ]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn()}
        steeringSessionId={null}
      />
    );

    // Real sessions (work, review) — unchanged link behavior.
    expect(screen.getByRole("link", { name: /work-uuid-1/ })).toHaveAttribute("href", "/?session=work-uuid-1");
    expect(screen.getByRole("link", { name: /real-review-uuid-1/ })).toHaveAttribute(
      "href",
      "/?session=real-review-uuid-1"
    );

    // Synthetic sessions — Collapsible header, click to expand inline.
    const headlessHeader = screen.getByTestId("collapsible-header-session-e-headless");
    const blockedHeader = screen.getByTestId("collapsible-header-session-e-blocked");
    const manualHeader = screen.getByTestId("collapsible-header-session-e-manual");
    for (const header of [headlessHeader, blockedHeader, manualHeader]) {
      expect(header).toHaveAttribute("aria-expanded", "false");
    }

    fireEvent.click(headlessHeader);
    expect(screen.getByText("Looks good.")).toBeInTheDocument();

    fireEvent.click(blockedHeader);
    expect(screen.getByText("Blocked by security check.")).toBeInTheDocument();

    fireEvent.click(manualHeader);
    expect(screen.getByText("Verified manually.")).toBeInTheDocument();
  });
});

/** Renders SessionsSection with sensible defaults, overridable per test. */
function renderSteerSection(
  linkedSessions: LinkedSession[],
  overrides: Partial<SessionsSectionProps> = {}
) {
  const onSteerSession = overrides.onSteerSession ?? jest.fn().mockResolvedValue(undefined);
  const props: SessionsSectionProps = {
    item: makeItem(linkedSessions),
    pipelineModes: [],
    latestWorkSession: undefined,
    deletingSessionId: null,
    defaultExpanded: true,
    onDeleteSession: jest.fn(),
    onSteerSession,
    steeringSessionId: null,
    ...overrides,
  };
  const view = render(<SessionsSection {...props} />);
  return { ...view, onSteerSession };
}

describe("SessionsSection steer control (Story 2.2.2, ADR-002)", () => {
  it("SessionsSection_should_NotRenderSteerControl_When_SessionIsHeadlessTriage", () => {
    const session = makeSession({
      sessionId: "headless-triage-a1b2c3d4",
      role: "triage",
      entityId: "e-headless",
    });
    renderSteerSection([session]);

    expect(screen.queryByTestId("session-steer-toggle-headless-triage-a1b2c3d4")).not.toBeInTheDocument();
  });

  it("SessionsSection_should_RenderEnabledSteerControl_When_SessionIsLiveWork", () => {
    const session = makeSession({ sessionId: "work-live-1", role: "work" });
    renderSteerSection([session]);

    const toggle = screen.getByTestId("session-steer-toggle-work-live-1");
    expect(toggle).toBeInTheDocument();
    expect(toggle).not.toBeDisabled();
  });

  it("SessionsSection_should_RenderEnabledSteerControl_When_SessionIsLiveReview", () => {
    // Mirrors the LiveWork case above — classifySessionKind maps role
    // "review" to kind "review", and isSteerable() treats "work"/"review"
    // identically (sessionKind.ts:52-55), so a live review session must get
    // the same enabled Steer control as a live work session.
    const session = makeSession({ sessionId: "review-live-1", role: "review" });
    renderSteerSection([session]);

    const toggle = screen.getByTestId("session-steer-toggle-review-live-1");
    expect(toggle).toBeInTheDocument();
    expect(toggle).not.toBeDisabled();
  });

  it("SessionsSection_should_RenderDisabledSteerControlWithTitle_When_WorkSessionHasEnded", () => {
    const session = makeSession({
      sessionId: "work-ended-1",
      role: "work",
      endedAt: new Date().toISOString(),
    });
    renderSteerSection([session]);

    const toggle = screen.getByTestId("session-steer-toggle-work-ended-1");
    expect(toggle).toBeDisabled();
    expect(toggle).toHaveAttribute("aria-disabled", "true");
    expect(toggle).toHaveAttribute("title", "Session has ended — steering is unavailable");
  });

  it("SessionsSection_should_CallOnSteerSessionWithTrimmedMessage_When_SendClicked", async () => {
    const session = makeSession({ sessionId: "work-live-2", role: "work" });
    const { onSteerSession } = renderSteerSection([session]);

    fireEvent.click(screen.getByTestId("session-steer-toggle-work-live-2"));
    const input = screen.getByTestId("session-steer-input-work-live-2");
    fireEvent.change(input, { target: { value: "  please pause and check X  " } });
    fireEvent.click(screen.getByTestId("session-steer-submit-work-live-2"));

    await waitFor(() => {
      expect(onSteerSession).toHaveBeenCalledWith(
        expect.objectContaining({ sessionId: "work-live-2" }),
        "please pause and check X"
      );
    });

    // Composer closes and clears on success.
    await waitFor(() => {
      expect(screen.queryByTestId("session-steer-input-work-live-2")).not.toBeInTheDocument();
    });
  });

  it("SessionsSection_should_SubmitOnEnterKey_When_ComposerOpen", async () => {
    const session = makeSession({ sessionId: "work-live-3", role: "work" });
    const { onSteerSession } = renderSteerSection([session]);

    fireEvent.click(screen.getByTestId("session-steer-toggle-work-live-3"));
    const input = screen.getByTestId("session-steer-input-work-live-3");
    fireEvent.change(input, { target: { value: "steer via enter" } });
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => {
      expect(onSteerSession).toHaveBeenCalledWith(
        expect.objectContaining({ sessionId: "work-live-3" }),
        "steer via enter"
      );
    });
  });

  it("SessionsSection_should_CancelAndReturnFocusToToggle_When_EscapePressed", () => {
    const session = makeSession({ sessionId: "work-live-4", role: "work" });
    renderSteerSection([session]);

    const toggle = screen.getByTestId("session-steer-toggle-work-live-4");
    fireEvent.click(toggle);
    const input = screen.getByTestId("session-steer-input-work-live-4");
    fireEvent.change(input, { target: { value: "draft to discard" } });
    fireEvent.keyDown(input, { key: "Escape" });

    expect(screen.queryByTestId("session-steer-input-work-live-4")).not.toBeInTheDocument();
    expect(toggle).toHaveFocus();
  });

  it("SessionsSection_should_DisableSendWithoutClosingComposer_When_SessionEndsWhileComposerOpen", () => {
    // Pre-mortem failure #2 (P2): the Send button must re-derive
    // isSteerable(s) from the current `s` prop on every render, not close
    // over the value from when the composer was opened.
    const session = makeSession({ sessionId: "work-live-5", role: "work" });
    const { rerender } = renderSteerSection([session]);

    fireEvent.click(screen.getByTestId("session-steer-toggle-work-live-5"));
    const input = screen.getByTestId("session-steer-input-work-live-5");
    fireEvent.change(input, { target: { value: "in-flight steer" } });

    const submitBtn = screen.getByTestId("session-steer-submit-work-live-5");
    expect(submitBtn).not.toBeDisabled();

    // Same session object, now ended — simulate a poll/refresh landing while
    // the composer is still open.
    const endedSession: LinkedSession = { ...session, endedAt: new Date().toISOString() };
    const item = makeItem([endedSession]);
    rerender(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn().mockResolvedValue(undefined)}
        steeringSessionId={null}
      />
    );

    // Composer stays mounted (no close/reopen required)...
    expect(screen.getByTestId("session-steer-input-work-live-5")).toBeInTheDocument();
    // ...but Send is now disabled.
    expect(screen.getByTestId("session-steer-submit-work-live-5")).toBeDisabled();
  });

  it("SessionsSection_should_KeepComposerOpenAndShowError_When_OnSteerSessionRejects", async () => {
    const session = makeSession({ sessionId: "work-live-6", role: "work" });
    const onSteerSession = jest.fn().mockRejectedValue(new Error("steer failed: session busy"));
    renderSteerSection([session], { onSteerSession });

    fireEvent.click(screen.getByTestId("session-steer-toggle-work-live-6"));
    const input = screen.getByTestId("session-steer-input-work-live-6");
    fireEvent.change(input, { target: { value: "retry me" } });
    fireEvent.click(screen.getByTestId("session-steer-submit-work-live-6"));

    await waitFor(() => {
      expect(screen.getByText("steer failed: session busy")).toBeInTheDocument();
    });
    // Composer remains open on failure — not closed optimistically.
    expect(screen.getByTestId("session-steer-input-work-live-6")).toBeInTheDocument();
  });

  it("SessionsSection_should_NotLeakDraftAcrossSessions_When_SwitchingSteerTargetWithoutSending", () => {
    // Regression test for PR #457 code review finding: steerDraft was a
    // single component-level string, not keyed per-session, so an unsent
    // draft typed for session A survived into session B's composer when the
    // operator switched Steer targets without sending — risking A's message
    // being sent to session B instead.
    const sessionA = makeSession({ sessionId: "work-live-a", role: "work" });
    const sessionB = makeSession({ sessionId: "work-live-b", role: "work" });
    renderSteerSection([sessionA, sessionB]);

    // Open session A's composer and type a partial, unsent message.
    fireEvent.click(screen.getByTestId("session-steer-toggle-work-live-a"));
    const inputA = screen.getByTestId("session-steer-input-work-live-a");
    fireEvent.change(inputA, { target: { value: "A's still-unsent message" } });

    // Switch to session B's composer without sending A's message.
    fireEvent.click(screen.getByTestId("session-steer-toggle-work-live-b"));

    // A's composer is closed; B's composer must open empty, not pre-filled
    // with A's draft.
    expect(screen.queryByTestId("session-steer-input-work-live-a")).not.toBeInTheDocument();
    const inputB = screen.getByTestId("session-steer-input-work-live-b");
    expect(inputB).toHaveValue("");
  });

  it("SessionsSection_should_ShowCommitCountAndLastMessage_When_SessionHasCommits", () => {
    const item = makeItem([
      makeSession({
        sessionId: "work-with-commits",
        role: "work",
        commitCountSinceSpawn: 3,
        lastCommitMessage: "fix(session): handle nil pointer\n\nLonger body text here.",
      }),
    ]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn()}
        steeringSessionId={null}
      />
    );

    const detail = screen.getByText("3 commits — fix(session): handle nil pointer");
    expect(detail).toHaveAttribute(
      "title",
      "fix(session): handle nil pointer\n\nLonger body text here."
    );
  });

  it("SessionsSection_should_UseSingularCommitLabel_When_ExactlyOneCommit", () => {
    const item = makeItem([
      makeSession({
        sessionId: "work-one-commit",
        role: "work",
        commitCountSinceSpawn: 1,
        lastCommitMessage: "chore: bump version",
      }),
    ]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn()}
        steeringSessionId={null}
      />
    );

    expect(screen.getByText("1 commit — chore: bump version")).toBeInTheDocument();
  });

  it("SessionsSection_should_NotRenderCommitDetail_When_SessionHasNoCommits", () => {
    const item = makeItem([makeSession({ sessionId: "work-no-commits", role: "work" })]);
    render(
      <SessionsSection
        item={item}
        pipelineModes={[]}
        latestWorkSession={undefined}
        deletingSessionId={null}
        defaultExpanded={true}
        onDeleteSession={jest.fn()}
        onSteerSession={jest.fn()}
        steeringSessionId={null}
      />
    );

    expect(screen.queryByText(/\d+ commits? —/)).not.toBeInTheDocument();
  });
});
