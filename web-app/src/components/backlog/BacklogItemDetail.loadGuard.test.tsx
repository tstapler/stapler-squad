/**
 * Tests for BacklogItemDetail's load() staleness guard (backlog/plan-approval
 * UI flicker fix — research/pitfalls.md #2 and #4):
 *
 *  - #4: out-of-order load()-vs-load() resolution must not let an earlier
 *    (but slower-resolving) call's response overwrite a later-issued call's
 *    response (loadSeqRef monotonic guard).
 *  - #2: a load() response strictly older than the currently-applied item
 *    must be dropped, but a same-millisecond ("not strictly older") response
 *    from the user's own just-triggered mutation must still apply.
 *  - TriageReviewPanel's onSkip must not trigger a load() call at all —
 *    dismissing the panel is a purely client-local action.
 */

import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

jest.mock("./SessionMonitor", () => ({ SessionMonitor: () => null }));
jest.mock("./GateVerdictBox", () => ({ GateVerdictBox: () => null }));
jest.mock("./TriageLoadingIndicator", () => ({ TriageLoadingIndicator: () => null }));

jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

const deleteSession = jest.fn().mockResolvedValue(undefined);
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ deleteSession }),
}));

jest.mock("@/lib/analytics", () => ({
  useAnalytics: () => ({ track: jest.fn() }),
}));

jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => ({ items: [], connectionState: "live" }),
}));
// Controllable per-test — simulates the raw proto item currently sitting in
// backlogItemsSlice for this itemId (selectBacklogItemById's return value).
let mockLiveRawItem: unknown = undefined;
jest.mock("@/lib/store", () => ({
  useAppSelector: () => mockLiveRawItem,
}));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    watchBacklogItems: () => (async function* () {})(),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));
jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: () => ({ items: [], isLoading: false, error: null }),
}));

const getBacklogItem = jest.fn();
const listPipelineModes = jest.fn().mockResolvedValue([]);

