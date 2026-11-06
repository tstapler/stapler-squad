import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { ReminderOutcome } from "@/gen/session/v1/wake_trigger_pb";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";
import { cancelWakeTrigger, createOperatorReminder } from "@/lib/api/wakeTriggerApi";
import { buildActivityRows, buildRollup, type ActivityRow } from "@/lib/utils/backgroundActivity";
import { resetReviewedMarksForTests } from "@/lib/hooks/useReviewedMarks";
import { BackgroundActivity } from "./BackgroundActivity";
import { formatClockTime } from "./OfflineBanner";
import { REMINDER_UNDO_MS, RemindMeMenu } from "./RemindMeMenu";
import { makeBackgroundRow, makeHiddenSession } from "./__fixtures__/notificationFixtures";
import { NOW, makeTrigger } from "./__fixtures__/scheduledPromptsFixtures";

jest.mock("@/lib/nav", () =>
  require("@/lib/nav/testing-mock").navMockFromNext({
    link: () => ({
      __esModule: true,
      default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [k: string]: unknown }) => (
        <a href={href} {...props}>
          {children}
        </a>
      ),
    }),
  }),
);
jest.mock("@/lib/api/wakeTriggerApi", () => ({ createOperatorReminder: jest.fn(), cancelWakeTrigger: jest.fn() }));
jest.mock("@/lib/hooks/useFeatureFlagValue", () => ({ useFeatureFlagValue: () => true }));

const create = createOperatorReminder as unknown as jest.Mock;
const cancel = cancelWakeTrigger as unknown as jest.Mock;
const announce = jest.fn();
const clock = () => new Date(NOW);
const created = (id = "t1") => ({ outcome: ReminderOutcome.CREATED, trigger: makeTrigger({ id }) });

function renderMenu() {
  return render(
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
      <RemindMeMenu sessionUuid="review-1" label="review:pr-1042 (review): finished" now={clock} />
    </AnnouncerContext.Provider>,
  );
}

const open = () => fireEvent.click(screen.getByRole("button", { name: "Remind me" }));
const pick = async (name: string) => {
  open();
  await act(async () => fireEvent.click(screen.getByRole("menuitem", { name })));
};

beforeEach(() => {
  jest.useFakeTimers();
  create.mockReset();
  cancel.mockReset();
  announce.mockClear();
});
afterEach(() => jest.useRealTimers());

