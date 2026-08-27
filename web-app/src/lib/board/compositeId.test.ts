import { makeCompositeId, parseCompositeId } from "./compositeId";

describe("makeCompositeId / parseCompositeId", () => {
  it("makeCompositeId_should_JoinRowKeyAndEntityIdWithColon_When_Called", () => {
    expect(makeCompositeId("__default__", "sess-123")).toBe("__default__:sess-123");
  });

  it("parseCompositeId_should_SplitOnFirstColon_When_GivenAWellFormedCompositeId", () => {
    expect(parseCompositeId("__default__:sess-123")).toEqual({ rowKey: "__default__", entityId: "sess-123" });
  });

  it("parseCompositeId_should_SplitOnlyOnFirstColon_When_RowKeyContainsAColon", () => {
    // A future rowKey (e.g. a branch name) could itself contain ":" — entityId (session IDs,
    // BoardColumnKey values) never does, so splitting on the *first* colon is unambiguous.
    expect(parseCompositeId("feature:foo:sess-123")).toEqual({ rowKey: "feature", entityId: "foo:sess-123" });
  });

  it("parseCompositeId_should_ReturnEmptyRowKey_When_IdHasNoColon", () => {
    expect(parseCompositeId("sess-123")).toEqual({ rowKey: "", entityId: "sess-123" });
  });
});
