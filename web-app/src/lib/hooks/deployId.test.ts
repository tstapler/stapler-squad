import { noteDeployId } from "./deployId";

describe("noteDeployId", () => {
  let warn: jest.SpyInstance;
  beforeEach(() => {
    warn = jest.spyOn(console, "warn").mockImplementation(() => {});
  });
  afterEach(() => warn.mockRestore());

  it("seeds on the first id without reporting a change", () => {
    const ref = { current: "" };
    expect(noteDeployId(ref, "a", "sessions")).toBe(false);
    expect(ref.current).toBe("a");
    expect(warn).not.toHaveBeenCalled();
  });

  it("is quiet while the id is unchanged", () => {
    const ref = { current: "a" };
    expect(noteDeployId(ref, "a", "sessions")).toBe(false);
    expect(warn).not.toHaveBeenCalled();
  });

  it("reports and logs a redeploy when the id changes", () => {
    const ref = { current: "a" };
    expect(noteDeployId(ref, "b", "sessions")).toBe(true);
    expect(ref.current).toBe("b");
    expect(warn).toHaveBeenCalledWith("[deploy] stream=sessions server redeployed a -> b");
  });

  it("ignores an empty id from an older server and keeps the last known one", () => {
    const ref = { current: "a" };
    expect(noteDeployId(ref, "", "sessions")).toBe(false);
    expect(ref.current).toBe("a");
  });
});
