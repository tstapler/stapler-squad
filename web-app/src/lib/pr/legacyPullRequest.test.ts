import { create } from "@bufbuild/protobuf";
import { SessionSchema } from "@/gen/session/v1/types_pb";
import { makePr } from "./__testUtils__/pullRequests";
import { displayedPullRequest, legacyBadgeProps, legacyPullRequest } from "./legacyPullRequest";

const legacy = () => create(SessionSchema, {
  githubOwner: "acme", githubRepo: "app", githubPrNumber: 7, githubPrUrl: "https://github.com/acme/app/pull/7", githubPrState: "open", githubCheckConclusion: "success",
  pullRequests: [makePr({ number: 12 }), makePr({ number: 13, isActive: true, checkConclusion: "failure" })],
});

describe("legacyPullRequest", () => {
  it("legacyBadgeProps_should_carryTheSingleBadgeInputs", () => {
    expect(legacyBadgeProps(legacy())).toMatchObject({ prNumber: 7, prUrl: "https://github.com/acme/app/pull/7", owner: "acme", repo: "app" });
  });

  it("legacyPullRequest_should_beUndefined_When_NoPrNumber", () => {
    expect(legacyPullRequest(create(SessionSchema, {}))).toBeUndefined();
    expect(legacyPullRequest(legacy())).toMatchObject({ number: 7, state: "open" });
  });

  it("displayedPullRequest_should_pickTheServerActivePr_When_MultiOn_AndLegacyWhenOff", () => {
    expect(displayedPullRequest(legacy(), true)).toMatchObject({ number: 13, checkConclusion: "failure" });
    expect(displayedPullRequest(legacy(), false)).toMatchObject({ number: 7, checkConclusion: "success" });
  });

  it("displayedPullRequest_should_fallBackToLegacy_When_MultiOnAndListEmpty", () => {
    const s = legacy();
    s.pullRequests = [];
    expect(displayedPullRequest(s, true)?.number).toBe(7);
    expect(displayedPullRequest(create(SessionSchema, {}), true)).toBeUndefined();
  });
});
