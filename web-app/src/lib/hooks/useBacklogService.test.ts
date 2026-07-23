/**
 * Unit tests for mapBacklogItem's triageStatus derivation (useBacklogService.ts).
 *
 * These call the real exported mapping function with realistic proto-shaped
 * fixtures (built via `create(Schema, ...)`) and assert on its output —
 * replacing a prior test that hardcoded `triageStatus` directly on the
 * assertion target, which could never fail regardless of the mapping logic.
 *
 * Derivation rules under test (see mapBacklogItem in useBacklogService.ts):
 *   - No triage session at all               -> triageStatus undefined
 *   - Most recent triage session has no endedAt, item status is "idea"
 *                                             -> "running"
 *   - Most recent triage session has no endedAt, item status !== "idea"
 *     (orphan detection — session died without recording its end)
 *                                             -> "failed"
 *   - Most recent triage session has endedAt and a non-empty result summary
 *                                             -> "completed"
 *   - Most recent triage session has endedAt but no (or empty) result summary
 *                                             -> "failed"
 */
import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { BacklogItemSchema, ItemSessionSchema, TriageResultSchema } from "@/gen/session/v1/backlog_pb";
import { mapBacklogItem } from "./useBacklogService";

describe("mapBacklogItem triageStatus derivation", () => {
  it("mapBacklogItem_should_LeaveTriageStatusUndefined_When_NoTriageSessionExists", () => {
    const proto = create(BacklogItemSchema, {
      id: "item-1",
      title: "Test item",
      status: "idea",
      itemSessions: [],
    });

    const item = mapBacklogItem(proto);

    expect(item.triageStatus).toBeUndefined();
  });

  it("mapBacklogItem_should_DeriveRunning_When_TriageSessionHasNoEndedAtAndItemIsIdea", () => {
    const proto = create(BacklogItemSchema, {
      id: "item-2",
      title: "Test item",
      status: "idea",
      itemSessions: [
        create(ItemSessionSchema, {
          id: "session-entity-2",
          sessionUuid: "uuid-2",
          sessionRole: "triage",
        }),
      ],
    });

    const item = mapBacklogItem(proto);

    expect(item.triageStatus).toBe("running");
  });

  it("mapBacklogItem_should_DeriveFailed_When_TriageSessionOrphanedPastIdeaStatus", () => {
    // Orphan detection: item advanced past "idea" but the triage session never
    // recorded an endedAt — the session died without cleanly finishing.
    const proto = create(BacklogItemSchema, {
      id: "item-3",
      title: "Test item",
      status: "ready",
      itemSessions: [
        create(ItemSessionSchema, {
          id: "session-entity-3",
          sessionUuid: "uuid-3",
          sessionRole: "triage",
        }),
      ],
    });

    const item = mapBacklogItem(proto);

    expect(item.triageStatus).toBe("failed");
  });

  it("mapBacklogItem_should_DeriveCompleted_When_TriageSessionEndedWithNonEmptySummary", () => {
    const proto = create(BacklogItemSchema, {
      id: "item-4",
      title: "Test item",
      status: "idea",
      itemSessions: [
        create(ItemSessionSchema, {
          id: "session-entity-4",
          sessionUuid: "uuid-4",
          sessionRole: "triage",
          endedAt: timestampFromDate(new Date()),
          triageResult: create(TriageResultSchema, {
            summary: "Item looks implementable.",
          }),
        }),
      ],
    });

    const item = mapBacklogItem(proto);

    expect(item.triageStatus).toBe("completed");
    expect(item.triageResult?.summary).toBe("Item looks implementable.");
  });

  it("mapBacklogItem_should_DeriveFailed_When_TriageSessionEndedWithoutResult", () => {
    const proto = create(BacklogItemSchema, {
      id: "item-5",
      title: "Test item",
      status: "idea",
      itemSessions: [
        create(ItemSessionSchema, {
          id: "session-entity-5",
          sessionUuid: "uuid-5",
          sessionRole: "triage",
          endedAt: timestampFromDate(new Date()),
          // no triageResult — session ended without producing one
        }),
      ],
    });

    const item = mapBacklogItem(proto);

    expect(item.triageStatus).toBe("failed");
    expect(item.triageResult).toBeUndefined();
  });

  it("mapBacklogItem_should_DeriveFailed_When_TriageResultSummaryIsEmpty", () => {
    const proto = create(BacklogItemSchema, {
      id: "item-6",
      title: "Test item",
      status: "idea",
      itemSessions: [
        create(ItemSessionSchema, {
          id: "session-entity-6",
          sessionUuid: "uuid-6",
          sessionRole: "triage",
          endedAt: timestampFromDate(new Date()),
          triageResult: create(TriageResultSchema, { summary: "" }),
        }),
      ],
    });

    const item = mapBacklogItem(proto);

    expect(item.triageStatus).toBe("failed");
  });

  it("mapBacklogItem_should_UseMostRecentTriageSession_When_MultipleTriageSessionsExist", () => {
    const proto = create(BacklogItemSchema, {
      id: "item-7",
      title: "Test item",
      status: "idea",
      itemSessions: [
        create(ItemSessionSchema, {
          id: "session-entity-7a",
          sessionUuid: "uuid-7a",
          sessionRole: "triage",
          endedAt: timestampFromDate(new Date(Date.now() - 60_000)),
          // first attempt ended without a result
        }),
        create(ItemSessionSchema, {
          id: "session-entity-7b",
          sessionUuid: "uuid-7b",
          sessionRole: "triage",
          endedAt: timestampFromDate(new Date()),
          triageResult: create(TriageResultSchema, { summary: "Retried and succeeded." }),
        }),
      ],
    });

    const item = mapBacklogItem(proto);

    expect(item.triageStatus).toBe("completed");
    expect(item.triageResult?.summary).toBe("Retried and succeeded.");
  });
});
