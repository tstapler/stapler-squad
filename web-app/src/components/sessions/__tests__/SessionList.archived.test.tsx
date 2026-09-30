import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { SessionList } from "../SessionList";
import type { Session } from "@/gen/session/v1/types_pb";
import { timestampNow } from "@bufbuild/protobuf/wkt";
import { makeSession } from "./sessionListTestFixtures";

// Heavy dependency mocks for SessionList — mirrors SessionList.mobile.test.tsx, except
// ActionBar renders its children (the mobile test mocks it to null, which hides the
// "Show Archived" checkbox under test here since it lives inside ActionBar).

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    getInsightsSummary: jest.fn(async () => ({ sessions: [] })),
    watchInsights: async function* () {},
  })),
}));

jest.mock("@connectrpc/connect-web", () => require("./sessionListTestFixtures").mockConnectWeb());

jest.mock("@/lib/contexts/ReviewQueueContext", () => require("./sessionListTestFixtures").mockReviewQueueContext());

jest.mock("@/lib/contexts/NotificationContext", () => require("./sessionListTestFixtures").mockNotificationContext());

jest.mock("@/lib/store", () => require("./sessionListTestFixtures").mockStore());

jest.mock("@/lib/store/sessionsSlice", () => require("./sessionListTestFixtures").mockSessionsSlice());

jest.mock("../SessionCard", () => require("./sessionListTestFixtures").mockSessionCard());

jest.mock("../SessionRow", () => require("./sessionListTestFixtures").mockSessionRow());

jest.mock("../BulkActions", () => require("./sessionListTestFixtures").mockBulkActions());

jest.mock("../TagEditor", () => require("./sessionListTestFixtures").mockTagEditor());

jest.mock("@/components/ui/ActionBar", () => require("./sessionListTestFixtures").mockActionBarPassthrough());

jest.mock("@/components/ui/Modal", () => require("./sessionListTestFixtures").mockModal());

jest.mock("@/components/ui/AppLink", () => require("./sessionListTestFixtures").mockAppLink());

jest.mock("@/lib/contexts/ApprovalsContext", () => require("./sessionListTestFixtures").mockApprovalsContext());

// SessionList's row mode virtualizes with @tanstack/react-virtual, which measures a
// real scroll container to decide the visible range — jsdom's zero-size layout means
// it never renders any row. Replace it with a non-virtualized stand-in that renders
// every item, matching the shape SessionList actually consumes (getTotalSize,
// getVirtualItems, measureElement).
jest.mock("@tanstack/react-virtual", () => require("./sessionListTestFixtures").mockReactVirtual());

beforeEach(() => {
  window.localStorage.clear();
});

describe("SessionList — show archived toggle", () => {
  it("SessionList_should_hideArchivedSessions_When_showArchivedIsOff", () => {
    const sessions = [
      makeSession("s1", "Active Session") as Session,
      makeSession("s2", "Archived Session", { archivedAt: timestampNow() }) as Session,
    ];
    render(<SessionList sessions={sessions} />);

    expect(screen.getByText("Active Session")).toBeInTheDocument();
    expect(screen.queryByText("Archived Session")).not.toBeInTheDocument();
  });

  it("SessionList_should_showArchivedSessions_When_toggleEnabled", () => {
    const sessions = [
      makeSession("s1", "Active Session") as Session,
      makeSession("s2", "Archived Session", { archivedAt: timestampNow() }) as Session,
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
