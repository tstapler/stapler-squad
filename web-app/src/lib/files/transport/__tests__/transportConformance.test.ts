import { Code, ConnectError, createClient, createRouterTransport } from "@connectrpc/connect";
import { ReadOnlyReason, SessionService } from "@/gen/session/v1/session_pb";
import { createFakeTransport } from "../../__tests__/fixtures/fakeTransport";
import { createFullFileTransport } from "../fullFileTransport";
import { runTransportConformance } from "./transportConformance";

runTransportConformance("FullFileTransport over the Connect handlers", () => {
  const files = new Map<string, string>();
  const need = (path: string) => {
    const text = files.get(path);
    if (text === undefined) throw new ConnectError("missing", Code.NotFound);
    return text;
  };
  const transport = createRouterTransport(({ service }) => {
    service(SessionService, {
      getFileContent: (req) => ({ content: need(req.path), size: BigInt(need(req.path).length), versionToken: "v1:1:1:x", readOnlyReason: ReadOnlyReason.NONE }),
      statFile: (req) => ({ versionToken: "v1:1:1:x", size: BigInt(need(req.path).length), readOnlyReason: ReadOnlyReason.NONE }),
    });
  });
  return { transport: createFullFileTransport(createClient(SessionService, transport)), seed: (p, t) => void files.set(p, t) };
});

runTransportConformance("in-memory fake with PATCH and WRITE_FULL", () => {
  const fake = createFakeTransport({ id: "connect_unary", ops: ["stat", "read_all", "write_full", "patch"], withPatch: true });
  return { transport: fake, seed: fake.seed };
});
