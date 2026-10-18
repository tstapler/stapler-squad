// Windowed reads over the unary ReadFileRange RPC (ADR-036): CRC check, torn retry, chunk cache, staleness.
import { Code, ConnectError, type Client } from "@connectrpc/connect";
import type { MessageInitShape } from "@bufbuild/protobuf";
import {
  LineEnding, RangeDir, type ReadFileRangeRequestSchema, type ReadFileRangeResponse, type SessionService,
} from "@/gen/session/v1/session_pb";
import type { FilesEmitter } from "@/lib/files/analytics/filesAnalytics";
import { bucketPayload } from "@/lib/files/analytics/events";
import type { ChunkCache } from "./chunkCache";
import { crc32c } from "./crc32c";
import {
  Rejected, TransportFailure,
  type FileCapabilities, type FileRef, type FileTransport, type ReadRequest, type ReadResult,
} from "./fileTransport";
import { guarded, lineEndingOf, readOnlyOf, statVia } from "./wireMapping";

const MIB = 1024 * 1024;
const MAX_WANT_LINES = 4000;
const UTF8_BOM = [0xef, 0xbb, 0xbf];

const CAPABILITIES: FileCapabilities = {
  protocolVersion: 1,
  features: [],
  ops: ["stat", "read_all", "read_lines", "read_bytes"],
  wires: ["connect_unary"],
  capsEpoch: 0,
  limits: {
    maxRangeBytes: MIB, maxRangeLines: MAX_WANT_LINES, preferredChunkBytes: 128 * 1024, maxPatchOps: 0, maxPatchBodyBytes: 0,
    maxInsertBytes: 0, maxWriteFullBytes: 0, hashCapBytes: 64 * MIB, maxConcurrentRanges: 4,
  },
};

const DIRS = { around: RangeDir.AROUND, fwd: RangeDir.FORWARD, back: RangeDir.BACKWARD } as const;

type RangeClient = Pick<Client<typeof SessionService>, "readFileRange" | "statFile">;
type Outcome = "ok" | "torn" | "stale" | "error" | "aborted";

export interface ChangedOnDisk { ref: FileRef; validator: string }

export interface ConnectUnaryOptions {
  /** Serves whole-file reads while the server does not advertise range reads. */
  floor: FileTransport;
  cache?: ChunkCache;
  /** True once the server advertises READ_LINES; until then `all` reads go through `floor`. */
  rangeServed?: () => boolean;
  /** A response carried a validator different from the one the cached chunks were read under. */
  onChanged?: (e: ChangedOnDisk) => void;
  analytics?: Pick<FilesEmitter, "time">;
}

/** 1-based inclusive line span the server would serve for a line request (mirrors firstLineFor). */
export function lineSpan(req: Extract<ReadRequest, { kind: "lines" }>): { first: number; last: number } {
  const want = Math.min(req.wantLines, MAX_WANT_LINES);
  const raw = req.dir === "fwd" ? req.anchorLine : req.dir === "back" ? req.anchorLine - want + 1 : req.anchorLine - Math.floor(want / 2);
  const first = Math.max(raw, 1);
  return { first, last: first + want - 1 };
}

function toRequest(ref: FileRef, req: ReadRequest, ifValidator: string | undefined): MessageInitShape<typeof ReadFileRangeRequestSchema> {
  const base = { sessionId: ref.sessionId, path: ref.path, ifValidator };
  switch (req.kind) {
    case "lines":
      return { ...base, maxBytes: BigInt(req.maxBytes), anchor: { case: "lines", value: { anchorLine: BigInt(req.anchorLine), dir: DIRS[req.dir], wantLines: req.wantLines } } };
    case "bytes":
      return { ...base, maxBytes: BigInt(req.length), anchor: { case: "bytes", value: { offset: BigInt(req.offset), length: BigInt(req.length), dir: DIRS[req.dir ?? "around"] } } };
    case "all": {
      const limit = Math.min(req.maxBytes ?? MIB, MIB);
      return { ...base, maxBytes: BigInt(MIB), preferFullIfLe: BigInt(limit), anchor: { case: "lines", value: { anchorLine: 1n, dir: RangeDir.FORWARD, wantLines: MAX_WANT_LINES } } };
    }
  }
}

/**
 * The server hashes the raw on-disk bytes but returns LF-normalized, BOM-less, UTF-8-repaired text, so the
 * raw bytes can be rebuilt only for LF or all-CRLF valid UTF-8 windows; others are not checked (null).
 */
function rawBytesOf(r: ReadFileRangeResponse): Uint8Array | null {
  if (r.isBinary || r.nonUtf8 || r.eolClass === LineEnding.MIXED) return null;
  const text = r.eolClass === LineEnding.CRLF ? r.content.replace(/\n/g, "\r\n") : r.content;
  const body = new TextEncoder().encode(text);
  if (!r.hasBom) return body;
  const raw = new Uint8Array(UTF8_BOM.length + body.length);
  raw.set(UTF8_BOM);
  raw.set(body, UTF8_BOM.length);
  return raw;
}

