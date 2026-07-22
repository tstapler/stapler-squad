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
import { render, screen, act, fireEvent, within } from "@testing-library/react";
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
    progressNotes: [],
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

describe("BacklogItemDetail — Story 1.1.2: current work session selector (D3)", () => {
  it("BacklogItemDetail_should_ReturnIdenticalWorkSessionAcrossAllCallSites_When_MultipleWorkSessionsLinked", async () => {
    useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
    useBacklogItemShipStatusMock.mockReturnValue({ data: null, loading: false, refetch: jest.fn() });

    const sessions = [
      makeSession({ entityId: "s1", sessionId: "session-older", role: "work", startedAt: "2026-07-01T00:00:00Z" }),
      makeSession({ entityId: "s2", sessionId: "session-newer", role: "work", startedAt: "2026-07-02T00:00:00Z" }),
    ];

    // Actions-section call site: item.status === "in_progress".
    getBacklogItem.mockReset().mockResolvedValue({ ...makeItem(sessions), status: "in_progress" });
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    // Header call site: useVcsStatus is called with the current work session's id.
    expect(useVcsStatusMock).toHaveBeenCalledWith("session-newer", expect.anything());

    // Actions-section call site: "View Session" link targets the same session.
    expect(screen.getByTestId("backlog-action-view-session")).toHaveAttribute(
      "href",
      "/?session=session-newer"
    );
  });

  it("BacklogItemDetail_should_ReturnIdenticalWorkSessionAcrossAllCallSites_When_StatusIsReview", async () => {
    useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
    useBacklogItemShipStatusMock.mockReturnValue({ data: null, loading: false, refetch: jest.fn() });

    const sessions = [
      makeSession({ entityId: "s1", sessionId: "session-older", role: "work", startedAt: "2026-07-01T00:00:00Z" }),
      makeSession({ entityId: "s2", sessionId: "session-newer", role: "work", startedAt: "2026-07-02T00:00:00Z" }),
    ];

    // Reviewing-section call site: item.status === "review".
    getBacklogItem.mockReset().mockResolvedValue({ ...makeItem(sessions), status: "review" });
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    // Header call site: useVcsStatus is called with the current work session's id.
    expect(useVcsStatusMock).toHaveBeenCalledWith("session-newer", expect.anything());

    // Reviewing-section call site: the "Work session" link targets the same session.
    const reviewingHeading = screen.getByText("Reviewing");
    const reviewingSection = reviewingHeading.closest("div") as HTMLElement;
    expect(within(reviewingSection).getByText("session-newer")).toBeInTheDocument();
    expect(within(reviewingSection).queryByText("session-older")).not.toBeInTheDocument();
  });
});

describe("BacklogItemDetail — Story 1.1.3: session kind classifier wired into the Sessions row", () => {
  it("renders a manual-review- session as a non-clickable span, not a dead <a href> link", async () => {
    useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
    useBacklogItemShipStatusMock.mockReturnValue({ data: null, loading: false, refetch: jest.fn() });

    const session = makeSession({
      entityId: "s1",
      sessionId: "manual-review-a1b2c3d4-1721577600000000000",
      role: "review",
    });
    getBacklogItem.mockReset().mockResolvedValue(makeItem([session]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(
      screen.queryByRole("link", { name: /manual-review-a1b2c3d4/ })
    ).not.toBeInTheDocument();
    expect(screen.getByText("manual-review-a1b2c3d4-1721577600000000000")).toBeInTheDocument();
  });

  it("renders a diff-error- session as a non-clickable span, not a dead <a href> link", async () => {
    useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
    useBacklogItemShipStatusMock.mockReturnValue({ data: null, loading: false, refetch: jest.fn() });

    const session = makeSession({
      entityId: "s1",
      sessionId: "diff-error-a1b2c3d4",
      role: "review",
    });
    getBacklogItem.mockReset().mockResolvedValue(makeItem([session]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.queryByRole("link", { name: /diff-error-a1b2c3d4/ })).not.toBeInTheDocument();
    expect(screen.getByText("diff-error-a1b2c3d4")).toBeInTheDocument();
  });

  it("still renders a normal work session as a clickable link", async () => {
    useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
    useBacklogItemShipStatusMock.mockReturnValue({ data: null, loading: false, refetch: jest.fn() });

    const session = makeSession({
      entityId: "s1",
      sessionId: "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
      role: "work",
    });
    getBacklogItem.mockReset().mockResolvedValue(makeItem([session]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(
      screen.getByRole("link", { name: /a1b2c3d4-e5f6-7890-abcd-1234567890ab/ })
    ).toHaveAttribute("href", "/?session=a1b2c3d4-e5f6-7890-abcd-1234567890ab");
  });
});
