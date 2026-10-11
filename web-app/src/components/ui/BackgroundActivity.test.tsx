import React from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { BackgroundActivity, type BackgroundActivityProps } from "./BackgroundActivity";
import type { BackgroundRow } from "@/lib/utils/backgroundActivity";

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [k: string]: unknown }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

const row = (over: Partial<BackgroundRow> = {}): BackgroundRow => ({
  key: "bg:review:h1",
  sessionId: "review:h1",
  sessionTitle: "review:h1",
  kind: "failure",
  statusLabel: "FAILED",
  title: "Review failed",
  message: "3 tests failing",
  timestampMs: Date.now() - 4 * 60_000,
  recordIds: ["n1"],
  primaryRecordId: "n1",
  sessionAvailable: true,
  pendingQuestion: false,
  ...over,
});

function setup(over: Partial<BackgroundActivityProps> = {}) {
  const props: BackgroundActivityProps = {
    view: { rows: [], completedOkToday: 0, running: 0 },
    hasHiddenSessions: true,
    loading: false,
    failed: false,
    offline: false,
    lastUpdatedAt: Date.now() - 30_000,
    onRefresh: jest.fn(),
    onOpenRow: jest.fn(),
    ...over,
  };
  render(<BackgroundActivity {...props} />);
  return props;
}

