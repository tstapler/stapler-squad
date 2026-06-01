/**
 * Tests for SessionCard click propagation behavior (Bug 1 fix).
 *
 * Covers:
 *  - TC-1.1: Normal click stops propagation (outerClick not called)
 *  - TC-1.2: Normal click calls onClick prop
 *  - TC-1.3: Alt+click stops propagation and calls onOpenInNewPane
 *  - TC-1.4: Select mode click stops propagation (regression guard)
 */

import React from "react";
import { render, fireEvent } from "@testing-library/react";
import { SessionCard } from "../SessionCard";
import type { Session } from "@/gen/session/v1/types_pb";

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

// ---------------------------------------------------------------------------
// Minimal session fixture
// ---------------------------------------------------------------------------

const minimalSession: Partial<Session> = {
  id: "s1",
  title: "Test Session",
  status: 1 as Session["status"],
  tags: [],
  category: "",
  path: "/tmp/session",
  branch: "",
  program: "claude",
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

import { screen } from "@testing-library/react";
import type { SessionGoalSummary } from "@/gen/session/v1/types_pb";

function makeGoalSummary(overrides?: Partial<SessionGoalSummary>): SessionGoalSummary {
  return {
    goalText: "implement feature X",
    status: "working",
    tasksTotal: 3,
    tasksDone: 1,
    tasksJson: "",
    ...overrides,
  } as unknown as SessionGoalSummary;
}

// ─── U-TS-11, U-TS-12, U-TS-13, U-TS-14, U-TS-15: Goal row tests ────────────

describe("SessionCard — goal row", () => {
  // U-TS-11
  it("goal row absent when session.goal is null", () => {
    const session = { ...minimalSession, goal: null } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.queryByText("Goal")).toBeNull();
  });

  // U-TS-12
  it("goal row absent when session.goal.goalText is empty string", () => {
    const session = {
      ...minimalSession,
      goal: makeGoalSummary({ goalText: "" }),
    } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.queryByText("Goal")).toBeNull();
  });

  // U-TS-13
  it("goal row present with truncated text when goal is set", () => {
    const longGoal = "X".repeat(70); // > 60 chars → truncated
    const session = {
      ...minimalSession,
      goal: makeGoalSummary({ goalText: longGoal, tasksTotal: 0, tasksDone: 0 }),
    } as unknown as Session;
    render(<SessionCard session={session} />);
    // Should show label
    expect(screen.getByText("Goal")).toBeInTheDocument();
    // Truncated text ends with ellipsis
    const truncated = "X".repeat(60) + "…";
    expect(screen.getByText(truncated)).toBeInTheDocument();
  });

  // U-TS-14
  it("goal row shows task fraction when tasks exist", () => {
    const session = {
      ...minimalSession,
      goal: makeGoalSummary({ goalText: "a goal", tasksTotal: 5, tasksDone: 3 }),
    } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.getByText(/3\/5 done/)).toBeInTheDocument();
  });

  // U-TS-15
  it("goal row hides task fraction when tasksTotal is 0", () => {
    const session = {
      ...minimalSession,
      goal: makeGoalSummary({ goalText: "a goal", tasksTotal: 0, tasksDone: 0 }),
    } as unknown as Session;
    render(<SessionCard session={session} />);
    expect(screen.getByText("Goal")).toBeInTheDocument();
    expect(screen.queryByText(/done/)).toBeNull();
  });
});

describe("SessionCard — stopPropagation", () => {
  it("SessionCard_should_stopPropagation_When_normalClick", () => {
    const outerClick = jest.fn();
    const onSessionClick = jest.fn();
    const { getByTestId } = render(
      <div onClick={outerClick}>
        <SessionCard
          session={minimalSession as Session}
          onClick={onSessionClick}
        />
      </div>
    );
    fireEvent.click(getByTestId("session-card"));
    expect(onSessionClick).toHaveBeenCalledTimes(1);
    expect(outerClick).not.toHaveBeenCalled(); // propagation stopped
  });

  it("SessionCard_should_callOnClick_When_normalClick", () => {
    const onSessionClick = jest.fn();
    const { getByTestId } = render(
      <SessionCard
        session={minimalSession as Session}
        onClick={onSessionClick}
      />
    );
    fireEvent.click(getByTestId("session-card"));
    expect(onSessionClick).toHaveBeenCalledTimes(1);
  });

  it("SessionCard_should_stopPropagation_When_altClick", () => {
    const outerClick = jest.fn();
    const onOpenInNewPane = jest.fn();
    const { getByTestId } = render(
      <div onClick={outerClick}>
        <SessionCard
          session={minimalSession as Session}
          onOpenInNewPane={onOpenInNewPane}
        />
      </div>
    );
    fireEvent.click(getByTestId("session-card"), { altKey: true });
    expect(onOpenInNewPane).toHaveBeenCalledTimes(1);
    expect(outerClick).not.toHaveBeenCalled();
  });

  it("SessionCard_should_stopPropagation_When_selectMode", () => {
    const outerClick = jest.fn();
    const onToggleSelect = jest.fn();
    const { getByTestId } = render(
      <div onClick={outerClick}>
        <SessionCard
          session={minimalSession as Session}
          selectMode={true}
          onToggleSelect={onToggleSelect}
        />
      </div>
    );
    fireEvent.click(getByTestId("session-card"));
    expect(onToggleSelect).toHaveBeenCalledTimes(1);
    expect(outerClick).not.toHaveBeenCalled();
  });
});
