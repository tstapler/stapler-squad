import { clearDraft, readDraft, writeDraft } from "./sessionDrafts";

describe("sessionDrafts", () => {
  afterEach(() => {
    jest.restoreAllMocks();
    clearDraft("q1");
  });

  it("Drafts_should_surviveCloseAndReload_And_fallBackToMemory_When_sessionStorageThrows", () => {
    writeDraft("q1", "half typed");
    expect(window.sessionStorage.getItem("ssq:draft:q1")).toBe("half typed");
    expect(readDraft("q1")).toBe("half typed");

    jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(() => writeDraft("q1", "typed offline")).not.toThrow();
    expect(readDraft("q1")).toBe("typed offline");
  });

  it("Drafts_should_clearOnSendOrDismiss", () => {
    writeDraft("q1", "x");
    clearDraft("q1");
    expect(readDraft("q1")).toBe("");
    writeDraft("q1", "y");
    writeDraft("q1", "");
    expect(readDraft("q1")).toBe("");
  });
});
