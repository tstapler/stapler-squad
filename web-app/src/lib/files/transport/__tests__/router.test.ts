import { Code, ConnectError } from "@connectrpc/connect";
import { createFakeTransport, fakeCapabilities } from "../../__tests__/fixtures/fakeTransport";
import { Rejected, TransportFailure, type AdvertisedOp, type TransportId } from "../fileTransport";
import { DEMOTE_AFTER_FAILURES, FALLBACK_CHIP_TEXT, FileTransportRouter } from "../router";

const REF = { sessionId: "s1", path: "a.go" };
const READ_ALL = { kind: "all" } as const;
const FULL_OPS: AdvertisedOp[] = ["stat", "read_all", "write_full"];
const RICH_OPS: AdvertisedOp[] = ["stat", "read_all", "write_full", "read_lines", "patch"];

function setup(caps: Parameters<typeof fakeCapabilities>[0] | Error, wires: TransportId[] = ["connect_unary"]) {
  const files = { "a.go": "package a\n" };
  const full = createFakeTransport({ id: "full", ops: FULL_OPS, files });
  const adapters = wires.map((id) => createFakeTransport({ id, ops: RICH_OPS, files, withPatch: true }));
  const fetchCapabilities = jest.fn(async () => {
    if (caps instanceof Error) throw caps;
    return fakeCapabilities({ wires, ...caps });
  });
  const analytics = { emit: jest.fn() };
  const onFallback = jest.fn();
  const router = new FileTransportRouter({ full, adapters, fetchCapabilities, analytics, onFallback });
  const selects = () => analytics.emit.mock.calls.filter((c) => c[0] === "files.transport.select").map((c) => c[1]);
  return { router, full, adapters, fetchCapabilities, analytics, onFallback, selects };
}

