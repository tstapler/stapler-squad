import { AttentionReason } from "@/gen/session/v1/types_pb";

export type PromotedKind = "approval" | "error";

/** Arrivals that skip the "N new - show" hold: they need the operator immediately. */
export function promotedKind(reason: AttentionReason): PromotedKind | null {
  if (reason === AttentionReason.APPROVAL_PENDING) return "approval";
  if (reason === AttentionReason.ERROR_STATE) return "error";
  return null;
}

/** "1 new approval", "2 new errors", or "3 new urgent" when mixed. */
export function promotedLabel(reasons: AttentionReason[]): string {
  const kinds = new Set(reasons.map(promotedKind));
  const n = reasons.length;
  if (kinds.size === 1) {
    const noun = kinds.has("approval") ? "approval" : "error";
    return `${n} new ${noun}${n === 1 ? "" : "s"}`;
  }
  return `${n} new urgent`;
}
