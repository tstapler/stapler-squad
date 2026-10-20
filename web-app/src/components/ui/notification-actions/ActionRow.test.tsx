import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActionKind, ActionOutcome, ActionTier } from "@/gen/session/v1/notification_action_pb";
import {
  makeActionResponse,
  makeApprovalDescriptors,
  makeDecisionDescriptors,
  makeDescriptor,
  makeQuestionDescriptors,
} from "../__fixtures__/notificationFixtures";
import { invokeMock, renderRow, respondWith } from "../__fixtures__/actionRowHarness";
import { APPROVE_EXTRA_GAP_PX, TOUCH_TARGET_PX } from "./ActionRow";

jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));

const approvals = () => makeApprovalDescriptors();
const status = () => screen.getByTestId("action-status").textContent;

beforeEach(() => {
  invokeMock.mockReset();
  window.sessionStorage.clear();
});

describe("ActionRow descriptors", () => {
  it("renders buttons from the descriptor list, in Approve, Deny, Open order on desktop", () => {
    renderRow({ descriptors: approvals() });
    const labels = screen.getAllByRole("button").map((b) => b.textContent);
    expect(labels).toEqual(["Approve", "Deny", "Open session"]);
    expect(screen.getByTestId("action-approve")).toBeInTheDocument();
    expect(screen.getByTestId("action-deny")).toBeInTheDocument();
  });

  it("puts Deny before Approve with a 24px gap and 44px targets on a phone", () => {
    renderRow({ descriptors: approvals() }, { mobile: true });
    const [first, second] = screen.getAllByRole("button");
    expect(first).toHaveTextContent("Deny");
    expect(second).toHaveTextContent("Approve");
    expect(second.style.marginBlockStart).toBe(`${APPROVE_EXTRA_GAP_PX}px`);
    expect(APPROVE_EXTRA_GAP_PX + 8).toBeGreaterThanOrEqual(24);
    for (const b of screen.getAllByRole("button")) expect(b.style.minHeight).toBe(`${TOUCH_TARGET_PX}px`);
  });

  it("degrades an unknown kind to Open only", () => {
    renderRow({ descriptors: [makeDescriptor({ kind: 99 as ActionKind, label: "Mystery" })] });
    expect(screen.getAllByRole("button").map((b) => b.textContent)).toEqual(["Open session"]);
  });

  it("calls onOpen from the default Open button", async () => {
    const onOpen = jest.fn();
    renderRow({ descriptors: approvals(), onOpen });
    await userEvent.click(screen.getByTestId("action-open"));
    expect(onOpen).toHaveBeenCalledTimes(1);
  });
});

describe("ActionRow dispatch", () => {
  it("sends the descriptor's target, and a new idempotency key per attempt", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRow({ descriptors: approvals() });
    await userEvent.click(screen.getByTestId("action-approve"));
    await screen.findByText("Approved");
    expect(invokeMock).toHaveBeenCalledWith(
      expect.objectContaining({ actionKind: ActionKind.APPROVE, targetKey: "appr-1", idempotencyKey: expect.any(String) }),
    );
  });

  it("ignores a second press while the first is pending", async () => {
    let resolve!: (v: unknown) => void;
    invokeMock.mockReturnValue(new Promise((r) => (resolve = r)));
    renderRow({ descriptors: approvals() });
    const approve = screen.getByTestId("action-approve");
    await userEvent.click(approve);
    expect(screen.getByTestId("action-approve")).toHaveTextContent("Approving...");
    expect(screen.getByTestId("action-approve")).toHaveAttribute("aria-disabled", "true");
    await userEvent.click(screen.getByTestId("action-approve"));
    expect(invokeMock).toHaveBeenCalledTimes(1);
    await act(async () => resolve(makeActionResponse(ActionOutcome.APPLIED)));
  });

  it("reuses the key on a transport failure's Retry and shows the transient copy", async () => {
    invokeMock.mockRejectedValueOnce(new Error("network"));
    respondWith(ActionOutcome.APPLIED);
    invokeMock.mockRejectedValueOnce(new Error("network"));
    renderRow({ descriptors: approvals() });
    await userEvent.click(screen.getByTestId("action-approve"));
    await waitFor(() => expect(status()).toBe("Couldn't send your action. Retry"));
    await userEvent.click(screen.getByTestId("action-retry"));
    const keys = invokeMock.mock.calls.map((c) => c[0].idempotencyKey);
    expect(keys).toHaveLength(2);
    expect(keys[0]).toBe(keys[1]);
  });

  it("maps each answer option to its label and sends the session id", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRow({ descriptors: makeQuestionDescriptors() });
    await userEvent.click(screen.getByTestId("action-answer_question-1"));
    await waitFor(() =>
      expect(invokeMock).toHaveBeenCalledWith(
        expect.objectContaining({ actionKind: ActionKind.ANSWER_QUESTION, sessionId: "sess-1", params: { answer: "Memcached" } }),
      ),
    );
    expect(await screen.findByText("Sent - waiting for the agent")).toBeInTheDocument();
  });

  it("calls onApplied once for an APPLIED outcome only", async () => {
    const onApplied = jest.fn();
    respondWith(ActionOutcome.ALREADY_RESOLVED);
    renderRow({ descriptors: approvals(), onApplied });
    await userEvent.click(screen.getByTestId("action-approve"));
    await screen.findByText("Already handled");
    expect(onApplied).not.toHaveBeenCalled();
  });
});

