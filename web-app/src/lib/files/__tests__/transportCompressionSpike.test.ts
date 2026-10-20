/**
 * Task 0.1.7a jest half (ADR-036 S-T1): what the browser transport factory itself does about compression.
 * Runs without the Go lane's throwaway unary handler. The wire-size, negotiated content-encoding and latency
 * numbers over :62871, :62872 and the phone need that handler and are PENDING (it.skip below).
 */
import { create, toJsonString } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { ListSessionsResponseSchema } from "@/gen/session/v1/session_pb";
import { getConnectTransport, _resetTransportForTesting } from "@/lib/api/transport";

function jsonResponse(extraHeaders: Record<string, string> = {}): Response {
  const headers = new Headers({ "content-type": "application/json", ...extraHeaders });
  const text = toJsonString(ListSessionsResponseSchema, create(ListSessionsResponseSchema, {}));
  return { ok: true, status: 200, statusText: "OK", headers, json: async () => JSON.parse(text) } as unknown as Response;
}

describe("transport compression spike (S-T1, jest half)", () => {
  const realFetch = global.fetch;
  beforeEach(() => _resetTransportForTesting());
  afterEach(() => {
    global.fetch = realFetch;
  });

  it("getConnectTransport_should_NotAdvertiseConnectLevelGzip_ByDefault", async () => {
    let sent: Record<string, string> = {};
    global.fetch = jest.fn(async (_url: unknown, init?: RequestInit) => {
      sent = Object.fromEntries(new Headers(init?.headers as HeadersInit).entries());
      return jsonResponse();
    }) as unknown as typeof fetch;
    await createClient(SessionService, getConnectTransport()).listSessions({});
    // connect-web ships no compression codecs: no connect-accept-encoding, and accept-encoding is left to the browser.
    expect(sent["connect-accept-encoding"]).toBeUndefined();
    expect(sent["connect-content-encoding"]).toBeUndefined();
    expect(sent["accept-encoding"]).toBeUndefined();
  });

  it("getConnectTransport_should_NotAdvertiseConnectLevelGzip_OnStreamingCallsEither", async () => {
    let sent: Record<string, string> = {};
    global.fetch = jest.fn(async (_url: unknown, init?: RequestInit) => {
      sent = Object.fromEntries(new Headers(init?.headers as HeadersInit).entries());
      const eos = new Uint8Array([2, 0, 0, 0, 2, 0x7b, 0x7d]);
      const body = new ReadableStream<Uint8Array>({ start: (c) => { c.enqueue(eos); c.close(); } });
      return { ok: true, status: 200, statusText: "OK", headers: new Headers({ "content-type": "application/connect+json" }), body } as unknown as Response;
    }) as unknown as typeof fetch;
    for await (const _ of createClient(SessionService, getConnectTransport()).watchSessions({})) void _;
    expect(sent["connect-accept-encoding"]).toBeUndefined();
  });

  it.skip("rangeResponse_should_RecordWireSizeContentEncodingAndLatency_For128KiBAnd1MiB [PENDING: needs the Go lane's throwaway unary handler on :62871/:62872]", () => {
    // Measured in the Playwright harness (tests/e2e/files-transport-spike.spec.ts) via resource timing transferSize.
  });
});
