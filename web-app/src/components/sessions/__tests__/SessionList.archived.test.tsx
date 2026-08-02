import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { SessionList } from "../SessionList";
import type { Session } from "@/gen/session/v1/types_pb";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { timestampNow } from "@bufbuild/protobuf/wkt";

// Heavy dependency mocks for SessionList — mirrors SessionList.mobile.test.tsx, except
// ActionBar renders its children (the mobile test mocks it to null, which hides the
// "Show Archived" checkbox under test here since it lives inside ActionBar).

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

// SessionList's row mode virtualizes with @tanstack/react-virtual, which measures a
// real scroll container to decide the visible range — jsdom's zero-size layout means
// it never renders any row. Replace it with a non-virtualized stand-in that renders
// every item, matching the shape SessionList actually consumes (getTotalSize,
// getVirtualItems, measureElement).
jest.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (opts: { count: number }) => ({
    getTotalSize: () => opts.count * 50,
    getVirtualItems: () =>
      Array.from({ length: opts.count }, (_, index) => ({
        index,
        key: index,
        start: index * 50,
        size: 50,
      })),
    measureElement: () => undefined,
  }),
}));

const makeSession = (id: string, title: string, archivedAt?: Timestamp): Partial<Session> => ({
  id,
  title,
  status: 1 as Session["status"],
  tags: [],
  category: "",
  path: "/tmp/session",
  branch: "",
  program: "claude",
  archivedAt,
});

beforeEach(() => {
  window.localStorage.clear();
});

describe("SessionList — show archived toggle", () => {
  it("SessionList_should_hideArchivedSessions_When_showArchivedIsOff", () => {
    const sessions = [
      makeSession("s1", "Active Session") as Session,
      makeSession("s2", "Archived Session", timestampNow()) as Session,
    ];
    render(<SessionList sessions={sessions} />);

    expect(screen.getByText("Active Session")).toBeInTheDocument();
    expect(screen.queryByText("Archived Session")).not.toBeInTheDocument();
  });

  it("SessionList_should_showArchivedSessions_When_toggleEnabled", () => {
    const sessions = [
      makeSession("s1", "Active Session") as Session,
      makeSession("s2", "Archived Session", timestampNow()) as Session,
    ];
    render(<SessionList sessions={sessions} />);

    fireEvent.click(screen.getByTestId("show-archived-toggle"));

    expect(screen.getByText("Active Session")).toBeInTheDocument();
    expect(screen.getByText("Archived Session")).toBeInTheDocument();
  });

  it("SessionList_should_callOnFetchArchivedSessions_When_toggleEnabled", () => {
    const onFetchArchivedSessions = jest.fn();
    render(
      <SessionList
        sessions={[makeSession("s1", "Active Session") as Session]}
        onFetchArchivedSessions={onFetchArchivedSessions}
      />
    );

    fireEvent.click(screen.getByTestId("show-archived-toggle"));

    expect(onFetchArchivedSessions).toHaveBeenCalledWith(true);
  });

  it("SessionList_should_notCallOnFetchArchivedSessions_When_notToggled", () => {
    const onFetchArchivedSessions = jest.fn();
    render(
      <SessionList
        sessions={[makeSession("s1", "Active Session") as Session]}
        onFetchArchivedSessions={onFetchArchivedSessions}
      />
    );

    expect(onFetchArchivedSessions).not.toHaveBeenCalled();
  });
});