describe("ActionRow outcomes", () => {
  it.each([
    [ActionOutcome.APPLIED, {}, "Approved"],
    [ActionOutcome.ALREADY_RESOLVED, { detail: "Redis" }, "Already handled: Redis"],
    [ActionOutcome.STALE, { staleReason: "dialog_gone" }, "Already handled in the session. No action needed."],
    [ActionOutcome.STALE, { staleReason: "dialog_replaced" }, "The session is now asking something else. Open session to answer it."],
    [ActionOutcome.STALE, { staleReason: "session_ended" }, "Session ended before this was answered."],
    [ActionOutcome.FLAG_OFF, {}, "This action is turned off. Open session."],
    [ActionOutcome.FORBIDDEN, { detail: "desktop_only" }, "Answer from desktop"],
  ])("outcome %s %j reads %j and has no alert", async (outcome, over, text) => {
    respondWith(outcome, over);
    renderRow({ descriptors: approvals() });
    await userEvent.click(screen.getByTestId("action-approve"));
    await waitFor(() => expect(status()).toBe(text));
    expect(screen.getByTestId("announcer-assertive")).toBeEmptyDOMElement();
    expect(screen.getByTestId("action-open")).toBeInTheDocument();
  });

  it("shows the rule name for an auto-handled approval's STALE", async () => {
    respondWith(ActionOutcome.STALE);
    renderRow({ descriptors: approvals(), approval: { ruleName: "allow-git" } });
    await userEvent.click(screen.getByTestId("action-approve"));
    await waitFor(() => expect(status()).toBe("Handled automatically by rule allow-git"));
  });

  it("NOT_SENT offers a direct Retry with Open session", async () => {
    respondWith(ActionOutcome.NOT_SENT);
    renderRow({ descriptors: makeQuestionDescriptors() });
    await userEvent.click(screen.getByTestId("action-answer_question-0"));
    await waitFor(() => expect(status()).toBe("Not sent. Nothing was typed into the session. Retry"));
    expect(screen.getByTestId("action-retry")).toBeInTheDocument();
    expect(screen.getByTestId("action-open")).toBeInTheDocument();
    const firstKey = invokeMock.mock.calls[0][0].idempotencyKey;
    await userEvent.click(screen.getByTestId("action-retry"));
    expect(invokeMock.mock.calls[1][0].idempotencyKey).toBe(firstKey);
  });

  it("NOT_SENT for a typing human uses its own copy", async () => {
    respondWith(ActionOutcome.NOT_SENT, { detail: "human_active" });
    renderRow({ descriptors: makeQuestionDescriptors() });
    await userEvent.click(screen.getByTestId("action-answer_question-0"));
    await waitFor(() => expect(status()).toBe("Not sent: someone is typing in that session. Retry when they are done."));
  });

  it("SEND_INDETERMINATE hides Retry behind a Retry anyway? confirm and never reads as answered", async () => {
    respondWith(ActionOutcome.SEND_INDETERMINATE);
    renderRow({ descriptors: makeQuestionDescriptors() });
    await userEvent.click(screen.getByTestId("action-answer_question-0"));
    await waitFor(() => expect(status()).toBe("Sent, but we couldn't confirm it arrived. Check the session before retrying."));
    expect(screen.queryByTestId("action-retry")).toBeNull();
    expect(screen.queryByText(/^Answered/)).toBeNull();
    const buttons = screen.getAllByRole("button").map((b) => b.textContent);
    expect(buttons[0]).toBe("Open session");
    await userEvent.click(screen.getByTestId("action-retry-anyway"));
    expect(screen.getByText("Retry anyway?")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
    respondWith(ActionOutcome.APPLIED);
    await userEvent.click(screen.getByTestId("action-retry-confirm"));
    await waitFor(() => expect(invokeMock).toHaveBeenCalledTimes(2));
    expect(invokeMock.mock.calls[1][0].idempotencyKey).not.toBe(invokeMock.mock.calls[0][0].idempotencyKey);
  });

  it("INVALID shows Refresh, which asks the caller to refetch descriptors once", async () => {
    respondWith(ActionOutcome.INVALID);
    const onRefresh = jest.fn();
    renderRow({ descriptors: approvals(), onRefresh });
    await userEvent.click(screen.getByTestId("action-approve"));
    await waitFor(() => expect(status()).toBe("That action isn't valid anymore. Refresh."));
    await userEvent.click(screen.getByTestId("action-refresh"));
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });
});

describe("ActionRow RATE_LIMITED", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("counts down in text and re-enables at zero", async () => {
    respondWith(ActionOutcome.RATE_LIMITED, { retryAfterSeconds: 3 });
    renderRow({ descriptors: approvals() });
    fireEvent.click(screen.getByTestId("action-approve"));
    await act(async () => {});
    expect(status()).toBe("Too many actions. Try again in 3s.");
    expect(screen.getByTestId("action-approve")).toHaveAttribute("aria-disabled", "true");
    for (const left of [2, 1]) {
      act(() => void jest.advanceTimersByTime(1_000));
      expect(status()).toBe(`Too many actions. Try again in ${left}s.`);
    }
    act(() => void jest.advanceTimersByTime(1_000));
    expect(screen.queryByTestId("action-status")).toBeNull();
    expect(screen.getByTestId("action-approve")).not.toHaveAttribute("aria-disabled");
  });
});

