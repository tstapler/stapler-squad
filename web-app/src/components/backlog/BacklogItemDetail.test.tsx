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
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem, LinkedSession, PipelineMode } from "@/lib/hooks/useBacklogService";
import { VCSStatusSchema, FileStatus } from "@/gen/session/v1/types_pb";
import {
  BacklogItemShipStatusSchema,
  ShippedFileStatSchema,
  StuckReason,
  type StuckBacklogItem,
} from "@/gen/session/v1/backlog_pb";

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

// BacklogItemDetail calls useStuckBacklogItems() once and passes the
// resolved StuckBacklogItem down to LifecycleSummary as a prop — stub it so
// this suite never attempts a real ConnectRPC call. Individual tests that
// care about BlockerChip behavior override this per-case; everything else
// gets a stable "nothing stuck" default.
const useStuckBacklogItemsMock = jest.fn();
jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: (...args: unknown[]) => useStuckBacklogItemsMock(...args),
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
// Hoisted to module scope (unlike the other jest.fn()s below, which are
// re-created fresh every render) so Epic 5.3 tests can assert on calls made
// across a Save/Save-Anyway click, not just within a single render.
const updateBacklogItem = jest.fn().mockResolvedValue(null);
// Hoisted (like getBacklogItem/listPipelineModes above) so individual tests
// can control resolution timing — needed for the actionLoading polling-
// suspend regression test (Story 3.1.3, Task 3.1.3c).
const overrideVerdict = jest.fn();

jest.mock("@/lib/hooks/useBacklogService", () => ({
  // mapBacklogItem is a real (unmocked) named export — BacklogItemDetail's
  // Epic 5.3 live-update effect calls it to convert the raw proto item read
  // off the mocked store below into the domain shape this component renders.
  ...jest.requireActual("@/lib/hooks/useBacklogService"),
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus: jest.fn().mockResolvedValue(true),
    triggerTriage: jest.fn(),
    cancelTriage: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    overrideVerdict,
    triggerReReview: jest.fn(),
    triggerShipPR: jest.fn(),
    submitManualReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    deleteBacklogItem: jest.fn(),
    updateBacklogItem,
    listPipelineModes,
    lastError: null,
  }),
}));

// Epic 5.3 (backlog-event-driven-updates): BacklogItemDetail also subscribes
// via useWatchBacklogItems + a Redux selector (Task 5.3.1b), and opens its
// own lightweight raw watch stream for archive/removal terminal-state
// detection (Task 5.3.1c). Both are controllable per-test via the
// module-scope `mock*` holders below, reset in `beforeEach`.
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => ({ items: [], connectionState: "live" }),
}));

let mockLiveItemsMap: Record<string, unknown> = {};
jest.mock("@/lib/store", () => ({
  useAppSelector: (selector: (state: unknown) => unknown) =>
    selector({ backlogItems: { items: mockLiveItemsMap } }),
}));

// Raw events the terminal-state watch stream (Task 5.3.1c) yields on its next
// connection — set before render() (the stream is opened once, on mount).
let mockTerminalStreamEvents: Array<{ event: { case: string; value: { itemId: string } } }> = [];
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    watchBacklogItems: () =>
      (async function* () {
        for (const e of mockTerminalStreamEvents) {
          yield e;
        }
      })(),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
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
  mockLiveItemsMap = {};
  mockTerminalStreamEvents = [];
  updateBacklogItem.mockClear().mockResolvedValue(null);
  useStuckBacklogItemsMock.mockReturnValue({ items: [], isLoading: false, error: null });
  overrideVerdict.mockReset();
  // Story 3.1.4's per-section expand state (useSectionExpandState) and
  // "Show N more" state (useShowMore) both persist to localStorage keyed
  // by itemId — clear between tests so one test's expand/collapse
  // interactions never leak into the next test reusing the same itemId.
  localStorage.clear();
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

