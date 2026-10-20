import { createFakeRangeServer } from "../../__tests__/fixtures/fakeRangeServer";
import { ChunkCache } from "../chunkCache";
import { createConnectUnaryTransport } from "../connectUnaryTransport";
import { createFullFileTransport } from "../fullFileTransport";
import { runTransportConformance, type ConformanceSubject } from "./transportConformance";

function connectUnarySubject(): ConformanceSubject {
  const server = createFakeRangeServer();
  const floor = createFullFileTransport(server.client);
  return { transport: createConnectUnaryTransport(server.client, { floor, cache: new ChunkCache(1 << 20) }), seed: (p, t) => void server.files.set(p, t) };
}

runTransportConformance("ConnectUnaryTransport over a fake ReadFileRange server", connectUnarySubject);

describe("full and connect_unary parity", () => {
  it("runTransportConformance_should_PassForFullAndConnectUnary_ReadAndPatchSides", async () => {
    // Read side: both rungs serve the same whole-file text and token shape; the patch side lands with Task 2.2.5c.
    const { transport, seed } = connectUnarySubject();
    seed("a.go", "package a\r\nfunc A() {}\r\n");
    const viaRange = await transport.read({ sessionId: "s1", path: "a.go" }, { kind: "all" });
    expect(viaRange).toMatchObject({ text: "package a\nfunc A() {}\n", lineEnding: "crlf", truncated: false, readOnly: "none" });
    expect(viaRange.range).toMatchObject({ startLine: 1, endLine: 2, bof: true, eof: true });
    expect(transport.capabilities().ops).toEqual(expect.arrayContaining(["stat", "read_all", "read_lines", "read_bytes"]));
    expect(transport.patch).toBeUndefined();
  });
});
