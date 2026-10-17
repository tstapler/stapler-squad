import {
  Rejected,
  type AdvertisedOp, type FileCapabilities, type FileOpName, type FileRef, type FileTransport, type ReadRequest, type ReadResult,
  type TransportId,
} from "@/lib/files/transport/fileTransport";

const MIB = 1024 * 1024;
export const DEFAULT_OPS: readonly AdvertisedOp[] = ["stat", "read_all", "write_full"];

export function fakeCapabilities(over: Partial<FileCapabilities> = {}): FileCapabilities {
  return {
    protocolVersion: 1, features: [], ops: DEFAULT_OPS, wires: ["connect_unary"], capsEpoch: 1,
    limits: {
      maxRangeBytes: 0, maxRangeLines: 0, preferredChunkBytes: 0, maxPatchOps: 0, maxPatchBodyBytes: 0,
      maxInsertBytes: 0, maxWriteFullBytes: 0, hashCapBytes: MIB, maxConcurrentRanges: 0,
    },
    ...over,
  };
}

export interface FakeTransportOptions {
  id?: TransportId;
  ops?: readonly AdvertisedOp[];
  files?: Record<string, string>;
  withPatch?: boolean;
  /** Stamped on every result; a function lets a test move the server epoch. */
  capsEpoch?: number | (() => number);
}

export interface FakeTransport extends FileTransport {
  /** `<op>:<path>` of every call, in order. */
  calls: string[];
  /** Errors thrown by the next calls of an op, consumed one per call. */
  failures: Partial<Record<FileOpName, unknown[]>>;
}

/** In-memory FileTransport; tests pass it through the harness and run `runTransportConformance` on it. */
export function createFakeTransport(opts: FakeTransportOptions = {}): FakeTransport {
  const files = new Map(Object.entries(opts.files ?? {}));
  const ops = opts.ops ?? DEFAULT_OPS;
  const epoch = () => (typeof opts.capsEpoch === "function" ? opts.capsEpoch() : opts.capsEpoch);
  const fake: FakeTransport = {
    id: opts.id ?? "full",
    calls: [],
    failures: {},
    capabilities: () => fakeCapabilities({ ops, wires: [fake.id] }),
    stat: async (ref) => step("stat", ref, () => ({ token: tokenOf(ref.path), size: sizeOf(ref.path), readOnly: "none", analysis: "unspecified", capsEpoch: epoch() })),
    read: async (ref, req) => step("read", ref, () => readResult(ref, req)),
    writeFull: async (ref, w) => step("write_full", ref, () => {
      files.set(ref.path, w.text);
      return { token: tokenOf(ref.path), capsEpoch: epoch() };
    }),
  };
  if (opts.withPatch) fake.patch = async (ref) => step("patch", ref, () => ({ token: tokenOf(ref.path), capsEpoch: epoch() }));

  const tokenOf = (path: string) => `v1:${sizeOf(path)}:1:fake`;
  const sizeOf = (path: string) => (files.get(path) ?? "").length;
  function step<T>(op: FileOpName, ref: FileRef, ok: () => T): T {
    fake.calls.push(`${op}:${ref.path}`);
    const queued = fake.failures[op]?.shift();
    if (queued !== undefined) throw queued;
    if (!files.has(ref.path) && op !== "write_full") throw new Rejected("not_found");
    return ok();
  }
  function readResult(ref: FileRef, req: ReadRequest): ReadResult {
    const text = files.get(ref.path) ?? "";
    return {
      text, token: tokenOf(ref.path), size: text.length, lineEnding: "lf", hasBom: false, readOnly: "none", isBinary: false,
      truncated: false, capsEpoch: epoch(), range: { startLine: 1, endLine: text.split("\n").length, startByte: 0, endByte: text.length, bof: true, eof: req.kind === "all" || true },
    };
  }
  return fake;
}
