import { Code, ConnectError, type Client } from "@connectrpc/connect";
import {
  FileAnalysisState, LineEnding, ReadOnlyReason, ReadOnlyReasonDetailSchema, type SessionService,
} from "@/gen/session/v1/session_pb";
import type { ReadOnlyKind } from "@/lib/files/bufferReducer";
import {
  Rejected, TransportFailure,
  type FileCapabilities, type FileStat, type FileTransport, type LineEndingKind, type ReadResult, type RejectReason,
} from "./fileTransport";

const MIB = 1024 * 1024;

/** Today's behavior: whole-file read over GetFileContent, StatFile, and a 1 MiB editable ceiling. */
const CAPABILITIES: FileCapabilities = {
  protocolVersion: 1,
  features: [],
  ops: ["stat", "read_all"],
  wires: ["connect_unary"],
  capsEpoch: 0,
  limits: {
    maxRangeBytes: 0, maxRangeLines: 0, preferredChunkBytes: 0, maxPatchOps: 0, maxPatchBodyBytes: 0,
    maxInsertBytes: 0, maxWriteFullBytes: 0, hashCapBytes: MIB, maxConcurrentRanges: 0,
  },
};

const READ_ONLY_KINDS: Partial<Record<ReadOnlyReason, ReadOnlyKind>> = {
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

/** Server reason, falling back to the legacy flags when an older server leaves it unspecified. */
function readOnlyOf(reason: ReadOnlyReason, isBinary: boolean, truncated: boolean): ReadOnlyKind {
  const known = READ_ONLY_KINDS[reason];
  if (known) return known;
  if (isBinary) return "binary";
  return truncated ? "truncated" : "none";
}

/** Typed refusals become `Rejected`; wire-level failures become `TransportFailure`; the rest pass through. */
function mapError(err: unknown): unknown {
  if (!(err instanceof ConnectError)) return err;
  switch (err.code) {
    case Code.NotFound:
      return new Rejected("not_found");
    case Code.Unavailable:
      return new TransportFailure("unavailable", err);
    case Code.DeadlineExceeded:
      return new TransportFailure("deadline", err);
    default: {
      const [detail] = err.findDetails(ReadOnlyReasonDetailSchema);
      const kind = detail && READ_ONLY_KINDS[detail.reason];
      return kind && kind !== "none" ? new Rejected(kind as RejectReason) : err;
    }
  }
}

async function guarded<T>(call: Promise<T>): Promise<T> {
  try {
    return await call;
  } catch (err) {
    throw mapError(err);
  }
}

type FileClient = Pick<Client<typeof SessionService>, "getFileContent" | "statFile">;

export function createFullFileTransport(client: FileClient): FileTransport {
  return {
    id: "full",
    capabilities: () => CAPABILITIES,
    async stat(ref, o, signal) {
      const r = await guarded(client.statFile({ sessionId: ref.sessionId, path: ref.path, wantAnalysis: o?.wantAnalysis }, { signal }));
      return {
        token: r.versionToken, size: Number(r.size), readOnly: readOnlyOf(r.readOnlyReason, false, false),
        analysis: ANALYSIS[r.analysis] ?? "unspecified",
      };
    },
    async read(ref, req, signal): Promise<ReadResult> {
      if (req.kind !== "all") throw new Rejected("unsupported", `${req.kind} reads need a higher transport`);
      const r = await guarded(client.getFileContent({ sessionId: ref.sessionId, path: ref.path }, { signal }));
      const lines = r.content === "" ? 1 : r.content.split("\n").length;
      return {
        text: r.content, token: r.versionToken, size: Number(r.size), lineEnding: LINE_ENDINGS[r.lineEnding] ?? "unknown",
        hasBom: r.hasBom, readOnly: readOnlyOf(r.readOnlyReason, r.isBinary, r.isTruncated), isBinary: r.isBinary,
        truncated: r.isTruncated,
        range: { startLine: 1, endLine: lines, startByte: 0, endByte: Number(r.size), bof: true, eof: !r.isTruncated },
      };
    },
    // The WriteFile RPC lands with Story 2.1.2; until then the floor cannot save.
    writeFull: () => Promise.reject(new Rejected("unsupported", "WriteFile is not available yet")),
  };
}
