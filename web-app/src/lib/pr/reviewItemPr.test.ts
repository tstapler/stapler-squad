import { create } from "@bufbuild/protobuf";
import { ReviewItemSchema, SessionSchema, UnfinishedWorktreeSchema } from "@/gen/session/v1/types_pb";
import { makePr } from "./__testUtils__/pullRequests";
import { legacyWorktreeBadgeProps } from "./legacyPullRequest";
import { pullRequestsOfSessions, reviewItemHasOpenPr, reviewItemPr, sessionsForPullRequest } from "./reviewItemPr";

const item = (githubPrUrl = "") => create(ReviewItemSchema, { sessionId: "s1", githubPrUrl });
const session = (prs = [makePr({ number: 12 })]) => create(SessionSchema, { id: "s1", title: "refactor-auth", pullRequests: prs });

describe("reviewItemHasOpenPr", () => {
  it("reviewItemHasOpenPr_should_beTrue_When_AnyListedPrIsOpen", () => {
    const s = session([makePr({ number: 11, state: "merged" }), makePr({ number: 12 })]);
    expect(reviewItemHasOpenPr(item(), s, true)).toBe(true);
  });

  it("reviewItemHasOpenPr_should_beFalse_When_EveryListedPrIsTerminal_EvenWithALegacyUrl", () => {
    const s = session([makePr({ number: 11, state: "merged" }), makePr({ number: 13, state: "closed" })]);
    expect(reviewItemHasOpenPr(item("https://github.com/acme/app/pull/11"), s, true)).toBe(false);
  });

  it("reviewItemHasOpenPr_should_useTheLegacyUrl_When_FlagOffOrListEmpty", () => {
    expect(reviewItemHasOpenPr(item("https://github.com/acme/app/pull/9"), session(), false)).toBe(true);
    expect(reviewItemHasOpenPr(item("https://github.com/acme/app/pull/9"), session([]), true)).toBe(true);
    expect(reviewItemHasOpenPr(item(), undefined, true)).toBe(false);
  });
});

describe("reviewItemPr", () => {
  it("reviewItemPr_should_nameTheActivePr_When_MultiOn", () => {
    const s = session([makePr({ number: 11, state: "merged" }), makePr({ number: 12, isActive: true })]);
    expect(reviewItemPr(item("https://github.com/acme/app/pull/1"), s, true)).toEqual({ url: "https://github.com/acme/app/pull/12", number: 12 });
  });

  it("reviewItemPr_should_returnTheLegacyUrlWithoutANumber_When_FlagOff", () => {
    expect(reviewItemPr(item("https://github.com/acme/app/pull/1"), session(), false)).toEqual({ url: "https://github.com/acme/app/pull/1" });
    expect(reviewItemPr(item(), session(), false)).toBeUndefined();
  });
});

describe("sessionsForPullRequest", () => {
  const other = create(SessionSchema, { id: "s2", title: "rework", pullRequests: [makePr({ number: 12 }), makePr({ number: 40 })] });

  it("sessionsForPullRequest_should_listEverySessionTrackingThePr", () => {
    const hit = sessionsForPullRequest([session(), other], { host: "", owner: "acme", repo: "app", number: 12 });
    expect(hit.map((s) => s.id)).toEqual(["s1", "s2"]);
  });

  it("sessionsForPullRequest_should_matchGithubComWhetherTheHostIsEmptyOrSpelledOut", () => {
    expect(sessionsForPullRequest([session()], { host: "github.com", owner: "acme", repo: "app", number: 12 })).toHaveLength(1);
    expect(sessionsForPullRequest([session()], { host: "ghe.example", owner: "acme", repo: "app", number: 12 })).toHaveLength(0);
  });

  it("sessionsForPullRequest_should_beEmpty_When_NoSessionTracksIt", () => {
    expect(sessionsForPullRequest([session()], { host: "", owner: "acme", repo: "app", number: 99 })).toEqual([]);
  });
});

describe("legacyWorktreeBadgeProps", () => {
  it("legacyWorktreeBadgeProps_should_carryTheWorktreePrScalars", () => {
    const wt = create(UnfinishedWorktreeSchema, { githubPrNumber: 5, githubPrUrl: "https://github.com/acme/app/pull/5", githubPrState: "open", githubPrPriority: "needs_review" });
    expect(legacyWorktreeBadgeProps(wt)).toEqual({ prNumber: 5, prUrl: "https://github.com/acme/app/pull/5", prState: "open", prPriority: "needs_review" });
  });
});

describe("pullRequestsOfSessions", () => {
  it("pullRequestsOfSessions_should_dedupeByPrAcrossTheNamedSessions", () => {
    const a = create(SessionSchema, { id: "s1", title: "one", pullRequests: [makePr({ number: 12 })] });
    const b = create(SessionSchema, { id: "s2", title: "two", pullRequests: [makePr({ number: 12 }), makePr({ number: 40 })] });
    const c = create(SessionSchema, { id: "s3", title: "three", pullRequests: [makePr({ number: 77 })] });
    expect(pullRequestsOfSessions([a, b, c], ["s1", "two"]).map((p) => p.number)).toEqual([12, 40]);
  });
});
