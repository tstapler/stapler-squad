import { Code, ConnectError, createClient, createRouterTransport } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import {
  FileAnalysisState, LineEnding, ReadOnlyReason, ReadOnlyReasonDetailSchema, SessionService,
} from "@/gen/session/v1/session_pb";
import { createFullFileTransport } from "../fullFileTransport";
import { Rejected, TransportFailure } from "../fileTransport";

const REF = { sessionId: "s1", path: "src/a.go" };
const BODY = { content: "package a\r\nfunc A() {}\r\n", size: 24n, versionToken: "v1:24:99:abc", lineEnding: LineEnding.CRLF, hasBom: true, readOnlyReason: ReadOnlyReason.NONE };

function setup(getFileContent: (req: { path: string }) => object = () => BODY) {
  const transport = createRouterTransport(({ service }) => {
    service(SessionService, {
      getFileContent: getFileContent as never,
      statFile: () => ({ versionToken: BODY.versionToken, size: BODY.size, readOnlyReason: ReadOnlyReason.NONE, analysis: FileAnalysisState.READY }),
    });
  });
  const client = createClient(SessionService, transport);
  return { client, full: createFullFileTransport(client) };
}

describe("FullFileTransport", () => {
  it("fullFileTransport_should_MatchDirectRpcResults_And_KeepTruncatedReadOnly", async () => {
    const { client, full } = setup();
    const direct = await client.getFileContent({ sessionId: REF.sessionId, path: REF.path });
    const viaPort = await full.read(REF, { kind: "all" });
    expect(viaPort).toMatchObject({
      text: direct.content, token: direct.versionToken, lineEnding: "crlf", hasBom: true, readOnly: "none", isBinary: false, truncated: false,
    });
    expect(viaPort.range).toMatchObject({ startLine: 1, endLine: 3, bof: true, eof: true });
    expect(full.capabilities().limits.hashCapBytes).toBe(1024 * 1024);
    expect(full.capabilities().ops).not.toContain("patch");

    const stat = await full.stat(REF);
    expect(stat).toEqual({ token: BODY.versionToken, size: 24, readOnly: "none", analysis: "ready" });

    const truncated = setup(() => ({ ...BODY, isTruncated: true, readOnlyReason: ReadOnlyReason.UNSPECIFIED }));
    expect(await truncated.full.read(REF, { kind: "all" })).toMatchObject({ truncated: true, readOnly: "truncated" });
    const binary = setup(() => ({ ...BODY, content: "", isBinary: true, readOnlyReason: ReadOnlyReason.BINARY }));
    expect(await binary.full.read(REF, { kind: "all" })).toMatchObject({ isBinary: true, readOnly: "binary" });
  });

  it("fullFileTransport_should_RefuseWindowedReadsAndPatch_And_MapErrorsToTypedReasons", async () => {
    const { full } = setup();
    await expect(full.read(REF, { kind: "lines", anchorLine: 1, dir: "fwd", wantLines: 5, maxBytes: 10 })).rejects.toMatchObject({ reason: "unsupported" });
    expect(full.patch).toBeUndefined();
    await expect(full.writeFull(REF, { text: "x", baseToken: "t" })).rejects.toBeInstanceOf(Rejected);

    const missing = setup(() => { throw new ConnectError("nope", Code.NotFound); });
    await expect(missing.full.read(REF, { kind: "all" })).rejects.toMatchObject({ reason: "not_found" });

    const down = setup(() => { throw new ConnectError("down", Code.Unavailable); });
    await expect(down.full.read(REF, { kind: "all" })).rejects.toBeInstanceOf(TransportFailure);
    const slow = setup(() => { throw new ConnectError("slow", Code.DeadlineExceeded); });
    await expect(slow.full.read(REF, { kind: "all" })).rejects.toMatchObject({ reason: "deadline" });

    const detail = create(ReadOnlyReasonDetailSchema, { reason: ReadOnlyReason.SYMLINK });
    const link = setup(() => { throw new ConnectError("link", Code.PermissionDenied, undefined, [{ desc: ReadOnlyReasonDetailSchema, value: detail }]); });
    await expect(link.full.read(REF, { kind: "all" })).rejects.toMatchObject({ reason: "symlink" });
  });
});
