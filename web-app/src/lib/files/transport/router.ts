import { Code, ConnectError } from "@connectrpc/connect";
import type { FilesEmitter } from "@/lib/files/analytics/filesAnalytics";
import {
  LADDER, Rejected, TransportFailure,
  type AdvertisedOp, type FileCapabilities, type FileOpName, type FileRef, type FileStat, type FileTransport,
  type PatchRequest, type PatchResult, type ReadRequest, type ReadResult, type TransportId, type WriteRequest, type WriteResult,
} from "./fileTransport";

export const DEMOTE_AFTER_FAILURES = 3;
/** A transport-failure demotion lapses after this long, so a network blip never pins a session to a lower rung. */
export const DEMOTE_COOLDOWN_MS = 60_000;
export const SUPPORTED_PROTOCOL_MAJOR = 1;
export const FALLBACK_CHIP_TEXT = "Using simpler file mode";

const OPS: readonly FileOpName[] = ["read", "patch", "stat", "write_full"];
const NEED_FOR_READ = { all: "read_all", lines: "read_lines", bytes: "read_bytes" } as const;

type SelectReason = "capabilities" | "flag" | "pinned_full" | "version";

export interface RouterOptions {
  /** The always-present floor adapter. */
  full: FileTransport;
  /** Higher rungs; each is used only when the server advertises its wire and the op. */
  adapters?: readonly FileTransport[];
  /** May throw a ConnectError `Unimplemented` (older server): the router then pins `full`. */
  /** `observedEpoch` is set when a response carried an epoch different from the cached caps, so a shared cache must refetch. */
  fetchCapabilities(sessionId: string, observedEpoch?: number): Promise<FileCapabilities>;
  analytics?: Pick<FilesEmitter, "emit">;
  /** Clock for demotion cool-downs (tests). */
  now?: () => number;
  /** Asks the banner stack for the quiet "Using simpler file mode" chip. */
  onFallback?: (info: { op: FileOpName; from: TransportId; to: TransportId; text: string }) => void;
}

/** Resolves each op to the highest advertised rung and demotes an op after repeated transport failures. */
export class FileTransportRouter {
  private caps: FileCapabilities | null = null;
  private capsPromise: Promise<void> | null = null;
  private pin: SelectReason | null = null;
  /** Why the last capabilities fetch failed (not Unimplemented); cleared on success. */
  private capsFailure: unknown = null;
  private readonly byId = new Map<TransportId, FileTransport>();
  private readonly ceiling = new Map<FileOpName, { rung: number; until: number }>();
  private readonly streak = new Map<FileOpName, { id: TransportId; failures: number }>();
  private readonly announced = new Map<FileOpName, TransportId>();

  constructor(private readonly opts: RouterOptions) {
    for (const a of opts.adapters ?? []) this.byId.set(a.id, a);
  }

  capabilities(): FileCapabilities {
    return this.caps ?? this.opts.full.capabilities();
  }

  stat(ref: FileRef, o?: { wantAnalysis?: boolean; wantSha256?: boolean }, signal?: AbortSignal): Promise<FileStat> {
    return this.run(ref, "stat", "stat", (t) => t.stat(ref, o, signal));
  }

  read(ref: FileRef, req: ReadRequest, signal?: AbortSignal): Promise<ReadResult> {
    return this.run(ref, "read", NEED_FOR_READ[req.kind], (t) => t.read(ref, req, signal));
  }

  patch(ref: FileRef, p: PatchRequest, signal?: AbortSignal): Promise<PatchResult> {
    return this.run(ref, "patch", "patch", (t) => (t.patch ? t.patch(ref, p, signal) : Promise.reject(new Rejected("unsupported"))));
  }

  writeFull(ref: FileRef, w: WriteRequest, signal?: AbortSignal): Promise<WriteResult> {
    return this.run(ref, "write_full", "write_full", (t) => t.writeFull(ref, w, signal));
  }

  /** The transport an op would use now (after capabilities are loaded). */
  transportFor(op: FileOpName): TransportId {
    return this.resolve(op, op === "read" ? "read_all" : op).id;
  }

  private async run<T extends { capsEpoch?: number }>(ref: FileRef, op: FileOpName, need: AdvertisedOp, call: (t: FileTransport) => Promise<T>): Promise<T> {
    await this.ensureCaps(ref.sessionId);
    this.requireCapsOrFloor(need);
    const transport = this.resolve(op, need);
    this.announce();
    try {
      const result = await call(transport);
      this.streak.delete(op);
      await this.checkEpoch(ref.sessionId, result.capsEpoch);
      return result;
    } catch (err) {
      this.recordFailure(op, transport, err);
      throw err;
    }
  }

