import { classifyFindPrHits, prLabel, prSearchHref, sourceLabel } from "./findSessionsForPr";

const pr = { host: "github.com", owner: "acme", repo: "app", number: 12 };

describe("findSessionsForPr", () => {
  it("prSearchHref_should_encodeOnlyTheHash_When_buildingTheArtifactsUrl", () => {
    expect(prSearchHref(pr)).toBe("/artifacts?pr=acme/app%2312");
  });

  it("prSearchHref_should_escapeUnsafeSegments_When_ownerHasSpecialCharacters", () => {
    expect(prSearchHref({ ...pr, owner: "a&b" })).toBe("/artifacts?pr=a%26b/app%2312");
  });

  it("prLabel_should_renderOwnerRepoHashNumber", () => {
    expect(prLabel(pr)).toBe("acme/app#12");
  });

  it("sourceLabel_should_mapSessionPrToTrackedAndArtifactToSeenInTranscript", () => {
    expect(sourceLabel("session_pr")).toBe("Tracked");
    expect(sourceLabel("artifact")).toBe("Seen in transcript");
  });

  it("classifyFindPrHits_should_openTheSession_When_exactlyOneHit", () => {
    expect(classifyFindPrHits([{ sessionId: "u1", title: "one", source: "session_pr" }], pr)).toEqual({
      kind: "single",
      sessionId: "u1",
      title: "one",
      sourceLabel: "Tracked",
      viewAllHref: "/artifacts?pr=acme/app%2312",
    });
  });

  it.each([[0], [2]])("classifyFindPrHits_should_goToTheList_When_%i_hits", (n) => {
    const hits = Array.from({ length: n }, (_, i) => ({ sessionId: `u${i}`, title: `t${i}`, source: "artifact" }));
    expect(classifyFindPrHits(hits, pr)).toEqual({ kind: "list", href: "/artifacts?pr=acme/app%2312" });
  });
});