describe("BackgroundActivity", () => {
  it("background_section_should_render_loading_empty_healthy_empty_none_error_stale_offline_states", () => {
    const loading = render(<BackgroundActivity {...({ view: { rows: [], completedOkToday: 0, running: 0 }, hasHiddenSessions: false, loading: true, failed: false, offline: false, lastUpdatedAt: null, onRefresh: jest.fn(), onOpenRow: jest.fn() } as BackgroundActivityProps)} />);
    expect(screen.getByTestId("background-loading")).toHaveAttribute("aria-busy", "true");
    loading.unmount();

    // empty, healthy
    const healthy = render(
      <BackgroundActivity
        view={{ rows: [], completedOkToday: 12, running: 0 }}
        hasHiddenSessions
        loading={false}
        failed={false}
        offline={false}
        lastUpdatedAt={Date.now()}
        onRefresh={jest.fn()}
        onOpenRow={jest.fn()}
      />,
    );
    expect(screen.getByTestId("background-empty-healthy")).toHaveTextContent(
      "Nothing needs attention in the background. 12 completed OK today.",
    );
    healthy.unmount();

    // no background sessions
    const none = render(
      <BackgroundActivity
        view={{ rows: [], completedOkToday: 0, running: 0 }}
        hasHiddenSessions={false}
        loading={false}
        failed={false}
        offline={false}
        lastUpdatedAt={Date.now()}
        onRefresh={jest.fn()}
        onOpenRow={jest.fn()}
      />,
    );
    expect(screen.getByText("No background sessions have run recently.")).toBeInTheDocument();
    none.unmount();

    // first-load error, no data
    const err = render(
      <BackgroundActivity
        view={{ rows: [], completedOkToday: 0, running: 0 }}
        hasHiddenSessions={false}
        loading={false}
        failed
        offline={false}
        lastUpdatedAt={null}
        onRefresh={jest.fn()}
        onOpenRow={jest.fn()}
      />,
    );
    expect(screen.getByText("Could not load background activity.")).toBeInTheDocument();
    err.unmount();

    // offline with no data
    render(
      <BackgroundActivity
        view={{ rows: [], completedOkToday: 0, running: 0 }}
        hasHiddenSessions={false}
        loading={false}
        failed={false}
        offline
        lastUpdatedAt={null}
        onRefresh={jest.fn()}
        onOpenRow={jest.fn()}
      />,
    );
    expect(screen.getByTestId("background-offline")).toBeInTheDocument();
  });

  it("ba5_should_show_error_retry_and_stale_age", () => {
    const props = setup({
      failed: true,
      view: { rows: [row()], completedOkToday: 0, running: 0 },
      lastUpdatedAt: Date.now() - 2 * 60_000,
    });
    expect(screen.getByText("Could not load background activity.")).toBeInTheDocument();
    expect(screen.getByTestId("background-stale")).toHaveTextContent("Showing data from 2m ago");
    expect(screen.getByTestId("background-row")).toBeInTheDocument(); // last good data stays
    fireEvent.click(within(screen.getByTestId("background-error")).getByRole("button", { name: "Retry" }));
    expect(props.onRefresh).toHaveBeenCalledTimes(1);
  });

  it("ba6_should_gate_success_state", () => {
    // failed fetch: never the healthy success state
    const a = render(
      <BackgroundActivity
        view={{ rows: [], completedOkToday: 3, running: 0 }}
        hasHiddenSessions
        loading={false}
        failed
        offline={false}
        lastUpdatedAt={Date.now() - 60_000}
        onRefresh={jest.fn()}
        onOpenRow={jest.fn()}
      />,
    );
    expect(screen.queryByTestId("background-empty-healthy")).toBeNull();
    a.unmount();
    // offline: same
    const b = render(
      <BackgroundActivity
        view={{ rows: [], completedOkToday: 3, running: 0 }}
        hasHiddenSessions
        loading={false}
        failed={false}
        offline
        lastUpdatedAt={Date.now() - 60_000}
        onRefresh={jest.fn()}
        onOpenRow={jest.fn()}
      />,
    );
    expect(screen.queryByTestId("background-empty-healthy")).toBeNull();
    expect(screen.getByTestId("background-stale")).toHaveTextContent("Offline - showing data from 1m ago");
    b.unmount();
    // loading with no data: skeleton, not success
    render(
      <BackgroundActivity
        view={{ rows: [], completedOkToday: 3, running: 0 }}
        hasHiddenSessions
        loading
        failed={false}
        offline={false}
        lastUpdatedAt={null}
        onRefresh={jest.fn()}
        onOpenRow={jest.fn()}
      />,
    );
    expect(screen.queryByTestId("background-empty-healthy")).toBeNull();
  });

  it("ba3_should_render_routine_as_summary_line", () => {
    setup({ view: { rows: [row()], completedOkToday: 12, running: 3 } });
    expect(screen.getAllByTestId("background-row")).toHaveLength(1);
    const summary = screen.getByTestId("background-summary");
    expect(summary).toHaveTextContent("12 completed OK today");
    expect(summary).toHaveTextContent("3 running");
  });

  it("rows carry the Background chip, a status text, and View output to the read-only link", () => {
    const props = setup({ view: { rows: [row()], completedOkToday: 0, running: 0 } });
    const r = screen.getByTestId("background-row");
    expect(within(r).getByTestId("notification-background-chip")).toHaveTextContent("Background");
    expect(within(r).getByText("FAILED")).toBeInTheDocument();
    const view = within(r).getByTestId("notification-view-output");
    expect(view).toHaveAttribute("href", "/?session=review%3Ah1&tab=terminal&notification=n1");
    fireEvent.click(view);
    expect(props.onOpenRow).toHaveBeenCalledWith(expect.objectContaining({ sessionId: "review:h1" }));
  });

  it("a row whose session is gone says so and keeps the captured message", () => {
    setup({ view: { rows: [row({ sessionAvailable: false })], completedOkToday: 0, running: 0 } });
    const r = screen.getByTestId("background-row");
    expect(r).toHaveTextContent("Session no longer available");
    expect(r).toHaveTextContent("3 tests failing");
    expect(within(r).getByTestId("notification-view-output")).toBeInTheDocument();
  });

  it("ba10_should_show_reply_only_for_pending_question", () => {
    const question = row({ key: "q", sessionId: "q1", sessionTitle: "q1", kind: "needs_human", statusLabel: "NEEDS INPUT", pendingQuestion: true, primaryRecordId: "n-q" });
    const failure = row();
    const approval = row({ key: "a", sessionId: "a1", kind: "needs_human", statusLabel: "NEEDS APPROVAL", pendingQuestion: false });
    const { unmount } = render(
      <BackgroundActivity
        view={{ rows: [failure, question, approval], completedOkToday: 0, running: 0 }}
        hasHiddenSessions
        loading={false}
        failed={false}
        offline={false}
        lastUpdatedAt={Date.now()}
        onRefresh={jest.fn()}
        onOpenRow={jest.fn()}
      />,
    );
    // before Story 5.6 ships: no Reply anywhere
    expect(screen.queryByTestId("notification-reply")).toBeNull();
    unmount();

    render(
      <BackgroundActivity
        view={{ rows: [failure, question, approval], completedOkToday: 0, running: 0 }}
        hasHiddenSessions
        loading={false}
        failed={false}
        offline={false}
        lastUpdatedAt={Date.now()}
        onRefresh={jest.fn()}
        onOpenRow={jest.fn()}
        replyEnabled
      />,
    );
    const replies = screen.getAllByTestId("notification-reply");
    expect(replies).toHaveLength(1);
    expect(replies[0]).toHaveAttribute("href", expect.stringContaining("reply=1"));
    expect(within(screen.getAllByTestId("background-row")[1]).getByTestId("notification-reply")).toBeInTheDocument();
  });

  it("refresh is inert while offline", () => {
    const props = setup({ offline: true });
    fireEvent.click(screen.getByTestId("background-refresh"));
    expect(props.onRefresh).not.toHaveBeenCalled();
  });
});
