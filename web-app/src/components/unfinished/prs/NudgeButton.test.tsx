import React from "react";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { Code, ConnectError } from "@connectrpc/connect";
import { NudgeOutcome } from "@/gen/session/v1/github_user_pb";
import { LinkedSessionStatus, type UserPR } from "@/gen/session/v1/types_pb";
import { MIN_TOUCH_TARGET_PX } from "@/lib/unfinished/prTouchTokens";
import type { NudgeClient } from "@/lib/hooks/useNudgePR";
import { NOT_IDLE_HINT, NudgeButton, SLOW_SEND_MS } from "./NudgeButton";
import { readStats, TAB_STATS_STORAGE_KEY } from "@/lib/unfinished/tabStats";
import { failingPR, fakeNudgeClient, makePR, session } from "./prTestFixtures";

jest.mock("./NudgeButton.css", () => new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") }));

const RUN = LinkedSessionStatus.RUNNING;
const PAUSE = LinkedSessionStatus.PAUSED;
const twoSessions = () =>
  failingPR({ linkedSessions: [session("a", RUN, 200), session("b", RUN, 100)] });

function ui(pr: UserPR, client?: NudgeClient) {
  return (
    <NudgeButton
      pr={pr}
      client={client}
      nudgeable
      sessions={pr.linkedSessions.map((s) => ({ sessionId: s.sessionId, status: s.status, steerReady: s.steerReady }))}
    />
  );
}
const mount = (pr: UserPR, client?: NudgeClient) => render(ui(pr, client));
const button = () => screen.getByTestId("pr-nudge-42");
const click = (el: HTMLElement) => act(async () => void fireEvent.click(el));
const status = () => screen.getByRole("status");
const alert = () => screen.getByRole("alert");

describe("NudgeButton", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("nudgeButton_should_SendOneRPCAndShowStatusWithOpenSessionLink_When_ClickedOnDelivered", async () => {
    const client = fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED });
    mount(failingPR(), client);
    await click(button());
    await click(button()); // ignored: aria-disabled
    expect(client.calls).toHaveLength(1);
    expect(within(status()).getByText("Request sent to fix-ci")).toBeInTheDocument();
    expect(within(status()).getByRole("link", { name: "Open session fix-ci" })).toHaveAttribute("href", "/?session=fix-ci");
    expect(button()).toHaveTextContent("Sent");
    expect(button()).toHaveAttribute("aria-disabled", "true");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("nudgeButton_should_SendOnOneClickWithoutDialog_When_DefaultSession", async () => {
    const client = fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED });
    mount(failingPR(), client);
    await click(button());
    expect(client.calls.map((c) => c.sessionId)).toEqual(["fix-ci"]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("nudgeButton_should_DisableWhilePendingAndReenableAfter60s_When_FakeTimersAdvance", async () => {
    let release: () => void = () => {};
    const gate = new Promise<void>((r) => (release = r));
    const calls: unknown[] = [];
    const client: NudgeClient = {
      nudgeSessionForPR: async (req) => {
        calls.push(req);
        await gate;
        return (await fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED }).nudgeSessionForPR(req));
      },
    };
    mount(failingPR(), client);
    await click(button());
    expect(button()).toHaveTextContent("Sending...");
    expect(button()).toHaveAttribute("aria-busy", "true");
    expect(button()).toHaveAttribute("aria-disabled", "true");
    act(() => void jest.advanceTimersByTime(SLOW_SEND_MS));
    expect(button()).toHaveTextContent("Still sending...");
    await click(button());
    expect(calls).toHaveLength(1);

    await act(async () => release());
    expect(button()).toHaveTextContent("Sent");
    act(() => void jest.advanceTimersByTime(60_000));
    expect(button()).toHaveTextContent("Ask fix-ci to fix");
    expect(button()).not.toHaveAttribute("aria-disabled");
  });

  const outcomes: Array<[string, NudgeOutcome, string, string, "status" | "alert"]> = [
    ["delivered", NudgeOutcome.DELIVERED, "", "Request sent to fix-ci", "status"],
    ["generic busy", NudgeOutcome.BUSY, "Session is busy. Try again when it is idle.", "Session is busy. Try again when it is idle.", "status"],
    ["no-controller busy", NudgeOutcome.BUSY, "Session isn't being monitored, so it can't safely take a request. Open it to restart it.", "Session isn't being monitored, so it can't safely take a request. Open it to restart it.", "status"],
    ["empty-detail busy", NudgeOutcome.BUSY, "", "Session can't take a request right now. Open it to continue.", "status"],
    ["paused", NudgeOutcome.PAUSED, "", "Session paused. Open it to resume", "status"],
    ["not tracked", NudgeOutcome.PAUSED, "Session is not running or not tracked. Open its page to restart it.", "Session is not running or not tracked. Open its page to restart it.", "status"],
    ["duplicate", NudgeOutcome.DUPLICATE, "", "Already requested in the last minute", "status"],
    ["nothing to fix", NudgeOutcome.NOTHING_TO_FIX, "", "Nothing to fix right now", "status"],
    ["not linked", NudgeOutcome.SESSION_NOT_LINKED, "", "Session no longer linked", "status"],
    ["pr not found", NudgeOutcome.PR_NOT_FOUND, "", "PR not found (closed or moved?)", "status"],
  ];

  it.each(outcomes)(
    "nudgeButton_should_ShowSpecifiedMessageAndNextAction_When_EachOutcome (%s)",
    async (_name, outcome, detail, text, role) => {
      mount(failingPR(), fakeNudgeClient({ outcome, detail }));
      await click(button());
      const region = role === "status" ? status() : alert();
      expect(region).toHaveTextContent(text);
      const hasLink = [NudgeOutcome.DELIVERED, NudgeOutcome.BUSY, NudgeOutcome.PAUSED, NudgeOutcome.DUPLICATE].includes(outcome);
      expect(within(region).queryAllByRole("link")).toHaveLength(hasLink ? 1 : 0);
    }
  );

  it("nudgeButton_should_ShowDistinctCopyAndRole_When_EachNudgeOutcome", () => {
    const texts = outcomes.map((o) => o[3]);
    expect(new Set(texts).size).toBe(texts.length);
  });

  const failures: Array<[string, Error, "alert", RegExp]> = [
    ["rate limit", new ConnectError("GitHub rate limit reached until 10:15", Code.ResourceExhausted), "alert", /rate limit reached until 10:15/],
    ["failed precondition", new ConnectError("This session's program does not support fix requests", Code.FailedPrecondition), "alert", /does not support fix requests/],
    ["network", new Error("socket hang up"), "alert", /^Could not send request: .*socket hang up/],
  ];

  it("nudgeButton_should_UseStatusForNonUrgentOutcomesAndAlertOnlyForActionErrors_When_EachOutcome", async () => {
    for (const [, outcome, detail] of outcomes) {
      const { unmount } = mount(failingPR(), fakeNudgeClient({ outcome, detail }));
      await click(button());
      expect(alert()).toBeEmptyDOMElement();
      expect(status()).not.toBeEmptyDOMElement();
      unmount();
    }
    for (const [, err, , text] of failures) {
      const { unmount } = mount(failingPR(), fakeNudgeClient(err));
      await click(button());
      expect(alert()).toHaveTextContent(text);
      expect(status()).toBeEmptyDOMElement();
      act(() => void jest.advanceTimersByTime(10 * 60_000));
      expect(alert()).toHaveTextContent(text); // alerts are never auto-cleared
      unmount();
    }
  });

  it("nudgeButton_should_RenderStatusAndAlertRegionsBeforeContent_When_Mounted", () => {
    mount(failingPR());
    expect(status()).toBeEmptyDOMElement();
    expect(alert()).toBeEmptyDOMElement();
  });

  it("nudgeButton_should_LabelAskSessionToFixWithTooltipAndNeverSayNudge_When_Rendered", () => {
    const { container } = mount(failingPR());
    expect(button()).toHaveTextContent("Ask fix-ci to fix");
    expect(button()).toHaveAttribute("aria-label", "Ask fix-ci to fix CI on PR #42");
    const hint = "Sends this session a message listing the failing checks, unresolved review threads and merge conflict for this PR, as links. Comment text is not included.";
    expect(button()).not.toHaveAttribute("title");
    expect(screen.getAllByText(hint)).toHaveLength(1);
    expect(button()).toHaveAccessibleDescription(hint);
    expect(container.textContent).not.toMatch(/nudge/i);
    expect(MIN_TOUCH_TARGET_PX).toBeGreaterThanOrEqual(44);
  });

  it("nudgeButton_should_RenderLabelledSessionSelectOnlyWhenTwoOrMoreSessions_When_Rendered", () => {
    const { unmount } = mount(failingPR());
    expect(screen.queryByLabelText("Session")).toBeNull();
    unmount();
    mount(twoSessions());
    const select = screen.getByLabelText("Session");
    expect(select.tagName).toBe("SELECT");
    expect(within(select).getAllByRole("option").map((o) => o.textContent)).toEqual(["a", "b"]);
  });

  it("nudgeButton_should_RetargetLabelWithoutSending_When_CaretSelectsOtherSession", async () => {
    const client = fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED });
    mount(twoSessions(), client);
    expect(button()).toHaveAccessibleName("Ask a to fix CI on PR #42");
    fireEvent.change(screen.getByLabelText("Session"), { target: { value: "b" } });
    expect(button()).toHaveAccessibleName("Ask b to fix CI on PR #42");
    expect(client.calls).toHaveLength(0);
    await click(button());
    expect(client.calls.map((c) => c.sessionId)).toEqual(["b"]);
  });

  it("nudgeButton_should_RetargetNameBeforeSend_When_CaretSelectsB", async () => {
    const client = fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED });
    const pr = failingPR({ linkedSessions: [session("a", RUN, 200), session("b", RUN, 100), session("c", PAUSE, 50)] });
    mount(pr, client);
    expect(within(screen.getByLabelText("Session")).getByRole("option", { name: /^c/ })).toBeDisabled();
    expect(screen.getByTestId("nudge-blocked-reason")).toHaveTextContent("c: Session paused. Open it to resume");
    fireEvent.change(screen.getByLabelText("Session"), { target: { value: "b" } });
    expect(client.calls).toHaveLength(0);
    await click(button());
    expect(client.calls[0].sessionId).toBe("b");
  });

  it("nudgeButton_should_RenderServerBusyDetailVerbatimAndFallBackOnlyWhenDetailEmpty_When_BusyOutcome", async () => {
    const generic = "Session is busy. Try again when it is idle.";
    const noController = "Session isn't being monitored, so it can't safely take a request. Open it to restart it.";
    const seen: string[] = [];
    for (const detail of [generic, noController, ""]) {
      const { unmount } = mount(failingPR(), fakeNudgeClient({ outcome: NudgeOutcome.BUSY, detail }));
      await click(button());
      seen.push(status().textContent ?? "");
      expect(within(status()).getByRole("link", { name: "Open session fix-ci" })).toBeInTheDocument();
      unmount();
    }
    expect(seen[0]).toContain(generic);
    expect(seen[1]).toContain(noController);
    expect(seen[2]).toContain("Session can't take a request right now. Open it to continue.");
    expect(seen[0]).not.toEqual(seen[1]);
  });

  it("nudgeButton_should_ContainVisibleLabelInAccessibleNameAndUseAriaDisabledKeepingFocus_When_PendingDeliveredDuplicate", async () => {
    for (const outcome of [NudgeOutcome.DELIVERED, NudgeOutcome.DUPLICATE]) {
      const { unmount } = mount(failingPR(), fakeNudgeClient({ outcome }));
      expect(button().getAttribute("aria-label")).toMatch(/^Ask fix-ci to fix/);
      act(() => button().focus());
      await click(button());
      expect(button()).toHaveAttribute("aria-disabled", "true");
      expect(button()).not.toBeDisabled();
      expect(document.activeElement).toBe(button());
      unmount();
    }
  });

  it("nudgeButton_should_KeepSelectionAcrossPollsAndResetWhenPaused_When_Rerendered", async () => {
    const client = fakeNudgeClient({ outcome: NudgeOutcome.BUSY });
    const pr = twoSessions();
    const { rerender } = mount(pr, client);
    fireEvent.change(screen.getByLabelText("Session"), { target: { value: "b" } });
    rerender(ui(twoSessions(), client)); // a poll delivers a fresh object
    expect(button()).toHaveAccessibleName("Ask b to fix CI on PR #42");

    await click(button());
    // BUSY is not a retry-later state: no cooldown, no retry framing, button stays usable.
    expect(button()).toHaveTextContent("Ask b to fix");
    expect(button()).not.toHaveAttribute("aria-disabled");
    expect(status().textContent).not.toMatch(/try again/i);

    rerender(ui(failingPR({ linkedSessions: [session("a", RUN, 200), session("b", PAUSE, 100)] }), client));
    expect(button()).toHaveAccessibleName("Ask a to fix CI on PR #42");
    expect(screen.getByTestId("nudge-blocked-reason")).toHaveTextContent("b: Session paused. Open it to resume");
  });

  it("nudgeButton_should_NameTheWindowOnButton_When_Duplicate", async () => {
    mount(failingPR(), fakeNudgeClient({ outcome: NudgeOutcome.DUPLICATE }));
    await click(button());
    expect(button()).toHaveTextContent("Already requested in the last minute");
    expect(button()).toHaveAttribute("aria-disabled", "true");
  });

  it("nudgeButton_should_StayHiddenAcrossPollsUntilAttentionChanges_When_NothingToFix", async () => {
    const client = fakeNudgeClient({ outcome: NudgeOutcome.NOTHING_TO_FIX });
    const { rerender } = mount(failingPR(), client);
    await click(button());
    expect(screen.queryByTestId("pr-nudge-42")).toBeNull();
    rerender(ui(failingPR(), client)); // poll: fresh object, same attention signature
    expect(screen.queryByTestId("pr-nudge-42")).toBeNull();
    rerender(ui(failingPR({ unresolvedThreadCount: 2 }), client)); // attention changed
    expect(screen.getByTestId("pr-nudge-42")).toBeInTheDocument();
  });

  it("nudgeButton_should_NotOfferPromptEditing_When_Rendered", () => {
    mount(failingPR());
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("nudgeButton_should_ShowReasonAndLinkAndDisableTarget_When_PausedOutcomeOrFailedPrecondition", async () => {
    const pr = twoSessions();
    mount(pr, fakeNudgeClient({ outcome: NudgeOutcome.PAUSED }, new ConnectError("unsupported", Code.FailedPrecondition)));
    await click(button());
    expect(button()).toHaveAccessibleName("Ask b to fix CI on PR #42"); // retargeted off the paused one
    await click(button());
    expect(alert()).toHaveTextContent("unsupported");
    expect(button()).toHaveAttribute("aria-disabled", "true");
  });

  it("nudgeButton_should_RenderNoControlsAndCallRefresh_When_NothingToFix", async () => {
    const onNothing = jest.fn();
    render(
      <NudgeButton
        pr={failingPR()}
        nudgeable
        sessions={[{ sessionId: "fix-ci", status: RUN, steerReady: true }]}
        client={fakeNudgeClient({ outcome: NudgeOutcome.NOTHING_TO_FIX })}
        onNothingToFix={onNothing}
      />
    );
    act(() => button().focus());
    await click(button());
    expect(screen.queryByTestId("pr-nudge-42")).toBeNull();
    expect(onNothing).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(screen.getByTestId("pr-nudge-status-42"));
  });

  it("nudgeButton_should_NotRender_When_NotNudgeable", () => {
    render(<NudgeButton pr={makePR()} nudgeable={false} sessions={[{ sessionId: "x", status: RUN, steerReady: true }]} />);
    expect(screen.queryByTestId("pr-nudge-42")).toBeNull();
  });

  describe("when no linked session is confirmed idle", () => {
    const notIdlePR = (...ids: string[]) =>
      failingPR({ linkedSessions: ids.map((id, i) => session(id, RUN, 200 - i, false)) });

    beforeEach(() => window.localStorage.removeItem(TAB_STATS_STORAGE_KEY));

    it("nudgeButton_should_LeadWithOpenSessionAndDisableNudge_When_NoSessionSteerReady", async () => {
      const client = fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED });
      mount(notIdlePR("fix-ci"), client);
      expect(screen.getByTestId("pr-open-session-42")).toHaveAttribute("href", "/?session=fix-ci");
      expect(button()).toHaveAttribute("aria-disabled", "true");
      expect(button()).toHaveAttribute("aria-describedby");
      expect(screen.getByText(NOT_IDLE_HINT)).toBeInTheDocument();
      await click(button());
      expect(client.calls).toHaveLength(0);
    });

    it("nudgeButton_should_CountOpenSessionClick_When_PrimaryLinkClicked", async () => {
      mount(notIdlePR("fix-ci"));
      expect(readStats().openSessionClicks).toBe(0);
      await act(async () => {
        fireEvent.click(screen.getByTestId("pr-open-session-42"));
      });
      expect(readStats().openSessionClicks).toBe(1);
    });

    it("nudgeButton_should_OfferNudgeAsPrimaryAndSkipPrimaryOpenLink_When_AnySessionSteerReady", async () => {
      const pr = failingPR({ linkedSessions: [session("busy", RUN, 300, false), session("idle", RUN, 100, true)] });
      const client = fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED });
      mount(pr, client);
      expect(screen.queryByTestId("pr-open-session-42")).toBeNull();
      expect(button()).toHaveAccessibleName("Ask idle to fix CI on PR #42");
      await click(button());
      expect(client.calls.map((c) => c.sessionId)).toEqual(["idle"]);
    });

    it("nudgeButton_should_KeepSentState_When_PollThenReportsSessionNotIdle", async () => {
      const client = fakeNudgeClient({ outcome: NudgeOutcome.DELIVERED });
      const { rerender } = mount(failingPR(), client);
      await click(button());
      rerender(ui(notIdlePR("fix-ci"), client));
      expect(button()).toHaveTextContent("Sent");
      expect(screen.queryByTestId("pr-open-session-42")).toBeNull();
    });
  });
});
