import React from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SessionBoard } from "./SessionBoard";
import { SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

// SessionCard pulls in a lot of machinery (terminal snapshots, tooltips, session actions)
// that's irrelevant to board bucketing/virtualization — replace it with a lightweight stub,
// same strategy SessionList's own tests use for the same component. Renders a selection
// toggle button whenever selectMode is active (Task 6.3.1b's isSelected/onToggleSelect
// wiring) so bulk-select tests can exercise the real prop plumbing from SessionBoard ->
// BoardCard -> SessionCard without pulling in the real, heavy SessionCard implementation.
jest.mock("./SessionCard", () => ({
  SessionCard: ({
    session,
    selectMode,
    isSelected,
    onToggleSelect,
  }: {
    session: Session;
    selectMode?: boolean;
    isSelected?: boolean;
    onToggleSelect?: () => void;
  }) => (
    <div data-testid="session-card">
      {session.title}
      {selectMode && (
        <button
          type="button"
          data-testid={`select-toggle-${session.id}`}
          aria-pressed={isSelected}
          onClick={() => onToggleSelect?.()}
        >
          {isSelected ? "Selected" : "Select"}
        </button>
      )}
    </div>
  ),
}));

// SessionBoard calls useSessionService() directly (for the drag-drop mutation path added in
// Phase 3) — mock the module rather than requiring a Redux <Provider>/WebSocket transport in
// these column-bucketing/virtualization tests, matching the established pattern (see
// SessionMonitor.test.tsx, BacklogItemDetail.test.tsx).
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({
    updateSession: jest.fn(),
    resumeHibernatedSession: jest.fn(),
  }),
}));

let rectHeight = 0;

beforeAll(() => {
  // @tanstack/react-virtual sizes its scroll container via offsetHeight/offsetWidth (see
  // virtual-core's getRect), not getBoundingClientRect — jsdom returns 0 for both by
  // default, which would make every column measure as zero-height and render no virtual
  // items regardless of whether virtualization is actually wired. Mock both so the
  // virtualization smoke test below exercises real windowing math.
  jest.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockImplementation(() => rectHeight);
  jest.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockImplementation(() => 320);
  // measureElement (per-card sizing) reads getBoundingClientRect().height directly.
  jest.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    () =>
      ({
        width: 320,
        height: 220,
        top: 0,
        left: 0,
        bottom: 220,
        right: 320,
        x: 0,
        y: 0,
        toJSON: () => {},
      }) as DOMRect
  );
});

afterAll(() => {
  jest.restoreAllMocks();
});

// SessionBoard's search/grouping-strategy state persists to real jsdom localStorage
// (Task 6.1.1a/6.2.1a share SessionList's storage keys) -- clear it between tests so one
// test's grouping selection can't leak into the next.
beforeEach(() => {
  localStorage.clear();
});

function makeSession(overrides: Partial<Session> & { id: string; title: string }): Session {
  return {
    status: SessionStatus.ACTIVE,
    subStatus: SubStatus.UNSPECIFIED,
    tags: [],
    category: "",
    path: "/tmp/session",
    branch: "",
    program: "claude",
    ...overrides,
  } as unknown as Session;
}

describe("SessionBoard — column bucketing", () => {
  // Matches the plan's worked example (2 running / 1 needs_review / 4 paused / 0 complete)
  // while also covering a CREATING session (-> running) and a HIBERNATED one (-> paused).
  const sessions: Session[] = [
    makeSession({ id: "run-1", title: "Active session", status: SessionStatus.ACTIVE }),
    makeSession({ id: "run-2", title: "Creating session", status: SessionStatus.CREATING }),
    makeSession({
      id: "review-1",
      title: "Needs approval session",
      status: SessionStatus.ACTIVE,
      subStatus: SubStatus.NEEDS_APPROVAL,
    }),
    makeSession({ id: "paused-1", title: "Paused 1", status: SessionStatus.PAUSED }),
    makeSession({ id: "paused-2", title: "Paused 2", status: SessionStatus.PAUSED }),
    makeSession({ id: "paused-3", title: "Paused 3", status: SessionStatus.PAUSED }),
    makeSession({ id: "hibernated-1", title: "Hibernated 1", status: SessionStatus.HIBERNATED }),
  ];

  beforeEach(() => {
    rectHeight = 600;
  });

  it("SessionBoard_should_ShowCorrectCountBadges_When_SessionsSpanAllColumns", () => {
    render(<SessionBoard sessions={sessions} />);

    const running = screen.getByTestId("board-column-running");
    expect(within(running).getByLabelText("2 sessions")).toBeInTheDocument();

    const needsReview = screen.getByTestId("board-column-needs_review");
    expect(within(needsReview).getByLabelText("1 sessions")).toBeInTheDocument();

    const paused = screen.getByTestId("board-column-paused");
    expect(within(paused).getByLabelText("4 sessions")).toBeInTheDocument();

    const complete = screen.getByTestId("board-column-complete");
    expect(within(complete).getByLabelText("0 sessions")).toBeInTheDocument();
  });

  it("SessionBoard_should_RenderEmptyStateMessage_When_CompleteColumnHasNoMatchingSessions", () => {
    render(<SessionBoard sessions={sessions} />);

    const complete = screen.getByTestId("board-column-complete");
    expect(within(complete).getByText("No sessions")).toBeInTheDocument();
  });

  it("SessionBoard_should_RenderColumnsInFixedOrder_When_Rendered", () => {
    render(<SessionBoard sessions={sessions} />);

    const labels = screen.getAllByRole("heading", { level: 3 }).map((el) => el.textContent);
    expect(labels).toEqual(["Running", "Needs Review", "Paused", "Complete"]);
  });
});

