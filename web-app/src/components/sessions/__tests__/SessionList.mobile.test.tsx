import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { SessionList } from "../SessionList";
import type { Session } from "@/gen/session/v1/types_pb";

// Heavy dependency mocks for SessionList

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    getInsightsSummary: jest.fn(async () => ({ sessions: [] })),
    watchInsights: async function* () {},
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
}));

jest.mock("@/lib/contexts/ReviewQueueContext", () => ({
  useReviewQueueContext: () => ({ items: [] }),
}));

jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({
    showUndoToast: jest.fn(() => "toast-id"),
    removeNotification: jest.fn(),
    addNotification: jest.fn(),
  }),
}));

jest.mock("@/lib/store", () => ({
  useAppSelector: jest.fn(() => ({})),
}));

jest.mock("@/lib/store/sessionsSlice", () => ({
  selectDetectedStatusMap: jest.fn(),
}));

jest.mock("../SessionCard", () => ({
  SessionCard: ({ session }: { session: { title: string } }) => (
    <div data-testid="session-card">{session.title}</div>
  ),
}));

jest.mock("../BulkActions", () => ({
  BulkActions: () => null,
}));

jest.mock("../TagEditor", () => ({
  TagEditor: () => null,
}));

jest.mock("@/components/ui/ActionBar", () => ({
  ActionBar: () => null,
}));

jest.mock("@/components/ui/Modal", () => ({
  Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

jest.mock("@/components/ui/AppLink", () => ({
  AppLink: ({ href, children, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...rest}>{children}</a>
  ),
}));

jest.mock("@/lib/contexts/ApprovalsContext", () => ({
  useApprovalsContext: () => ({ clearedSessions: new Set() }),
}));

const makeSession = (id: string, title: string): Partial<Session> => ({
  id,
  title,
  status: 1 as Session["status"],
  tags: [],
  category: "",
  path: "/tmp/session",
  branch: "",
  program: "claude",
});

describe("SessionList — mobile new session flow", () => {
  it("renders the + button in the header when sessions exist", () => {
    render(
      <SessionList
        sessions={[makeSession("s1", "My Session") as Session]}
        onNewSession={jest.fn()}
      />
    );
    expect(screen.getByRole("button", { name: "Create new session" })).toBeInTheDocument();
  });

  it("calls onNewSession when the header + button is clicked", () => {
    const onNewSession = jest.fn();
    render(
      <SessionList
        sessions={[makeSession("s1", "My Session") as Session]}
        onNewSession={onNewSession}
      />
    );
    fireEvent.click(screen.getByRole("button", { name: "Create new session" }));
    expect(onNewSession).toHaveBeenCalledTimes(1);
  });

  it("shows the + button even when session list is empty", () => {
    render(<SessionList sessions={[]} onNewSession={jest.fn()} />);
    expect(screen.getByRole("button", { name: "Create new session" })).toBeInTheDocument();
  });

  it("does not crash when onNewSession is not provided", () => {
    expect(() =>
      render(<SessionList sessions={[makeSession("s1", "My Session") as Session]} />)
    ).not.toThrow();
  });
});

describe("SessionList — bulk-select state machine", () => {
  it("SessionList_should_showSelectModeButton_When_rendered", () => {
    render(
      <SessionList
        sessions={[makeSession("s1", "Session 1") as Session, makeSession("s2", "Session 2") as Session]}
      />
    );
    const btn = screen.getByRole("button", { name: /enter select mode/i });
    expect(btn).toBeInTheDocument();
    expect(btn).toHaveAttribute("aria-pressed", "false");
  });

  it("SessionList_should_setAriaPressed_When_selectModeToggled", () => {
    render(
      <SessionList
        sessions={[makeSession("s1", "Session 1") as Session]}
      />
    );
    const btn = screen.getByRole("button", { name: /enter select mode/i });
    fireEvent.click(btn);
    // After entering select mode, button label changes to "Exit select mode" and aria-pressed is true
    expect(screen.getByRole("button", { name: /exit select mode/i })).toHaveAttribute("aria-pressed", "true");
  });

  it("SessionList_should_exitSelectModeAndClearSelection_When_EscapePressed", () => {
    render(
      <SessionList
        sessions={[makeSession("s1", "Session 1") as Session]}
      />
    );
    // Enter select mode
    fireEvent.click(screen.getByRole("button", { name: /enter select mode/i }));
    expect(screen.getByRole("button", { name: /exit select mode/i })).toBeInTheDocument();

    // Press Escape
    fireEvent.keyDown(document, { key: "Escape" });

    // Should exit select mode
    expect(screen.getByRole("button", { name: /enter select mode/i })).toHaveAttribute("aria-pressed", "false");
  });

  it("SessionList_should_selectAllSessions_When_CmdAPressedInSelectMode", () => {
    const sessions = [
      makeSession("s1", "Session 1") as Session,
      makeSession("s2", "Session 2") as Session,
    ];
    render(<SessionList sessions={sessions} />);

    // Enter select mode
    fireEvent.click(screen.getByRole("button", { name: /enter select mode/i }));

    // Press Cmd+A
    fireEvent.keyDown(document, { key: "a", metaKey: true });

    // The session-list container should have data-select-mode="true"
    const container = document.querySelector('[data-context="session-list"]');
    expect(container).toHaveAttribute("data-select-mode", "true");
  });
});
