import { makeCompositeId, parseCompositeId } from "./compositeId";

describe("makeCompositeId / parseCompositeId", () => {
  it("makeCompositeId_should_JoinRowKeyAndEntityIdWithColon_When_Called", () => {
    expect(makeCompositeId("__default__", "sess-123")).toBe("__default__:sess-123");
  });

  it("parseCompositeId_should_SplitOnLastColon_When_GivenAWellFormedCompositeId", () => {
    expect(parseCompositeId("__default__:sess-123")).toEqual({ rowKey: "__default__", entityId: "sess-123" });
  });

  it("parseCompositeId_should_SplitOnlyOnLastColon_When_RowKeyContainsAColon", () => {
    // A real grouping-strategy rowKey (a tag/category/path value, Task 6.1.1b) can itself
    // contain ":" — entityId (session IDs, BoardColumnKey values) never does, so splitting on
    // the *last* colon is unambiguous even when rowKey has one (or more) of its own.
    expect(parseCompositeId("feature:foo:sess-123")).toEqual({ rowKey: "feature:foo", entityId: "sess-123" });
  });

  it("parseCompositeId_should_ReturnEmptyRowKey_When_IdHasNoColon", () => {
    expect(parseCompositeId("sess-123")).toEqual({ rowKey: "", entityId: "sess-123" });
  });
});
