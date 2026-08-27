import React from "react";
import { render, screen, within } from "@testing-library/react";
import { SessionBoard } from "./SessionBoard";
import { SessionStatus, SubStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

// SessionCard pulls in a lot of machinery (terminal snapshots, tooltips, session actions)
// that's irrelevant to board bucketing/virtualization — replace it with a lightweight stub,
// same strategy SessionList's own tests use for the same component.
jest.mock("./SessionCard", () => ({
  SessionCard: ({ session }: { session: Session }) => (
    <div data-testid="session-card">{session.title}</div>
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
