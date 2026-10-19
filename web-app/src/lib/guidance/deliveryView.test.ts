import { ANNOUNCE } from "@/lib/copy/notificationCopy";
import { announceTransition, deliveryView, displayAnswer, snapshotOf, type ClientDelivery, type DeliverySnapshot } from "./deliveryView";

const base: DeliverySnapshot = {
  status: "pending",
  deliveryRoute: "",
  deliveryState: "",
  unacknowledged: false,
  sessionEnded: false,
  sessionArchived: false,
  cancelReason: "",
  answer: "",
};
const row = (over: Partial<DeliverySnapshot>) => ({ ...base, ...over });

// design/ux.md 3.1: the 17 settled-state strings, one per table row. Row 5's human_active variant is the 18th literal.
const TABLE: [number, string, DeliverySnapshot | null, ClientDelivery][] = [
  [1, "Sending...", row({ deliveryState: "pending", deliveryRoute: "live_keystroke" }), {}],
  [2, "Sent - waiting for the agent", row({ status: "answered", deliveryState: "sent", answer: "Redis" }), {}],
  [3, "Sent, but the agent hasn't confirmed yet. Open session.", row({ status: "answered", deliveryState: "sent", unacknowledged: true }), {}],
  [4, "Answered: Redis", row({ status: "answered", deliveryState: "acknowledged", answer: "Redis" }), {}],
  [5, "Not sent. Nothing was typed into the session. Retry", row({ deliveryState: "delivery_failed" }), {}],
  [6, "Sent, but we couldn't confirm it arrived. Check the session before retrying.", row({ status: "answered", deliveryState: "sent", unacknowledged: true }), { indeterminate: true }],
  [7, "Couldn't send your answer. Retry", null, { clientFailure: true }],
  [8, "Your answer is saved. The agent can read it once you resume the session.", row({ status: "answered", deliveryState: "saved_for_later", deliveryRoute: "resume" }), {}],
  [9, "This session is archived. Your answer will be saved but not delivered unless you resume it.", row({ status: "answered", deliveryState: "saved_for_later", deliveryRoute: "read_only" }), {}],
  [10, "Session ended before this was answered.", row({ status: "cancelled", cancelReason: "session_ended" }), {}],
  [11, "Already handled in the session. No action needed.", row({ status: "cancelled", cancelReason: "answered_in_terminal" }), {}],
  [12, "The session is now asking something else. Open session to answer it.", row({ status: "cancelled", cancelReason: "dialog_replaced" }), {}],
  [13, "Answered elsewhere: Redis", row({}), { elsewhere: "Redis" }],
  [14, "Part of your answer was sent, then the prompt changed. Open session to finish.", row({ status: "answered", deliveryState: "sent" }), { abortedPartial: true }],
  [15, "Answer from desktop", row({}), { desktopOnly: true }],
  [16, "Sent, but the question is still showing. Check the session before retrying.", row({ status: "answered", deliveryState: "delivery_failed" }), {}],
  [17, "Sent, but the session ended before confirming it.", row({ status: "answered", deliveryState: "sent", sessionEnded: true }), {}],
];