function crcMatches(r: ReadFileRangeResponse): boolean {
  const raw = rawBytesOf(r);
  return raw === null || crc32c(raw) === r.chunkCrc32c;
}

function toResult(r: ReadFileRangeResponse): ReadResult {
  const truncated = !(r.bof && r.eof);
  return {
    text: r.content, token: r.versionToken, size: Number(r.size), lineEnding: lineEndingOf(r.eolClass), hasBom: r.hasBom,
    readOnly: readOnlyOf(r.readOnlyReason, r.isBinary, truncated), isBinary: r.isBinary, truncated,
    range: { startLine: Number(r.startLine), endLine: Number(r.endLine), startByte: Number(r.startByte), endByte: Number(r.endByte), bof: r.bof, eof: r.eof },
    capsEpoch: Number(r.capsEpoch), chunkCrc32c: r.chunkCrc32c, validator: r.validator,
    lineCount: r.lineCountKnown ? Number(r.lineCount) : undefined, partialLastLine: r.partialLastLine, longLine: r.longLine,
  };
}

const isAbort = (err: unknown, signal?: AbortSignal) =>
  signal?.aborted === true || (err instanceof ConnectError && err.code === Code.Canceled) || (err instanceof DOMException && err.name === "AbortError");

export function createConnectUnaryTransport(client: RangeClient, opts: ConnectUnaryOptions): FileTransport {
  const { floor, cache, analytics } = opts;
  const served = opts.rangeServed ?? (() => true);

  /** Up to two attempts: a torn window or a CRC mismatch is retried once. */
  async function fetchVerified(ref: FileRef, req: ReadRequest, signal?: AbortSignal): Promise<{ resp: ReadFileRangeResponse; retried: boolean }> {
    for (let attempt = 0; ; attempt++) {
      const resp = await guarded(client.readFileRange(toRequest(ref, req, cache?.validatorOf(ref)), { signal }));
      const ok = !resp.torn && crcMatches(resp);
      if (ok) return { resp, retried: attempt > 0 };
      if (attempt === 1) {
        if (resp.torn) {
          cache?.invalidate(ref);
          throw new Rejected("stale", "the file changed while it was being read");
        }
        throw new TransportFailure("malformed");
      }
    }
  }

  /** Detects a validator change against the cached chunks; returns true when the file moved under the reader. */
  function noteValidator(ref: FileRef, validator: string): boolean {
    const prev = cache?.validatorOf(ref);
    if (prev === undefined || prev === validator) return false;
    cache?.invalidate(ref);
    opts.onChanged?.({ ref, validator });
    return true;
  }

  async function readWindow(ref: FileRef, req: ReadRequest, signal?: AbortSignal): Promise<{ result: ReadResult; outcome: Outcome; hit: boolean }> {
    if (req.kind === "lines" && cache) {
      const { first, last } = lineSpan(req);
      const hit = cache.get(ref, first, last);
      if (hit) return { result: hit, outcome: "ok", hit: true };
    }
    const { resp, retried } = await fetchVerified(ref, req, signal);
    const result = toResult(resp);
    const changed = noteValidator(ref, resp.validator);
    if (req.kind === "lines" && cache && result.range.endLine >= result.range.startLine && result.range.startLine > 0) {
      cache.put(ref, resp.validator, result);
    }
    return { result, outcome: changed ? "stale" : retried ? "torn" : "ok", hit: false };
  }

  return {
    id: "connect_unary",
    capabilities: () => CAPABILITIES,
    stat: (ref, o, signal) => statVia(client, ref, o, signal),
    async read(ref, req, signal) {
      if (!served()) {
        if (req.kind === "all") return floor.read(ref, req, signal);
        throw new Rejected("unsupported", `${req.kind} reads are not served`);
      }
      signal?.throwIfAborted();
      const stop = analytics?.time("files.read.window");
      const labels = (outcome: Outcome, hit: boolean, payload: number) => ({
        transport: "connect_unary" as const, payload: bucketPayload(payload), outcome, cache: hit ? ("hit" as const) : ("miss" as const),
      });
      try {
        const { result, outcome, hit } = await readWindow(ref, req, signal);
        stop?.(labels(outcome, hit, result.range.endByte - result.range.startByte));
        return result;
      } catch (err) {
        stop?.(labels(isAbort(err, signal) ? "aborted" : err instanceof Rejected && err.reason === "stale" ? "stale" : "error", false, 0));
        throw err;
      }
    },
    writeFull: (ref, w, signal) => floor.writeFull(ref, w, signal),
  };
}
