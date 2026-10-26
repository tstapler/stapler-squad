// @feature session-list
import React from "react";
import { render } from "@testing-library/react";
import { SessionList } from "../SessionList";
import type { Session } from "@/gen/session/v1/types_pb";
import { makeSession } from "./sessionListTestFixtures";

jest.mock("@connectrpc/connect", () => ({
  createContextKey: (defaultValue: unknown) => ({ id: Symbol("ctx"), defaultValue }),
  createClient: jest.fn(() => ({
    listProjects: jest.fn(async () => ({ projects: [] })),
    getInsightsSummary: jest.fn(async () => ({ sessions: [] })),
    watchInsights: async function* () {},
  })),
}));
jest.mock("@connectrpc/connect-web", () => require("./sessionListTestFixtures").mockConnectWeb());

// Captures the options SessionList hands to the virtualizer so the test can drive measureElement.
const virtualizerOptions: Array<{ measureElement?: (el: Element, entry?: unknown) => number }> = [];
jest.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (opts: { count: number; measureElement?: (el: Element, entry?: unknown) => number }) => {
    virtualizerOptions.push(opts);
    return {
      getVirtualItems: () =>
        Array.from({ length: opts.count }, (_, index) => ({ index, key: index, start: index * 50, size: 50 })),
      getTotalSize: () => opts.count * 50,
      measureElement: () => {},
    };
  },
}));
jest.mock("react-virtuoso", () => require("./sessionListTestFixtures").mockReactVirtuoso());
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

describe("SessionList row measurement", () => {
  it("sessionList_should_CallNoGetBoundingClientRect_When_200RowsSteadyState", () => {
    const sessions = Array.from({ length: 200 }, (_, i) => makeSession(`s${i}`, `Session ${i}`) as Session);
    render(<SessionList sessions={sessions} />);
    const { measureElement } = virtualizerOptions[virtualizerOptions.length - 1];
    expect(measureElement).toBeDefined();

    const rectSpy = jest.spyOn(Element.prototype, "getBoundingClientRect");
    const el = document.createElement("div");
    // Steady state: every re-measure arrives with a ResizeObserver entry.
    for (let i = 0; i < 200; i++) {
      const entry = { borderBoxSize: [{ blockSize: 40 + i, inlineSize: 300 }] };
      expect(measureElement!(el, entry)).toBe(40 + i);
    }
    // First measure of a mounted row has no entry; it must not use a rect read either.
    Object.defineProperty(el, "offsetHeight", { configurable: true, value: 64 });
    expect(measureElement!(el)).toBe(64);
    expect(rectSpy).not.toHaveBeenCalled();
    rectSpy.mockRestore();
  });
});
