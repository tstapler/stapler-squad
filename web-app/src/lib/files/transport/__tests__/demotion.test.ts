import { Code, ConnectError } from "@connectrpc/connect";
import { busyError, createFakeRangeServer, numberedLines } from "../../__tests__/fixtures/fakeRangeServer";
import { createFakeTransport, fakeCapabilities } from "../../__tests__/fixtures/fakeTransport";
import { ChunkCache } from "../chunkCache";
import { createConnectUnaryTransport } from "../connectUnaryTransport";
import { FALLBACK_CHIP_TEXT, FileTransportRouter } from "../router";

const REF = { sessionId: "s1", path: "big.log" };
const lines = (anchorLine: number) => ({ kind: "lines", anchorLine, dir: "fwd", wantLines: 10, maxBytes: 65536 }) as const;

function setup() {
  const server = createFakeRangeServer({ "big.log": numberedLines(100), "small.txt": "hi\n" });
  const full = createFakeTransport({ id: "full", files: { "small.txt": "hi\n" }, ops: ["stat", "read_all"] });
  const unary = createConnectUnaryTransport(server.client, { floor: full, cache: new ChunkCache(1 << 20) });
  const analytics = { emit: jest.fn() };
  const onFallback = jest.fn();
  const caps = fakeCapabilities({ ops: ["stat", "read_all", "read_lines", "read_bytes"], wires: ["connect_unary"] });
  const router = new FileTransportRouter({ full, adapters: [unary], fetchCapabilities: async () => caps, analytics, onFallback });
  const fallbacks = () => analytics.emit.mock.calls.filter((c) => c[0] === "files.transport.fallback");
  return { server, full, router, onFallback, fallbacks };
}

describe("read demotion over connect_unary", () => {
  it("router_should_DemoteOneRungAfterThreeTransportFailures_And_NeverOnConflictOrPermissionErrors", async () => {
    const t = setup();
    const application = [
      busyError(), new ConnectError("line index is still being built", Code.FailedPrecondition), new ConnectError("denied", Code.PermissionDenied),
      new ConnectError("missing", Code.NotFound), busyError(), new ConnectError("denied", Code.PermissionDenied),
      new ConnectError("line index is still being built", Code.FailedPrecondition), busyError(), new ConnectError("denied", Code.PermissionDenied), busyError(),
    ];
    for (const err of application) {
      t.server.faults.throwNext.push(err);
      await t.router.read(REF, lines(1)).catch(() => undefined);
    }
    expect(t.fallbacks()).toHaveLength(0);
    expect(t.router.transportFor("read")).toBe("connect_unary");

    // Two failures then a success reset the streak; three in a row demote.
    for (const code of [Code.Unavailable, Code.DeadlineExceeded]) {
      t.server.faults.throwNext.push(new ConnectError("net", code));
      await t.router.read(REF, lines(1)).catch(() => undefined);
    }
    await t.router.read(REF, lines(1));
    expect(t.fallbacks()).toHaveLength(0);
    for (let i = 0; i < 3; i++) {
      t.server.faults.throwNext.push(new ConnectError("net", Code.Unavailable));
      await t.router.read(REF, lines(51)).catch(() => undefined); // uncached, so it reaches the wire
    }
    expect(t.fallbacks()).toHaveLength(1);
    expect(t.fallbacks()[0][1]).toMatchObject({ op: "read", from: "connect_unary", to: "full", fallback_reason: "unavailable" });
    expect(t.onFallback).toHaveBeenCalledWith(expect.objectContaining({ text: FALLBACK_CHIP_TEXT }));

    // The floor serves whole small files; windowed reads are refused with a typed reason, and nothing promotes back.
    expect(t.router.transportFor("read")).toBe("full");
    await expect(t.router.read({ ...REF, path: "small.txt" }, { kind: "all" })).resolves.toMatchObject({ text: "hi\n" });
    await expect(t.router.read(REF, lines(71))).rejects.toMatchObject({ reason: "unsupported" });
  });

  it("router_should_CountRepeatedCrcMismatchesAsTransportFailures", async () => {
    const t = setup();
    for (let i = 0; i < 3; i++) {
      t.server.faults.badCrc = 2; // both attempts of one read fail verification
      await t.router.read(REF, lines(1)).catch(() => undefined);
    }
    expect(t.fallbacks()).toHaveLength(1);
    expect(t.fallbacks()[0][1]).toMatchObject({ fallback_reason: "malformed" });
  });
});
