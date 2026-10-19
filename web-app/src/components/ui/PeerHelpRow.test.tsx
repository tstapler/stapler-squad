import React from "react";
import fs from "fs";
import path from "path";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { ActionKind, ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";
import { invokeNotificationAction } from "@/lib/api/notificationActionApi";
import { PeerHelpRow } from "./PeerHelpRow";
import { PeerHelpGroups } from "./ScheduledPromptsSection";
import { MIN, NOW, makePeer, makeState } from "./__fixtures__/scheduledPromptsFixtures";
import type { PeerHelpInfo } from "@/gen/session/v1/wake_trigger_pb";

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [k: string]: unknown }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));
jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));
jest.mock("@/lib/api/wakeTriggerApi", () => ({}));

const invoke = invokeNotificationAction as unknown as jest.Mock;
const announce = jest.fn();
const respond = (outcome: ActionOutcome, detail = "") => invoke.mockResolvedValue({ outcome, detail, staleReason: "", retryAfterSeconds: 0 });

function renderRow(info: PeerHelpInfo, onChanged = jest.fn()) {
  const view = render(
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
      <ul>
        <PeerHelpRow info={info} onChanged={onChanged} now={NOW} />
      </ul>
    </AnnouncerContext.Provider>,
  );
  return { ...view, onChanged };
}

beforeEach(() => {
  invoke.mockReset();
  announce.mockClear();
});