describe("SessionBoard — per-column virtualization", () => {
  it("BoardColumn_should_RenderFarFewerThanTotalDomNodes_When_ColumnHasManySessions", () => {
    rectHeight = 600;
    const manySessions: Session[] = Array.from({ length: 200 }, (_, i) =>
      makeSession({ id: `paused-${i}`, title: `Paused ${i}`, status: SessionStatus.PAUSED })
    );

    render(<SessionBoard sessions={manySessions} />);

    const paused = screen.getByTestId("board-column-paused");
    expect(within(paused).getByLabelText("200 sessions")).toBeInTheDocument();

    const renderedCards = within(paused).getAllByTestId("session-card");
    expect(renderedCards.length).toBeGreaterThan(0);
    expect(renderedCards.length).toBeLessThan(50);
  });
});

// Task 6.1.1c: swimlane rows crossed with status columns, and Tag grouping's multi-membership
// rendering the same session in every matching row.
describe("SessionBoard — swimlanes (Task 6.1.1a-c, AC6)", () => {
  beforeEach(() => {
    rectHeight = 600;
  });

  it("SessionBoard_should_RenderOneSwimlaneRowPerBranch_When_GroupingStrategyIsBranch", async () => {
    const user = userEvent.setup();
    const sessions: Session[] = [
      makeSession({ id: "s1", title: "Login flow", status: SessionStatus.ACTIVE, branch: "feature/login" }),
      makeSession({ id: "s2", title: "Login cleanup", status: SessionStatus.PAUSED, branch: "feature/login" }),
      makeSession({ id: "s3", title: "Main session", status: SessionStatus.STOPPED, branch: "main" }),
    ];

    render(<SessionBoard sessions={sessions} />);
    await user.selectOptions(screen.getByTestId("board-grouping-select"), "branch");

    const loginRow = screen.getByTestId("board-swimlane-feature/login");
    expect(within(loginRow).getByText("feature/login")).toBeInTheDocument();
    expect(
      within(within(loginRow).getByTestId("board-column-running")).getByLabelText("1 sessions")
    ).toBeInTheDocument();
    expect(
      within(within(loginRow).getByTestId("board-column-paused")).getByLabelText("1 sessions")
    ).toBeInTheDocument();

    const mainRow = screen.getByTestId("board-swimlane-main");
    expect(
      within(within(mainRow).getByTestId("board-column-complete")).getByLabelText("1 sessions")
    ).toBeInTheDocument();
  });

  it("SessionBoard_should_OmitEmptyGroupRow_When_NoSessionsMatchThatGroupValue", async () => {
    const user = userEvent.setup();
    const sessions: Session[] = [
      makeSession({ id: "s1", title: "Login flow", status: SessionStatus.ACTIVE, branch: "feature/login" }),
      makeSession({ id: "s2", title: "Main session", status: SessionStatus.STOPPED, branch: "main" }),
    ];

    render(<SessionBoard sessions={sessions} />);
    await user.selectOptions(screen.getByTestId("board-grouping-select"), "branch");

    expect(screen.getAllByTestId(/^board-swimlane-/)).toHaveLength(2);
    expect(screen.queryByTestId("board-swimlane-develop")).not.toBeInTheDocument();
  });

  it("SessionBoard_should_RenderCardInBothMatchingTagRows_When_SessionHasTwoTags", async () => {
    const user = userEvent.setup();
    const sessions: Session[] = [
      makeSession({
        id: "sess-1",
        title: "Shared session",
        status: SessionStatus.ACTIVE,
        tags: ["frontend", "urgent"],
      }),
    ];

    render(<SessionBoard sessions={sessions} />);
    await user.selectOptions(screen.getByTestId("board-grouping-select"), "tag");

    const frontendRow = screen.getByTestId("board-swimlane-frontend");
    const urgentRow = screen.getByTestId("board-swimlane-urgent");
    expect(within(frontendRow).getByText("Shared session")).toBeInTheDocument();
    expect(within(urgentRow).getByText("Shared session")).toBeInTheDocument();

    // Two independent DOM instances of the same session -- not deduped to a single row.
    expect(screen.getAllByText("Shared session")).toHaveLength(2);
  });
});

