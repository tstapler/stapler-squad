import { Code, ConnectError } from "@connectrpc/connect";
import { busyError, createFakeRangeServer, numberedLines } from "../../__tests__/fixtures/fakeRangeServer";
import { createFilesEmitter } from "../../analytics/filesAnalytics";
import { createFakeAnalytics } from "../../__tests__/fixtures/fakeAnalytics";
import { fakeClock } from "../../__tests__/fixtures/fakeClock";
import { labelProblems } from "../../__tests__/fixtures/privacyHarness";
import { createFakeTransport } from "../../__tests__/fixtures/fakeTransport";
import { ChunkCache } from "../chunkCache";
import { createConnectUnaryTransport, lineSpan } from "../connectUnaryTransport";
import { crc32c } from "../crc32c";
import { Rejected, TransportFailure, type FileRef } from "../fileTransport";

const REF: FileRef = { sessionId: "s1", path: "big.log" };
const fwd = (anchorLine: number, wantLines = 10) => ({ kind: "lines", anchorLine, dir: "fwd", wantLines, maxBytes: 128 * 1024 }) as const;

function setup(files: Record<string, string> = { "big.log": numberedLines(100) }, over: { rangeServed?: () => boolean } = {}) {
  const server = createFakeRangeServer(files);
  const cache = new ChunkCache(1024 * 1024);
  const changed: string[] = [];
  const floor = createFakeTransport({ files: { "small.txt": "hi" }, ops: ["stat", "read_all"] });
  const transport = createConnectUnaryTransport(server.client, { floor, cache, onChanged: (e) => changed.push(e.validator), ...over });
  return { server, cache, changed, floor, transport };
}

describe("crc32c", () => {
  it("crc32c_should_MatchTheCastagnoliCheckValue", () => {
    expect(crc32c(new TextEncoder().encode("123456789"))).toBe(0xe3069283);
    expect(crc32c(new Uint8Array())).toBe(0);
    const a = new TextEncoder().encode("1234");
    const b = new TextEncoder().encode("56789");
    expect(crc32c(b, crc32c(a))).toBe(0xe3069283);
  });
});

