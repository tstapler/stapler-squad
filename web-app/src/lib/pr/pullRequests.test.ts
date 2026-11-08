import { create } from "@bufbuild/protobuf";
import { PrHoldSchema, PullRequestRefSchema } from "@/gen/session/v1/types_pb";
import { makePr } from "./__testUtils__/pullRequests";
import {
  prUrl,
  activePullRequest, earlierCount, holdsFor, isTerminal, moreCount, planRows, prKey, prRowState, prTarget, sortPullRequests,
} from "./pullRequests";

const DAY = 86_400_000;

const merged = (n: number, daysAgo: number) => makePr({ number: n, state: "merged", mergedAt: Date.now() - daysAgo * DAY });

describe("pullRequests helpers", () => {
  it("sortPullRequests_should_putActiveFirstThenOpenThenTerminalByRecency", () => {
    const prs = [merged(10, 4), makePr({ number: 14, createdAt: 2 }), merged(11, 2), makePr({ number: 12, isActive: true, createdAt: 1 }), makePr({ number: 13, createdAt: 5 })];
    expect(sortPullRequests(prs).map((p) => p.number)).toEqual([12, 13, 14, 11, 10]);
  });

  it("prRowState_should_separateDraftUnverifiedAndUnknown", () => {
    expect(prRowState(makePr({ number: 1, isDraft: true }))).toBe("draft");
    expect(prRowState(makePr({ number: 1, state: "unknown", outcomeUnverified: true }))).toBe("unverified");
    expect(prRowState(makePr({ number: 1, state: "unknown" }))).toBe("unknown");
    expect(isTerminal(makePr({ number: 1, state: "unknown", outcomeUnverified: true }))).toBe(true);
    expect(isTerminal(makePr({ number: 1, state: "unknown" }))).toBe(false);
  });

  it("planRows_should_collapseAllButTwoTerminalRows_When_FiveMergedAndOneOpen", () => {
    const prs = [makePr({ number: 12, isActive: true }), merged(11, 1), merged(10, 2), merged(9, 3), merged(8, 4)];
    const plan = planRows(prs, { expanded: false, compact: false });
    expect(plan.visible.map((p) => p.number)).toEqual([12, 11, 10]);
    expect(plan.hidden).toBe(2);
    expect(plan.disclosure).toBe("earlier");
    expect(earlierCount(prs)).toBe(2);
    expect(planRows(prs, { expanded: true, compact: false }).visible).toHaveLength(5);
  });

  it("planRows_should_showEverything_When_ThreeTerminalRowsOrFewer", () => {
    const plan = planRows([makePr({ number: 12, isActive: true }), merged(11, 1), merged(10, 2), merged(9, 3)], { expanded: false, compact: false });
    expect(plan).toMatchObject({ hidden: 0, disclosure: null });
    expect(plan.visible).toHaveLength(4);
  });

  it("planRows_should_capAtFiveRows_When_Compact", () => {
    const prs = [makePr({ number: 20, isActive: true }), ...[1, 2, 3, 4, 5, 6].map((n) => merged(n, n))];
    const plan = planRows(prs, { expanded: false, compact: true });
    expect(plan.visible).toHaveLength(5);
    expect(plan).toMatchObject({ hidden: 2, disclosure: "showAll" });
    expect(planRows(prs, { expanded: true, compact: true }).visible).toHaveLength(7);
  });

  it("activePullRequest_should_followServerFlagWithoutRecomputing", () => {
    const prs = [makePr({ number: 1 }), makePr({ number: 2, isActive: true, state: "merged" })];
    expect(activePullRequest(prs)?.number).toBe(2);
    expect(activePullRequest([makePr({ number: 1 })])).toBeUndefined();
  });

  it("moreCount_prKey_prTarget_should_nameThePr", () => {
    expect(moreCount([makePr({ number: 1 }), makePr({ number: 2 }), makePr({ number: 3 })])).toBe(2);
    expect(moreCount([])).toBe(0);
    expect(prKey(makePr({ number: 7 }))).toBe("github.com/acme/app#7");
    expect(prTarget(makePr({ number: 7 }))).toBe("#7 acme/app");
  });

  it("holdsFor_should_matchHoldsByRef", () => {
    const hold = create(PrHoldSchema, { pr: create(PullRequestRefSchema, { owner: "acme", repo: "app", number: 12 }) });
    expect(holdsFor([hold], makePr({ number: 12 }))).toHaveLength(1);
    expect(holdsFor([hold], makePr({ number: 13 }))).toHaveLength(0);
  });
});

describe("prUrl", () => {
  it("prefers_the_server_url", () => {
    expect(prUrl(makePr({ number: 3, url: "https://x/y" }))).toBe("https://x/y");
  });
  it("derives_from_ref_when_url_empty_including_ghe_host", () => {
    expect(prUrl(makePr({ number: 3, url: "" }))).toBe("https://github.com/acme/app/pull/3");
    expect(prUrl(makePr({ number: 3, url: "", host: "github.example.net" }))).toBe("https://github.example.net/acme/app/pull/3");
  });
});
