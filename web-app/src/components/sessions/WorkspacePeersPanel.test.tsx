import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import { WorkspacePeersPanel, peerLifecycle, GOAL_STALE_THRESHOLD_MS } from "./WorkspacePeersPanel";
import sessionsReducer from "@/lib/store/sessionsSlice";
import type { Session, SessionGoalSummary } from "@/gen/session/v1/types_pb";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";

// Minimal factories for test proto objects — mirrors GoalPanel.test.tsx's makeGoal pattern.
function makeSession(overrides: Partial<Session>): Session {
  return {
    id: "id",
    title: "title",
    status: SessionStatus.ACTIVE,
    path: "/home/user/repo",
    ...overrides,
  } as unknown as Session;
}

function makeGoal(overrides: Partial<SessionGoalSummary>): SessionGoalSummary {
  return {
    goalText: "",
    status: "working",
    tasksTotal: 0,
    tasksDone: 0,
    tasksJson: "[]",
    ...overrides,
  } as unknown as SessionGoalSummary;
}

function renderWithStore(session: Session, peers: Session[]) {
  const store = configureStore({
    reducer: { sessions: sessionsReducer },
    preloadedState: {
      sessions: {
        ids: peers.map((p) => p.id),
        entities: Object.fromEntries(peers.map((p) => [p.id, p])),
        loading: false,
        hasLoadedOnce: false,
        error: null,
        connectionState: "connected" as const,
        detectedStatusMap: {},
        deletedIds: {},
      },
    },
  });
  return render(
    <Provider store={store}>
      <WorkspacePeersPanel session={session} />
    </Provider>
  );
}

describe("WorkspacePeersPanel", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("renders nothing when the session has no path", () => {
    const self = makeSession({ id: "self", path: "" });
    renderWithStore(self, []);
    expect(screen.queryByTestId("workspace-peers-panel")).toBeNull();
  });

  it("renders nothing when there are no peers", () => {
    const self = makeSession({ id: "self" });
    renderWithStore(self, [self]);
    expect(screen.queryByTestId("workspace-peers-panel")).toBeNull();
  });

  it("excludes the caller's own session and sessions with a different path", () => {
    const self = makeSession({ id: "self" });
    const samePeer = makeSession({ id: "peer-1", title: "peer one" });
    const otherDirectory = makeSession({ id: "peer-2", path: "/home/user/other-repo" });
    renderWithStore(self, [self, samePeer, otherDirectory]);
    const items = screen.getAllByTestId("workspace-peer-item");
    expect(items).toHaveLength(1);
    expect(screen.getByText("peer one")).toBeInTheDocument();
  });

  // Regression test for the false-positive collision bug: session.path is the
  // original/logical repo path and is never updated for worktree sessions (see
  // session/instance_snapshot.go's Path field and Instance.GetEffectiveRootDir()), so two
  // sessions each in their own isolated worktree of the same repo report identical
  // session.path despite operating in different directories on disk.
  it("does not flag two sessions in their own separate worktrees of the same repo as peers", () => {
    const self = makeSession({
      id: "self",
      path: "/home/user/repo",
      gitWorktree: { worktreePath: "/home/user/repo-worktrees/self" } as Session["gitWorktree"],
    });
    const otherWorktree = makeSession({
      id: "peer-1",
      title: "peer one",
      path: "/home/user/repo",
      gitWorktree: { worktreePath: "/home/user/repo-worktrees/peer-1" } as Session["gitWorktree"],
    });
    renderWithStore(self, [self, otherWorktree]);
    expect(screen.queryByTestId("workspace-peers-panel")).toBeNull();
  });

  it("flags two sessions genuinely sharing one worktree directory as peers", () => {
    const self = makeSession({
      id: "self",
      path: "/home/user/repo",
      gitWorktree: { worktreePath: "/home/user/repo-worktrees/shared" } as Session["gitWorktree"],
    });
    const samePeer = makeSession({
      id: "peer-1",
      title: "peer one",
      path: "/home/user/repo",
      gitWorktree: { worktreePath: "/home/user/repo-worktrees/shared" } as Session["gitWorktree"],
    });
    renderWithStore(self, [self, samePeer]);
    const items = screen.getAllByTestId("workspace-peer-item");
    expect(items).toHaveLength(1);
    expect(screen.getByText("peer one")).toBeInTheDocument();
  });

  it("shows the peer's goal text when set", () => {
    const self = makeSession({ id: "self" });
    const peer = makeSession({
      id: "peer-1",
      goal: makeGoal({ goalText: "fix the bug" }),
    });
    renderWithStore(self, [self, peer]);
    expect(screen.getByText("fix the bug")).toBeInTheDocument();
  });

  it("hides itself when dismissed, and remembers the dismissal for this session", () => {
    const self = makeSession({ id: "self" });
    const peer = makeSession({ id: "peer-1" });
    const { unmount } = renderWithStore(self, [self, peer]);
    fireEvent.click(screen.getByTestId("workspace-peers-dismiss"));
    expect(screen.queryByTestId("workspace-peers-panel")).toBeNull();
    unmount();

    renderWithStore(self, [self, peer]);
    expect(screen.queryByTestId("workspace-peers-panel")).toBeNull();
  });
});

describe("peerLifecycle", () => {
  const now = Date.parse("2026-01-01T00:00:00Z");

  it("returns gone when status is stopped, regardless of goal recency", () => {
    const peer = makeSession({ status: SessionStatus.STOPPED });
    expect(peerLifecycle(peer, now)).toBe("gone");
  });

  it("returns active when live with a recently updated goal", () => {
    const peer = makeSession({
      status: SessionStatus.ACTIVE,
      goal: makeGoal({ goalText: "x", updatedAt: timestampFromDate(new Date(now - 60_000)) }),
    });
    expect(peerLifecycle(peer, now)).toBe("active");
  });

  it("returns stuck when live but goal is stale (>30min)", () => {
    const peer = makeSession({
      status: SessionStatus.ACTIVE,
      goal: makeGoal({ goalText: "x", updatedAt: timestampFromDate(new Date(now - 31 * 60_000)) }),
    });
    expect(peerLifecycle(peer, now)).toBe("stuck");
  });

  it("returns active when live with no goal set at all", () => {
    const peer = makeSession({ status: SessionStatus.ACTIVE });
    expect(peerLifecycle(peer, now)).toBe("active");
  });

  it("treats elapsed exactly equal to the stale threshold as not yet stale", () => {
    const peer = makeSession({
      status: SessionStatus.ACTIVE,
      goal: makeGoal({ goalText: "x", updatedAt: timestampFromDate(new Date(now - GOAL_STALE_THRESHOLD_MS)) }),
    });
    expect(peerLifecycle(peer, now)).toBe("active");
  });

  it("treats elapsed one ms past the stale threshold as stuck", () => {
    const peer = makeSession({
      status: SessionStatus.ACTIVE,
      goal: makeGoal({ goalText: "x", updatedAt: timestampFromDate(new Date(now - GOAL_STALE_THRESHOLD_MS - 1)) }),
    });
    expect(peerLifecycle(peer, now)).toBe("stuck");
  });
});
