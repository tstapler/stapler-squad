import { Code, ConnectError } from "@connectrpc/connect";
import { failure, type PrActionResult, type PrFailureKind } from "./actionResult";

const FIXED_COPY: Partial<Record<PrFailureKind, string>> = {
  not_attached: "This pull request is no longer attached to this session.",
  auth: "GitHub rejected the request (auth)",
  network: "Couldn't reach the server",
};

/** Maps a thrown RPC error (or a `success:false` message) to the failure kinds the dialogs word. */
export function classifyPrError(err: unknown): PrActionResult {
  const e = ConnectError.from(err);
  const text = e.rawMessage || e.message;
  const lower = text.toLowerCase();
  let kind: PrFailureKind = "other";
  if (e.code === Code.FailedPrecondition && lower.includes("not attached")) kind = "not_attached";
  else if (lower.includes("already merged")) kind = "already_merged";
  else if (e.code === Code.Unauthenticated || e.code === Code.PermissionDenied) kind = "auth";
  else if (e.code === Code.ResourceExhausted) kind = "rate_limited";
  else if (e.code === Code.Unavailable || e.code === Code.DeadlineExceeded) kind = "network";
  else if (e.code === Code.InvalidArgument) kind = "invalid_url";
  else if (e.code === Code.NotFound) kind = "not_found";
  return failure(kind, FIXED_COPY[kind] ?? text);
}

/** `success:false` responses carry their reason in `message`. */
export function classifyPrMessage(message: string): PrActionResult {
  return classifyPrError(new ConnectError(message || "The request failed"));
}
