import { create } from "@bufbuild/protobuf";
import { SessionService, ListFilesRequestSchema, StatFileRequestSchema } from "@/gen/session/v1/session_pb";
import { InsightsService } from "@/gen/session/v1/insights_pb";
import allowlist from "@/lib/transport/__fixtures__/ws-unary-allowlist.json";
import { IDEMPOTENT_METHODS, buildDedupeKey, isCoalescible } from "./idempotentMethods";

describe("idempotentMethodTable", () => {
  it("idempotentMethodTable_should_ListOnlyMethodsInServiceDescriptor_And_KeepSubsetOrdering_When_Validated", () => {
    const known = new Set([...SessionService.methods, ...InsightsService.methods].map((m) => m.name));
    const { coalescible, replayable, wsEligible, cacheable } = IDEMPOTENT_METHODS;
    for (const name of [...coalescible, ...replayable, ...wsEligible, ...cacheable]) {
      expect(known.has(name)).toBe(true);
    }
    expect(coalescible).toHaveLength(14);
    expect(wsEligible).toHaveLength(9);
    for (const name of replayable) expect(coalescible).toContain(name);
    for (const name of wsEligible) expect(coalescible).toContain(name);
    expect([...wsEligible].sort()).toEqual([...allowlist.wsUnaryAllowed].sort());
    expect(wsEligible).not.toContain("GetFeatureFlags");
    const { batchEligible } = IDEMPOTENT_METHODS;
    for (const name of batchEligible) {
      expect(known.has(name)).toBe(true);
      expect(wsEligible).toContain(name);
    }
    for (const slow of ["GetVCSStatus", "GetSessionDiff", "GetFileContent"]) {
      expect(batchEligible).not.toContain(slow);
    }
    expect([...batchEligible].sort()).toEqual([...allowlist.batchAllowed].sort());
    expect(isCoalescible("GetFeatureFlags")).toBe(true);
    expect(InsightsService.typeName).toBe("session.v1.InsightsService");
    for (const name of ["GetInsightsSummary", "ListAliases", "ListSessions", "ListPendingApprovals"]) {
      expect(isCoalescible(name)).toBe(true);
      expect(replayable).not.toContain(name);
      expect(wsEligible).not.toContain(name);
    }
    expect(isCoalescible("UpdateFeatureFlag")).toBe(false);
    expect(isCoalescible("NoSuchMethod")).toBe(false);
  });

  it("buildDedupeKey_should_DifferByMethodAndArgs_And_MatchForEqualArgs", () => {
    const listFiles = SessionService.method.listFiles;
    const a = create(ListFilesRequestSchema, { sessionId: "s", path: "." });
    const b = create(ListFilesRequestSchema, { sessionId: "s", path: "." });
    const c = create(ListFilesRequestSchema, { sessionId: "s", path: "docs" });
    expect(buildDedupeKey(listFiles, a)).toBe(buildDedupeKey(listFiles, b));
    expect(buildDedupeKey(listFiles, a)).not.toBe(buildDedupeKey(listFiles, c));
    const stat = create(StatFileRequestSchema, { sessionId: "s", path: "." });
    expect(buildDedupeKey(SessionService.method.statFile, stat)).not.toBe(buildDedupeKey(listFiles, a));
  });
});
