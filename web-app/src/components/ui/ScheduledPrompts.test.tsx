import React from "react";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { WakeCancelOutcome } from "@/gen/session/v1/wake_trigger_pb";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";
import { cancelWakeTrigger } from "@/lib/api/wakeTriggerApi";
import { SCHEDULED_COPY } from "@/lib/copy/scheduledPromptsCopy";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { ScheduledPrompts, type ScheduledPromptsProps } from "./ScheduledPrompts";
import { formatClockTime } from "./OfflineBanner";
import { MIN, NOW, makePeer, makeState, makeTrigger } from "./__fixtures__/scheduledPromptsFixtures";

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
jest.mock("@/lib/api/wakeTriggerApi", () => ({ cancelWakeTrigger: jest.fn(), createOperatorReminder: jest.fn() }));
jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));

const cancelMock = cancelWakeTrigger as unknown as jest.Mock;
const announce = jest.fn();

function renderSection(over: Partial<ScheduledPromptsProps> = {}) {
  const props: ScheduledPromptsProps = { state: makeState(), resumeFlag: jest.fn().mockResolvedValue(undefined), now: NOW, ...over };
  return render(
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
      <ScheduledPrompts {...props} />
    </AnnouncerContext.Provider>,
  );
}

beforeEach(() => {
  cancelMock.mockReset();
  announce.mockClear();
});