describe("deliveryView", () => {
  it.each(TABLE)("row %i reads %j", (rowNo, text, snap, client) => {
    const v = deliveryView(snap, client);
    expect(v?.row).toBe(rowNo);
    expect(v?.text).toBe(text);
  });

  it("covers every one of the 17 rows exactly once", () => {
    expect(new Set(TABLE.map(([n]) => n)).size).toBe(17);
  });

  it("uses the human_active wording for row 5 when someone is typing", () => {
    const v = deliveryView(null, { notSent: "human_active" });
    expect(v?.row).toBe(5);
    expect(v?.text).toBe("Not sent: someone is typing in that session. Retry when they are done.");
  });

  it("never gives row 16 the Nothing was typed string, and never row 17 the Session ended before this was answered string", () => {
    expect(deliveryView(row({ status: "answered", deliveryState: "delivery_failed" }))?.text).not.toMatch(/Nothing was typed/);
    expect(deliveryView(row({ status: "answered", deliveryState: "delivery_failed", sessionEnded: true }))?.text).not.toMatch(/before this was answered/);
  });

  it("keeps rows 3, 5, 6, 14 and 16 in Needs you, and no other row", () => {
    const needs = TABLE.filter(([, , snap, client]) => deliveryView(snap, client)?.needsYou).map(([n]) => n);
    expect(needs).toEqual([3, 5, 6, 14, 16]);
  });

  it("carries the Unconfirmed chip on rows 3 and 6 only", () => {
    const chipped = TABLE.filter(([, , snap, client]) => deliveryView(snap, client)?.chip === "Unconfirmed").map(([n]) => n);
    expect(chipped).toEqual([3, 6]);
  });

  it("offers Retry anyway (not a bare Retry) on rows 6 and 16, and Mark as handled only on row 3", () => {
    const actions = (n: number) => {
      const [, , snap, client] = TABLE.find(([r]) => r === n)!;
      return deliveryView(snap, client)?.actions;
    };
    expect(actions(6)).toEqual(["open", "retryAnyway"]);
    expect(actions(16)).toEqual(["open", "retryAnyway"]);
    expect(actions(3)).toEqual(["open", "markHandled"]);
    expect(actions(5)).toEqual(["retry", "open"]);
  });

  it("shows the archived copy before the operator acts, with the card still answerable (UX-AC27)", () => {
    const v = deliveryView(row({ sessionArchived: true }));
    expect(v?.row).toBe(9);
    expect(v?.locked).toBe(false);
  });

  it("shows no line for a fresh pending question", () => {
    expect(deliveryView(row({}))).toBeNull();
  });

  it("treats an applied answer whose row has not been re-read as row 2", () => {
    expect(deliveryView(row({}), { applied: true })?.row).toBe(2);
  });

  it("lets the row's own answered-elsewhere value win over a stale snapshot", () => {
    expect(deliveryView(row({ status: "answered", deliveryState: "acknowledged", answer: "x" }), { elsewhere: "No" })?.text).toBe("Answered elsewhere: No");
  });

  it("tolerates a snapshot from an older server", () => {
    expect(snapshotOf({ status: "answered", answer: "a" })?.sessionEnded).toBe(false);
    expect(snapshotOf(undefined)).toBeNull();
  });
});

describe("announceTransition", () => {
  const view = (snap: DeliverySnapshot | null, client: ClientDelivery = {}) => deliveryView(snap, client);

  it("says one distinct string per asynchronous transition", () => {
    expect(announceTransition(null, view(row({ deliveryState: "delivery_failed" })))).toBe("Answer not delivered");
    expect(announceTransition(2, view(row({ status: "answered", deliveryState: "delivery_failed" })))).toBe("Answer sent, question still showing");
    expect(announceTransition(2, view(row({ status: "answered", deliveryState: "sent", unacknowledged: true })))).toBe("Answer not confirmed");
    expect(announceTransition(null, view(null, { abortedPartial: true }))).toBe("Answer partly sent");
    expect(announceTransition(1, view(null, { applied: true }))).toBe(ANNOUNCE.answerSent);
  });

  it("says nothing for the same row twice, a late acknowledgement, a first render or a session ending", () => {
    const failed = view(row({ deliveryState: "delivery_failed" }));
    expect(announceTransition(5, failed)).toBeUndefined();
    expect(announceTransition(3, view(row({ status: "answered", deliveryState: "acknowledged", answer: "x" })))).toBeUndefined();
    expect(announceTransition(undefined, failed)).toBeUndefined();
    expect(announceTransition(3, view(row({ status: "answered", deliveryState: "sent", sessionEnded: true })))).toBeUndefined();
  });

  it("announces a card flipping to Already handled once (UX-AC26)", () => {
    expect(announceTransition(null, view(row({ status: "cancelled", cancelReason: "answered_in_terminal" })))).toBe("Already handled in the session. No action needed.");
  });
});

describe("multi-question answers", () => {
  it("renders a stored label-list sequence as one readable answer and leaves other text alone", () => {
    expect(displayAnswer('[["Redis"],["A","B"]]')).toBe("Redis; A, B");
    expect(displayAnswer("Redis")).toBe("Redis");
    expect(displayAnswer("[[not json")).toBe("[[not json");
  });

  it("pins an aborted-partial durable row in Needs you with only Open session and a confirmed Dismiss", () => {
    const v = deliveryView(row({ deliveryState: "aborted_partial" }));
    expect(v).toMatchObject({ row: 14, needsYou: true, locked: true, actions: ["open", "dismissConfirm"] });
  });
});
