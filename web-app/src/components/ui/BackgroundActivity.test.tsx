import React from "react";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { BackgroundActivity, type BackgroundActivityProps } from "./BackgroundActivity";
import { buildActivityRows, buildRollup, type ActivityRow } from "@/lib/utils/backgroundActivity";
import { formatClockTime } from "./OfflineBanner";
import { makeBackgroundRow, makeElevenStopped, makeHiddenSession } from "./__fixtures__/notificationFixtures";

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [k: string]: unknown }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

const exceptionRow = (over: Parameters<typeof makeBackgroundRow>[0] = {}): ActivityRow => {
  const ex = makeBackgroundRow(over);
  return {
    key: ex.key,
    sessionId: ex.sessionId,
    sessionTitle: ex.sessionTitle,
    role: "review",
    state: ex.kind === "failure" ? "exception_failure" : "exception_needs_human",
    timestampMs: ex.timestampMs,
    exception: ex,
  };
};

function propsFor(rows: ActivityRow[], over: Partial<BackgroundActivityProps> = {}): BackgroundActivityProps {
  return {
    rows,
    rollup: buildRollup(rows),
    loading: false,
    failed: false,
    offline: false,
    lastUpdatedAt: Date.now() - 30_000,
    onRefresh: jest.fn(),
    onOpenRow: jest.fn(),
    ...over,
  };
}

function setup(rows: ActivityRow[], over: Partial<BackgroundActivityProps> = {}) {
  const props = propsFor(rows, over);
  const view = render(<BackgroundActivity {...props} />);
  const update = (nextRows: ActivityRow[], more: Partial<BackgroundActivityProps> = {}) =>
    view.rerender(<BackgroundActivity {...propsFor(nextRows, { ...over, ...more })} />);
  return { props, update, ...view };
}

const eleven = (outcome: "completed" | "unknown" = "completed") => buildActivityRows([], makeElevenStopped(outcome));