describe("ActionRow inline sub-steps", () => {
  it("Deny opens a reason field with no dialog; Back keeps the draft and restores the buttons", async () => {
    renderRow({ descriptors: approvals() });
    await userEvent.click(screen.getByTestId("action-deny"));
    const field = screen.getByTestId("deny-reason");
    expect(field).toHaveFocus();
    expect(screen.queryByRole("dialog")).toBeNull();
    await userEvent.type(field, "too risky");
    await userEvent.click(screen.getByTestId("action-back"));
    expect(screen.getByTestId("action-approve")).toBeInTheDocument();
    await userEvent.click(screen.getByTestId("action-deny"));
    expect(screen.getByTestId("deny-reason")).toHaveValue("too risky");
  });

  it("sends the denial from the keyboard's Enter, with the single-line field attributes", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRow({ descriptors: approvals() });
    await userEvent.click(screen.getByTestId("action-deny"));
    const field = screen.getByTestId("deny-reason");
    expect(field).toHaveAttribute("enterkeyhint", "send");
    expect(field).toHaveAttribute("autocapitalize", "sentences");
    expect(field).toHaveAttribute("autocomplete", "off");
    await userEvent.type(field, "nope{Enter}");
    await screen.findByText("Denied");
    expect(invokeMock).toHaveBeenCalledTimes(1);
  });

  it("Send denial dispatches DENY with the reason", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRow({ descriptors: approvals() });
    await userEvent.click(screen.getByTestId("action-deny"));
    await userEvent.type(screen.getByTestId("deny-reason"), "no");
    await userEvent.click(screen.getByTestId("action-send-denial"));
    await screen.findByText("Denied");
    expect(invokeMock).toHaveBeenCalledWith(expect.objectContaining({ actionKind: ActionKind.DENY, params: { reason: "no" } }));
  });

  it("Merge asks for confirmation with focus on Confirm merge before sending", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRow({ descriptors: makeDecisionDescriptors(), prNumber: "901" });
    await userEvent.click(screen.getByTestId("action-backlog_decision-merge"));
    expect(screen.getByText("Merge PR #901?")).toBeInTheDocument();
    expect(screen.getByText("Confirm merge")).toHaveFocus();
    expect(invokeMock).not.toHaveBeenCalled();
    await userEvent.click(screen.getByText("Confirm merge"));
    await screen.findByText("Decided: Merge");
    expect(invokeMock).toHaveBeenCalledWith(
      expect.objectContaining({ actionKind: ActionKind.BACKLOG_DECISION, params: { choice: "merge", confirm: true } }),
    );
  });

  it("sends a non-merge decision with one press", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRow({ descriptors: makeDecisionDescriptors() });
    await userEvent.click(screen.getByTestId("action-backlog_decision-rework"));
    await screen.findByText("Decided: Send back");
  });

  it("Escape closes only the sub-step and never reaches the panel", async () => {
    const panelEsc = jest.fn();
    document.addEventListener("keydown", panelEsc);
    renderRow({ descriptors: approvals() });
    await userEvent.click(screen.getByTestId("action-deny"));
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByTestId("deny-step")).toBeNull();
    expect(panelEsc).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByTestId("action-deny")).toHaveFocus());
    document.removeEventListener("keydown", panelEsc);
  });

  it("asks before cancelling a prompt and uses Keep, never a bare Cancel", async () => {
    const d = makeDescriptor({ kind: ActionKind.CANCEL_WAKE, label: "Cancel prompt", targetKey: "w1" });
    renderRow({ descriptors: [d] });
    await userEvent.click(screen.getByText("Cancel prompt"));
    expect(screen.getByText("Cancel this prompt?")).toBeInTheDocument();
    expect(screen.getByText("Keep")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Cancel$/ })).toBeNull();
  });
});

