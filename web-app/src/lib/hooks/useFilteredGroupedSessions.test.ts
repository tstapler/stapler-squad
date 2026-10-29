/**
 * Characterization tests (Story 1.4.1b): the "updatedAt" sort keeps reading Session.updatedAt
 * (whole-second precision), independent of lastMeaningfulOutput, which only "lastActivity" uses.
 */
import { renderHook } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { SessionSchema, SessionStatus } from "@/gen/session/v1/types_pb";
import { GroupingStrategy } from "@/lib/grouping/strategies";
import { useFilteredGroupedSessions, type SortDir, type SortField } from "./useFilteredGroupedSessions";

const ts = (seconds: number, nanos = 0) => ({ seconds: BigInt(seconds), nanos });

function session(id: string, updated: number, output: number) {
  return create(SessionSchema, {
    id,
    title: id,
    status: SessionStatus.ACTIVE,
    updatedAt: ts(updated),
    lastMeaningfulOutput: ts(output),
  });
}

function sortedIds(sessions: ReturnType<typeof session>[], sortField: SortField, sortDir: SortDir) {
  const { result } = renderHook(() =>
    useFilteredGroupedSessions({
      sessions,
      searchQuery: "",
      selectedStatus: "all",
      selectedCategory: "all",
      selectedTag: "all",
      hidePaused: false,
      showArchived: false,
      showHidden: false,
      filterNeedsApproval: false,
      pendingDeleteIds: new Set<string>(),
      sortField,
      sortDir,
      costById: new Map<string, number>(),
      groupingStrategy: GroupingStrategy.None,
      staleThresholdMinutes: 30,
      staleRecomputeTick: 0,
    })
  );
  return result.current.sortedSessions.map((s) => s.id);
}

describe("useFilteredGroupedSessions updatedAt sort", () => {
  // "touched" was renamed recently (new updatedAt) but has the oldest output.
  const touched = session("touched", 1000, 10);
  const active = session("active", 500, 900);
  const quiet = session("quiet", 100, 100);

  it("sorts by updatedAt descending, ignoring lastMeaningfulOutput", () => {
    expect(sortedIds([quiet, active, touched], "updatedAt", "desc")).toEqual(["touched", "active", "quiet"]);
  });

  it("sorts by updatedAt ascending", () => {
    expect(sortedIds([touched, active, quiet], "updatedAt", "asc")).toEqual(["quiet", "active", "touched"]);
  });

  it("lastActivity sort uses lastMeaningfulOutput, not updatedAt", () => {
    expect(sortedIds([quiet, touched, active], "lastActivity", "desc")).toEqual(["active", "quiet", "touched"]);
  });

  it("treats sub-second updatedAt differences as equal (whole-second precision)", () => {
    const a = create(SessionSchema, { id: "a", status: SessionStatus.ACTIVE, updatedAt: ts(100, 900_000_000) });
    const b = create(SessionSchema, { id: "b", status: SessionStatus.ACTIVE, updatedAt: ts(100, 100_000_000) });
    expect(sortedIds([b, a], "updatedAt", "desc")).toEqual(["b", "a"]);
    expect(sortedIds([a, b], "updatedAt", "desc")).toEqual(["a", "b"]);
  });
});
