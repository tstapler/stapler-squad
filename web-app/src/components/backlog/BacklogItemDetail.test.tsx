/**
 * Tests for BacklogItemDetail's Epic 3.4 "what ran" surface — the per-session
 * "Pipeline" group that renders `ItemSession.pipelineModeSnapshot` and
 * content-drift detection against the currently-fetched mode list.
 * See project_plans/backlog-configurable-pipeline/implementation/plan.md
 * Story 3.4.1 and project_plans/backlog-configurable-pipeline/design/ux.md
 * section F.
 *
 * Covers the 4 cases from plan.md's acceptance criteria:
 *  1. Found + unchanged mode → mode name only, no drift annotation
 *  2. Found + drifted (content hash mismatch) → "<name> (content since changed)"
 *  3. Unrecognized/deleted mode slug → "custom (unrecognized mode: '<slug>')"
 *  4. Default mode (pipelineModeSnapshot === "") → "default", no drift check
 */

import React from "react";
import { render, screen, act, fireEvent } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem, LinkedSession, PipelineMode } from "@/lib/hooks/useBacklogService";
import { VCSStatusSchema } from "@/gen/session/v1/types_pb";
import { BacklogItemShipStatusSchema } from "@/gen/session/v1/backlog_pb";

// Heavy children pull their own hooks/timers; stub them out so this test is
// focused on BacklogItemDetail's own render behavior.
jest.mock("./SessionMonitor", () => ({ SessionMonitor: () => null }));
jest.mock("./GateVerdictBox", () => ({ GateVerdictBox: () => null }));
jest.mock("./TriageReviewPanel", () => ({ TriageReviewPanel: () => null }));
jest.mock("./TriageLoadingIndicator", () => ({ TriageLoadingIndicator: () => null }));

// ReviewChangesModal makes a real ConnectRPC call on mount — stub it to a marker
// element so Story 2.2.3's "View Diff opens the modal" tests can assert it opened
// without standing up a transport.
jest.mock("./ReviewChangesModal", () => ({
  ReviewChangesModal: () => <div data-testid="review-changes-modal-stub" />,
}));

// BacklogFileBrowserModal pulls in FileTree/FileContentViewer, which need a
// real ConnectRPC transport — stub it the same way as ReviewChangesModal so
// the "Browse files" wiring test can assert it opened without standing one up.
jest.mock("./BacklogFileBrowserModal", () => ({
  BacklogFileBrowserModal: () => <div data-testid="file-browser-modal-stub" />,
}));

const useVcsStatusMock = jest.fn();
jest.mock("@/lib/hooks/useVcsStatus", () => ({
  useVcsStatus: (...args: unknown[]) => useVcsStatusMock(...args),
}));

const useBacklogItemShipStatusMock = jest.fn();
jest.mock("@/lib/hooks/useBacklogItemShipStatus", () => ({
  useBacklogItemShipStatus: (...args: unknown[]) => useBacklogItemShipStatusMock(...args),
}));

// The edit-mode branch renders BacklogItemForm -> RepoPathInput, which uses
// useSessionRepoPaths (Redux) and usePathCompletions (RPC). Stub both so this
// test doesn't need a Redux store or ConnectRPC transport. Not exercised by
// these tests (editMode is never entered) but required at import time.
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

// useSessionService pulls in useAnalytics, which requires an
// AnalyticsContextProvider we don't want to stand up for this focused test.
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ deleteSession: jest.fn() }),
}));

// BacklogItemDetail itself also calls useAnalytics() directly for the
// session-delete tracking event — mock it the same way.
jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

const getBacklogItem = jest.fn();
const listPipelineModes = jest.fn();

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus: jest.fn().mockResolvedValue(true),
    triggerTriage: jest.fn(),
    cancelTriage: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    triggerShipPR: jest.fn(),
    submitManualReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    deleteBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

// The jest styleMock for `.css.ts` files wraps every export (including plain
// `style()` string exports) in a callable proxy function, which triggers a
// benign "Invalid value for prop className" React warning. Pre-existing
// jest/vanilla-extract mock limitation — see RadioGroup.test.tsx and
// BacklogItemForm.test.tsx, which silence it the same way.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
  useBacklogItemShipStatusMock.mockReturnValue({ data: null, loading: false, refetch: jest.fn() });
});

function makeMode(overrides: Partial<PipelineMode> & Pick<PipelineMode, "slug" | "name">): PipelineMode {
  return {
    id: `id-${overrides.slug}`,
    description: "",
    enabled: true,
    statusCommandTemplate: "",
    doneCommandTemplate: "",
    failCommandTemplate: "",
    reviewCommandTemplate: "",
    shipCommandTemplate: "",
    helpCommandTemplate: "",
    triagePromptTemplate: "",
    reviewPromptTemplate: "",
    initialPromptTemplate: "",
    contentHash: "hash-v1",
    ...overrides,
  };
}

const QUICK_MODE = makeMode({ slug: "quick", name: "Quick Fix", contentHash: "hash-v1" });

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: "session-entity-1",
    sessionId: "session-1",
    role: "triage",
    estimatedCostUsd: 0,
    pipelineModeSnapshot: "",
    pipelineModeSnapshotHash: "",
    ...overrides,
  };
}

function makeItem(linkedSessions: LinkedSession[]): BacklogItem {
  return {
    id: "item-1",
    title: "Refactor auth middleware",
    description: "desc",
    status: "idea",
    priority: 3,
    repoPath: "/tmp/repo",
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    planApproved: false,
    acCriteria: [],
    linkedSessions,
    notes: "",
    createdAt: "2026-07-12T14:02:00Z",
    updatedAt: "2026-07-12T14:02:00Z",
    statusEvents: [],
    totalEstimatedCostUsd: 0,
  };
}

