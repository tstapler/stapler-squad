// Proto-to-port mapping shared by the `full` and `connect_unary` adapters.
import { Code, ConnectError, type Client } from "@connectrpc/connect";
import {
  FileAnalysisState, LineEnding, RangeBusyDetailSchema, ReadOnlyReason, ReadOnlyReasonDetailSchema, type SessionService,
} from "@/gen/session/v1/session_pb";
import type { ReadOnlyKind } from "@/lib/files/bufferReducer";
import { Rejected, TransportFailure, type FileRef, type FileStat, type LineEndingKind, type RejectReason } from "./fileTransport";

export const READ_ONLY_KINDS: Partial<Record<ReadOnlyReason, ReadOnlyKind>> = {
  [ReadOnlyReason.NONE]: "none",
  [ReadOnlyReason.FLAG_OFF]: "flag_off",
  [ReadOnlyReason.BINARY]: "binary",
  [ReadOnlyReason.TRUNCATED]: "truncated",
  [ReadOnlyReason.TOO_LARGE]: "too_large",
  [ReadOnlyReason.NON_UTF8]: "non_utf8",
  [ReadOnlyReason.MIXED_EOL]: "mixed_eol",
  [ReadOnlyReason.SYMLINK]: "symlink",
  [ReadOnlyReason.WORKTREE_MISSING]: "worktree_missing",
  [ReadOnlyReason.HIDDEN_SESSION]: "hidden_session",
  [ReadOnlyReason.GIT_INTERNAL]: "git_internal",
};

const LINE_ENDINGS: Partial<Record<LineEnding, LineEndingKind>> = {
  [LineEnding.LF]: "lf", [LineEnding.CRLF]: "crlf", [LineEnding.MIXED]: "mixed",
};

const ANALYSIS: Partial<Record<FileAnalysisState, FileStat["analysis"]>> = {
  [FileAnalysisState.PENDING]: "pending", [FileAnalysisState.READY]: "ready", [FileAnalysisState.FAILED]: "failed",
};

export const lineEndingOf = (le: LineEnding): LineEndingKind => LINE_ENDINGS[le] ?? "unknown";
export const analysisOf = (a: FileAnalysisState): FileStat["analysis"] => ANALYSIS[a] ?? "unspecified";

/** Server reason, falling back to the legacy flags when an older server leaves it unspecified. */
export function readOnlyOf(reason: ReadOnlyReason, isBinary: boolean, truncated: boolean): ReadOnlyKind {
  const known = READ_ONLY_KINDS[reason];
  if (known) return known;
  if (isBinary) return "binary";
  return truncated ? "truncated" : "none";
}

// The server sends no typed detail for "index still building"; this is its message (file_ops_range.go lineRangeWindow).
const INDEX_PENDING = /line index/i;

// fetch() rejects with a TypeError worded per browser (Chrome, Firefox, Safari); other TypeErrors are bugs.
const FETCH_FAILURE = /failed to fetch|networkerror|load failed|network request failed|^network error$|^fetch failed$/i;
const isFetchFailure = (e: unknown): boolean => e instanceof TypeError && FETCH_FAILURE.test(e.message);

/** Typed refusals become `Rejected`; wire-level failures become `TransportFailure`; the rest pass through. */
export function mapError(err: unknown): unknown {
  if (isFetchFailure(err)) return new TransportFailure("network", err);
  if (!(err instanceof ConnectError)) return err;
  if (err.code === Code.Unknown && isFetchFailure(err.cause)) return new TransportFailure("network", err);
  switch (err.code) {
    case Code.NotFound:
      return new Rejected("not_found");
    case Code.Unavailable:
      return new TransportFailure("unavailable", err);
    case Code.DeadlineExceeded:
      return new TransportFailure("deadline", err);
    case Code.ResourceExhausted:
      return err.findDetails(RangeBusyDetailSchema).length > 0 ? new Rejected("busy") : err;
  }
  const [detail] = err.findDetails(ReadOnlyReasonDetailSchema);
  const kind = detail && READ_ONLY_KINDS[detail.reason];
  if (kind && kind !== "none") return new Rejected(kind as RejectReason);
  if (err.code === Code.FailedPrecondition) return new Rejected(INDEX_PENDING.test(err.rawMessage) ? "index_pending" : "unsupported", err.rawMessage);
  return err;
}

export async function guarded<T>(call: Promise<T>): Promise<T> {
  try {
    return await call;
  } catch (err) {
    throw mapError(err);
  }
}

export async function statVia(
  client: Pick<Client<typeof SessionService>, "statFile">, ref: FileRef,
  o: { wantAnalysis?: boolean; wantSha256?: boolean } | undefined, signal: AbortSignal | undefined,
): Promise<FileStat> {
  const r = await guarded(client.statFile({ sessionId: ref.sessionId, path: ref.path, wantAnalysis: o?.wantAnalysis, wantSha256: o?.wantSha256 }, { signal }));
  return { token: r.versionToken, size: Number(r.size), readOnly: readOnlyOf(r.readOnlyReason, false, false), analysis: analysisOf(r.analysis) };
}