  private ensureCaps(sessionId: string, observedEpoch?: number): Promise<void> {
    if (this.caps || this.pin) return Promise.resolve();
    this.capsPromise ??= this.loadCaps(sessionId, observedEpoch).finally(() => { this.capsPromise = null; });
    return this.capsPromise;
  }

  private async loadCaps(sessionId: string, observedEpoch?: number): Promise<void> {
    try {
      const caps = await this.opts.fetchCapabilities(sessionId, observedEpoch);
      if (caps.protocolVersion > SUPPORTED_PROTOCOL_MAJOR) this.pin = "version";
      else this.caps = caps;
      this.capsFailure = null;
    } catch (err) {
      // Older server: today's behaviour. Any other failure retries on the next call.
      if (err instanceof ConnectError && err.code === Code.Unimplemented) this.pin = "capabilities";
      else this.capsFailure = err;
    }
  }

  /** Unknown capabilities after a failed fetch is transient, so an op only a higher rung serves must not look unsupported. */
  private requireCapsOrFloor(need: AdvertisedOp): void {
    if (this.caps || this.pin || !this.capsFailure) return;
    if (!this.opts.full.capabilities().ops.includes(need)) throw new TransportFailure("unavailable", this.capsFailure);
  }

  private async checkEpoch(sessionId: string, epoch: number | undefined): Promise<void> {
    if (epoch === undefined || !this.caps || epoch === this.caps.capsEpoch) return;
    this.caps = null;
    await this.ensureCaps(sessionId, epoch);
    this.announce();
  }

  private resolve(op: FileOpName, need: AdvertisedOp): FileTransport {
    const { full } = this.opts;
    const floor = () => {
      if (!full.capabilities().ops.includes(need)) throw new Rejected("unsupported", `${need} needs a higher transport`);
      return full;
    };
    const caps = this.caps;
    if (!caps || this.pin) return floor();
    for (const rung of LADDER.slice(this.ceilingOf(op))) {
      if (rung === "full") break;
      const adapter = this.byId.get(rung);
      if (!adapter || !caps.wires.includes(rung) || !caps.ops.includes(need)) continue;
      if (op === "patch" && !adapter.patch) continue;
      return adapter;
    }
    return floor();
  }

  private now(): number {
    return (this.opts.now ?? Date.now)();
  }

  /** First ladder rung the op may use; a lapsed demotion is forgotten so the higher rung gets another try. */
  private ceilingOf(op: FileOpName): number {
    const c = this.ceiling.get(op);
    if (!c) return 0;
    if (this.now() >= c.until) {
      this.ceiling.delete(op);
      return 0;
    }
    return c.rung;
  }

  private selectReason(op: FileOpName): SelectReason {
    if (this.pin) return this.pin;
    return this.ceilingOf(op) > 0 && this.transportFor(op) === "full" ? "pinned_full" : "capabilities";
  }

  /** One `files.transport.select` per op whenever its resolved transport changes. */
  private announce(): void {
    for (const op of OPS) {
      let id: TransportId;
      try {
        id = this.transportFor(op);
      } catch {
        continue;
      }
      if (this.announced.get(op) === id) continue;
      this.announced.set(op, id);
      this.opts.analytics?.emit("files.transport.select", { op, transport: id, reason: this.selectReason(op) });
    }
  }

  private recordFailure(op: FileOpName, transport: FileTransport, err: unknown): void {
    if (!(err instanceof TransportFailure)) {
      this.streak.delete(op); // an application error proves the wire works
      return;
    }
    if (transport.id === "full") return;
    const cur = this.streak.get(op);
    const failures = cur?.id === transport.id ? cur.failures + 1 : 1;
    this.streak.set(op, { id: transport.id, failures });
    if (failures < DEMOTE_AFTER_FAILURES) return;

    this.streak.delete(op);
    this.ceiling.set(op, { rung: LADDER.indexOf(transport.id) + 1, until: this.now() + DEMOTE_COOLDOWN_MS });
    let to: TransportId = "full";
    try {
      to = this.transportFor(op);
    } catch {
      // floor cannot serve this op; keep "full" as the nominal target
    }
    this.opts.analytics?.emit("files.transport.fallback", { op, from: transport.id, to, fallback_reason: err.reason });
    this.opts.onFallback?.({ op, from: transport.id, to, text: FALLBACK_CHIP_TEXT });
    this.announce();
  }
}
