// A FileTransportPort over a synthetic file of `lines` numbered lines ("line N"), served in windows like the
// server's ReadFileRange: far starts are refused with index_pending until `ready()`, the count is exact only when known.
import { Rejected, type FileCapabilities, type ReadRequest, type ReadResult, type FileTransportPort } from "@/lib/files/transport/fileTransport";
import { fakeCapabilities } from "./fakeTransport";

const BYTES_PER_LINE = 10;

export interface VirtualRangeOptions {
  lines: number;
  /** Reported file size when it is not lines x 10 (skews the "about N" estimate). */
  size?: number;
  /** Server analysis done: far reads work and results carry the exact count. */
  ready?: () => boolean;
  /** Throws the next reads' errors, one per call. */
  failures?: unknown[];
  caps?: Partial<FileCapabilities>;
}

export interface VirtualRangeTransport extends FileTransportPort {
  reads: Extract<ReadRequest, { kind: "lines" }>[];
  stats: number;
  failures: unknown[];
  size: number;
}

export const lineText = (n: number) => `line ${n}`;

export function createVirtualRangeTransport(o: VirtualRangeOptions): VirtualRangeTransport {
  const ready = o.ready ?? (() => true);
  const size = o.size ?? o.lines * BYTES_PER_LINE;
  const t: VirtualRangeTransport = {
    reads: [], stats: 0, failures: o.failures ?? [], size,
    capabilities: () => fakeCapabilities({ ops: ["stat", "read_all", "read_lines"], wires: ["connect_unary"], ...o.caps }),
    async stat() {
      t.stats++;
      return { token: "-", size, readOnly: "too_large", analysis: ready() ? "ready" : "pending" };
    },
    async read(_ref, req, signal): Promise<ReadResult> {
      signal?.throwIfAborted();
      if (req.kind !== "lines") throw new Rejected("unsupported");
      t.reads.push(req);
      const failure = t.failures.shift();
      if (failure !== undefined) throw failure;
      const want = Math.min(req.wantLines, 4000);
      const raw = req.dir === "fwd" ? req.anchorLine : req.dir === "back" ? req.anchorLine - want + 1 : req.anchorLine - Math.floor(want / 2);
      let first = Math.max(raw, 1);
      if (first > 1 && !ready()) throw new Rejected("index_pending", "line index is still being built");
      first = Math.min(first, Math.max(o.lines - want + 1, 1));
      const last = Math.min(first + want - 1, o.lines);
      const text = Array.from({ length: last - first + 1 }, (_, i) => `${lineText(first + i)}\n`).join("");
      return {
        text, token: "-", size, lineEnding: "lf", hasBom: false, readOnly: "too_large", isBinary: false, truncated: true,
        range: { startLine: first, endLine: last, startByte: (first - 1) * BYTES_PER_LINE, endByte: last * BYTES_PER_LINE, bof: first === 1, eof: last === o.lines },
        lineCount: ready() ? o.lines : undefined,
      };
    },
    writeFull: () => Promise.reject(new Rejected("unsupported")),
  };
  return t;
}