describe("ScheduledPrompts", () => {
  it("ScheduledPrompts_should_listOwnerPlainLanguageConditionSinceNextFireAndCancel_When_twoPending", () => {
    const at = makeTrigger({ ownerTitle: "diagnose:abc", nextFireAt: NOW + 25 * MIN });
    const ci = makeTrigger({
      ownerTitle: "review:pr-1042",
      triggerType: "ci_event",
      condition: "when PR #901 (o/r) CI passes",
      nextFireAt: undefined,
      createdAt: NOW - 12 * MIN,
    });
    renderSection({ state: makeState({ triggers: [at, { ...ci, nextFireAt: undefined } as typeof ci] }) });

    const rows = screen.getAllByTestId("scheduled-prompt-row");
    expect(rows).toHaveLength(2);
    expect(screen.getByRole("button", { name: "Scheduled prompts (2)" })).toHaveAttribute("aria-expanded", "true");
    const first = within(rows[0]);
    expect(first.getByText("diagnose:abc")).toBeInTheDocument();
    expect(first.getByText(`prompts again at ${formatClockTime(NOW + 25 * MIN)}`)).toBeInTheDocument();
    expect(first.getByText(new RegExp(`since 12m ago - next: ${formatClockTime(NOW + 25 * MIN)}`))).toBeInTheDocument();
    expect(within(rows[1]).getByText("prompts again when PR #901 (o/r) CI passes")).toBeInTheDocument();
    expect(within(rows[1]).getByText(/next check: when it happens/)).toBeInTheDocument();
    rows.forEach((r) => expect(within(r).getByRole("button", { name: "Cancel prompt" })).toBeInTheDocument());
  });

  it("ux_ac35_should_listFieldsWithCancelPrompt_And_showNoScheduledPromptsWhenEmpty", () => {
    const view = renderSection({ state: makeState() });
    expect(screen.getByTestId("scheduled-empty")).toHaveTextContent(SCHEDULED_COPY.empty);
    view.unmount();

    renderSection({ state: makeState({ triggers: [makeTrigger()], peers: [makePeer()] }) });
    expect(screen.getAllByTestId("scheduled-prompt-row")).toHaveLength(1);
    expect(screen.getAllByTestId("peer-help-row")).toHaveLength(1);
    expect(screen.queryByTestId("scheduled-empty")).toBeNull();
  });

  it("ScheduledPrompts_should_showCancelConfirmThenPromptCancelledAnnouncement_When_cancelConfirmed", async () => {
    cancelMock.mockResolvedValue({ outcome: WakeCancelOutcome.CANCELLED });
    const state = makeState({ triggers: [makeTrigger()], refresh: jest.fn().mockResolvedValue(true) });
    renderSection({ state });

    fireEvent.click(screen.getByRole("button", { name: "Cancel prompt" }));
    expect(screen.getByText("Cancel this scheduled prompt? The agent will not be re-prompted.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Yes, cancel" })).toHaveFocus();
    expect(screen.getByRole("button", { name: "Keep" })).toBeInTheDocument();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Yes, cancel" })));

    expect(cancelMock).toHaveBeenCalledWith(state.triggers[0].id);
    expect(screen.queryByTestId("scheduled-prompt-row")).toBeNull();
    expect(announce).toHaveBeenCalledWith("Prompt cancelled", "polite", expect.any(String));
    expect(state.refresh).toHaveBeenCalled();
  });

  it("ux_ac36_should_confirmInlineThenAnnouncePromptCancelled_And_keepOnKeepOrEscape", () => {
    renderSection({ state: makeState({ triggers: [makeTrigger()] }) });
    fireEvent.click(screen.getByRole("button", { name: "Cancel prompt" }));
    fireEvent.click(screen.getByRole("button", { name: "Keep" }));
    expect(screen.getByRole("button", { name: "Cancel prompt" })).toBeInTheDocument();
    expect(cancelMock).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Cancel prompt" }));
    fireEvent.keyDown(screen.getByRole("button", { name: "Yes, cancel" }), { key: "Escape" });
    expect(screen.getByRole("button", { name: "Cancel prompt" })).toBeInTheDocument();
  });

  it("ScheduledPrompts_should_showAlreadyFiredAtTimeWithoutToast_When_cancelRaces", async () => {
    const firedAt = NOW + 30 * MIN;
    cancelMock.mockResolvedValue({ outcome: WakeCancelOutcome.ALREADY_FIRED, deliveredAt: timestampFromMs(firedAt) });
    renderSection({ state: makeState({ triggers: [makeTrigger()] }) });

    fireEvent.click(screen.getByRole("button", { name: "Cancel prompt" }));
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Yes, cancel" })));

    expect(screen.getByText(`Already fired at ${formatClockTime(firedAt)}.`)).toBeInTheDocument();
    expect(document.querySelector('[role="alert"]')).toBeNull();
    expect(announce).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Cancel prompt" })).toBeNull();
  });

  it("ux_ac37_should_showAlreadyFiredWithoutToast_When_cancelRaces", async () => {
    cancelMock.mockResolvedValue({ outcome: WakeCancelOutcome.ALREADY_ENDED });
    renderSection({ state: makeState({ triggers: [makeTrigger()] }) });
    fireEvent.click(screen.getByRole("button", { name: "Cancel prompt" }));
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Yes, cancel" })));
    expect(screen.getByText("Already ended.")).toBeInTheDocument();
    expect(document.querySelector('[role="alert"]')).toBeNull();
  });

  it("ScheduledPrompts_should_offerRetryAndDismissInline_When_cancelFails", async () => {
    cancelMock.mockRejectedValueOnce(new Error("boom")).mockResolvedValue({ outcome: WakeCancelOutcome.CANCELLED });
    renderSection({ state: makeState({ triggers: [makeTrigger()] }) });
    fireEvent.click(screen.getByRole("button", { name: "Cancel prompt" }));
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Yes, cancel" })));

    expect(screen.getByText("Couldn't cancel.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeInTheDocument();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Retry" })));
    expect(cancelMock).toHaveBeenCalledTimes(2);
    expect(screen.queryByTestId("scheduled-prompt-row")).toBeNull();
  });

  it("ScheduledPrompts_should_render20RowsWithShowAllN_When_25Pending", () => {
    const triggers = Array.from({ length: 25 }, (_, i) => makeTrigger({ nextFireAt: NOW + (i + 1) * MIN }));
    renderSection({ state: makeState({ triggers }) });

    expect(screen.getAllByTestId("scheduled-prompt-row")).toHaveLength(20);
    fireEvent.click(screen.getByRole("button", { name: "Show all (25)" }));
    expect(screen.getAllByTestId("scheduled-prompt-row")).toHaveLength(25);
    expect(screen.queryByTestId("scheduled-show-all")).toBeNull();
  });

  it("ScheduledPrompts_should_showCouldntLoadWithRetry_When_listFails", () => {
    const state = makeState({ failed: true, lastUpdatedAt: null, refresh: jest.fn().mockResolvedValue(true) });
    renderSection({ state });

    expect(screen.getByText("Couldn't load scheduled prompts.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(state.refresh).toHaveBeenCalledTimes(1);
  });

  it("ScheduledPrompts_should_keepRowsAndOfferRetry_When_aLaterPollFails", () => {
    const state = makeState({ failed: true, triggers: [makeTrigger()], refresh: jest.fn().mockResolvedValue(true) });
    renderSection({ state });
    expect(screen.getAllByTestId("scheduled-prompt-row")).toHaveLength(1);
    fireEvent.click(within(screen.getByTestId("scheduled-stale")).getByRole("button", { name: "Retry" }));
    expect(state.refresh).toHaveBeenCalled();
  });

  it("ScheduledPrompts_should_showMissedReasonExceptionRowWithCancelAndOpenOwner_When_missed", () => {
    const missed = makeTrigger({ status: "missed", endReason: "expired", ownerTitle: "review:pr-9", ownerSessionUuid: "uuid-9" });
    renderSection({ state: makeState({ triggers: [makeTrigger(), missed] }) });

    const row = screen.getAllByTestId("scheduled-prompt-row")[0];
    expect(row).toHaveAttribute("data-status", "missed");
    expect(within(row).getByTestId("scheduled-missed")).toHaveTextContent("Missed: it expired before it could be delivered");
    expect(within(row).getByText("!")).toBeInTheDocument();
    expect(within(row).getByRole("button", { name: "Cancel prompt" })).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: "Open owner" })).toHaveAttribute("href", "/?session=uuid-9&tab=terminal");
  });

  it("ScheduledPrompts_should_showScheduledPromptsPausedBannerResumeConfirmAndFlagLink_When_flagOff", async () => {
    const refresh = jest.fn().mockResolvedValue(true);
    const resumeFlag = jest.fn().mockResolvedValue(undefined);
    renderSection({ state: makeState({ enabled: false, triggers: [makeTrigger()], refresh }), resumeFlag });

    expect(screen.getByText("Scheduled prompts paused. Agents will not be prompted automatically.")).toBeInTheDocument();
    expect(screen.getByTestId("scheduled-flag-settings")).toHaveAttribute("href", "/settings/features");
    expect(screen.getByText("Paused")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel prompt" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Resume scheduled prompts" }));
    expect(resumeFlag).not.toHaveBeenCalled();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Yes, resume" })));
    expect(resumeFlag).toHaveBeenCalledTimes(1);
    expect(refresh).toHaveBeenCalled();
    expect(announce).toHaveBeenCalledWith("Scheduled prompts resumed", "polite", expect.any(String));
  });

  it("ux_ac38_should_offerRetry_When_theServerStillReportsPausedAfterResume", async () => {
    const refresh = jest.fn().mockResolvedValue(false);
    renderSection({ state: makeState({ enabled: false, triggers: [makeTrigger()], refresh }) });
    fireEvent.click(screen.getByRole("button", { name: "Resume scheduled prompts" }));
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Yes, resume" })));

    expect(screen.getByText("Couldn't resume.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeInTheDocument();
  });

  it("ux_ac39_should_showOwnerArchivedAndSessionGoneNotes_With_CancelStillAvailable", () => {
    const archived = makeTrigger({ ownerArchived: true });
    const gone = makeTrigger({ ownerExists: false, ownerTitle: "" });
    renderSection({ state: makeState({ triggers: [archived, gone] }) });
    expect(screen.getByText("Owner archived. This prompt will not revive it.")).toBeInTheDocument();
    expect(screen.getByText("Session no longer exists")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Cancel prompt" })).toHaveLength(2);
  });

  it("ScheduledPrompts_should_labelOperatorRemindersAndOfferCancelReminder", () => {
    renderSection({ state: makeState({ triggers: [makeTrigger({ operatorReminder: true, actionKind: "notify_operator" })] }) });
    expect(screen.getByText("Reminder (you)")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Cancel reminder" }));
    expect(screen.getByText("Cancel this reminder?")).toBeInTheDocument();
  });

  it("ScheduledPrompts_should_renderNothing_When_neverUsedAndFlagOff", () => {
    const { container } = renderSection({ state: makeState({ enabled: false }) });
    expect(container).toBeEmptyDOMElement();
  });
});
