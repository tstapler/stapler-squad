// Shared contract every FileTransport adapter must meet (ADR-036). Stories 1.6.3, 2.2.5 and 2.6.2 add cases.
import { Rejected, type FileTransport } from "../fileTransport";

export interface ConformanceSubject {
  transport: FileTransport;
  /** Creates (or replaces) a file the adapter can read. */
  seed(path: string, text: string): void;
}

const REF = (path: string) => ({ sessionId: "s1", path });

export function runTransportConformance(name: string, make: () => ConformanceSubject): void {
  describe(`transport conformance: ${name}`, () => {
    it("capabilityHonesty_should_ServeAdvertisedOpsAndRefuseTheRest", async () => {
      const { transport, seed } = make();
      seed("a.go", "package a\n");
      const caps = transport.capabilities();
      expect(caps.protocolVersion).toBeGreaterThanOrEqual(1);
      expect(caps.limits.hashCapBytes).toBeGreaterThan(0);

      if (caps.ops.includes("stat")) expect((await transport.stat(REF("a.go"))).token).not.toBe("");
      if (caps.ops.includes("read_all")) {
        const r = await transport.read(REF("a.go"), { kind: "all" });
        expect(r.text).toBe("package a\n");
        expect(r.token).not.toBe("");
      }
      expect(typeof transport.patch === "function").toBe(caps.ops.includes("patch"));
      if (!caps.ops.includes("read_lines")) {
        await expect(transport.read(REF("a.go"), { kind: "lines", anchorLine: 1, dir: "fwd", wantLines: 5, maxBytes: 100 })).rejects.toBeInstanceOf(Rejected);
      }
      if (caps.ops.includes("write_full")) {
        await transport.writeFull(REF("w.go"), { text: "w", baseToken: "-" });
        expect((await transport.read(REF("w.go"), { kind: "all" })).text).toBe("w");
      } else {
        await expect(transport.writeFull(REF("w.go"), { text: "w", baseToken: "-" })).rejects.toBeInstanceOf(Rejected);
      }
    });

    it("rangeEdges_should_ClampAtBothEndsAndStayLineAligned_When_ReadLinesIsAdvertised", async () => {
      const { transport, seed } = make();
      if (!transport.capabilities().ops.includes("read_lines")) return;
      seed("n.txt", Array.from({ length: 50 }, (_, i) => `l${i + 1}\n`).join(""));
      const lines = (r: { text: string }) => r.text.split("\n").filter(Boolean);
      const at = (anchorLine: number, dir: "around" | "fwd" | "back") => transport.read(REF("n.txt"), { kind: "lines", anchorLine, dir, wantLines: 10, maxBytes: 4096 });

      const head = await at(1, "around");
      expect(head.range).toMatchObject({ startLine: 1, endLine: 10, startByte: 0, bof: true, eof: false });
      expect(lines(head)).toHaveLength(10);
      const tail = await at(50, "fwd");
      expect(tail.range).toMatchObject({ startLine: 50, endLine: 50, eof: true });
      expect(lines(tail)).toEqual(["l50"]);
      const before = await at(20, "back");
      expect(before.range).toMatchObject({ startLine: 11, endLine: 20 });
      expect(lines(before)[0]).toBe("l11");
      expect(head.range.endByte).toBe(new TextEncoder().encode(head.text).length);
    });

    it("errorMapping_should_RejectMissingFilesWithATypedReason", async () => {
      const { transport } = make();
      await expect(transport.read(REF("missing.go"), { kind: "all" })).rejects.toMatchObject({ reason: "not_found" });
      await expect(transport.stat(REF("missing.go"))).rejects.toMatchObject({ reason: "not_found" });
    });

    it("abort_should_RejectWithoutHanging_When_SignalAlreadyAborted", async () => {
      const { transport, seed } = make();
      seed("a.go", "x");
      const ctl = new AbortController();
      ctl.abort();
      await expect(transport.read(REF("a.go"), { kind: "all" }, ctl.signal)).rejects.toBeDefined();
    });
  });
}
