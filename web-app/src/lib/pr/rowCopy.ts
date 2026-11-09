import { timestampMs } from "@bufbuild/protobuf/wkt";
import { PollError, PrHold_Kind, type PrHold, type SessionPullRequest } from "@/gen/session/v1/types_pb";
import { prRowState } from "./pullRequests";

/** "just now", "5m ago", "3h ago", "2d ago". */
export function ageShort(thenMs: number, nowMs: number): string {
  const s = Math.max(0, Math.floor((nowMs - thenMs) / 1000));
  if (s < 60) return "just now";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  return h < 24 ? `${h}h ago` : `${Math.floor(h / 24)}d ago`;
}

function whenMs(pr: SessionPullRequest): number | undefined {
  const ts = pr.mergedAt ?? pr.closedAt;
  return ts ? timestampMs(ts) : undefined;
}

/** The state words beside a row's glyph. */
export function stateText(pr: SessionPullRequest, nowMs: number): string {
  const state = prRowState(pr);
  const at = whenMs(pr);
  const age = at === undefined ? "" : ` ${ageShort(at, nowMs).replace(" ago", "")}`;
  switch (state) {
    case "open": return "Open";
    case "draft": return "Draft";
    case "merged": return `Merged${age}`;
    case "closed": return `Closed${age}`;
    case "unverified": return "Closed or merged (unverified)";
    default: return pr.lastPolledAt ? "State unknown" : "Checking...";
  }
}

type VerifyFields = Pick<SessionPullRequest, "verifyAttempts" | "verifyAttemptLimit" | "verifyExhausted">;

/**
 * Second line under an unverified terminal row. The two strings never show together:
 * counting ("Checking... (attempt 2 of 5)") and exhausted ("Couldn't verify ..."). The server owns both numbers.
 */
export function unverifiedLine(pr: VerifyFields | undefined): string {
  if (pr?.verifyExhausted) return `Couldn't verify after ${pr.verifyAttemptLimit} attempts. Treated as closed.`;
  if (pr && pr.verifyAttempts > 0 && pr.verifyAttemptLimit > 0) return `Checking... (attempt ${pr.verifyAttempts} of ${pr.verifyAttemptLimit})`;
  return "Checking...";
}

function clock(ms: number): string {
  return new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
}

/** Inline chip for a failed poll; null when the last poll was fine. */
export function pollErrorText(pr: SessionPullRequest, nowMs: number): string | null {
  switch (pr.pollError) {
    case PollError.AUTH: return "Can't refresh: GitHub auth failed";
    case PollError.RATE_LIMITED:
      return pr.rateLimitResetAt ? `Rate limited until ${clock(timestampMs(pr.rateLimitResetAt))}` : "Rate limited";
    case PollError.NOT_FOUND:
    case PollError.REPO_INACCESSIBLE: {
      const seen = pr.lastPolledAt ? ` (last seen ${ageShort(timestampMs(pr.lastPolledAt), nowMs)})` : "";
      return `Can't reach this PR on GitHub${seen}`;
    }
    case PollError.NETWORK: return "Can't refresh";
    default: return null;
  }
}

export type HoldSurface = "session" | "item";

/**
 * Plain-wording line explaining why a hold keeps things. Never says "worktree".
 * The server decides holds; this only words them.
 */
export function holdLine(hold: PrHold, pr: SessionPullRequest | undefined, surface: HoldSurface, nowMs: number): string {
  const n = hold.pr?.number ?? pr?.number ?? 0;
  const ref = `#${n}`;
  switch (hold.kind) {
    case PrHold_Kind.OPEN_PR:
      return `Local files kept: ${ref} is still open. They are removed after ${ref} is merged or closed.`;
    case PrHold_Kind.UNVERIFIED:
      return `Local files kept until ${ref} is confirmed closed or merged. ${unverifiedLine(pr)}`;
    case PrHold_Kind.UNKNOWN: {
      const seen = pr?.lastPolledAt ? ` (last seen ${ageShort(timestampMs(pr.lastPolledAt), nowMs)})` : "";
      return `Local files kept: GitHub can't be reached for ${ref}${seen}.`;
    }
    case PrHold_Kind.SUPERSEDED_ROUND:
      return `Kept for ${ref} from an earlier round; it is still open.`;
    case PrHold_Kind.SESSION_ACTIVE: {
      if (surface === "session") return "Merged. The session is still active, so the item isn't done yet.";
      const minutes = Math.round(hold.idleWindowSeconds / 60);
      const last = hold.lastActivityAt ? `last activity ${ageShort(timestampMs(hold.lastActivityAt), nowMs)}; ` : "";
      return `${ref} merged. Waiting for the session to go idle (${last}the item completes after ${minutes} minutes idle or when the session ends).`;
    }
    case PrHold_Kind.MAX_HOLD:
      return `The session was still active 1 hour after ${ref} merged, so the item was marked done. The session keeps running, and its local files are kept, until it goes idle or you end it.`;
    default:
      return "";
  }
}

export function isWarningHold(hold: PrHold): boolean {
  return hold.kind === PrHold_Kind.MAX_HOLD;
}

/** CI/review summary for non-terminal rows. */
export function reviewSummary(pr: SessionPullRequest): string[] {
  const out: string[] = [];
  if (pr.checkConclusion === "success") out.push("CI passing");
  else if (pr.checkConclusion === "failure") out.push("CI failing");
  else if (pr.checkConclusion) out.push("CI pending");
  if (pr.approvedCount > 0) out.push(`${pr.approvedCount} approval${pr.approvedCount > 1 ? "s" : ""}`);
  if (pr.changesRequestedCount > 0) out.push("changes requested");
  return out;
}
