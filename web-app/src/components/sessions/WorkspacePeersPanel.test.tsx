import React from "react";
import { render, screen } from "@testing-library/react";
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
    workspaceKey: "gh:acme/widgets",
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
  it("renders nothing when the session has no workspace key", () => {
    const self = makeSession({ id: "self", workspaceKey: "" });
    renderWithStore(self, []);
    expect(screen.queryByTestId("workspace-peers-panel")).toBeNull();
  });

  it("renders nothing when there are no peers", () => {
    const self = makeSession({ id: "self" });
    renderWithStore(self, [self]);
    expect(screen.queryByTestId("workspace-peers-panel")).toBeNull();
  });

  it("excludes the caller's own session and sessions on other workspaces", () => {
    const self = makeSession({ id: "self" });
    const samePeer = makeSession({ id: "peer-1", title: "peer one" });
    const otherRepo = makeSession({ id: "peer-2", workspaceKey: "gh:other/repo" });
    renderWithStore(self, [self, samePeer, otherRepo]);
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
