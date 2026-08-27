/**
 * Focus-return regression coverage exercising BacklogItemDetail's OWN
 * `onClick` handlers (the real `e.currentTarget` → `*TriggerRef.current`
 * assignments at BacklogItemDetail.tsx's ReviewingSection/VersionControlSection
 * call sites), not a hand-built harness that re-implements the same pattern.
 *
 * ReviewChangesModal.focusReturn.test.tsx and
 * BacklogFileBrowserModal.focusReturn.test.tsx already prove each modal
 * correctly restores focus to whatever `triggerRef` it's given — this file
 * closes the remaining gap: that BacklogItemDetail actually wires the real
 * "View Changes"/"View Diff"/"Browse files" buttons into that `triggerRef`,
 * so a future edit that drops one of those three assignments fails a test.
 */

import React from "react";
import { render, screen, act, fireEvent, waitFor } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem, LinkedSession, PipelineMode } from "@/lib/hooks/useBacklogService";
import { BacklogItemShipStatusSchema } from "@/gen/session/v1/backlog_pb";

// Heavy children unrelated to the modals under test — stubbed the same way
// as BacklogItemDetail.test.tsx.
jest.mock("./SessionMonitor", () => ({ SessionMonitor: () => null }));
jest.mock("./GateVerdictBox", () => ({ GateVerdictBox: () => null }));
jest.mock("./TriageReviewPanel", () => ({ TriageReviewPanel: () => null }));
jest.mock("./TriageLoadingIndicator", () => ({ TriageLoadingIndicator: () => null }));

// ReviewChangesModal and BacklogFileBrowserModal are intentionally left
// UNMOCKED (unlike BacklogItemDetail.test.tsx) so this suite exercises the
// real useFocusTrap wiring end to end. Their own heavy sub-dependencies are
// stubbed instead, mirroring ReviewChangesModal.focusReturn.test.tsx and
// BacklogFileBrowserModal.focusReturn.test.tsx.
const getBacklogItemDiff = jest.fn();
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    getBacklogItemDiff: (...args: unknown[]) => getBacklogItemDiff(...args),
    watchBacklogItems: () => (async function* () {})(),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => (next: unknown) => next,
}));
jest.mock("@/components/sessions/FileTree", () => ({
  FileTree: () => <div data-testid="mock-file-tree" />,
}));
jest.mock("@/components/sessions/FileContentViewer", () => ({
  FileContentViewer: () => <div data-testid="mock-file-content-viewer" />,
}));
jest.mock("@/lib/hooks/useSessionVcs", () => ({
  useSessionVcs: () => ({ status: null }),
}));

const useVcsStatusMock = jest.fn();
jest.mock("@/lib/hooks/useVcsStatus", () => ({
  useVcsStatus: (...args: unknown[]) => useVcsStatusMock(...args),
}));

const useBacklogItemShipStatusMock = jest.fn();
jest.mock("@/lib/hooks/useBacklogItemShipStatus", () => ({
  useBacklogItemShipStatus: (...args: unknown[]) => useBacklogItemShipStatusMock(...args),
}));

const useStuckBacklogItemsMock = jest.fn();
jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: (...args: unknown[]) => useStuckBacklogItemsMock(...args),
}));

jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ deleteSession: jest.fn() }),
}));
jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

const getBacklogItem = jest.fn();
const listPipelineModes = jest.fn();
jest.mock("@/lib/hooks/useBacklogService", () => ({
  ...jest.requireActual("@/lib/hooks/useBacklogService"),
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus: jest.fn().mockResolvedValue(true),
    triggerTriage: jest.fn().mockResolvedValue(undefined),
    cancelTriage: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    rejectPlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    triggerShipPR: jest.fn(),
    submitManualReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    unarchiveBacklogItem: jest.fn(),
    deleteBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => ({ items: [], connectionState: "live" }),
}));
jest.mock("@/lib/store", () => ({
  useAppSelector: (selector: (state: unknown) => unknown) => selector({ backlogItems: { items: {} } }),
}));

beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  useVcsStatusMock.mockReturnValue({ data: null, loading: false, error: null, refetch: jest.fn() });
  useBacklogItemShipStatusMock.mockReturnValue({ data: null, loading: false, refetch: jest.fn() });
  useStuckBacklogItemsMock.mockReturnValue({ items: [], isLoading: false, error: null });
  getBacklogItemDiff.mockReset().mockResolvedValue({ diff: "", added: 0, removed: 0 });
  localStorage.clear();
});

function makeSession(overrides: Partial<LinkedSession> = {}): LinkedSession {
  return {
    entityId: "session-entity-1",
    sessionId: "session-1",
    role: "work",
    estimatedCostUsd: 0,
    pipelineModeSnapshot: "",
    pipelineModeSnapshotHash: "",
    worktreePath: "/tmp/repo-wt",
    ...overrides,
  };
}

function makeItem(overrides: Partial<BacklogItem>, linkedSessions: LinkedSession[]): BacklogItem {
  return {
    id: "item-1",
    title: "Refactor auth middleware",
    description: "desc",
    status: "review",
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
    ...overrides,
  };
}

async function renderDetail(itemOverrides: Partial<BacklogItem> = {}, modes: PipelineMode[] = []) {
  const session = makeSession();
  getBacklogItem.mockReset().mockResolvedValue(makeItem(itemOverrides, [session]));
  listPipelineModes.mockReset().mockResolvedValue(modes);

  render(<BacklogItemDetail itemId="item-1" />);

  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("BacklogItemDetail — real trigger-ref wiring for focus return", () => {
  it("BacklogItemDetail_should_RestoreFocusToViewChangesButton_When_OpenedFromReviewingSectionAndClosed", async () => {
    await renderDetail({ status: "review" });

    const viewChanges = screen.getByRole("button", { name: /view changes/i });
    fireEvent.click(viewChanges);

    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /close changes viewer/i }));

    expect(document.activeElement).toBe(viewChanges);
  });

  it("BacklogItemDetail_should_RestoreFocusToViewDiffButtonNotViewChanges_When_OpenedFromVersionControlSectionAndClosed", async () => {
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
    await renderDetail({ status: "review" });

    // Version Control auto-expands for status "review" (BacklogItemDetail.tsx's
    // sectionExpandEntries effect) — no header click needed, and clicking it
    // here would toggle it closed.
    const viewDiff = screen.getByTestId("vcs-widget-view-diff");
    fireEvent.click(viewDiff);

    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /close changes viewer/i }));

    expect(document.activeElement).toBe(viewDiff);
    expect(document.activeElement).not.toBe(screen.getByRole("button", { name: /view changes/i }));
  });

  it("BacklogItemDetail_should_RestoreFocusToBrowseFilesButton_When_ModalClosed", async () => {
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
    await renderDetail({ status: "idea" });

    fireEvent.click(screen.getByTestId("collapsible-header-version-control"));
    const browseFiles = screen.getByRole("button", { name: "Browse files in this worktree" });
    fireEvent.click(browseFiles);

    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /close/i }));

    expect(document.activeElement).toBe(browseFiles);
  });
});