describe("BackgroundActivity", () => {
  it("background_section_should_render_loading_none_error_offline_states_without_a_spinner_on_empty", () => {
    const loading = setup([], { loading: true, lastUpdatedAt: null });
    expect(screen.getByTestId("background-loading")).toHaveAttribute("aria-busy", "true");
    loading.unmount();

    const none = setup([]);
    expect(screen.getByTestId("background-empty-none")).toHaveTextContent(
      "No background work. Review and diagnose sessions will appear here while they run.",
    );
    expect(screen.queryByTestId("background-loading")).toBeNull();
    none.unmount();

    const err = setup([], { failed: true, lastUpdatedAt: null });
    expect(screen.getByText("Could not load background activity.")).toBeInTheDocument();
    err.unmount();

    setup([], { offline: true, lastUpdatedAt: null });
    expect(screen.getByTestId("background-offline")).toBeInTheDocument();
  });

  it("BackgroundActivity_should_renderRollupTestIdWithNoFailuresCopy_When_11StoppedSessions", () => {
    setup(eleven("unknown"));
    expect(screen.getAllByTestId("background-row")).toHaveLength(11);
    const rollup = screen.getByTestId("background-rollup");
    expect(rollup).toHaveTextContent("11 background sessions, no failures reported");
    expect(rollup.textContent?.toLowerCase()).not.toContain("healthy");
    expect(screen.getByTestId("background-empty-healthy")).toContainElement(rollup);
    expect(screen.getByText("No failures reported (11)")).toBeInTheDocument();
  });

  it("BackgroundActivity_should_renderDistinctStoppedUnknownChip_When_noRecordedOutcome", () => {
    setup(eleven("completed"));
    const chips = screen.getAllByTestId("background-state-chip").map((c) => c.textContent);
    expect(chips.filter((t) => t === "Finished")).toHaveLength(10);
    expect(chips.filter((t) => t === "Stopped, outcome unknown")).toHaveLength(1);
  });

  it("BackgroundActivity_should_showNNewButtonAndKeepRowPositions_When_pollAddsSession", () => {
    const sessions = makeElevenStopped("completed");
    const onMergeNew = jest.fn();
    const { update } = setup(buildActivityRows([], sessions), { onMergeNew });
    const before = screen.getAllByTestId("background-row").map((r) => r.getAttribute("data-session-id"));

    const added = makeHiddenSession({ id: "review:new", updatedAtMs: Date.now() });
    update(buildActivityRows([], [...sessions, added]));
    expect(screen.getAllByTestId("background-row").map((r) => r.getAttribute("data-session-id"))).toEqual(before);
    const pill = screen.getByRole("button", { name: "1 new" });

    fireEvent.click(pill);
    expect(onMergeNew).toHaveBeenCalledWith(1);
    expect(screen.queryByTestId("background-new-pill")).toBeNull();
    expect(screen.getAllByTestId("background-row")).toHaveLength(12);
  });

  it("BackgroundActivity_should_showNewExceptionImmediately_When_pollAddsFailure", () => {
    const sessions = makeElevenStopped("completed");
    const { update } = setup(buildActivityRows([], sessions));
    update(buildActivityRows([], [...sessions, makeHiddenSession({ id: "review:bad", backgroundOutcome: "failed" })]));
    expect(screen.queryByTestId("background-new-pill")).toBeNull();
    expect(screen.getAllByTestId("background-row")[0]).toHaveAttribute("data-session-id", "review:bad");
  });

  it("BackgroundActivity_should_capAt100RowsWithShowAll_When_150Sessions", () => {
    const sessions = Array.from({ length: 150 }, (_, i) => makeHiddenSession({ id: `review:s${i}`, updatedAtMs: 1000 + i }));
    setup(buildActivityRows([], sessions));
    expect(screen.getAllByTestId("background-row")).toHaveLength(100);
    fireEvent.click(screen.getByRole("button", { name: "Show all (150)" }));
    expect(screen.getAllByTestId("background-row")).toHaveLength(150);
    expect(screen.queryByTestId("background-show-all")).toBeNull();
  });

  it("BackgroundActivity_should_renderRowsOnFirstCommit_When_cachedPollStateProvided", () => {
    const props = propsFor(eleven("unknown"));
    const seen: number[] = [];
    const Probe = () => {
      React.useLayoutEffect(() => {
        seen.push(document.querySelectorAll("[data-testid='background-row']").length);
      }, []);
      return null;
    };
    render(
      <>
        <BackgroundActivity {...props} />
        <Probe />
      </>,
    );
    expect(seen).toEqual([11]);
    expect(screen.queryByTestId("background-loading")).toBeNull();
  });

  it("ba5_should_show_error_retry_and_stale_age", () => {
    const at = new Date(2026, 9, 18, 10, 3).getTime();
    const { props } = setup([exceptionRow()], { failed: true, lastUpdatedAt: at });
    expect(screen.getByTestId("background-error")).toHaveTextContent(
      `Couldn't refresh background sessions. Showing data from ${formatClockTime(at)}.`,
    );
    expect(screen.getByTestId("background-row")).toBeInTheDocument(); // last good data stays
    fireEvent.click(within(screen.getByTestId("background-error")).getByRole("button", { name: "Retry" }));
    expect(props.onRefresh).toHaveBeenCalledTimes(1);
  });

  it("ba6_should_gate_the_no_failures_wrapper_on_a_settled_poll", () => {
    const rows = eleven("unknown");
    const failed = setup(rows, { failed: true });
    expect(screen.queryByTestId("background-empty-healthy")).toBeNull();
    expect(screen.getByTestId("background-rollup")).toBeInTheDocument();
    failed.unmount();
    const offline = setup(rows, { offline: true, lastUpdatedAt: Date.now() - 60_000 });
    expect(screen.queryByTestId("background-empty-healthy")).toBeNull();
    expect(screen.getByTestId("background-connectivity")).toHaveTextContent("Offline. Showing last known state.");
    offline.unmount();
    setup(rows, { loading: true, lastUpdatedAt: null });
    expect(screen.queryByTestId("background-empty-healthy")).toBeNull();
  });

  it("rows carry the Background chip, a status text, and View output to the read-only link", () => {
    const { props } = setup([exceptionRow()]);
    const r = screen.getByTestId("background-row");
    expect(within(r).getByTestId("notification-background-chip")).toHaveTextContent("Background");
    expect(within(r).getByText("FAILED")).toBeInTheDocument();
    const view = within(r).getByTestId("notification-view-output");
    expect(view).toHaveAttribute("href", "/?session=review%3Ah1&tab=terminal&notification=n1");
    fireEvent.click(view);
    expect(props.onOpenRow).toHaveBeenCalledWith(expect.objectContaining({ sessionId: "review:h1" }));
  });

  it("a row whose session is gone says so and keeps the captured message", () => {
    setup([exceptionRow({ sessionAvailable: false })]);
    const r = screen.getByTestId("background-row");
    expect(r).toHaveTextContent("Session no longer available");
    expect(r).toHaveTextContent("3 tests failing");
    expect(within(r).getByTestId("notification-view-output")).toBeInTheDocument();
  });

  it("ux_ac6_should_showSessionNoLongerExists_When_openOnDepartedRow", () => {
    const sessions = [makeHiddenSession({ id: "review:old" }), makeHiddenSession({ id: "review:keep" })];
    const first = buildActivityRows([], sessions);
    const rows = buildActivityRows([], sessions.slice(1), { previous: first });
    setup(rows);
    const departed = screen.getAllByTestId("background-row").find((r) => r.getAttribute("data-state") === "departed");
    expect(departed).toHaveTextContent("Session no longer exists");
    const open = within(departed as HTMLElement).getByTestId("notification-view-output");
    expect(open).not.toHaveAttribute("href");
    expect(open).toHaveAttribute("aria-disabled", "true");
    const reason = document.getElementById(open.getAttribute("aria-describedby") as string);
    expect(reason).toHaveTextContent("Session no longer exists");
  });

  it("ba10_should_show_reply_only_for_pending_question", () => {
    const question = exceptionRow({ key: "q", sessionId: "q1", sessionTitle: "q1", kind: "needs_human", statusLabel: "NEEDS INPUT", pendingQuestion: true, primaryRecordId: "n-q" });
    const failure = exceptionRow();
    const approval = exceptionRow({ key: "a", sessionId: "a1", kind: "needs_human", statusLabel: "NEEDS APPROVAL" });
    const rows = [failure, question, approval];
    const first = setup(rows);
    expect(screen.queryByTestId("notification-reply")).toBeNull();
    first.unmount();

    setup(rows, { replyEnabled: true });
    const replies = screen.getAllByTestId("notification-reply");
    expect(replies).toHaveLength(1);
    expect(replies[0]).toHaveAttribute("href", expect.stringContaining("reply=1"));
    expect(within(screen.getAllByTestId("background-row")[1]).getByTestId("notification-reply")).toBeInTheDocument();
  });

  it("refresh is inert while offline", () => {
    const { props } = setup(eleven("unknown"), { offline: true });
    fireEvent.click(screen.getByTestId("background-refresh"));
    expect(props.onRefresh).not.toHaveBeenCalled();
    act(() => {});
  });

  it("ux_ac5_should_showCouldntRefreshBannerAndRetryRecovers_When_listFails", () => {
    const at = new Date(2026, 9, 18, 10, 3).getTime();
    const failed = setup(eleven(), { failed: true, lastUpdatedAt: at });
    expect(screen.getAllByTestId("background-row")).toHaveLength(11);
    expect(screen.getByTestId("background-error")).toHaveTextContent(
      `Couldn't refresh background sessions. Showing data from ${formatClockTime(at)}.`,
    );
    fireEvent.click(within(screen.getByTestId("background-error")).getByRole("button", { name: "Retry" }));
    expect(failed.props.onRefresh).toHaveBeenCalledTimes(1);
    failed.update(eleven(), { failed: false });
    expect(screen.queryByTestId("background-error")).toBeNull();
  });

  it("BackgroundActivity_should_reachCantReachTheServerWithRetryAfter30s_And_keepRows_When_pollKeepsFailing", () => {
    jest.useFakeTimers();
    const { props } = setup(eleven(), { failed: true });
    expect(screen.getByTestId("background-connectivity")).toHaveAttribute("data-state", "reconnecting");
    act(() => void jest.advanceTimersByTime(30_000));
    expect(screen.getByTestId("background-connectivity")).toHaveTextContent("Can't reach the server.");
    expect(screen.getAllByTestId("background-row")).toHaveLength(11);
    expect(props.onRefresh).toHaveBeenCalled(); // backoff retries
    jest.useRealTimers();
  });

  it("BackgroundActivity_should_showLegacyEmptyCopyAndNoActivityRows_When_feedFlagOff", () => {
    const off = setup([], { legacy: { hasHiddenSessions: true } });
    expect(screen.getByTestId("background-empty-healthy")).toHaveTextContent("Nothing needs attention in the background.");
    expect(screen.queryByTestId("background-rollup")).toBeNull();
    off.unmount();
    setup([], { legacy: { hasHiddenSessions: false } });
    expect(screen.getByTestId("background-empty-none")).toHaveTextContent("No background sessions have run recently.");
  });
});
