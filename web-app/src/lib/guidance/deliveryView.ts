import { ANNOUNCE, COPY, DELIVERY_COPY, answered, answeredElsewhere } from "@/lib/copy/notificationCopy";

/**
 * The delivery-state line of a question card (design/ux.md 3.1, UX-AC23, UX-AC24). One pure
 * function decides which of the 17 settled-state rows a card shows, from the durable row the
 * server sends and the few facts only this browser knows. It is the single place that maps a
 * delivery state to copy, actions and Needs-you retention.
 */

/** The fields of a `session.v1.GuidanceRequest` the line depends on. */
export interface DeliverySnapshot {
  /** "pending" | "answered" | "cancelled". */
  status: string;
  deliveryRoute: string;
  deliveryState: string;
  unacknowledged: boolean;
  sessionEnded: boolean;
  /** The session is archived: a stored answer will not be delivered (UX-AC27). */
  sessionArchived: boolean;
  cancelReason: string;
  answer: string;
}

/** What only this browser knows: the in-flight request and the last typed outcome. */
export interface ClientDelivery {
  sending?: boolean;
  clientFailure?: boolean;
  notSent?: "plain" | "human_active";
  indeterminate?: boolean;
  /** The answer landed (APPLIED) but the durable row has not been re-read yet. */
  applied?: boolean;
  /** Answered in another tab or channel; the value that won. */
  elsewhere?: string;
  desktopOnly?: boolean;
  abortedPartial?: boolean;
  staleReason?: "dialog_gone" | "dialog_replaced" | "session_ended";
}

export type DeliveryAction = "open" | "retry" | "retryAnyway" | "markHandled" | "dismiss" | "dismissConfirm";

export interface DeliveryView {
  /** Row number in the section 3.1 table (1-17). */
  row: number;
  text: string;
  /** "Unconfirmed" on rows 3 and 6. */
  chip?: string;
  actions: readonly DeliveryAction[];
  /** The card stays in Needs you and counts in the bell badge (rows 3, 5, 6, 14, 16). */
  needsYou: boolean;
  /** The card can no longer take an answer. */
  locked: boolean;
}

const view = (row: number, text: string, actions: readonly DeliveryAction[], extra: Partial<DeliveryView> = {}): DeliveryView => ({
  row,
  text,
  actions,
  needsYou: false,
  locked: true,
  ...extra,
});

const OPEN: readonly DeliveryAction[] = ["open"];

function unconfirmed(indeterminate: boolean): DeliveryView {
  return indeterminate
    ? view(6, COPY.indeterminate, ["open", "retryAnyway"], { chip: DELIVERY_COPY.unconfirmedChip, needsYou: true })
    : view(3, DELIVERY_COPY.unconfirmed, ["open", "markHandled"], { chip: DELIVERY_COPY.unconfirmedChip, needsYou: true });
}

function cancelled(snap: DeliverySnapshot): DeliveryView {
  if (snap.cancelReason === "dialog_replaced") return view(12, COPY.staleReplaced, OPEN);
  if (snap.cancelReason === "answered_in_terminal" || snap.cancelReason === "dialog_gone") return view(11, COPY.staleGone, OPEN);
  return view(10, COPY.sessionEnded, ["open", "dismiss"]);
}

/** A multi-question answer is stored as a JSON list of label lists; show it as "Redis; A, B". */
export function displayAnswer(answer: string): string {
  if (!answer.startsWith("[[")) return answer;
  try {
    const picks: unknown = JSON.parse(answer);
    if (Array.isArray(picks) && picks.every((p) => Array.isArray(p) && p.every((l) => typeof l === "string"))) {
      return (picks as string[][]).map((p) => p.join(", ")).join("; ");
    }
  } catch {
    // not a sequence: show it as typed
  }
  return answer;
}

function answeredRow(snap: DeliverySnapshot, client: ClientDelivery): DeliveryView | null {
  switch (snap.deliveryState) {
    case "acknowledged":
      return view(4, answered(displayAnswer(snap.answer)), []);
    case "sent":
      if (snap.sessionEnded) return view(17, DELIVERY_COPY.sentSessionEnded, OPEN);
      return snap.unacknowledged || client.indeterminate ? unconfirmed(!!client.indeterminate) : view(2, COPY.sentWaiting, []);
    case "delivery_failed":
      return snap.sessionEnded ? view(17, DELIVERY_COPY.sentSessionEnded, OPEN) : view(16, DELIVERY_COPY.stillShowing, ["open", "retryAnyway"], { needsYou: true });
    case "saved_for_later":
      return snap.deliveryRoute === "read_only" ? view(9, DELIVERY_COPY.savedArchived, OPEN) : view(8, DELIVERY_COPY.savedStopped, OPEN);
    default:
      return view(4, answered(displayAnswer(snap.answer)), []);
  }
}