jest.mock("@/lib/hooks/useBacklogService", () => ({
  ...jest.requireActual("@/lib/hooks/useBacklogService"),
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus: jest.fn().mockResolvedValue(true),
    triggerTriage: jest.fn(),
    cancelTriage: jest.fn().mockResolvedValue(undefined),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    triggerShipPR: jest.fn(),
    archiveBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

function makeSession(id: string): LinkedSession {
  return {
    entityId: `entity-${id}`,
    sessionId: id,
    role: "work",
    estimatedCostUsd: 0,
  };
}

const baseItem: BacklogItem = {
  id: "item-1",
  title: "Jira hygiene checker",
  description: "desc",
  status: "in_progress",
  priority: 3,
  repoPath: "/tmp/repo",
  skipPlanning: false,
  skipReviewGate: false,
  autoSpawnSession: false,
  autoCreatePR: false,
  planApproved: false,
  triageStatus: undefined,
  acCriteria: [],
  linkedSessions: [makeSession("s1"), makeSession("s2")],
  notes: "initial notes",
  createdAt: "2026-07-01T00:00:00.000Z",
  updatedAt: "2026-07-01T00:00:00.000Z",
  statusEvents: [],
  progressNotes: [],
  totalEstimatedCostUsd: 0,
};

/** A controllable, manually-resolved getBacklogItem() call. */
function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("BacklogItemDetail load() staleness guards", () => {
  beforeEach(() => {
    getBacklogItem.mockReset();
    deleteSession.mockClear();
    mockLiveRawItem = undefined;
    jest.spyOn(window, "confirm").mockReturnValue(true);
    localStorage.clear();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("BacklogItemDetail_should_keepLaterIssuedCallsResult_When_EarlierCallResolvesAfter", async () => {
    const initial = deferred<BacklogItem>();
    getBacklogItem.mockReturnValueOnce(initial.promise);
    render(<BacklogItemDetail itemId="item-1" />);
    initial.resolve(baseItem);
    await waitFor(() => expect(screen.getByText("initial notes")).toBeInTheDocument());

    const call1 = deferred<BacklogItem>();
    const call2 = deferred<BacklogItem>();
    getBacklogItem.mockReturnValueOnce(call1.promise);
    getBacklogItem.mockReturnValueOnce(call2.promise);

    // Two different sessions' Delete buttons each trigger their own load()
    // call after deleteSession resolves — call for s1 is issued first, call
    // for s2 second.
    const deleteButtons = screen.getAllByRole("button", { name: "Delete session" });
    fireEvent.click(deleteButtons[0]);
    await waitFor(() => expect(getBacklogItem).toHaveBeenCalledTimes(2));
    fireEvent.click(deleteButtons[1]);
    await waitFor(() => expect(getBacklogItem).toHaveBeenCalledTimes(3));

    // Resolve the SECOND-issued call first (out-of-order), with a newer
    // updatedAt than what the first-issued call will resolve with — so if
    // the seq guard were absent, the later-resolving (but earlier-issued)
    // call would win instead.
    call2.resolve({
      ...baseItem,
      notes: "second-issued-call-data",
      updatedAt: "2026-07-01T00:00:02.000Z",
    });
    await waitFor(() => expect(screen.getByText("second-issued-call-data")).toBeInTheDocument());

    call1.resolve({
      ...baseItem,
      notes: "first-issued-call-data",
      updatedAt: "2026-07-01T00:00:05.000Z",
    });

    // Give the first-issued (now-stale) response a chance to apply if the
    // guard were broken, then assert it never did.
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByText("second-issued-call-data")).toBeInTheDocument();
    expect(screen.queryByText("first-issued-call-data")).not.toBeInTheDocument();
  });

  it("BacklogItemDetail_should_dropStaleLoadResult_When_UpdatedAtIsOlderThanCurrentItem", async () => {
    const initial = deferred<BacklogItem>();
    getBacklogItem.mockReturnValueOnce(initial.promise);
    render(<BacklogItemDetail itemId="item-1" />);
    initial.resolve({ ...baseItem, updatedAt: "2026-07-01T00:00:10.000Z", notes: "current-notes" });
    await waitFor(() => expect(screen.getByText("current-notes")).toBeInTheDocument());

    const staleCall = deferred<BacklogItem>();
    getBacklogItem.mockReturnValueOnce(staleCall.promise);
    fireEvent.click(screen.getAllByRole("button", { name: "Delete session" })[0]);
    await waitFor(() => expect(getBacklogItem).toHaveBeenCalledTimes(2));

    // Resolves with an OLDER updatedAt than what's currently applied —
    // simulating a slow response racing behind a fresher live-store update.
    staleCall.resolve({
      ...baseItem,
      notes: "stale-notes",
      updatedAt: "2026-07-01T00:00:01.000Z",
    });

    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByText("current-notes")).toBeInTheDocument();
    expect(screen.queryByText("stale-notes")).not.toBeInTheDocument();
  });

  it("BacklogItemDetail_should_applyLoadResult_When_UpdatedAtEqualsCurrentItem", async () => {
    const initial = deferred<BacklogItem>();
    getBacklogItem.mockReturnValueOnce(initial.promise);
    render(<BacklogItemDetail itemId="item-1" />);
    initial.resolve({ ...baseItem, updatedAt: "2026-07-01T00:00:10.000Z", notes: "current-notes" });
    await waitFor(() => expect(screen.getByText("current-notes")).toBeInTheDocument());

    const sameMsCall = deferred<BacklogItem>();
    getBacklogItem.mockReturnValueOnce(sameMsCall.promise);
    fireEvent.click(screen.getAllByRole("button", { name: "Delete session" })[0]);
    await waitFor(() => expect(getBacklogItem).toHaveBeenCalledTimes(2));

    // Same-millisecond updatedAt as what's currently applied (e.g. the
    // user's own just-triggered mutation) — must still apply, not be
    // treated as "not strictly newer" and dropped.
    sameMsCall.resolve({
      ...baseItem,
      notes: "same-ms-notes",
      updatedAt: "2026-07-01T00:00:10.000Z",
    });

    await waitFor(() => expect(screen.getByText("same-ms-notes")).toBeInTheDocument());
  });

  it("BacklogItemDetail_should_dropStaleLiveStoreEvent_When_ItsOlderThanTheCurrentlyDisplayedItem", async () => {
    // Root-caused via this fix's own e2e spec: a freshly-seeded item's
    // "created" live-store event (no plan_artifacts_path yet) can arrive
    // AFTER a separately-issued load()/GetBacklogItem fetch already applied
    // a newer, more-complete state (e.g. plan_artifacts_path set moments
    // later) — backlogItemsSlice's own upsertItem guard only protects the
    // 2nd+ write for a given item id, so this component's liveRawItem effect
    // needs its own guard too, mirroring load()'s.
    const initial = deferred<BacklogItem>();
    getBacklogItem.mockReturnValueOnce(initial.promise);
    const { rerender } = render(<BacklogItemDetail itemId="item-1" />);
    initial.resolve({ ...baseItem, updatedAt: "2026-07-01T00:00:10.000Z", notes: "current-notes" });
    await waitFor(() => expect(screen.getByText("current-notes")).toBeInTheDocument());

    mockLiveRawItem = {
      id: "item-1",
      title: baseItem.title,
      status: baseItem.status,
      priority: baseItem.priority,
      notes: "stale-live-notes",
      updatedAt: timestampFromDate(new Date("2026-07-01T00:00:01.000Z")),
    };
    rerender(<BacklogItemDetail itemId="item-1" />);

    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByText("current-notes")).toBeInTheDocument();
    expect(screen.queryByText("stale-live-notes")).not.toBeInTheDocument();
  });

  it("BacklogItemDetail_should_applyLiveStoreEvent_When_ItsNewerThanTheCurrentlyDisplayedItem", async () => {
    const initial = deferred<BacklogItem>();
    getBacklogItem.mockReturnValueOnce(initial.promise);
    const { rerender } = render(<BacklogItemDetail itemId="item-1" />);
    initial.resolve({ ...baseItem, updatedAt: "2026-07-01T00:00:10.000Z", notes: "current-notes" });
    await waitFor(() => expect(screen.getByText("current-notes")).toBeInTheDocument());

    mockLiveRawItem = {
      id: "item-1",
      title: baseItem.title,
      status: baseItem.status,
      priority: baseItem.priority,
      notes: "fresh-live-notes",
      updatedAt: timestampFromDate(new Date("2026-07-01T00:00:20.000Z")),
    };
    rerender(<BacklogItemDetail itemId="item-1" />);

    await waitFor(() => expect(screen.getByText("fresh-live-notes")).toBeInTheDocument());
  });

  it("BacklogItemDetail_should_notCallLoad_When_TriageReviewPanelIsDismissed", async () => {
    const initial = deferred<BacklogItem>();
    getBacklogItem.mockReturnValueOnce(initial.promise);
    render(<BacklogItemDetail itemId="item-1" />);
    initial.resolve({
      ...baseItem,
      status: "idea",
      triageStatus: "completed",
      triageResult: {
        summary: "s",
        suggestions: [],
        clarifyingQuestions: [],
      },
    });
    await waitFor(() => expect(getBacklogItem).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole("button", { name: "Dismiss triage review" }));

    // Give any accidental async load() a tick to fire, then assert it never did.
    await new Promise((r) => setTimeout(r, 0));
    expect(getBacklogItem).toHaveBeenCalledTimes(1);
  });
});
