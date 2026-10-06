// @feature session-list, session:stale-detection
import React from "react";
import { render, screen, act } from "@testing-library/react";
import { SessionList } from "../SessionList";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

// Heavy dependency mocks for SessionList (mirrors SessionList.collapse.test.tsx),
// plus getSessionDefaults for useStaleSessionConfig.
const mockGetSessionDefaults = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    listProjects: jest.fn(async () => ({ projects: [] })),
    getInsightsSummary: jest.fn(async () => ({ sessions: [] })),
    watchInsights: async function* () {},
    getSessionDefaults: (...args: unknown[]) => mockGetSessionDefaults(...args),
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
}));

jest.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({ index, key: index, start: index * 50, size: 50 })),
    getTotalSize: () => count * 50,
    measureElement: () => {},
  }),
}));

jest.mock("react-virtuoso", () => ({
  GroupedVirtuoso: () => null,
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

jest.mock("@/components/ui/AppLink", () => ({
  AppLink: ({ href, children, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...rest}>{children}</a>
  ),
}));

jest.mock("@/lib/contexts/ApprovalsContext", () => ({
  useApprovalsContext: () => ({ clearedSessions: new Set() }),
}));

const NOW_MS = 1_700_000_000_000;

function minutesAgo(minutes: number): { seconds: bigint; nanos: number } {
  return { seconds: BigInt(Math.floor(NOW_MS / 1000) - minutes * 60), nanos: 0 };
}

function makeActiveSession(id: string, title: string, lastActivityMinutesAgo: number): Session {
  return {
    id,
    title,
    status: SessionStatus.ACTIVE,
    tags: [],
    category: "",
    path: "/tmp/session",
    branch: "",
    program: "claude",
    lastMeaningfulOutput: minutesAgo(lastActivityMinutesAgo),
    lastTerminalUpdate: minutesAgo(lastActivityMinutesAgo),
  } as unknown as Session;
}

const GROUPING_KEY = "stapler-squad-grouping-strategy";

describe("SessionList — stale-session reclassification on wall-clock tick", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.localStorage.setItem(GROUPING_KEY, JSON.stringify("stale"));
    mockGetSessionDefaults.mockResolvedValue({
      defaults: { staleSessionThresholdMinutes: 30, staleSessionNotifyEnabled: true },
    });
    jest.useFakeTimers();
    jest.spyOn(Date, "now").mockReturnValue(NOW_MS);
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
    jest.clearAllMocks();
  });

  it("reclassifies a session from Not Stale to Stale after a 60s+ tick with no new session data", async () => {
    // 29 minutes ago is fresh relative to a 30-minute threshold at render time.
    const session = makeActiveSession("s1", "Freshish Session", 29);

    render(<SessionList sessions={[session]} />);

    // Let the getSessionDefaults() fetch (and its state update) resolve before asserting.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.getByText("Not Stale (1)")).toBeInTheDocument();
    expect(screen.queryByText(/^Stale \(/)).not.toBeInTheDocument();

    // Advance wall-clock time past the 30-minute threshold (29 + 2 = 31 minutes elapsed)
    // AND past the 60s re-render tick, with no new `sessions` prop — the component must
    // pick this up purely from the interval-driven re-render.
    act(() => {
      jest.spyOn(Date, "now").mockReturnValue(NOW_MS + 2 * 60_000);
      jest.advanceTimersByTime(120_000);
    });

    expect(screen.getByText("Stale (1)")).toBeInTheDocument();
    expect(screen.queryByText(/^Not Stale \(/)).not.toBeInTheDocument();
  });

  it("cleans up the 60s interval on unmount (no further recompute/timer firing after unmount)", async () => {
    const clearIntervalSpy = jest.spyOn(window, "clearInterval");
    const session = makeActiveSession("s1", "Session", 5);

    const { unmount } = render(<SessionList sessions={[session]} />);
    await act(async () => {
      await Promise.resolve();
    });

    unmount();

    expect(clearIntervalSpy).toHaveBeenCalled();
  });
});
