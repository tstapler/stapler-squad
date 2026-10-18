// In-memory ReadFileRange/StatFile server following server/services/file_ops_range*.go: 1-based lines,
// CRC32C over the raw bytes, LF-normalized content, BOM stripped on the first window.
import { Code, ConnectError, createClient, createRouterTransport } from "@connectrpc/connect";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import {
  FileAnalysisState, LineEnding, RangeBusyDetailSchema, RangeDir, ReadOnlyReason, SessionService,
  type ReadFileRangeRequest, type ReadFileRangeResponseSchema,
} from "@/gen/session/v1/session_pb";
import { crc32c } from "@/lib/files/transport/crc32c";

const enc = new TextEncoder();
const BOM = "﻿";

export interface FakeRangeServer {
  client: ReturnType<typeof createClient<typeof SessionService>>;
  files: Map<string, string>;
  /** Every ReadFileRange request, in order. */
  calls: ReadFileRangeRequest[];
  /** Rewrites the stored file, moving the validator. */
  touch(path: string, text: string): void;
  /** Mutators consumed one response at a time. */
  faults: { torn: number; badCrc: number; throwNext: ConnectError[] };
}

function eolOf(raw: string): LineEnding {
  const crlf = (raw.match(/\r\n/g) ?? []).length;
  const lf = (raw.match(/\n/g) ?? []).length - crlf;
  const cr = (raw.match(/\r/g) ?? []).length - crlf;
  if (cr > 0 || (crlf > 0 && lf > 0)) return LineEnding.MIXED;
  return crlf > 0 ? LineEnding.CRLF : LineEnding.LF;
}

function firstLine(dir: RangeDir, anchor: number, lines: number): number {
  const first = dir === RangeDir.FORWARD ? anchor : dir === RangeDir.BACKWARD ? anchor - lines + 1 : anchor - Math.floor(lines / 2);
  return Math.max(first, 1);
}

export function createFakeRangeServer(initial: Record<string, string> = {}): FakeRangeServer {
  const files = new Map(Object.entries(initial));
  const mtimes = new Map<string, number>();
  const calls: ReadFileRangeRequest[] = [];
  const faults = { torn: 0, badCrc: 0, throwNext: [] as ConnectError[] };
  const text = (path: string) => {
    const t = files.get(path);
    if (t === undefined) throw new ConnectError("file not found", Code.NotFound);
    return t;
  };
  const validatorOf = (path: string) => `${enc.encode(text(path)).length}:${mtimes.get(path) ?? 1}:7`;

  function window(req: ReadFileRangeRequest): MessageInitShape<typeof ReadFileRangeResponseSchema> {
    const all = text(req.path);
    const lines = all.match(/[^\n]*\n|[^\n]+/g) ?? [];
    const whole = req.preferFullIfLe > 0n && BigInt(enc.encode(all).length) <= req.preferFullIfLe;
    const want = whole ? lines.length : req.anchor.case === "lines" && req.anchor.value.wantLines > 0 ? req.anchor.value.wantLines : 400;
    const first = whole || req.anchor.case !== "lines" ? 1 : firstLine(req.anchor.value.dir, Number(req.anchor.value.anchorLine), want);
    const picked = lines.slice(first - 1, first - 1 + want);
    const raw = picked.join("");
    const startByte = enc.encode(lines.slice(0, first - 1).join("")).length;
    const endByte = startByte + enc.encode(raw).length;
    const hasBom = startByte === 0 && raw.startsWith(BOM);
    return {
      content: (hasBom ? raw.slice(1) : raw).replace(/\r\n/g, "\n"),
      startLine: BigInt(picked.length ? first : 0), endLine: BigInt(picked.length ? first + picked.length - 1 : 0),
      startByte: BigInt(startByte), endByte: BigInt(endByte), bof: startByte === 0, eof: first - 1 + picked.length >= lines.length,
      chunkCrc32c: crc32c(enc.encode(raw)), eolClass: eolOf(raw), hasBom,
    };
  }

  const transport = createRouterTransport(({ service }) => {
    service(SessionService, {
      readFileRange: (req) => {
        calls.push(req);
        const thrown = faults.throwNext.shift();
        if (thrown) throw thrown;
        const w = window(req);
        const torn = faults.torn > 0 && faults.torn-- > 0;
        const badCrc = faults.badCrc > 0 && faults.badCrc-- > 0;
        return {
          ...w, chunkCrc32c: ((w.chunkCrc32c ?? 0) ^ (badCrc ? 1 : 0)) >>> 0, torn, validator: validatorOf(req.path),
          versionToken: "-", size: BigInt(enc.encode(text(req.path)).length), analysis: FileAnalysisState.PENDING,
          readOnlyReason: ReadOnlyReason.NONE, capsEpoch: 1n,
        };
      },
      statFile: (req) => ({ versionToken: "-", size: BigInt(enc.encode(text(req.path)).length), readOnlyReason: ReadOnlyReason.NONE, analysis: FileAnalysisState.PENDING }),
    });
  });
  return { client: createClient(SessionService, transport), files, calls, faults, touch: (p, t) => { files.set(p, t); mtimes.set(p, (mtimes.get(p) ?? 1) + 1); } };
}

export const busyError = () =>
  new ConnectError("too many range reads are running", Code.ResourceExhausted, undefined, [
    { desc: RangeBusyDetailSchema, value: create(RangeBusyDetailSchema, { reason: "per_client" }) },
  ]);

export const numberedLines = (n: number, width = 0) =>
  Array.from({ length: n }, (_, i) => `line ${i + 1}${"x".repeat(width)}\n`).join("");