describe("ConnectUnaryTransport", () => {
  it("connectUnary_should_VerifyCrc32cRetryOnceAndDropCacheOnValidatorChange", async () => {
    const { server, cache, changed, transport } = setup();

    // The window is verified and returned with the served range and validator.
    const first = await transport.read(REF, fwd(1));
    expect(first.range).toMatchObject({ startLine: 1, endLine: 10, startByte: 0, bof: true, eof: false });
    expect(first.text.startsWith("line 1\n")).toBe(true);
    expect(first.validator).toMatch(/^\d+:1:7$/);

    // One bad CRC is retried and succeeds; two in a row are a transport-class failure.
    server.faults.badCrc = 1;
    const callsBefore = server.calls.length;
    expect((await transport.read(REF, fwd(40))).range.startLine).toBe(40);
    expect(server.calls.length - callsBefore).toBe(2);
    server.faults.badCrc = 2;
    await expect(transport.read(REF, fwd(70))).rejects.toBeInstanceOf(TransportFailure);

    // A changed validator empties the file's chunks and raises the stale event.
    expect(cache.bytes).toBeGreaterThan(0);
    server.touch(REF.path, numberedLines(100) + "tail\n");
    await transport.read(REF, fwd(90));
    expect(changed).toHaveLength(1);
    expect(cache.get(REF, 1, 10)).toBeUndefined();
    expect(cache.get(REF, 90, 99)).toBeDefined();

    // Torn once is retried; torn twice is stale and nothing from it stays cached.
    server.faults.torn = 1;
    await expect(transport.read(REF, fwd(20))).resolves.toBeDefined();
    server.faults.torn = 2;
    await expect(transport.read(REF, fwd(60))).rejects.toMatchObject({ reason: "stale" });
    expect(cache.get(REF, 90, 99)).toBeUndefined();
  });

  it("connectUnary_should_VerifyCrcAcrossBomAndCrlf_And_SkipUnrebuildableWindows", async () => {
    const { server, transport } = setup({ "bom.txt": "﻿a\r\nb\r\n", "mixed.txt": "a\r\nb\nc\n" });
    server.faults.badCrc = 2;
    await expect(transport.read({ ...REF, path: "bom.txt" }, fwd(1))).rejects.toBeInstanceOf(TransportFailure);
    const ok = await transport.read({ ...REF, path: "bom.txt" }, fwd(1));
    expect(ok).toMatchObject({ text: "a\nb\n", hasBom: true, lineEnding: "crlf" });
    // MIXED windows cannot be rebuilt from the normalized text, so a bad CRC is not detectable and not enforced.
    server.faults.badCrc = 1;
    await expect(transport.read({ ...REF, path: "mixed.txt" }, fwd(1))).resolves.toMatchObject({ lineEnding: "mixed" });
  });

  it("connectUnary_should_MapTypedErrors_And_NeverRetryApplicationErrors", async () => {
    const { server, transport } = setup();
    server.faults.throwNext.push(busyError());
    await expect(transport.read(REF, fwd(1))).rejects.toMatchObject({ reason: "busy" });
    server.faults.throwNext.push(new ConnectError("line index is still being built; retry or read from line 1", Code.FailedPrecondition));
    await expect(transport.read(REF, fwd(500))).rejects.toMatchObject({ reason: "index_pending" });
    server.faults.throwNext.push(new ConnectError("file operation not served: FILE_OP_READ_LINES", Code.FailedPrecondition));
    await expect(transport.read(REF, fwd(1))).rejects.toMatchObject({ reason: "unsupported" });
    server.faults.throwNext.push(new ConnectError("gone", Code.Unavailable));
    await expect(transport.read(REF, fwd(1))).rejects.toBeInstanceOf(TransportFailure);
    await expect(transport.read({ ...REF, path: "nope" }, fwd(1))).rejects.toMatchObject({ reason: "not_found" });
    expect(server.calls).toHaveLength(5); // one call each: nothing was retried
  });

  it("connectUnary_should_ReadAllInOneTripWithPreferFull_And_FallBackToFloorUntilRangesAreServed", async () => {
    const { server, transport, floor } = setup({ "a.txt": "x\ny\n", "big.log": numberedLines(30_000, 40) });
    const all = await transport.read({ ...REF, path: "a.txt" }, { kind: "all" });
    expect(all).toMatchObject({ text: "x\ny\n", truncated: false });
    expect(server.calls).toHaveLength(1);
    expect(server.calls[0].preferFullIfLe).toBe(1024n * 1024n);
    expect((await transport.read(REF, { kind: "all" })).truncated).toBe(true);

    let served = false;
    const gated = setup({ "a.txt": "x" }, { rangeServed: () => served });
    await gated.transport.read({ ...REF, path: "small.txt" }, { kind: "all" });
    expect(gated.server.calls).toHaveLength(0);
    expect(gated.floor.calls).toEqual(["read:small.txt"]);
    await expect(gated.transport.read(REF, fwd(1))).rejects.toBeInstanceOf(Rejected);
    served = true;
    await gated.transport.read({ ...REF, path: "a.txt" }, { kind: "all" });
    expect(gated.server.calls).toHaveLength(1);
    expect(floor.calls).toEqual([]);
  });

  it("connectUnary_should_ServeCachedWindowsWithoutAnotherCall_And_EmitReadWindowWithoutPathOrOffset", async () => {
    const sink = createFakeAnalytics();
    const analytics = createFilesEmitter(sink.track, fakeClock().clock);
    const server = createFakeRangeServer({ "big.log": numberedLines(100) });
    const floor = createFakeTransport();
    const transport = createConnectUnaryTransport(server.client, { floor, cache: new ChunkCache(1 << 20), analytics });
    await transport.read(REF, fwd(1, 20));
    await transport.read(REF, fwd(5, 10));
    expect(server.calls).toHaveLength(1);
    const events = sink.events.filter((e) => e.name === "files.read.window");
    expect(events.map((e) => e.labels)).toEqual([
      expect.objectContaining({ transport: "connect_unary", outcome: "ok", cache: "miss" }),
      expect.objectContaining({ cache: "hit" }),
    ]);
    expect(JSON.stringify(events)).not.toContain("big.log");
    for (const e of events) expect(labelProblems(e)).toEqual([]);
  });

  it("connectUnary_should_StatWithAnalysisFlags_And_AbortPromptly", async () => {
    const { server, transport } = setup();
    expect(await transport.stat(REF, { wantAnalysis: true })).toMatchObject({ token: "-", analysis: "pending" });
    const ctl = new AbortController();
    ctl.abort();
    await expect(transport.read(REF, fwd(1), ctl.signal)).rejects.toBeDefined();
    expect(server.calls).toHaveLength(0);
  });

  it("lineSpan_should_MirrorTheServerWindowMath", () => {
    expect(lineSpan({ kind: "lines", anchorLine: 100, dir: "around", wantLines: 40, maxBytes: 1 })).toEqual({ first: 80, last: 119 });
    expect(lineSpan({ kind: "lines", anchorLine: 100, dir: "back", wantLines: 40, maxBytes: 1 })).toEqual({ first: 61, last: 100 });
    expect(lineSpan({ kind: "lines", anchorLine: 3, dir: "back", wantLines: 40, maxBytes: 1 }).first).toBe(1);
  });
});
