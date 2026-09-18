import React from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import type { Timestamp } from "@bufbuild/protobuf/wkt";

// Shared mock-factory bodies and fixture helpers for SessionList.*.test.tsx.
//
// jest.mock(...) factories are hoisted by babel-jest above imports, so each test file
// keeps its own thin `jest.mock("path", () => require("./sessionListTestFixtures").xyz())`
// call — only the factory *bodies* live here. Every export below is a function (not a
// pre-built object) so each test file gets its own fresh jest.fn() instances, matching
// the semantics the inline factories had before extraction.

export const mockConnectWeb = () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
});

export const mockReviewQueueContext = () => ({
  useReviewQueueContext: () => ({ items: [] }),
});

export const mockNotificationContext = () => ({
  useNotifications: () => ({
    showUndoToast: jest.fn(() => "toast-id"),
    removeNotification: jest.fn(),
    addNotification: jest.fn(),
  }),
});

export const mockStore = () => ({
  useAppSelector: jest.fn(() => ({})),
});

export const mockSessionsSlice = () => ({
  selectDetectedStatusMap: jest.fn(),
});

export const mockSessionCard = () => ({
  SessionCard: ({ session }: { session: { title: string } }) => (
    <div data-testid="session-card">{session.title}</div>
  ),
});

export const mockSessionRow = () => ({
  SessionRow: ({ session }: { session: { title: string } }) => (
    <div data-testid="session-row">{session.title}</div>
  ),
});

export const mockBulkActions = () => ({
  BulkActions: () => null,
});

export const mockTagEditor = () => ({
  TagEditor: () => null,
});

// Two ActionBar variants are in use: a no-op (mobile flow, which doesn't exercise
// anything inside ActionBar) and a pass-through that renders children (needed wherever
// a test asserts on something ActionBar renders, e.g. the "Show Archived" checkbox).
export const mockActionBarNull = () => ({
  ActionBar: () => null,
});

export const mockActionBarPassthrough = () => ({
  ActionBar: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
});

export const mockModal = () => ({
  Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
});

export const mockAppLink = () => ({
  AppLink: ({ href, children, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...rest}>{children}</a>
  ),
});

export const mockApprovalsContext = () => ({
  useApprovalsContext: () => ({ clearedSessions: new Set() }),
});

// @tanstack/react-virtual and react-virtuoso both skip rendering off-screen items based
// on real layout measurements, which jsdom doesn't provide. Both mocks below replace
// them with pass-through implementations that render every item.
export const mockReactVirtual = () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({ index, key: index, start: index * 50, size: 50 })),
    getTotalSize: () => count * 50,
    measureElement: () => {},
  }),
});

export const mockReactVirtuoso = () => ({
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
});

export const makeSession = (
  id: string,
  title: string,
  opts: { category?: string; archivedAt?: Timestamp } = {}
): Partial<Session> => ({
  id,
  title,
  status: 1 as Session["status"],
  tags: [],
  category: opts.category ?? "",
  path: "/tmp/session",
  branch: "",
  program: "claude",
  archivedAt: opts.archivedAt,
});