describe("ActionRow availability and connectivity", () => {
  it("renders a desktop-only descriptor aria-disabled with its reason, beside an enabled Open", async () => {
    const descriptors = makeQuestionDescriptors({ available: false, unavailableReason: "desktop_only", tier: ActionTier.B });
    renderRow({ descriptors, onOpen: jest.fn() });
    const option = screen.getByTestId("action-answer_question-0");
    expect(option).toHaveAttribute("aria-disabled", "true");
    expect(option).not.toBeDisabled();
    const described = option.getAttribute("aria-describedby");
    expect(document.getElementById(described!)).toHaveTextContent("Answer from desktop");
    await userEvent.click(option);
    expect(invokeMock).not.toHaveBeenCalled();
    expect(screen.getByTestId("action-open")).not.toHaveAttribute("aria-disabled");
  });

  it("keeps a Tier A approval enabled alongside a Tier B one that is not", () => {
    renderRow({ descriptors: [...makeApprovalDescriptors(), ...makeQuestionDescriptors({ available: false, unavailableReason: "desktop_only" })] });
    expect(screen.getByTestId("action-approve")).not.toHaveAttribute("aria-disabled");
  });

  it("disables every action with the offline reason and keeps them focusable", async () => {
    renderRow({ descriptors: approvals(), disabledReason: "Offline" });
    const approve = screen.getByTestId("action-approve");
    expect(approve).toHaveAttribute("aria-disabled", "true");
    expect(document.getElementById(approve.getAttribute("aria-describedby")!)).toHaveTextContent("Offline");
    approve.focus();
    expect(approve).toHaveFocus();
    await userEvent.click(approve);
    expect(invokeMock).not.toHaveBeenCalled();
  });
});

describe("ActionRow polite region", () => {
  it("announces the short result once through the shared status region", async () => {
    respondWith(ActionOutcome.APPLIED);
    renderRow({ descriptors: approvals() });
    await userEvent.click(screen.getByTestId("action-approve"));
    await waitFor(() => expect(screen.getByTestId("announcer-polite")).toHaveTextContent("Approved"));
    expect(document.querySelectorAll("[role=status]")).toHaveLength(1);
  });
});