// AC #12/#32 (Phase 5 spec-compliance sweep, backlog-event-driven-updates):
// the header status-label badge had no aria-live/aria-atomic, so a plain
// status change with no verdict change produced zero screen-reader
// announcement. Mirrors GateVerdictBox.tsx's role="status" aria-live="polite"
// aria-atomic="true" live-region pattern (Epic 6.2).
//
// PR #208's progressive-disclosure redesign replaced the old standalone
// status-label badge with StageTracker (part of LifecycleSummary, D1 dedup)
// — "Idea" is now rendered as a label span nested inside StageTracker's
// tracker <ol>, not as the direct content of the live-region element itself.
// The live-region role/aria-live/aria-atomic now live on StageTracker's own
// container (data-testid="stage-tracker"), which is what actually gets
// re-announced on a status change — assert there instead of on the raw text
// node.
describe("BacklogItemDetail — AC #12/#32: status-label live region", () => {
  it("marks the StageTracker container as a polite, atomic live region", async () => {
    const session = makeSession({});
    await renderWithSession(session, []);

    expect(screen.getByText("Idea")).toBeInTheDocument();
    const stageTracker = screen.getByTestId("stage-tracker");
    expect(stageTracker).toHaveAttribute("role", "status");
    expect(stageTracker).toHaveAttribute("aria-live", "polite");
    expect(stageTracker).toHaveAttribute("aria-atomic", "true");
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

    // VersionControlSection (Story 3.1.4) is collapsed by default for an
    // "idea"-status item — expand it before asserting on its contents.
    fireEvent.click(screen.getByTestId("collapsible-header-version-control"));

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

    // VersionControlSection (Story 3.1.4) is collapsed by default for an
    // "idea"-status item — expand it before asserting on its contents.
    fireEvent.click(screen.getByTestId("collapsible-header-version-control"));

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

    // VersionControlSection (Story 3.1.4) is collapsed by default for an
    // "idea"-status item — expand it before asserting on its contents.
    fireEvent.click(screen.getByTestId("collapsible-header-version-control"));

    expect(screen.queryByTestId("file-browser-modal-stub")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Browse files in this worktree" }));

    expect(screen.getByTestId("file-browser-modal-stub")).toBeInTheDocument();
  });

  it("BacklogItemDetail_should_RenderShippedCiConclusionAndReviewCounts_When_ShipStatusHasSnapshot", async () => {
    useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
    useBacklogItemShipStatusMock.mockReturnValue({
      data: create(BacklogItemShipStatusSchema, {
        shipped: true,
        shippedVia: "pr",
        branchName: "feature/foo",
        branchExists: false,
        prUrl: "https://github.com/tstapler/stapler-squad/pull/999",
        shippedCheckConclusion: "success",
        shippedApprovedCount: 2,
        shippedChangesReqCount: 1,
        fileStats: [
          create(ShippedFileStatSchema, { path: "server/foo.go", status: FileStatus.MODIFIED, additions: 5, deletions: 1 }),
        ],
        snapshotAt: timestampFromDate(new Date("2026-07-17T10:00:00Z")),
        snapshotCaptureFailed: false,
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

    fireEvent.click(screen.getByTestId("collapsible-header-version-control"));

    expect(screen.getByText("CI: success")).toBeInTheDocument();
    expect(screen.getByLabelText("2 approved")).toBeInTheDocument();
    expect(screen.getByLabelText("1 changes requested")).toBeInTheDocument();
  });

  it("BacklogItemDetail_should_RenderCaptureFailedCopy_When_ShipStatusSnapshotCaptureFailedTrue", async () => {
    useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
    useBacklogItemShipStatusMock.mockReturnValue({
      data: create(BacklogItemShipStatusSchema, {
        shipped: true,
        shippedVia: "pr",
        branchName: "feature/foo",
        branchExists: false,
        prUrl: "https://github.com/tstapler/stapler-squad/pull/999",
        shippedCheckConclusion: "success",
        shippedApprovedCount: 2,
        shippedChangesReqCount: 1,
        fileStats: [
          create(ShippedFileStatSchema, { path: "server/foo.go", status: FileStatus.MODIFIED, additions: 5, deletions: 1 }),
        ],
        snapshotCaptureFailed: true,
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

    fireEvent.click(screen.getByTestId("collapsible-header-version-control"));

    expect(screen.getByText("Couldn't capture PR status at ship time")).toBeInTheDocument();
  });
});

// project_plans/backlog-event-driven-updates Epic 5.3: Story 5.3.1 removes
// the 5s shouldPoll interval in favor of a live store subscription, and adds
// terminal-state handling (Task 5.3.1c); Story 5.3.2 adds edit-mode
// buffering + a warn-before-overwrite confirmation.
describe("BacklogItemDetail — Story 5.3.1: shouldPoll removal / live updates", () => {
  it("does not re-fetch on an interval for a triage-running item — the deleted shouldPoll timer never fires", async () => {
    jest.useFakeTimers();
    try {
      getBacklogItem.mockReset().mockResolvedValue({
        ...makeItem([]),
        status: "idea",
        triageStatus: "running",
      });
      listPipelineModes.mockReset().mockResolvedValue([]);

      render(<BacklogItemDetail itemId="item-1" />);
      await act(async () => {
        await Promise.resolve();
      });
      expect(getBacklogItem).toHaveBeenCalledTimes(1);

      await act(async () => {
        jest.advanceTimersByTime(10_000);
        await Promise.resolve();
      });

      // Pre-Epic-5.3, `shouldPoll` would have fired twice more (5s, 10s) for
      // a triage-running item. It's deleted outright — no re-fetch at all.
      expect(getBacklogItem).toHaveBeenCalledTimes(1);
    } finally {
      jest.useRealTimers();
    }
  });

  it("applies a live update from the shared store to the displayed item when not editing", async () => {
    getBacklogItem.mockReset().mockResolvedValue(makeItem([]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    const { rerender } = render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.getByText("Refactor auth middleware")).toBeInTheDocument();

    // Simulate a BacklogItemUpdatedEvent landing in backlogItemsSlice.
    mockLiveItemsMap = {
      "item-1": {
        id: "item-1",
        title: "Refactor auth middleware (renamed live)",
        status: "idea",
        priority: 3,
      },
    };

    await act(async () => {
      rerender(<BacklogItemDetail itemId="item-1" />);
      await Promise.resolve();
    });

    expect(screen.getByText("Refactor auth middleware (renamed live)")).toBeInTheDocument();
  });
});

describe("BacklogItemDetail — Story 5.3.2: edit-mode buffering", () => {
  it("does not apply a buffered event to visible form fields while editMode is true, and applies it on Reload", async () => {
    getBacklogItem.mockReset().mockResolvedValue(makeItem([]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    const { rerender } = render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit item" }));
    expect(screen.getByTestId("backlog-form-submit")).toBeInTheDocument();

    // A live update lands for the open item while the form is open.
    mockLiveItemsMap = {
      "item-1": {
        id: "item-1",
        title: "Refactor auth middleware (renamed live)",
        status: "idea",
        priority: 3,
        repoPath: "/tmp/repo",
      },
    };

    await act(async () => {
      rerender(<BacklogItemDetail itemId="item-1" />);
      await Promise.resolve();
    });

    // Buffered, not applied — the form still shows the original title, and a
    // non-blocking InlineNotice offers to reload instead of silently
    // dropping or silently applying the update.
    expect(screen.getByDisplayValue("Refactor auth middleware")).toBeInTheDocument();
    expect(screen.getByTestId("backlog-detail-buffered-update-notice")).toHaveTextContent(
      "This item changed elsewhere."
    );

    fireEvent.click(screen.getByRole("button", { name: "Reload" }));

    expect(screen.queryByTestId("backlog-detail-buffered-update-notice")).not.toBeInTheDocument();
    expect(screen.getByDisplayValue("Refactor auth middleware (renamed live)")).toBeInTheDocument();
  });

  it("warns before overwriting when Save is clicked while a live update is buffered — Save Anyway proceeds with the original save", async () => {
    getBacklogItem.mockReset().mockResolvedValue(makeItem([]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    const { rerender } = render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit item" }));

    mockLiveItemsMap = {
      "item-1": {
        id: "item-1",
        title: "Refactor auth middleware",
        status: "idea",
        priority: 3,
        repoPath: "/tmp/repo",
      },
    };
    await act(async () => {
      rerender(<BacklogItemDetail itemId="item-1" />);
      await Promise.resolve();
    });

    fireEvent.click(screen.getByTestId("backlog-form-submit"));

    const conflictNotice = await screen.findByTestId("backlog-detail-save-conflict-notice");
    expect(conflictNotice).toHaveTextContent("Saving will overwrite a change made elsewhere");
    expect(updateBacklogItem).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Save Anyway" }));

    await act(async () => {
      await Promise.resolve();
    });

    expect(updateBacklogItem).toHaveBeenCalledTimes(1);
    expect(updateBacklogItem).toHaveBeenCalledWith("item-1", expect.objectContaining({ title: "Refactor auth middleware" }));
  });

  it("warns before overwriting when Save is clicked while a live update is buffered — Reload discards the edit and returns to view mode", async () => {
    getBacklogItem.mockReset().mockResolvedValue(makeItem([]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    const { rerender } = render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
    });

    fireEvent.click(screen.getByRole("button", { name: "Edit item" }));

    mockLiveItemsMap = {
      "item-1": {
        id: "item-1",
        title: "Refactor auth middleware (renamed live)",
        status: "idea",
        priority: 3,
        repoPath: "/tmp/repo",
      },
    };
    await act(async () => {
      rerender(<BacklogItemDetail itemId="item-1" />);
      await Promise.resolve();
    });

    fireEvent.click(screen.getByTestId("backlog-form-submit"));
    await screen.findByTestId("backlog-detail-save-conflict-notice");

    fireEvent.click(screen.getByRole("button", { name: "Reload" }));

    expect(updateBacklogItem).not.toHaveBeenCalled();
    // Returns to view mode (not still editing) with the buffered data applied.
    expect(screen.getByRole("button", { name: "Edit item" })).toBeInTheDocument();
    expect(screen.getByText("Refactor auth middleware (renamed live)")).toBeInTheDocument();
  });
});

describe("BacklogItemDetail — Task 5.3.1c: terminal-state banner", () => {
  it("shows an archived banner and hides action buttons when BacklogItemArchivedEvent arrives for the open item", async () => {
    getBacklogItem.mockReset().mockResolvedValue(makeItem([]));
    listPipelineModes.mockReset().mockResolvedValue([]);
    mockTerminalStreamEvents = [{ event: { case: "itemArchived", value: { itemId: "item-1" } } }];

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.getByTestId("backlog-detail-terminal-notice")).toHaveTextContent(
      "This item was archived elsewhere."
    );
    expect(screen.queryByRole("button", { name: "Edit item" })).not.toBeInTheDocument();
    expect(screen.queryByTestId("backlog-action-mark-ready")).not.toBeInTheDocument();
  });

  it("shows a removed banner when BacklogItemRemovedEvent arrives for the open item", async () => {
    getBacklogItem.mockReset().mockResolvedValue(makeItem([]));
    listPipelineModes.mockReset().mockResolvedValue([]);
    mockTerminalStreamEvents = [{ event: { case: "itemRemoved", value: { itemId: "item-1" } } }];

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.getByTestId("backlog-detail-terminal-notice")).toHaveTextContent(
      "This item was removed elsewhere."
    );
  });

  it("ignores a terminal event for a different item id", async () => {
    getBacklogItem.mockReset().mockResolvedValue(makeItem([]));
    listPipelineModes.mockReset().mockResolvedValue([]);
    mockTerminalStreamEvents = [{ event: { case: "itemArchived", value: { itemId: "some-other-item" } } }];

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.queryByTestId("backlog-detail-terminal-notice")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit item" })).toBeInTheDocument();
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

describe("BacklogItemDetail — Story 2.1.4: LifecycleSummary replaces the old status badge", () => {
  it("BacklogItemDetail_should_NotRenderLegacyStatusBadgeMarkup_When_LifecycleSummaryReplacesIt", async () => {
    getBacklogItem.mockReset().mockResolvedValue(makeItem([]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    // The old standalone status badge (`styles.statusBadge`, previously at
    // BacklogItemDetail.tsx:700-716) is gone entirely — LifecycleSummary is
    // the sole authoritative status display (D1 duplication regression guard).
    expect(screen.queryByLabelText(/^Status: /)).not.toBeInTheDocument();
    expect(screen.getByTestId("lifecycle-summary")).toBeInTheDocument();
  });

  it("BacklogItemDetail_should_RenderLifecycleSummaryFromLoadedItem_When_GetBacklogItemResolves", async () => {
    const item = makeItem([]);
    getBacklogItem.mockReset().mockResolvedValue({ ...item, status: "review" });
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const summary = screen.getByTestId("lifecycle-summary");
    expect(summary).toBeInTheDocument();
    expect(screen.getByTestId("stage-node-review")).toHaveAttribute("aria-current", "step");
  });

  it("BacklogItemDetail_should_PassMatchingStuckItemToLifecycleSummary_When_UseStuckBacklogItemsReturnsMatchingItemId", async () => {
    // Regression guard for the MAJOR finding on PR #208: BacklogItemDetail
    // (not LifecycleSummary) must own the single useStuckBacklogItems() call
    // and resolve the `.find(i => i.itemId === item.id)` lookup itself,
    // passing the result down as a plain `stuckItem` prop.
    const stuckItem: StuckBacklogItem = {
      itemId: "item-1",
      title: "Refactor auth middleware",
      status: "in_progress",
      reason: StuckReason.STALE_WORK,
      firstDetectedAt: timestampFromDate(new Date(Date.now() - 4 * 60 * 60 * 1000)),
      lastCheckedAt: timestampFromDate(new Date()),
      prNumber: 0,
      prUrl: "",
      context: "",
    } as StuckBacklogItem;
    useStuckBacklogItemsMock.mockReturnValue({ items: [stuckItem], isLoading: false, error: null });
    getBacklogItem.mockReset().mockResolvedValue(makeItem([]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.getByTestId("blocker-chip")).toBeInTheDocument();
  });

  it("BacklogItemDetail_should_NotPassStuckItemToLifecycleSummary_When_UseStuckBacklogItemsReturnsNoMatchingItemId", async () => {
    const stuckItem: StuckBacklogItem = {
      itemId: "some-other-item",
      title: "Unrelated item",
      status: "in_progress",
      reason: StuckReason.STALE_WORK,
      firstDetectedAt: timestampFromDate(new Date(Date.now() - 4 * 60 * 60 * 1000)),
      lastCheckedAt: timestampFromDate(new Date()),
      prNumber: 0,
      prUrl: "",
      context: "",
    } as StuckBacklogItem;
    useStuckBacklogItemsMock.mockReturnValue({ items: [stuckItem], isLoading: false, error: null });
    getBacklogItem.mockReset().mockResolvedValue(makeItem([]));
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.queryByTestId("blocker-chip")).not.toBeInTheDocument();
  });
});

describe("BacklogItemDetail — Story 3.1.3: polling suspends for manual-review + in-flight actions", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });
  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  function makeReviewItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
    return {
      ...makeItem([]),
      status: "review",
      gateVerdict: "PENDING",
      ...overrides,
    };
  }

  it("BacklogItemDetail_should_SuspendPollingLoad_When_ShowManualReviewIsTrueEvenWithEditModeFalse", async () => {
    getBacklogItem.mockReset().mockResolvedValue(makeReviewItem());
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(getBacklogItem).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByTestId("backlog-action-manual-review"));
    expect(screen.getByTestId("manual-review-form")).toBeInTheDocument();

    await act(async () => {
      jest.advanceTimersByTime(5_000);
      await Promise.resolve();
    });

    // showManualReview === true, editMode === false — the poll must still be
    // suspended (extends the existing editMode-only guard, Task 3.1.3c).
    expect(getBacklogItem).toHaveBeenCalledTimes(1);
  });

  it("BacklogItemDetail_should_NotClobberManualReviewDraftText_When_PollTickFiresWhileFormOpen", async () => {
    getBacklogItem.mockReset().mockResolvedValue(makeReviewItem());
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    fireEvent.click(screen.getByTestId("backlog-action-manual-review"));
    fireEvent.change(screen.getByTestId("manual-review-summary"), {
      target: { value: "Verified the fix locally" },
    });

    await act(async () => {
      jest.advanceTimersByTime(10_000);
      await Promise.resolve();
    });

    expect(getBacklogItem).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("manual-review-summary")).toHaveValue("Verified the fix locally");
  });

  it("BacklogItemDetail_should_SuspendPollingAndPreservePendingState_When_ActionLoadingIsNonNull", async () => {
    const reviewSession = makeSession({
      entityId: "review-session-1",
      sessionId: "session-1",
      role: "review",
    });
    getBacklogItem.mockReset().mockResolvedValue(makeReviewItem({ linkedSessions: [reviewSession] }));
    listPipelineModes.mockReset().mockResolvedValue([]);
    // Never resolves — keeps actionLoading non-null for the duration of this test.
    overrideVerdict.mockReturnValue(new Promise(() => {}));

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(getBacklogItem).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByTestId("backlog-action-override-done"));
    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.getByTestId("backlog-action-override-done")).toHaveAttribute("aria-busy", "true");

    await act(async () => {
      jest.advanceTimersByTime(5_000);
      await Promise.resolve();
    });

    // actionLoading !== null — the poll must be suspended (pre-mortem P1 #4,
    // Task 3.1.3c), and the pending button's state must survive untouched
    // (no unmount, no double-submit risk).
    expect(getBacklogItem).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("backlog-action-override-done")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByTestId("backlog-action-override-done")).toBeDisabled();
  });
});

describe("BacklogItemDetail — Story 3.1.4 Task 3.1.4i/3.1.4j: shared CollapsibleGroup keyboard nav", () => {
  it("BacklogItemDetail_should_MoveFocusBetweenAllSiblingSectionHeaders_When_ArrowKeyPressedInSharedCollapsibleGroup", async () => {
    const session = makeSession({ entityId: "s1", sessionId: "session-1", role: "work" });
    getBacklogItem.mockReset().mockResolvedValue({
      ...makeItem([session]),
      status: "in_progress",
      planArtifactsPath: "/tmp/plans/item-1.md",
      notes: "some notes",
    });
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    // Full composed tree — not an isolated 2-section fixture (that's
    // Collapsible.test.tsx's unit-level proof) — asserting the real
    // sibling sections wired into one CollapsibleGroup (Task 3.1.4i)
    // actually deliver ADR-027's cross-header keyboard-nav benefit.
    const descriptionHeader = screen.getByTestId("collapsible-header-description");
    const planArtifactsHeader = screen.getByTestId("collapsible-header-plan-artifacts");

    descriptionHeader.focus();
    expect(descriptionHeader).toHaveFocus();

    fireEvent.keyDown(descriptionHeader, { key: "ArrowDown", code: "ArrowDown" });

    expect(planArtifactsHeader).toHaveFocus();
  });
});

describe("BacklogItemDetail — Story 3.1.5: auto-expand-on-status-change, first-render-only", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });
  afterEach(() => {
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
  });

  it("BacklogItemDetail_should_ExpandVersionControlSectionOnFirstMount_When_ItemStatusIsInProgress", async () => {
    useVcsStatusMock.mockReturnValue({
      data: create(VCSStatusSchema, { branch: "feat/x", isClean: true }),
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    const session = makeSession({ entityId: "s1", sessionId: "session-1", role: "work" });
    getBacklogItem.mockReset().mockResolvedValue({ ...makeItem([session]), status: "in_progress" });
    listPipelineModes.mockReset().mockResolvedValue([]);

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.getByTestId("collapsible-header-version-control")).toHaveAttribute("aria-expanded", "true");
  });

  it("BacklogItemDetail_should_KeepSectionCollapsed_When_ALiveUpdateReturnsAFreshItemAfterUserManuallyCollapsedIt", async () => {
    useVcsStatusMock.mockReturnValue({
      data: create(VCSStatusSchema, { branch: "feat/x", isClean: true }),
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    const session = makeSession({ entityId: "s1", sessionId: "session-1", role: "work" });
    getBacklogItem.mockReset().mockResolvedValue({ ...makeItem([session]), status: "in_progress" });
    listPipelineModes.mockReset().mockResolvedValue([]);

    const { rerender } = render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    // Default-expanded (per the in_progress rule) — user manually collapses it.
    const vcsHeader = screen.getByTestId("collapsible-header-version-control");
    expect(vcsHeader).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(vcsHeader);
    expect(vcsHeader).toHaveAttribute("aria-expanded", "false");

    // Epic 5.3 replaced the 5s poll with a live store subscription — simulate
    // a fresh live update for the same item/status landing in the shared
    // store instead of advancing a (now-deleted) poll timer.
    mockLiveItemsMap = {
      "item-1": {
        id: "item-1",
        title: "Refactor auth middleware",
        status: "in_progress",
        priority: 3,
        itemSessions: [
          { id: "s1", sessionUuid: "session-1", sessionRole: "work", estimatedCostUsd: 0 },
        ],
      },
    };
    await act(async () => {
      rerender(<BacklogItemDetail itemId="item-1" />);
      await Promise.resolve();
    });

    expect(screen.getByTestId("collapsible-header-version-control")).toHaveAttribute("aria-expanded", "false");
  });

  it("BacklogItemDetail_should_NotReapplyDefaultExpand_When_StatusTransitionsViaALiveUpdateWithoutItemIdChange", async () => {
    getBacklogItem.mockReset().mockResolvedValue({ ...makeItem([]), status: "idea" });
    listPipelineModes.mockReset().mockResolvedValue([]);

    const { rerender } = render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.queryByTestId("collapsible-header-reviewing")).not.toBeInTheDocument();

    // Epic 5.3 replaced the 5s poll with a live store subscription — a live
    // update arrives for the same item transitioning it to "review",
    // ReviewingSection becomes newly relevant mid-session without an
    // itemId/key change.
    mockLiveItemsMap = {
      "item-1": {
        id: "item-1",
        title: "Refactor auth middleware",
        status: "review",
        priority: 3,
      },
    };
    await act(async () => {
      rerender(<BacklogItemDetail itemId="item-1" />);
      await Promise.resolve();
    });

    // ReviewingSection now exists (status is "review"), but its one-shot
    // mount-time default was already consumed by the *first* load (when
    // status was "idea") — it must not retroactively open just because it
    // only became relevant after that.
    expect(screen.getByTestId("collapsible-header-reviewing")).toHaveAttribute("aria-expanded", "false");
  });
});

describe("BacklogItemDetail — regression: Collapsible's defaultExpanded-in-group dev warning", () => {
  it("BacklogItemDetail_should_NotWarn_When_AllGroupedSectionsRenderWithTheirOwnDefaultExpanded", async () => {
    // 1d8b6cd1 added a dev-mode console.warn in Collapsible.tsx for any
    // CollapsibleSection inside a CollapsibleGroup that receives a truthy
    // defaultExpanded — but every one of this component's 8 grouped
    // sections legitimately passes defaultExpanded={<sectionKey>Expanded},
    // the same state that also feeds the group's own `value` prop via
    // sectionExpandEntries/openSectionKeys. That's redundant, not
    // misuse — Collapsible.tsx must only warn on a genuine divergence
    // between defaultExpanded and the group's actual state for that
    // section, never on this component's own correct, consistent usage.
    // Use "review" status so every optional section (Reviewing,
    // Plan Artifacts, Version Control) also mounts, maximizing the
    // number of grouped sections rendered in one pass. VersionControlSection
    // additionally requires non-null VCS/ship-status data to render at all.
    useVcsStatusMock.mockReturnValue({
      data: create(VCSStatusSchema, { branch: "feat/live-branch", isClean: true }),
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    const session = makeSession({ entityId: "s1", sessionId: "session-1", role: "review" });
    getBacklogItem.mockReset().mockResolvedValue({
      ...makeItem([session]),
      status: "review",
      planArtifactsPath: "/tmp/plans/item-1.md",
      notes: "some notes",
    });
    listPipelineModes.mockReset().mockResolvedValue([]);

    const warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});

    render(<BacklogItemDetail itemId="item-1" />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    // Sanity check: the grouped sections this test cares about actually
    // mounted, so a passing assertion below isn't just "nothing rendered".
    expect(screen.getByTestId("collapsible-header-reviewing")).toBeInTheDocument();
    expect(screen.getByTestId("collapsible-header-plan-artifacts")).toBeInTheDocument();
    expect(screen.getByTestId("collapsible-header-version-control")).toBeInTheDocument();
    expect(screen.getByTestId("collapsible-header-sessions")).toBeInTheDocument();
    expect(screen.getByTestId("collapsible-header-notes")).toBeInTheDocument();

    expect(warnSpy).not.toHaveBeenCalled();

    // Toggling a section keeps defaultExpanded and the group's value in
    // lockstep (both driven by the same useSectionExpandState setter via
    // handleGroupValueChange) — must still not warn after a re-render.
    fireEvent.click(screen.getByTestId("collapsible-header-notes"));
    expect(warnSpy).not.toHaveBeenCalled();

    warnSpy.mockRestore();
  });
});
