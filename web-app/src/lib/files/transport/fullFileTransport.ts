import type { Client } from "@connectrpc/connect";
import type { SessionService } from "@/gen/session/v1/session_pb";
import { Rejected, type FileCapabilities, type FileTransport, type ReadResult } from "./fileTransport";
import { guarded, lineEndingOf, readOnlyOf, statVia } from "./wireMapping";

const MIB = 1024 * 1024;

/** Today's behavior: whole-file read over GetFileContent, StatFile, and a 1 MiB editable ceiling. */
const CAPABILITIES: FileCapabilities = {
  protocolVersion: 1,
  features: [],
  ops: ["stat", "read_all"],
  wires: ["connect_unary"],
  capsEpoch: 0,
  limits: {
    maxRangeBytes: 0, maxRangeLines: 0, preferredChunkBytes: 0, maxPatchOps: 0, maxPatchBodyBytes: 0,
    maxInsertBytes: 0, maxWriteFullBytes: 0, hashCapBytes: MIB, maxConcurrentRanges: 0,
  },
};

type FileClient = Pick<Client<typeof SessionService>, "getFileContent" | "statFile">;

export function createFullFileTransport(client: FileClient): FileTransport {
  return {
    id: "full",
    capabilities: () => CAPABILITIES,
    stat: (ref, o, signal) => statVia(client, ref, o, signal),
    async read(ref, req, signal): Promise<ReadResult> {
      if (req.kind !== "all") throw new Rejected("unsupported", `${req.kind} reads need a higher transport`);
      const r = await guarded(client.getFileContent({ sessionId: ref.sessionId, path: ref.path }, { signal }));
      const lines = r.content === "" ? 1 : r.content.split("\n").length;
      return {
        text: r.content, token: r.versionToken, size: Number(r.size), lineEnding: lineEndingOf(r.lineEnding),
        hasBom: r.hasBom, readOnly: readOnlyOf(r.readOnlyReason, r.isBinary, r.isTruncated), isBinary: r.isBinary,
        truncated: r.isTruncated,
        range: { startLine: 1, endLine: lines, startByte: 0, endByte: Number(r.size), bof: true, eof: !r.isTruncated },
      };
    },
    // The WriteFile RPC lands with Story 2.1.2; until then the floor cannot save.
    writeFull: () => Promise.reject(new Rejected("unsupported", "WriteFile is not available yet")),
  };
}
