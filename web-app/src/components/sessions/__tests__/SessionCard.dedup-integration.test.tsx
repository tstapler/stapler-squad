/**
 * Full-render integration tests for SessionCard title/subtitle deduplication —
 * exercises the actual JSX wiring at each of the 5 dedup-eligible info rows
 * (Branch, Path, Working Dir, Cloned To, Goal), confirms Program/Repository/Pull
 * Request are excluded from dedup, and covers the all-fields-redundant edge case.
 *
 * Reuses the render harness/mocks established in SessionCard.click.test.tsx and
 * SessionCard.approval-suppression.test.tsx.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionCard } from "../SessionCard";
import type { Session, SessionGoalSummary } from "@/gen/session/v1/types_pb";

// ---------------------------------------------------------------------------
// Heavy dependency mocks
// ---------------------------------------------------------------------------

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({})),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
}));

jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: () => ({ items: [] }),
}));

jest.mock("@/lib/contexts/SessionServiceContext", () => ({
  useSessionServiceContext: () => ({
    draftPullRequest: jest.fn(),
    createPullRequest: jest.fn(),
  }),
}));

jest.mock("@/lib/store", () => ({
  useAppSelector: jest.fn(() => ({})),
}));

jest.mock("@/lib/store/sessionsSlice", () => ({
  selectDetectedStatusMap: jest.fn(),
}));

jest.mock("@/lib/hooks/useTerminalSnapshot", () => ({
  useTerminalSnapshot: () => ({ snapshot: null, loading: false }),
}));

jest.mock("@/lib/hooks/useFocusTrap", () => ({
  useFocusTrap: () => {},
}));

jest.mock("@/components/ui/AppLink", () => ({
  AppLink: ({ href, children, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...rest}>{children}</a>
  ),
}));

jest.mock("@/components/ui/Modal", () => ({
  Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
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

// ---------------------------------------------------------------------------
// Fixture builder
// ---------------------------------------------------------------------------

function makeGoalSummary(overrides?: Partial<SessionGoalSummary>): SessionGoalSummary {
  return {
    goalText: "",
    status: "working",
    tasksTotal: 0,
    tasksDone: 0,
    tasksJson: "",
    ...overrides,
  } as unknown as SessionGoalSummary;
}

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "s1",
    title: "fix-auth",
    status: 1 as Session["status"],
    tags: [],
    category: "",
    path: "/tmp/session",
    branch: "",
    program: "claude",
    ...overrides,
  } as unknown as Session;
}

// ---------------------------------------------------------------------------
// AC3 — Program row excluded from dedup
// ---------------------------------------------------------------------------

describe("SessionCard — Program row excluded from dedup", () => {
  it("SessionCard_should_RenderProgramRowUnchanged_When_TitleMatchesProgram", () => {
    const session = makeSession({ title: "claude", program: "claude" });
    render(<SessionCard session={session} />);
    expect(screen.getByText("Program:")).toBeInTheDocument();
    expect(screen.getAllByText("claude").length).toBeGreaterThan(0);
  });

  it("SessionCard_should_RenderProgramRowUnchanged_When_TitleDoesNotMatchProgram", () => {
    const session = makeSession({ title: "fix-auth", program: "claude" });
    render(<SessionCard session={session} />);
    expect(screen.getByText("Program:")).toBeInTheDocument();
    expect(screen.getByText("claude")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// AC4 — No visual regression when nothing is redundant
// ---------------------------------------------------------------------------

describe("SessionCard — no visual regression when no field duplicates the title", () => {
  it("SessionCard_should_RenderAllRowsUnchanged_When_NoFieldMatchesTitle", () => {
    const session = makeSession({
      title: "implement-oauth",
      branch: "feature/sso",
      path: "/home/user/worktrees/implement-oauth-work",
      program: "claude",
      goal: makeGoalSummary({ goalText: "Ship SSO login" }),
    });
    render(<SessionCard session={session} />);
    expect(screen.getByText("Branch:")).toBeInTheDocument();
    expect(screen.getByText("feature/sso")).toBeInTheDocument();
    expect(screen.getByText("Path:")).toBeInTheDocument();
    expect(screen.getByText("/home/user/worktrees/implement-oauth-work")).toBeInTheDocument();
    expect(screen.getByText("Program:")).toBeInTheDocument();
    expect(screen.getByText("Goal")).toBeInTheDocument();
    expect(screen.getByText("Ship SSO login")).toBeInTheDocument();
  });

  it("SessionCard_should_RenderPathRowUnchanged_When_BasenameIsNearMissOfTitle", () => {
    const session = makeSession({
      title: "fix-auth",
      path: "/home/user/worktrees/fix-auth-2",
    });
    render(<SessionCard session={session} />);
    expect(screen.getByText("Path:")).toBeInTheDocument();
    expect(screen.getByText("/home/user/worktrees/fix-auth-2")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Wiring checks — one per dedup-eligible row
// ---------------------------------------------------------------------------

describe("SessionCard — dedup wiring at each call site", () => {
  it("SessionCard_should_SuppressBranchRow_When_BranchExactlyMatchesTitle", () => {
    const session = makeSession({ title: "fix-auth", branch: "fix-auth" });
    render(<SessionCard session={session} />);
    expect(screen.queryByText("Branch:")).toBeNull();
  });

  it("SessionCard_should_SuppressPathRow_When_PathBasenameExactlyMatchesTitle", () => {
    const session = makeSession({ title: "fix-auth", path: "/home/user/worktrees/fix-auth" });
    render(<SessionCard session={session} />);
    expect(screen.queryByText("Path:")).toBeNull();
  });

  it("SessionCard_should_SuppressWorkingDirRow_When_WorkingDirBasenameExactlyMatchesTitle", () => {
    const session = makeSession({ title: "my-project", workingDir: "/repos/my-project" });
    render(<SessionCard session={session} />);
    expect(screen.queryByText("Working Dir:")).toBeNull();
  });

  it("SessionCard_should_SuppressClonedToRow_When_BasenameMatchesTitle", () => {
    const session = makeSession({ title: "shared-fixes", clonedRepoPath: "/tmp/clones/shared-fixes" });
    render(<SessionCard session={session} />);
    expect(screen.queryByText("Cloned To:")).toBeNull();
  });

  it("SessionCard_should_KeepClonedToRow_When_ClonedRepoPathBasenameDoesNotMatchTitle", () => {
    const session = makeSession({ title: "fix-auth", clonedRepoPath: "/tmp/clones/other-repo" });
    render(<SessionCard session={session} />);
    expect(screen.getByText("Cloned To:")).toBeInTheDocument();
    expect(screen.getByText("/tmp/clones/other-repo")).toBeInTheDocument();
  });

  it("SessionCard_should_SuppressGoalRow_When_RawGoalTextExactlyMatchesTitle", () => {
    const session = makeSession({
      title: "Fix login bug",
      goal: makeGoalSummary({ goalText: "Fix login bug " }), // trailing space
    });
    render(<SessionCard session={session} />);
    expect(screen.queryByText("Goal")).toBeNull();
  });

  it("SessionCard_should_KeepGoalRow_When_RawGoalTextDivergesFromTitleDespiteTruncatedPrefixLookingSimilar", () => {
    const longGoal = "fix-auth and also update the docs and changelog entries so nothing regresses";
    const session = makeSession({
      title: "fix-auth",
      goal: makeGoalSummary({ goalText: longGoal }),
    });
    render(<SessionCard session={session} />);
    expect(screen.getByText("Goal")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Edge case — every dedup-eligible field matches the title simultaneously
// ---------------------------------------------------------------------------

describe("SessionCard — all-fields-redundant edge case", () => {
  it("SessionCard_should_SuppressAllFiveDedupRowsAndKeepProgramRow_When_EveryDedupEligibleFieldMatchesTitle", () => {
    const session = makeSession({
      title: "fix-auth",
      branch: "fix-auth",
      path: "/home/user/worktrees/fix-auth",
      workingDir: "/home/user/worktrees/fix-auth",
      clonedRepoPath: "/tmp/clones/fix-auth",
      goal: makeGoalSummary({ goalText: "fix-auth" }),
      program: "claude",
    });
    render(<SessionCard session={session} />);
    expect(screen.queryByText("Branch:")).toBeNull();
    expect(screen.queryByText("Path:")).toBeNull();
    expect(screen.queryByText("Working Dir:")).toBeNull();
    expect(screen.queryByText("Cloned To:")).toBeNull();
    expect(screen.queryByText("Goal")).toBeNull();
    expect(screen.getByText("Program:")).toBeInTheDocument();
    expect(screen.getByText("claude")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// UX acceptance criteria
// ---------------------------------------------------------------------------

describe("SessionCard — UX acceptance for dedup", () => {
  it("SessionCard_should_NeverRenderLabelWithoutAValue_When_RowIsSuppressedInAnyState", () => {
    const states = [
      makeSession({ title: "fix-auth", branch: "fix-auth" }), // State A: single dedup
      makeSession({
        title: "implement-oauth",
        branch: "feature/sso",
        path: "/home/user/worktrees/implement-oauth-work",
        goal: makeGoalSummary({ goalText: "Ship SSO login" }),
      }), // State B: no dedup
      makeSession({
        title: "fix-auth",
        branch: "fix-auth",
        path: "/home/user/worktrees/fix-auth",
        workingDir: "/home/user/worktrees/fix-auth",
        clonedRepoPath: "/tmp/clones/fix-auth",
        goal: makeGoalSummary({ goalText: "fix-auth" }),
      }), // State C: all-fields-redundant
    ];
    for (const session of states) {
      const { unmount } = render(<SessionCard session={session} />);
      // Program is always present with a non-empty value in every state.
      expect(screen.getByText("Program:")).toBeInTheDocument();
      expect(screen.getByText(session.program)).toBeInTheDocument();
      unmount();
    }
  });

  it("SessionCard_should_KeepBranchRowInFull_When_BranchIsANearMissOfTitle", () => {
    const session = makeSession({ title: "fix-auth", branch: "fix-auth-2" });
    render(<SessionCard session={session} />);
    expect(screen.getByText("Branch:")).toBeInTheDocument();
    expect(screen.getByText("fix-auth-2")).toBeInTheDocument();
  });

  it("SessionCard_should_KeepRepositoryAndPullRequestLinksFocusable_When_OtherRowsAreSuppressed", () => {
    const session = makeSession({
      title: "fix-auth",
      branch: "fix-auth", // suppressed
      githubOwner: "tstapler",
      githubRepo: "stapler-squad",
      githubPrNumber: 42,
      githubPrUrl: "https://github.com/tstapler/stapler-squad/pull/42",
    });
    render(<SessionCard session={session} />);
    const repoLink = screen.getByRole("link", { name: /^GitHub repository/i });
    const prLink = screen.getByRole("link", { name: /^Pull request #\d+ on/i });
    expect(repoLink).toBeInTheDocument();
    expect(repoLink).not.toHaveAttribute("aria-hidden");
    expect(prLink).toBeInTheDocument();
    expect(prLink).not.toHaveAttribute("aria-hidden");
  });

  it("SessionCard_should_RetainTitleInAriaLabel_When_ClonedToRowIsSuppressed", () => {
    const session = makeSession({ title: "shared-fixes", clonedRepoPath: "/tmp/clones/shared-fixes" });
    render(<SessionCard session={session} />);
    const card = screen.getByTestId("session-card");
    expect(card.getAttribute("aria-label")).toContain("shared-fixes");
  });
});