describe("FileTransportRouter", () => {
  it("router_should_ResolveEachOpIndependently_And_NeverCallAnUnadvertisedOp", async () => {
    const { router, full, adapters } = setup({ ops: ["stat", "read_all", "write_full", "read_lines", "patch"] });
    const [unary] = adapters;
    await router.read(REF, READ_ALL);
    await router.patch(REF, { baseToken: "t", edits: [] });
    await router.stat(REF);
    await router.writeFull(REF, { text: "x", baseToken: "t" });
    expect(unary.calls).toEqual(["read:a.go", "patch:a.go", "stat:a.go", "write_full:a.go"]);
    expect(full.calls).toEqual([]);

    const noPatch = setup({ ops: ["stat", "read_all", "write_full"] });
    await expect(noPatch.router.patch(REF, { baseToken: "t", edits: [] })).rejects.toBeInstanceOf(Rejected);
    expect(noPatch.adapters[0].calls).toEqual([]);
    await noPatch.router.read(REF, { kind: "lines", anchorLine: 1, dir: "fwd", wantLines: 10, maxBytes: 100 }).catch((e) => expect(e).toBeInstanceOf(Rejected));
    expect(noPatch.adapters[0].calls).toEqual([]);

    const noStat = setup({ ops: ["read_all", "write_full"] });
    await noStat.router.stat(REF);
    expect(noStat.full.calls).toEqual(["stat:a.go"]);
    expect(noStat.adapters[0].calls).toEqual([]);
  });

  it("router_should_PinFullAndEmitSelectOnce_When_CapabilitiesUnimplementedOrVersionNewer", async () => {
    const old = setup(new ConnectError("no", Code.Unimplemented));
    await old.router.read(REF, READ_ALL);
    await old.router.stat(REF);
    await old.router.read(REF, READ_ALL);
    expect(old.full.calls).toEqual(["read:a.go", "stat:a.go", "read:a.go"]);
    expect(old.fetchCapabilities).toHaveBeenCalledTimes(1);
    expect(old.selects()).toHaveLength(3); // patch has no transport on the floor
    expect(old.selects().every((s) => s.transport === "full" && s.reason === "capabilities")).toBe(true);

    const newer = setup({ protocolVersion: 2 });
    await newer.router.read(REF, READ_ALL);
    expect(newer.adapters[0].calls).toEqual([]);
    expect(newer.selects().map((s) => s.reason)).toEqual(Array(3).fill("version"));
  });

  it("router_should_RefetchCapabilitiesOnce_When_CapsEpochDiffers_And_IgnoreUnknownFeatures", async () => {
    const files = { "a.go": "x" };
    const full = createFakeTransport({ id: "full", ops: FULL_OPS, files });
    let epoch = 1;
    const unary = createFakeTransport({ id: "connect_unary", ops: RICH_OPS, files, withPatch: true, capsEpoch: () => epoch });
    const fetchCapabilities = jest.fn(async () => fakeCapabilities({ capsEpoch: epoch, features: ["from-the-future"], ops: epoch === 1 ? RICH_OPS : FULL_OPS, wires: ["connect_unary", "connect_stream"] }));
    const analytics = { emit: jest.fn() };
    const router = new FileTransportRouter({ full, adapters: [unary], fetchCapabilities, analytics });
    await router.read(REF, READ_ALL);
    expect(fetchCapabilities).toHaveBeenCalledTimes(1);
    expect(router.transportFor("patch")).toBe("connect_unary");
    epoch = 2;
    await router.read(REF, READ_ALL);
    await router.read(REF, READ_ALL);
    expect(fetchCapabilities).toHaveBeenCalledTimes(2);
    expect(() => router.transportFor("patch")).toThrow(Rejected); // epoch 2 no longer advertises PATCH
  });

  it("router_should_DemoteOneRungAfterThreeTransportFailures_And_NeverOnApplicationErrors", async () => {
    const t = setup({ ops: RICH_OPS, wires: ["ws_stream", "connect_stream", "connect_unary"] }, ["ws_stream", "connect_stream", "connect_unary"]);
    const [ws, stream, unary] = t.adapters;
    ws.failures.read = Array.from({ length: DEMOTE_AFTER_FAILURES }, () => new TransportFailure("network"));
    for (let i = 0; i < DEMOTE_AFTER_FAILURES; i++) await expect(t.router.read(REF, READ_ALL)).rejects.toBeInstanceOf(TransportFailure);
    await t.router.read(REF, READ_ALL);
    expect(stream.calls).toEqual(["read:a.go"]);
    expect(ws.calls).toHaveLength(DEMOTE_AFTER_FAILURES);
    expect(t.router.transportFor("stat")).toBe("ws_stream");
    expect(t.onFallback).toHaveBeenCalledTimes(1);
    expect(t.onFallback).toHaveBeenCalledWith(expect.objectContaining({ op: "read", from: "ws_stream", to: "connect_stream", text: FALLBACK_CHIP_TEXT }));
    const falls = t.analytics.emit.mock.calls.filter((c) => c[0] === "files.transport.fallback");
    expect(falls).toEqual([["files.transport.fallback", { op: "read", from: "ws_stream", to: "connect_stream", fallback_reason: "network" }]]);

    unary.failures.write_full = Array.from({ length: 10 }, () => new Rejected("hidden_session"));
    for (let i = 0; i < 10; i++) await t.router.writeFull(REF, { text: "x", baseToken: "t" }).catch(() => undefined);
    expect(t.router.transportFor("write_full")).toBe("ws_stream");
    expect(t.onFallback).toHaveBeenCalledTimes(1);
  });

  it("router_should_ResetTheFailureStreak_When_ACallSucceedsBetweenFailures", async () => {
    const t = setup({ ops: RICH_OPS });
    const [unary] = t.adapters;
    unary.failures.read = [new TransportFailure("deadline"), new TransportFailure("deadline")];
    await t.router.read(REF, READ_ALL).catch(() => undefined);
    await t.router.read(REF, READ_ALL).catch(() => undefined);
    await t.router.read(REF, READ_ALL);
    unary.failures.read = [new TransportFailure("deadline"), new TransportFailure("deadline")];
    await t.router.read(REF, READ_ALL).catch(() => undefined);
    await t.router.read(REF, READ_ALL).catch(() => undefined);
    expect(t.router.transportFor("read")).toBe("connect_unary");
    expect(t.onFallback).not.toHaveBeenCalled();
  });
  it("router_should_ThrowTransportFailureNotUnsupported_When_CapabilitiesFetchFailsTransiently", async () => {
    const { router, fetchCapabilities } = setup(new TypeError("Failed to fetch"));
    const lines = { kind: "lines", anchorLine: 1, dir: "fwd", wantLines: 10, maxBytes: 100 } as const;

    const err = await router.read(REF, lines).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(TransportFailure);
    expect(err).not.toBeInstanceOf(Rejected);
    await router.read(REF, lines).catch(() => undefined);
    expect(fetchCapabilities).toHaveBeenCalledTimes(2); // the failure is retried, not remembered
  });
});
