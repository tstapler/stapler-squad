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