const abortedPartial = () => view(14, DELIVERY_COPY.aborted, ["open", "dismissConfirm"], { needsYou: true });

function pendingRow(snap: DeliverySnapshot, client: ClientDelivery): DeliveryView | null {
  if (snap.deliveryState === "aborted_partial") return abortedPartial(); // survives a reload: the durable row says so
  // UX-AC27: the archived copy shows before the operator acts, and the card stays answerable.
  if (snap.sessionArchived && !snap.deliveryState) return view(9, DELIVERY_COPY.savedArchived, OPEN, { locked: false });
  if (snap.deliveryState === "pending") return view(1, DELIVERY_COPY.sending, [], { locked: false });
  if (snap.deliveryState === "delivery_failed") return notSent(client);
  if (snap.deliveryState === "stale") return client.staleReason === "dialog_replaced" ? view(12, COPY.staleReplaced, OPEN) : view(11, COPY.staleGone, OPEN);
  return null;
}

const notSent = (client: ClientDelivery) =>
  view(5, client.notSent === "human_active" ? COPY.notSentHumanActive : COPY.notSent, ["retry", "open"], { needsYou: true, locked: false });

function clientRow(client: ClientDelivery): DeliveryView | null {
  if (client.sending) return view(1, DELIVERY_COPY.sending, [], { locked: false });
  if (client.clientFailure) return view(7, DELIVERY_COPY.clientFailure, ["retry", "dismiss"], { locked: false });
  if (client.notSent) return notSent(client);
  if (client.indeterminate) return unconfirmed(true);
  if (client.staleReason === "session_ended") return view(10, COPY.sessionEnded, ["open", "dismiss"]);
  if (client.staleReason === "dialog_replaced") return view(12, COPY.staleReplaced, OPEN);
  if (client.staleReason === "dialog_gone") return view(11, COPY.staleGone, OPEN);
  if (client.applied) return view(2, COPY.sentWaiting, []);
  return null;
}

/**
 * The line for a card, or null when nothing was sent and nothing is wrong ("Delivery: (none
 * until sent)"). Precedence: facts about another tab or the listener, then the durable row,
 * then this browser's in-flight state.
 */
export function deliveryView(snap: DeliverySnapshot | null, client: ClientDelivery = {}): DeliveryView | null {
  if (client.elsewhere !== undefined) return view(13, answeredElsewhere(client.elsewhere), []);
  if (client.abortedPartial) return abortedPartial();
  if (client.desktopOnly) return view(15, COPY.desktopOnly, OPEN, { locked: false });
  if (client.sending) return clientRow(client);
  if (!snap) return clientRow(client);
  if (snap.status === "cancelled") return cancelled(snap);
  if (snap.status === "answered") return answeredRow(snap, client);
  return pendingRow(snap, client) ?? clientRow(client);
}

/**
 * The one polite announcement for a card moving from one row to another, or undefined
 * (design/ux.md 11.1). `prevRow` is undefined on a card's first render (nothing to announce) and
 * null while it showed no line. A late acknowledgement and a session ending on a sent answer are silent.
 */
export function announceTransition(prevRow: number | null | undefined, next: DeliveryView | null): string | undefined {
  if (prevRow === undefined || !next || prevRow === next.row) return undefined;
  switch (next.row) {
    case 2:
      return ANNOUNCE.answerSent;
    case 5:
      return ANNOUNCE.notDelivered;
    case 16:
      return ANNOUNCE.stillShowing;
    case 3:
    case 6:
      return ANNOUNCE.notConfirmed;
    case 14:
      return ANNOUNCE.partlySent;
    case 11:
      return ANNOUNCE.handledInSession; // UX-AC26: flipped by a terminal answer or a vanished dialog
    default:
      return undefined;
  }
}

/** A snapshot from the plain wire object, tolerating fields an older server omits. */
export function snapshotOf(row: Partial<DeliverySnapshot> | undefined | null): DeliverySnapshot | null {
  if (!row) return null;
  return {
    status: row.status ?? "pending",
    deliveryRoute: row.deliveryRoute ?? "",
    deliveryState: row.deliveryState ?? "",
    unacknowledged: row.unacknowledged ?? false,
    sessionEnded: row.sessionEnded ?? false,
    sessionArchived: row.sessionArchived ?? false,
    cancelReason: row.cancelReason ?? "",
    answer: row.answer ?? "",
  };
}
