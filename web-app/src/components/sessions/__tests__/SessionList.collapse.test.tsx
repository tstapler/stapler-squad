// @feature session-list
import React from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SessionList } from "../SessionList";
import type { Session } from "@/gen/session/v1/types_pb";

// Heavy dependency mocks for SessionList (mirrors SessionList.mobile.test.tsx)

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({ listProjects: jest.fn(async () => ({ projects: [] })) })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
}));

// @tanstack/react-virtual and react-virtuoso both skip rendering off-screen items
// based on real layout measurements, which jsdom doesn't provide. Replace both with
// pass-through implementations that render every item, so collapse filtering (which
// happens upstream of virtualization, in flatItems/cardGroupCounts) is observable.
jest.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({ index, key: index, start: index * 50, size: 50 })),
    getTotalSize: () => count * 50,
    measureElement: () => {},
  }),
}));

jest.mock("react-virtuoso", () => ({
  GroupedVirtuoso: ({
    groupCounts,
    groupContent,
    itemContent,
  }: {
    groupCounts: number[];
    groupContent: (groupIndex: number) => React.ReactNode;
    itemContent: (index: number) => React.ReactNode;
  }) => {
    const nodes: React.ReactNode[] = [];
    let flatIndex = 0;
    groupCounts.forEach((count, groupIndex) => {
      nodes.push(<React.Fragment key={`g-${groupIndex}`}>{groupContent(groupIndex)}</React.Fragment>);
      for (let i = 0; i < count; i++) {
        nodes.push(<React.Fragment key={`i-${flatIndex}`}>{itemContent(flatIndex)}</React.Fragment>);
        flatIndex++;
      }
    });
    return <div data-testid="grouped-virtuoso">{nodes}</div>;
  },
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

jest.mock("../SessionRow", () => ({
  SessionRow: ({ session }: { session: { title: string } }) => (
    <div data-testid="session-row">{session.title}</div>
  ),
}));

jest.mock("../BulkActions", () => ({
  BulkActions: () => null,
}));

jest.mock("../TagEditor", () => ({
  TagEditor: () => null,
}));

jest.mock("@/components/ui/ActionBar", () => ({
  ActionBar: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
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

const makeSession = (id: string, title: string, category: string): Partial<Session> => ({
  id,
  title,
  status: 1 as Session["status"],
  tags: [],
  category,
  path: "/tmp/session",
  branch: "",
  program: "claude",
});

const twoGroupSessions = [
  makeSession("s1", "Backlog Session A", "Backlog") as Session,
  makeSession("s2", "Backlog Session B", "Backlog") as Session,
  makeSession("s3", "Working Session A", "Working") as Session,
];

const STORAGE_KEY = "stapler-squad-collapsed-groups";
const GROUPING_KEY = "stapler-squad-grouping-strategy";

describe("SessionList — collapsible categories (row mode)", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("renders a collapse toggle per group, expanded by default", () => {
    render(<SessionList sessions={twoGroupSessions} />);
    const toggles = screen.getAllByTestId("category-collapse-toggle");
    expect(toggles).toHaveLength(2);
    toggles.forEach((t) => expect(t).toHaveAttribute("aria-expanded", "true"));
  });

  it("clicking a group's toggle hides its sessions and sets aria-expanded=false (AC 0, 3)", async () => {
    const user = userEvent.setup();
    render(<SessionList sessions={twoGroupSessions} />);

    expect(screen.getAllByTestId("session-row")).toHaveLength(3);

    const backlogHeader = screen.getByText("Backlog (2)").closest('[role="heading"]') as HTMLElement;
    const toggle = within(backlogHeader).getByTestId("category-collapse-toggle");

    await user.click(toggle);

    expect(toggle).toHaveAttribute("aria-expanded", "false");
    const rows = screen.getAllByTestId("session-row").map((el) => el.textContent);
    expect(rows).not.toContain("Backlog Session A");
    expect(rows).not.toContain("Backlog Session B");
    expect(rows).toContain("Working Session A");
    // Header + count remain visible while collapsed (AC 5 / validation #8)
    expect(screen.getByText("Backlog (2)")).toBeVisible();
  });

  it("clicking a collapsed group's toggle again restores its sessions (AC 1)", async () => {
    const user = userEvent.setup();
    render(<SessionList sessions={twoGroupSessions} />);

    const backlogHeader = screen.getByText("Backlog (2)").closest('[role="heading"]') as HTMLElement;
    const toggle = within(backlogHeader).getByTestId("category-collapse-toggle");

    await user.click(toggle);
    expect(screen.getAllByTestId("session-row")).toHaveLength(1);

    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(screen.getAllByTestId("session-row")).toHaveLength(3);
  });

  it("is keyboard-operable via Enter and Space (AC 2)", async () => {
    const user = userEvent.setup();
    render(<SessionList sessions={twoGroupSessions} />);

    const backlogHeader = screen.getByText("Backlog (2)").closest('[role="heading"]') as HTMLElement;
    const toggle = within(backlogHeader).getByTestId("category-collapse-toggle");

    toggle.focus();
    await user.keyboard("{Enter}");
    expect(toggle).toHaveAttribute("aria-expanded", "false");

    await user.keyboard(" ");
    expect(toggle).toHaveAttribute("aria-expanded", "true");
  });

  it("persists collapsed state to localStorage and restores it on remount (AC 4)", async () => {
    const user = userEvent.setup();
    const { unmount } = render(<SessionList sessions={twoGroupSessions} />);

    const backlogHeader = screen.getByText("Backlog (2)").closest('[role="heading"]') as HTMLElement;
    await user.click(within(backlogHeader).getByTestId("category-collapse-toggle"));

    expect(JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "[]")).toContain("Backlog");
    unmount();

    render(<SessionList sessions={twoGroupSessions} />);
    const rows = screen.getAllByTestId("session-row").map((el) => el.textContent);
    expect(rows).not.toContain("Backlog Session A");
    expect(rows).toContain("Working Session A");
    const reopenedHeader = screen.getByText("Backlog (2)").closest('[role="heading"]') as HTMLElement;
    expect(within(reopenedHeader).getByTestId("category-collapse-toggle")).toHaveAttribute("aria-expanded", "false");
  });

  it("does not render a toggle when grouping strategy is None (AC 6)", () => {
    window.localStorage.setItem(GROUPING_KEY, JSON.stringify("none"));
    render(<SessionList sessions={twoGroupSessions} />);
    expect(screen.queryByTestId("category-collapse-toggle")).not.toBeInTheDocument();
    expect(screen.getByText("All Sessions (3)")).toBeInTheDocument();
  });

  it("keeps independent collapse state across two SessionList instances via storageKeyPrefix (AC 8)", async () => {
    const user = userEvent.setup();
    render(
      <>
        <SessionList sessions={twoGroupSessions} storageKeyPrefix="pane-a-" />
        <SessionList sessions={twoGroupSessions} storageKeyPrefix="pane-b-" />
      </>
    );

    const headers = screen.getAllByText("Backlog (2)");
    const paneAToggle = within(headers[0].closest('[role="heading"]') as HTMLElement).getByTestId("category-collapse-toggle");
    await user.click(paneAToggle);

    expect(paneAToggle).toHaveAttribute("aria-expanded", "false");
    const paneBHeaders = screen.getAllByText("Backlog (2)");
    const paneBToggle = within(paneBHeaders[1].closest('[role="heading"]') as HTMLElement).getByTestId("category-collapse-toggle");
    expect(paneBToggle).toHaveAttribute("aria-expanded", "true");

    expect(window.localStorage.getItem("pane-a-stapler-squad-collapsed-groups")).toContain("Backlog");
    expect(window.localStorage.getItem("pane-b-stapler-squad-collapsed-groups")).not.toContain("Backlog");
  });
});

describe("SessionList — collapsible categories (card mode)", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("renders a collapse toggle per group in card view mode", () => {
    render(<SessionList sessions={twoGroupSessions} viewMode="card" />);
    const toggles = screen.getAllByTestId("category-collapse-toggle");
    expect(toggles.length).toBeGreaterThanOrEqual(1);
  });

  it("collapsing a group hides its cards while other groups render under the correct header (AC 4)", async () => {
    const user = userEvent.setup();
    render(<SessionList sessions={twoGroupSessions} viewMode="card" />);

    const backlogHeader = screen.getByText("Backlog (2)").closest('[role="heading"]') as HTMLElement;
    await user.click(within(backlogHeader).getByTestId("category-collapse-toggle"));

    expect(screen.queryByText("Backlog Session A")).not.toBeInTheDocument();
    expect(screen.queryByText("Backlog Session B")).not.toBeInTheDocument();
    // "Working" group is untouched and still renders its own session under its own header.
    expect(screen.getByText("Working Session A")).toBeInTheDocument();
    expect(screen.getByText("Backlog (2)")).toBeInTheDocument();
  });
});