describe("RemindMeMenu", () => {
  it("RemindMeMenu_should_createNotifyOperatorTriggerAndShowUndo10s_When_presetPicked", async () => {
    create.mockResolvedValue(created());
    renderMenu();
    await pick("In 1 hour");

    expect(create).toHaveBeenCalledWith({
      sessionUuid: "review-1",
      fireAt: new Date(NOW + 3_600_000),
      label: "review:pr-1042 (review): finished",
    });
    expect(screen.getByText(`Reminder set for ${formatClockTime(NOW + 3_600_000)}`)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Undo" })).toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith(expect.stringContaining("Reminder set for"), "polite", expect.any(String));

    act(() => void jest.advanceTimersByTime(REMINDER_UNDO_MS - 1));
    expect(screen.getByRole("button", { name: "Undo" })).toBeInTheDocument();
    act(() => void jest.advanceTimersByTime(1));
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
    expect(screen.getByText(/Reminder set for/)).toBeInTheDocument();
    expect(cancel).not.toHaveBeenCalled();
  });

  it("ux_ac40_should_offerUndoThatCancelsTheTriggerWithin10s", async () => {
    create.mockResolvedValue(created("t9"));
    cancel.mockResolvedValue({});
    renderMenu();
    await pick("Tomorrow 9:00");
    const expected = new Date(NOW);
    expected.setDate(expected.getDate() + 1);
    expected.setHours(9, 0, 0, 0);
    expect(create.mock.calls[0][0].fireAt).toEqual(expected);

    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Undo" })));

    expect(cancel).toHaveBeenCalledWith("t9");
    expect(screen.getByText("Reminder removed")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("RemindMeMenu_should_rejectPastTimeInline_And_letTheOperatorRepick", async () => {
    renderMenu();
    open();
    fireEvent.click(screen.getByRole("menuitem", { name: "Pick time..." }));
    fireEvent.change(screen.getByLabelText("Reminder time"), { target: { value: "2020-01-01T09:00" } });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Set reminder" })));

    expect(screen.getByText("Choose a time in the future")).toBeInTheDocument();
    expect(create).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Reminder time"), { target: { value: "2026-10-20T09:00" } });
    create.mockResolvedValue(created());
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Set reminder" })));
    expect(create).toHaveBeenCalledTimes(1);
  });

  it("ux_ac41_should_rejectPastTimeWithChooseFutureCopy_When_serverReportsPastTime", async () => {
    create.mockResolvedValue({ outcome: ReminderOutcome.PAST_TIME });
    renderMenu();
    await pick("In 1 hour");
    expect(screen.getByText("Choose a time in the future")).toBeInTheDocument();
    expect(screen.getByLabelText("Reminder time")).toBeInTheDocument();
  });

  it("RemindMeMenu_should_showCapCopyWithLink_When_capExceeded", async () => {
    create.mockResolvedValue({ outcome: ReminderOutcome.CAP_EXCEEDED });
    renderMenu();
    await pick("In 1 hour");

    expect(screen.getByText("Too many pending reminders. Cancel one in Scheduled prompts.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Scheduled prompts" })).toHaveAttribute("href", "#scheduled-prompts-heading");
  });

  it("RemindMeMenu_should_offerRetry_When_createFails_And_createOnlyOnceOnDuplicate", async () => {
    create.mockRejectedValueOnce(new Error("down")).mockResolvedValue({ outcome: ReminderOutcome.DUPLICATE, trigger: makeTrigger() });
    renderMenu();
    await pick("In 1 hour");

    expect(screen.getByText("Couldn't set the reminder.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeInTheDocument();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Retry" })));

    expect(create).toHaveBeenCalledTimes(2);
    expect(create.mock.calls[0][0].fireAt).toEqual(create.mock.calls[1][0].fireAt);
    expect(screen.getByText(/Reminder set for/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("RemindMeMenu_should_moveFocusWithArrowKeys_And_closeOnEscapeWithFocusBackOnTheButton", () => {
    renderMenu();
    open();
    const items = screen.getAllByRole("menuitem");
    expect(items.map((i) => i.textContent)).toEqual(["In 1 hour", "Tomorrow 9:00", "Pick time..."]);
    expect(items[0]).toHaveFocus();
    fireEvent.keyDown(items[0], { key: "ArrowDown" });
    expect(items[1]).toHaveFocus();
    fireEvent.keyDown(items[1], { key: "ArrowUp" });
    fireEvent.keyDown(items[0], { key: "ArrowUp" });
    expect(items[2]).toHaveFocus();
    fireEvent.keyDown(items[2], { key: "Escape" });
    expect(screen.queryByRole("menu")).toBeNull();
    expect(screen.getByRole("button", { name: "Remind me" })).toHaveFocus();
  });
});

describe("Remind me on Background rows", () => {
  beforeEach(() => resetReviewedMarksForTests());

  it("ux_ac40_should_appearOnlyOnInformationalRowsAndOfferUndo10s", () => {
    jest.useRealTimers();
    const finished = makeHiddenSession({ id: "review:done", title: "review:done", state: "stopped", backgroundOutcome: "completed" });
    const running = makeHiddenSession({ id: "diagnose:run", title: "diagnose:run", state: "running" });
    const exception = makeBackgroundRow({ key: "bg:q", sessionId: "review:ask", sessionTitle: "review:ask", kind: "needs_human", statusLabel: "NEEDS INPUT" });
    const needsHuman: ActivityRow = {
      key: exception.key,
      sessionId: exception.sessionId,
      sessionTitle: exception.sessionTitle,
      role: "review",
      state: "exception_needs_human",
      timestampMs: exception.timestampMs,
      exception,
    };
    const rows = [needsHuman, ...buildActivityRows([], [finished, running])];
    render(
      <BackgroundActivity
        rows={rows}
        rollup={buildRollup(rows)}
        loading={false}
        failed={false}
        offline={false}
        lastUpdatedAt={Date.now()}
        onRefresh={jest.fn()}
        onOpenRow={jest.fn()}
      />,
    );

    const reminded = screen.getAllByTestId("background-row").filter((r) => r.querySelector('[data-testid="remind-me-button"]'));
    expect(reminded.map((r) => r.getAttribute("data-session-id"))).toEqual(["review:done"]);
  });
});
