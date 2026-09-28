/**
 * Tests for DiagnoseHistoryList (plan.md Story 8.2.2): chronological
 * (newest-first) order, refresh-persistence sourced from a fresh
 * ListDiagnoseDispatches call each time (never a client-only cache), and
 * keyboard-navigable list/listitem ARIA semantics.
 */

import { render, screen, waitFor, act } from "@testing-library/react";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import type { DiagnoseDispatchProto } from "@/gen/session/v1/diagnose_pb";
import { DiagnoseDispatchStatus } from "@/gen/session/v1/diagnose_pb";

const mockListDiagnoseDispatches = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    listDiagnoseDispatches: (...args: unknown[]) => mockListDiagnoseDispatches(...args),
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => ({}),
}));

import { DiagnoseHistoryList } from "./DiagnoseHistoryList";

function makeTimestamp(secondsAgo: number): Timestamp {
  return { seconds: BigInt(Math.floor(Date.now() / 1000) - secondsAgo), nanos: 0 } as Timestamp;
}

function makeDispatch(overrides: Partial<DiagnoseDispatchProto> = {}): DiagnoseDispatchProto {
  return {
    id: `dispatch-${Math.random()}`,
    itemId: "itm_abc123",
    targetSessionUuid: "target-uuid",
    diagnosticSessionId: "headless-diagnose-itm_abc123-uuid",
    status: DiagnoseDispatchStatus.COMPLETED,
    createdAt: makeTimestamp(120),
    completedAt: makeTimestamp(5),
    ...overrides,
  } as DiagnoseDispatchProto;
}

describe("DiagnoseHistoryList", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockListDiagnoseDispatches.mockReset();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it("renders the empty state, distinct from a fetch error, when the item has never been diagnosed", async () => {
    mockListDiagnoseDispatches.mockResolvedValue({ dispatches: [] });
    render(<DiagnoseHistoryList itemId="itm_abc123" />);

    await waitFor(() => expect(screen.getByTestId("diagnose-history-empty")).toBeInTheDocument());
    expect(screen.getByText("This item hasn't been diagnosed yet.")).toBeInTheDocument();
    expect(screen.queryByTestId("diagnose-history-error")).not.toBeInTheDocument();
  });

  it("shows its own inline error with Retry, never a silently empty list, when the fetch fails", async () => {
    mockListDiagnoseDispatches.mockRejectedValue(new Error("network down"));
    render(<DiagnoseHistoryList itemId="itm_abc123" />);

    await waitFor(() => expect(screen.getByTestId("diagnose-history-error")).toBeInTheDocument());
    expect(screen.getByRole("alert")).toHaveTextContent("Couldn't load diagnosis history — network down");
    expect(screen.queryByTestId("diagnose-history-empty")).not.toBeInTheDocument();

    mockListDiagnoseDispatches.mockResolvedValue({ dispatches: [] });
    screen.getByRole("button", { name: "Retry" }).click();
    await waitFor(() => expect(screen.getByTestId("diagnose-history-empty")).toBeInTheDocument());
  });

  it("renders 3 prior dispatches newest-first with role=list/listitem semantics", async () => {
    const dispatches = [
      makeDispatch({ id: "d1", outcomeKind: "nudged", createdAt: makeTimestamp(300) }),
      makeDispatch({ id: "d2", outcomeKind: "skipped_safety_gate", safetyGateReason: "not_idle", createdAt: makeTimestamp(200) }),
      makeDispatch({ id: "d3", outcomeKind: "bug_filed", bugItemId: "itm_bug999", createdAt: makeTimestamp(100) }),
    ];
    mockListDiagnoseDispatches.mockResolvedValue({ dispatches });
    render(<DiagnoseHistoryList itemId="itm_abc123" />);

    await waitFor(() => expect(screen.getByRole("list", { name: "Diagnose dispatch history" })).toBeInTheDocument());
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(3);
    // Newest dispatch (d3, bug_filed) first.
    expect(items[0]).toHaveTextContent("filed a bug instead of nudging");
    expect(items[1]).toHaveTextContent("nudge skipped (session wasn't idle)");
    expect(items[2]).toHaveTextContent("nudged the session");

    // Each linked entry exposes a real, Tab-reachable focusable child.
    const bugLink = screen.getByRole("link", { name: "View bug" });
    expect(bugLink).toHaveAttribute("href", "/backlog?item=itm_bug999");
    const diagnosisLink = screen.getByRole("link", { name: "View diagnosis session" });
    expect(diagnosisLink).toHaveAttribute("href", "/?session=headless-diagnose-itm_abc123-uuid");
  });

  it("renders the identical order across two fresh ListDiagnoseDispatches calls, surviving a simulated refresh", async () => {
    const dispatches = [
      makeDispatch({ id: "d1", outcomeKind: "nudged", createdAt: makeTimestamp(300) }),
      makeDispatch({ id: "d2", outcomeKind: "inconclusive_note_filed", createdAt: makeTimestamp(100) }),
    ];
    mockListDiagnoseDispatches.mockResolvedValue({ dispatches });

    const first = render(<DiagnoseHistoryList itemId="itm_abc123" />);
    await waitFor(() => expect(screen.getAllByRole("listitem")).toHaveLength(2));
    const firstOrder = screen.getAllByRole("listitem").map((li) => li.textContent);
    first.unmount();

    // Simulates a page refresh: a brand-new mount, sourced from a second,
    // independent ListDiagnoseDispatches call -- never a client-only cache.
    render(<DiagnoseHistoryList itemId="itm_abc123" />);
    await waitFor(() => expect(screen.getAllByRole("listitem")).toHaveLength(2));
    const secondOrder = screen.getAllByRole("listitem").map((li) => li.textContent);

    expect(mockListDiagnoseDispatches).toHaveBeenCalledTimes(2);
    expect(secondOrder).toEqual(firstOrder);
  });

  it("does not re-render an InconclusiveNoteFiled entry's note text inline, only a link into the Activity Log", async () => {
    mockListDiagnoseDispatches.mockResolvedValue({
      dispatches: [makeDispatch({ outcomeKind: "inconclusive_note_filed", noteText: "the write outcome was ambiguous" })],
    });
    render(<DiagnoseHistoryList itemId="itm_abc123" />);

    await waitFor(() => expect(screen.getAllByRole("listitem")).toHaveLength(1));
    expect(screen.queryByText("the write outcome was ambiguous")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View note" })).toHaveAttribute("href", "#backlog-activity-log");
  });
});
