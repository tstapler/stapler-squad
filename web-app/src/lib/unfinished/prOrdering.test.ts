import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { UserPRSchema, type UserPR } from "@/gen/session/v1/types_pb";
import { orderPRs, severityRank, type PRSortBy } from "./prOrdering";

let n = 0;
function pr(over: MessageInitShape<typeof UserPRSchema> & { updated?: number }): UserPR {
  const { updated = 0, ...rest } = over;
  return create(UserPRSchema, {
    owner: "acme",
    repo: "web",
    number: ++n,
    host: "github.com",
    checkConclusion: "success",
    detailsLoaded: true,
    unresolvedThreadCount: 0,
    hasMergeConflict: false,
    updatedAt: timestampFromDate(new Date(updated * 1000)),
    ...rest,
  });
}

const layout = (groups: ReturnType<typeof orderPRs>) =>
  groups.map((g) => [g.key, g.prs.map((p) => p.title)] as const);

describe("orderPRs", () => {
  it("orderPRs_should_RankFailingGroupFirstAcrossRepos_When_AttentionFirstAndOtherSortsDocumented", () => {
    const green = pr({ title: "A green", updated: 900, approvedCount: 1 });
    const failing = pr({ title: "B failing", owner: "zeta", repo: "api", checkConclusion: "failure", updated: 100 });
    const threads = pr({ title: "C threads", owner: "mid", repo: "svc", unresolvedThreadCount: 2, updated: 500 });
    const changes = pr({ title: "D changes", owner: "mid", repo: "svc", changesReqCount: 1, updated: 800 });
    const conflict = pr({ title: "E conflict", owner: "zeta", repo: "api", hasMergeConflict: true, updated: 50 });
    const draftFail = pr({ title: "F draft", checkConclusion: "failure", isDraft: true, updated: 950 });
    const all = [green, failing, threads, changes, conflict, draftFail];

    expect([failing, conflict, threads, changes, green, draftFail].map(severityRank)).toEqual([3, 3, 2, 1, 0, 0]);

    // attention-first: group rank desc (zeta 3, mid 2, acme 0); cards rank desc then newest first
    expect(layout(orderPRs(all, "attention-first"))).toEqual([
      ["github.com/zeta/api", ["B failing", "E conflict"]],
      ["github.com/mid/svc", ["C threads", "D changes"]],
      ["github.com/acme/web", ["F draft", "A green"]],
    ]);

    // repo A-Z: alphabetical by host/owner/repo; cards by rank desc then updated desc
    expect(layout(orderPRs(all, "repo")).map(([k]) => k)).toEqual([
      "github.com/acme/web",
      "github.com/mid/svc",
      "github.com/zeta/api",
    ]);
    expect(layout(orderPRs(all, "repo"))[2][1]).toEqual(["B failing", "E conflict"]);

    // updated down: groups by newest member, cards by updated only (rank ignored)
    expect(layout(orderPRs(all, "updated-desc"))).toEqual([
      ["github.com/acme/web", ["F draft", "A green"]],
      ["github.com/mid/svc", ["D changes", "C threads"]],
      ["github.com/zeta/api", ["B failing", "E conflict"]],
    ]);

    // updated up: groups by oldest member, cards oldest first
    expect(layout(orderPRs(all, "updated-asc"))).toEqual([
      ["github.com/zeta/api", ["E conflict", "B failing"]],
      ["github.com/mid/svc", ["C threads", "D changes"]],
      ["github.com/acme/web", ["A green", "F draft"]],
    ]);

    // ci status: groups by worst CI state, cards by CI state then updated desc
    expect(layout(orderPRs(all, "ci-status"))).toEqual([
      ["github.com/acme/web", ["F draft", "A green"]],
      ["github.com/zeta/api", ["B failing", "E conflict"]],
      ["github.com/mid/svc", ["D changes", "C threads"]],
    ]);
  });

  it("orderPRs_should_PlaceFailingPRFirst_When_GreenApprovedPRInOtherRepoIsNewer", () => {
    const a = pr({ title: "A", approvedCount: 1, updated: 999 });
    const b = pr({ title: "B", owner: "zeta", repo: "api", checkConclusion: "failure", updated: 1 });
    expect(orderPRs([a, b], "attention-first")[0].key).toBe("github.com/zeta/api");
  });

  it("orderPRs_should_SeparateSameOwnerRepoOnDifferentHosts_When_Grouping", () => {
    const g = pr({ title: "gh" });
    const e = pr({ title: "ghe", host: "ghe.corp" });
    const sorts: PRSortBy[] = ["attention-first", "repo"];
    for (const s of sorts) expect(orderPRs([g, e], s)).toHaveLength(2);
  });
});