// Task 6.2.1a-b: SessionBoard buckets from filteredSessions (the search-filtered output of
// useFilteredGroupedSessions), not the raw `sessions` prop.
describe("SessionBoard — search parity (Task 6.2.1a-b, AC7)", () => {
  beforeEach(() => {
    rectHeight = 600;
  });

  const sessions: Session[] = [
    makeSession({ id: "s1", title: "Login flow", status: SessionStatus.ACTIVE }),
    makeSession({ id: "s2", title: "Login bugfix", status: SessionStatus.PAUSED }),
    makeSession({ id: "s3", title: "Billing sync", status: SessionStatus.STOPPED }),
    makeSession({
      id: "s4",
      title: "Onboarding",
      status: SessionStatus.ACTIVE,
      subStatus: SubStatus.NEEDS_APPROVAL,
    }),
  ];

  it("SessionBoard_should_BucketFromFilteredSessions_When_SearchQueryIsSet", async () => {
    const user = userEvent.setup();
    render(<SessionBoard sessions={sessions} />);

    await user.type(screen.getByTestId("board-search-input"), "login");

    expect(screen.getAllByTestId("session-card")).toHaveLength(2);
    expect(
      within(screen.getByTestId("board-column-running")).getByLabelText("1 sessions")
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("board-column-paused")).getByLabelText("1 sessions")
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("board-column-complete")).getByLabelText("0 sessions")
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("board-column-needs_review")).getByLabelText("0 sessions")
    ).toBeInTheDocument();
  });

  it("SessionBoard_should_ShowEmptyStateInEveryColumn_When_SearchMatchesZeroSessions", async () => {
    const user = userEvent.setup();
    render(<SessionBoard sessions={sessions} />);

    await user.type(screen.getByTestId("board-search-input"), "zzz-no-match");

    expect(screen.queryAllByTestId("session-card")).toHaveLength(0);
    for (const key of ["running", "needs_review", "paused", "complete"]) {
      const col = screen.getByTestId(`board-column-${key}`);
      expect(within(col).getByText("No sessions")).toBeInTheDocument();
    }
  });
});

// Task 6.3.1a-b: cross-column selection state and BulkActions parity with SessionList.
describe("SessionBoard — cross-column bulk select (Task 6.3.1a-b, AC8)", () => {
  beforeEach(() => {
    rectHeight = 600;
  });

  const sessions: Session[] = [
    makeSession({ id: "sess-1", title: "Running one", status: SessionStatus.ACTIVE }),
    makeSession({
      id: "sess-2",
      title: "Needs review one",
      status: SessionStatus.ACTIVE,
      subStatus: SubStatus.NEEDS_APPROVAL,
    }),
  ];

  it("SessionBoard_should_ComputeSelectedCountAcrossColumns_When_CardsSelectedInDifferentColumns", async () => {
    const user = userEvent.setup();
    render(<SessionBoard sessions={sessions} />);

    await user.click(screen.getByTestId("board-select-mode-toggle"));
    await user.click(screen.getByTestId("select-toggle-sess-1"));
    await user.click(screen.getByTestId("select-toggle-sess-2"));

    expect(screen.getByText("2 of 2 selected")).toBeInTheDocument();
  });

  it("onPauseAll_should_CallUpdateSessionOncePerSelectedId_When_BulkPauseTriggeredAcrossColumns", async () => {
    const user = userEvent.setup();
    const onPauseSession = jest.fn();
    render(<SessionBoard sessions={sessions} onPauseSession={onPauseSession} />);

    await user.click(screen.getByTestId("board-select-mode-toggle"));
    await user.click(screen.getByTestId("select-toggle-sess-1"));
    await user.click(screen.getByTestId("select-toggle-sess-2"));
    await user.click(screen.getByTestId("bulk-pause-button"));

    expect(onPauseSession).toHaveBeenCalledTimes(2);
    expect(onPauseSession).toHaveBeenCalledWith("sess-1");
    expect(onPauseSession).toHaveBeenCalledWith("sess-2");
  });
});
