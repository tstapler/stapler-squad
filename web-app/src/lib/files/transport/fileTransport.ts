// Client port for file I/O (ADR-036). Hooks depend on this, never on the generated Connect client,
// so range reads, deltas and extra wires are adapters behind the router.
import type { ReadOnlyKind } from "@/lib/files/bufferReducer";

export type TransportId = "full" | "connect_unary" | "connect_stream" | "ws_stream" | "http_range";
/** Router-level operations; `read` covers the all/lines/bytes request kinds. */
export type FileOpName = "read" | "patch" | "stat" | "write_full";
/** Server-advertised operations (mirrors the FileOp proto enum). */
export type AdvertisedOp = "stat" | "read_all" | "read_lines" | "read_bytes" | "read_stream" | "patch" | "write_full";
export type FallbackReason = "network" | "unavailable" | "deadline" | "malformed" | "capability" | "version";

export interface FileRef { sessionId: string; path: string }

export interface FileLimits {
  maxRangeBytes: number;
  maxRangeLines: number;
  preferredChunkBytes: number;
  maxPatchOps: number;
  maxPatchBodyBytes: number;
  maxInsertBytes: number;
  maxWriteFullBytes: number;
  /** Editable ceiling: files above it get the "-" token. */
  hashCapBytes: number;
  maxConcurrentRanges: number;
}

export interface FileCapabilities {
  protocolVersion: number;
  features: readonly string[];
  ops: readonly AdvertisedOp[];
  wires: readonly TransportId[];
  limits: FileLimits;
  capsEpoch: number;
}

export type ReadRequest =
  | { kind: "all"; maxBytes?: number }
  | { kind: "lines"; anchorLine: number; dir: "around" | "fwd" | "back"; wantLines: number; maxBytes: number }
  | { kind: "bytes"; offset: number; length: number; dir?: "fwd" | "back" };

export interface ServedRange { startLine: number; endLine: number; startByte: number; endByte: number; bof: boolean; eof: boolean }

export type LineEndingKind = "lf" | "crlf" | "mixed" | "unknown";

export interface ReadResult {
  text: string;
  /** Optimistic-concurrency validator; "-" hash segment when the file is not editable. */
  token: string;
  size: number;
  lineEnding: LineEndingKind;
  hasBom: boolean;
  readOnly: ReadOnlyKind;
  isBinary: boolean;
  truncated: boolean;
  range: ServedRange;
  /** Present when the response carries the server's caps epoch (a mismatch makes the router refetch). */
  capsEpoch?: number;
  chunkCrc32c?: number;
  /** "<size>:<mtime_ns>:<inode>" the window was read from; a change means the file moved under the reader. */
  validator?: string;
  /** Exact total only when the server's whole-file analysis is ready. */
  lineCount?: number;
  /** The window ends mid-line (byte cap or byte-window mode). */
  partialLastLine?: boolean;
  /** A line over 256 KiB put the region in read-only byte-window mode. */
  longLine?: boolean;
}

export interface FileStat {
  token: string;
  size: number;
  readOnly: ReadOnlyKind;
  analysis: "unspecified" | "pending" | "ready" | "failed";
  capsEpoch?: number;
}

export interface WriteRequest { text: string; baseToken: string }
export interface WriteResult { token: string; capsEpoch?: number }
export interface PatchRequest { baseToken: string; edits: readonly { startByte: number; endByte: number; insert: string }[] }
export interface PatchResult { token: string; capsEpoch?: number }

export interface FileTransport {
  readonly id: TransportId;
  capabilities(): FileCapabilities;
  stat(ref: FileRef, o?: { wantAnalysis?: boolean; wantSha256?: boolean }, signal?: AbortSignal): Promise<FileStat>;
  read(ref: FileRef, req: ReadRequest, signal?: AbortSignal): Promise<ReadResult>;
  readStream?(ref: FileRef, req: ReadRequest, signal?: AbortSignal): AsyncIterable<ReadResult>;
  patch?(ref: FileRef, p: PatchRequest, signal?: AbortSignal): Promise<PatchResult>;
  writeFull(ref: FileRef, w: WriteRequest, signal?: AbortSignal): Promise<WriteResult>;
}

/** What hooks consume: a single adapter or the router (which has no single id). */
export type FileTransportPort = Omit<FileTransport, "id">;

/** `busy`: range limiter refusal; `index_pending`: line index still building (read from line 1 or retry); `stale`: window torn twice. */
export type RejectReason = Exclude<ReadOnlyKind, "none"> | "not_found" | "unsupported" | "busy" | "index_pending" | "stale";

/** The server (or the active rung) refused with a reason the UI can state; never triggers demotion. */
export class Rejected extends Error {
  constructor(readonly reason: RejectReason, message?: string) {
    super(message ?? `rejected: ${reason}`);
    this.name = "Rejected";
  }
}

/** Transport-class failure (network, Unavailable, deadline, malformed envelope): counts toward demotion. */
export class TransportFailure extends Error {
  constructor(readonly reason: Extract<FallbackReason, "network" | "unavailable" | "deadline" | "malformed">, cause?: unknown) {
    super(`transport failure: ${reason}`, { cause });
    this.name = "TransportFailure";
  }
}

/** Highest rung first. `http_range` is for byte-only consumers and never takes part in the ladder. */
export const LADDER: readonly TransportId[] = ["ws_stream", "connect_stream", "connect_unary", "full"];
