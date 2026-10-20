/**
 * Story 0.1.1 spike, kept as a regression test: a short-lived server-streaming RPC must reach
 * the caller message by message (not buffered to completion) through the exact transport
 * useFileService builds (getConnectTransport(), plain Connect fetch streaming, path (a) of
 * ADR-030), and aborting the caller's signal must reach fetch.
 *
 * SessionService.WatchSessions is used only as a message-shape stand-in (SearchContent does not
 * exist yet); it is not the transport under test, so its WebSocket bridge is never involved.
 */
import { create, toJsonString } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { SessionEventSchema } from "@/gen/session/v1/events_pb";
import { getConnectTransport, _resetTransportForTesting } from "@/lib/api/transport";

const MESSAGE_GAP_MS = 100;
const MESSAGE_COUNT = 4; // 3 data messages plus a final one

function envelope(flags: number, payload: Uint8Array): Uint8Array {
  const out = new Uint8Array(5 + payload.length);
  out[0] = flags;
  new DataView(out.buffer).setUint32(1, payload.length);
  out.set(payload, 5);
  return out;
}

function eventFrame(seq: number): Uint8Array {
  return envelope(0, new TextEncoder().encode(toJsonString(SessionEventSchema, create(SessionEventSchema, { seq: BigInt(seq) }))));
}

interface FakeFetchHandle {
  signal: AbortSignal | null;
  cancelled: boolean;
}

/** Streams MESSAGE_COUNT frames MESSAGE_GAP_MS apart, then an end-of-stream envelope. */
function installStreamingFetch(): FakeFetchHandle {
  const handle: FakeFetchHandle = { signal: null, cancelled: false };
  const eos = envelope(2, new TextEncoder().encode("{}"));
  global.fetch = jest.fn(async (_url: unknown, init?: RequestInit) => {
    handle.signal = init?.signal ?? null;
    let sent = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        // Real fetch errors the body stream when its signal aborts.
        init?.signal?.addEventListener("abort", () => {
          if (timer) clearTimeout(timer);
          controller.error(new DOMException("aborted", "AbortError"));
        });
      },
      pull(controller) {
        return new Promise<void>((resolve) => {
          timer = setTimeout(() => {
            if (sent < MESSAGE_COUNT) {
              controller.enqueue(eventFrame(++sent));
            } else {
              controller.enqueue(eos);
              controller.close();
            }
            resolve();
          }, MESSAGE_GAP_MS);
        });
      },
      cancel() {
        handle.cancelled = true;
        if (timer) clearTimeout(timer);
      },
    });
    const headers = new Map([["content-type", "application/connect+json"]]);
    return {
      ok: true,
      status: 200,
      statusText: "OK",
      headers: { get: (k: string) => headers.get(k.toLowerCase()) ?? null, forEach: (cb: (v: string, k: string) => void) => headers.forEach(cb) },
      body,
    } as unknown as Response;
  }) as unknown as typeof fetch;
  return handle;
}

describe("streaming spike (Story 0.1.1)", () => {
  const realFetch = global.fetch;
  beforeEach(() => _resetTransportForTesting());
  afterEach(() => {
    global.fetch = realFetch;
  });

  it("streamingTransport_should_DeliverMessagesIncrementally_When_ThreeMessagesSent100msApart", async () => {
    installStreamingFetch();
    const client = createClient(SessionService, getConnectTransport());
    const start = performance.now();
    const arrivals: number[] = [];
    for await (const ev of client.watchSessions({})) {
      arrivals.push(performance.now() - start);
      expect(ev.seq).toBe(BigInt(arrivals.length));
    }
    expect(arrivals).toHaveLength(MESSAGE_COUNT);
    // The first message arrives well before the stream ends: not buffered to completion.
    const total = arrivals[arrivals.length - 1];
    expect(arrivals[0]).toBeLessThan(total - 2 * MESSAGE_GAP_MS);
    // Each gap is near the server's 100 ms spacing (tolerant lower bound for timer jitter).
    for (let i = 1; i < arrivals.length; i++) {
      expect(arrivals[i] - arrivals[i - 1]).toBeGreaterThanOrEqual(MESSAGE_GAP_MS * 0.6);
    }
  });

  it("streamingTransport_should_PropagateAbortToFetchSignal_When_CallerAborts", async () => {
    const handle = installStreamingFetch();
    const client = createClient(SessionService, getConnectTransport());
    const ctrl = new AbortController();
    const seen: bigint[] = [];
    await expect(
      (async () => {
        for await (const ev of client.watchSessions({}, { signal: ctrl.signal })) {
          seen.push(ev.seq);
          if (seen.length === 1) ctrl.abort();
        }
      })()
    ).rejects.toMatchObject({ code: expect.any(Number) });
    expect(seen).toHaveLength(1);
    expect(handle.signal?.aborted).toBe(true);
  });
});