async function renderWithSession(session: LinkedSession, modes: PipelineMode[]) {
  getBacklogItem.mockReset().mockResolvedValue(makeItem([session]));
  listPipelineModes.mockReset().mockResolvedValue(modes);

  render(<BacklogItemDetail itemId="item-1" />);

  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("BacklogItemDetail — Epic 3.4 'what ran' Pipeline surface", () => {
  it("Case 1 — found, unchanged: shows the mode's name with no drift annotation", async () => {
    const session = makeSession({
      pipelineModeSnapshot: "quick",
      pipelineModeSnapshotHash: "hash-v1", // matches QUICK_MODE.contentHash
    });
    await renderWithSession(session, [QUICK_MODE]);

    const group = screen.getByRole("group", { name: "Pipeline" });
    expect(group).toHaveTextContent("Quick Fix");
    expect(group).not.toHaveTextContent("content since changed");
  });

  it("Case 2 — found, but drifted: appends '(content since changed)' to the mode name", async () => {
    const session = makeSession({
      pipelineModeSnapshot: "quick",
      pipelineModeSnapshotHash: "hash-v0", // stale — differs from QUICK_MODE's current contentHash
    });
    await renderWithSession(session, [QUICK_MODE]);

    const group = screen.getByRole("group", { name: "Pipeline" });
    expect(group).toHaveTextContent("Quick Fix (content since changed)");
  });

  it("Case 3 — unrecognized/deleted mode: shows the custom-unrecognized fallback, takes priority over drift", async () => {
    const session = makeSession({
      pipelineModeSnapshot: "legacy-fast",
      pipelineModeSnapshotHash: "some-hash",
    });
    // "legacy-fast" is not in the currently-fetched mode list — deleted/renamed.
    await renderWithSession(session, [QUICK_MODE]);

    const group = screen.getByRole("group", { name: "Pipeline" });
    expect(group).toHaveTextContent("custom (unrecognized mode: 'legacy-fast')");
    expect(group).not.toHaveTextContent("content since changed");
  });

  it("Case 4 — default mode: renders 'default' with no drift check attempted", async () => {
    const session = makeSession({
      pipelineModeSnapshot: "",
      pipelineModeSnapshotHash: "",
    });
    await renderWithSession(session, [QUICK_MODE]);

    const group = screen.getByRole("group", { name: "Pipeline" });
    expect(group).toHaveTextContent("default");
    expect(group).not.toHaveTextContent("content since changed");
  });
});

describe("BacklogItemDetail — Story 2.2.3: VcsWidget wiring", () => {
  it("BacklogItemDetail_should_RenderShippedPillWithViewDiff_When_VcsStatusNullAndShipStatusShipped", async () => {
    useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
    useBacklogItemShipStatusMock.mockReturnValue({
      data: create(BacklogItemShipStatusSchema, {
        shipped: true,
        shippedVia: "pr",
        branchName: "feature/foo",
        branchExists: false,
      }),
      loading: false,
      refetch: jest.fn(),
    });

    const session = makeSession({ role: "work", worktreePath: "/tmp/repo-wt" });
    getBacklogItem.mockReset().mockResolvedValue(makeItem([session]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.getByText("Shipped")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("vcs-widget-view-diff"));

    expect(screen.getByTestId("review-changes-modal-stub")).toBeInTheDocument();
  });

  it("BacklogItemDetail_should_PreferLiveVcsStatusOverShipStatus_When_BothResolveNonNull", async () => {
    useVcsStatusMock.mockReturnValue({
      data: create(VCSStatusSchema, { branch: "feat/live-branch", isClean: true }),
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    useBacklogItemShipStatusMock.mockReturnValue({
      data: create(BacklogItemShipStatusSchema, { shipped: true, branchName: "feat/historical-branch" }),
      loading: false,
      refetch: jest.fn(),
    });

    const sessions = [
      makeSession({ entityId: "s1", sessionId: "session-1", role: "work" }),
      makeSession({ entityId: "s2", sessionId: "session-2", role: "work" }),
    ];
    getBacklogItem.mockReset().mockResolvedValue(makeItem(sessions));
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    // Live vcsStatus wins over the historical shipStatus when both resolve non-null.
    expect(screen.getByText("feat/live-branch")).toBeInTheDocument();
    expect(screen.queryByText("feat/historical-branch")).not.toBeInTheDocument();

    // 2 linked sessions with role "work" and no endedAt (active) → activeSessionCount=2.
    expect(screen.getByText("2 active sessions")).toBeInTheDocument();
  });

  it("BacklogItemDetail_should_OpenFileBrowserModal_When_BrowseFilesButtonClicked", async () => {
    useVcsStatusMock.mockReturnValue({
      data: create(VCSStatusSchema, { branch: "feat/live-branch", isClean: true }),
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    useBacklogItemShipStatusMock.mockReturnValue({ data: null, loading: false, refetch: jest.fn() });

    const session = makeSession({ role: "work", worktreePath: "/tmp/repo-wt" });
    getBacklogItem.mockReset().mockResolvedValue(makeItem([session]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.queryByTestId("file-browser-modal-stub")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Browse files in this worktree" }));

    expect(screen.getByTestId("file-browser-modal-stub")).toBeInTheDocument();
  });
});