describe("PeerHelpRow", () => {
  it("PeerHelpRow_should_showAskerAnswererQuestionAgeState_And_notCountInBadge_When_waiting", () => {
    renderRow(makePeer());

    const row = screen.getByTestId("peer-help-row");
    expect(within(row).getByText("review:pr-1042 asked diagnose:abc")).toBeInTheDocument();
    expect(within(row).getByText("Which approach for the retry logic?")).toBeInTheDocument();
    expect(within(row).getByText("waiting 3m")).toBeInTheDocument();
    expect(within(row).getByTestId("peer-help-state")).toHaveTextContent("Waiting for diagnose:abc");
    expect(within(row).getByRole("link", { name: "Open asker" })).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: "Open peer" })).toBeInTheDocument();
    expect(within(row).queryByRole("button")).toBeNull();
  });

  it("ux_ac31_should_showFieldsAndNotIncrementBadge_When_waiting", () => {
    // A waiting request is not a notification, so it can never reach the Needs-you badge:
    // the row must stay clear of the notification stores and of any alert role.
    const source = fs.readFileSync(path.join(__dirname, "PeerHelpRow.tsx"), "utf8");
    expect(source).not.toMatch(/NotificationContext|useNotifications|addNotification/);
    renderRow(makePeer());
    expect(document.querySelector('[role="alert"], [data-needs-you]')).toBeNull();
    expect(screen.getByTestId("peer-help-row")).toHaveAttribute("data-escalated", "false");
  });

  it("PeerHelpRow_should_offerAnswerForPeerCancelRequestOpen_When_escalated", () => {
    renderRow(makePeer({ escalated: true, createdAt: NOW - 11 * MIN }));

    const row = screen.getByTestId("peer-help-row");
    expect(row).toHaveAttribute("data-escalated", "true");
    expect(within(row).getByText("HELP STALLED")).toBeInTheDocument();
    expect(within(row).getByText("review:pr-1042 asked diagnose:abc: no reply in 10m")).toBeInTheDocument();
    expect(within(row).getByRole("button", { name: "Answer for diagnose:abc" })).toBeInTheDocument();
    expect(within(row).getByRole("button", { name: "Cancel request" })).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: "Open" })).toHaveAttribute("href", "/?session=review-uuid&tab=terminal");
  });

  it("PeerHelpRow_should_leadWithThePlainReason_When_peerStopped", () => {
    renderRow(makePeer({ escalated: true, stateReason: "Peer is stopped. Not delivered unless resumed." }));
    expect(screen.getByText(/asked diagnose:abc: Peer is stopped. Not delivered unless resumed./)).toBeInTheDocument();
  });

  it("ux_ac32_should_offerAnswerForPeerCancelRequestOpen_AndEachShowsRetryOnError", async () => {
    const { onChanged } = renderRow(makePeer({ escalated: true }));

    fireEvent.click(screen.getByRole("button", { name: "Answer for diagnose:abc" }));
    fireEvent.change(screen.getByLabelText(/Your answer/), { target: { value: "Use option B" } });
    respond(ActionOutcome.NOT_SENT);
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Send answer" })));
    expect(screen.getByText("Couldn't send your answer.")).toBeInTheDocument();
    respond(ActionOutcome.APPLIED);
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Retry" })));

    const second = invoke.mock.calls[1][0];
    expect(second).toMatchObject({ actionKind: ActionKind.ANSWER_GUIDANCE, targetKey: expect.any(String), params: { answer: "Use option B" } });
    expect(invoke.mock.calls[0][0].idempotencyKey).not.toBe(second.idempotencyKey);
    expect(announce).toHaveBeenCalledWith("Answer sent", "polite", expect.any(String));
    expect(onChanged).toHaveBeenCalledTimes(1);

    invoke.mockRejectedValueOnce(new Error("network"));
    fireEvent.click(screen.getByRole("button", { name: "Cancel request" }));
    expect(screen.getByRole("button", { name: "Yes, cancel" })).toHaveFocus();
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Yes, cancel" })));
    expect(screen.getByText("Couldn't cancel the request.")).toBeInTheDocument();
    respond(ActionOutcome.APPLIED);
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Retry" })));
    expect(invoke.mock.calls[3][0]).toMatchObject({ actionKind: ActionKind.CANCEL_PEER_HELP });
    expect(announce).toHaveBeenCalledWith("Request cancelled", "polite", expect.any(String));
  });

  it("PeerHelpRow_should_showFlagOffCopyWithoutRetry_When_actionFlagIsOff", async () => {
    renderRow(makePeer({ escalated: true }));
    respond(ActionOutcome.FLAG_OFF);
    fireEvent.click(screen.getByRole("button", { name: "Cancel request" }));
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Yes, cancel" })));
    expect(screen.getByText("This action is turned off. Open session.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeInTheDocument();
  });

  it("PeerHelpRow_should_showNotSentPlainReason_When_acceptedThenAnswererDeletedOrArchived", () => {
    const { unmount } = renderRow(makePeer({ state: "not_sent", stateReason: "peer no longer exists" }));
    expect(screen.getByTestId("peer-help-state")).toHaveTextContent("Not sent: peer no longer exists");
    expect(screen.queryByRole("link", { name: "Open peer" })).toBeNull();
    unmount();
    renderRow(makePeer({ state: "not_sent", stateReason: "peer is archived" }));
    expect(screen.getByTestId("peer-help-state")).toHaveTextContent("Not sent: peer is archived");
  });

  it("ux_ac33_should_showNotSentReason_When_answererArchivedOrDeletedAfterCreation", () => {
    renderRow(makePeer({ state: "not_sent", stateReason: "peer works in a different repository", escalated: true }));
    expect(screen.getByTestId("peer-help-state")).toHaveTextContent("Not sent: peer works in a different repository");
    expect(screen.getByRole("link", { name: "Open" })).toBeInTheDocument();
  });

  it("ux_ac34_should_showAnsweredByWithProvenanceAndMoveToRecent", () => {
    const answered = makePeer({ state: "answered", answeredBy: "diagnose:abc", answer: "use B" });
    render(
      <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
        <PeerHelpGroups state={makeState({ peers: [answered, makePeer()] })} now={NOW} />
      </AnnouncerContext.Provider>,
    );

    const recent = screen.getByTestId("peer-help-recent");
    expect(within(recent).getByTestId("peer-help-state")).toHaveTextContent("Answered by diagnose:abc (from session diagnose:abc)");
    expect(within(screen.getByRole("list", { name: "Help between agents" })).getAllByTestId("peer-help-row")).toHaveLength(1);
  });

  it("PeerHelpRow_should_showOperatorOnBehalfProvenance_When_operatorAnsweredForPeer", () => {
    renderRow(makePeer({ state: "answered", answeredBy: "operator", onBehalfOf: "diagnose:abc", answer: "use B" }));
    expect(screen.getByTestId("peer-help-state")).toHaveTextContent("Answered by the operator on behalf of diagnose:abc");
  });

  it("PeerHelpRow_should_showAlreadyAnsweredBySessionAndValue_When_peerAnsweredFirst", async () => {
    const { onChanged, rerender } = renderRow(makePeer({ escalated: true }));
    respond(ActionOutcome.ALREADY_RESOLVED, "use B");
    fireEvent.click(screen.getByRole("button", { name: "Answer for diagnose:abc" }));
    fireEvent.change(screen.getByLabelText(/Your answer/), { target: { value: "use A" } });
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Send answer" })));

    expect(onChanged).toHaveBeenCalled();
    expect(screen.getByTestId("peer-help-state")).toHaveTextContent("use B");

    rerender(
      <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
        <ul>
          <PeerHelpRow info={makePeer({ state: "answered", answeredBy: "diagnose:abc", answer: "use B" })} onChanged={onChanged} now={NOW} />
        </ul>
      </AnnouncerContext.Provider>,
    );
    expect(screen.getByTestId("peer-help-state")).toHaveTextContent("Already answered by diagnose:abc: use B");
  });

  it("PeerHelpRow_should_showCancelledState", () => {
    renderRow(makePeer({ state: "cancelled", stateReason: "cancelled by the operator" }));
    expect(screen.getByTestId("peer-help-state")).toHaveTextContent("cancelled by the operator");
  });
});
