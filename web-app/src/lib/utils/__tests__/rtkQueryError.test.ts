import { toErrorOrNull } from "../rtkQueryError";

describe("toErrorOrNull", () => {
  it("returns null for null input", () => {
    expect(toErrorOrNull(null)).toBeNull();
  });

  it("returns null for undefined input", () => {
    expect(toErrorOrNull(undefined)).toBeNull();
  });

  it("returns null for falsy 0", () => {
    expect(toErrorOrNull(0)).toBeNull();
  });

  it("extracts message from RTK Query error object with .error string", () => {
    const err = toErrorOrNull({ error: "Network error" });
    expect(err).toBeInstanceOf(Error);
    expect(err?.message).toBe("Network error");
  });

  it("extracts message from RTK Query error object with non-string .error", () => {
    const err = toErrorOrNull({ error: 404 });
    expect(err).toBeInstanceOf(Error);
    expect(err?.message).toBe("404");
  });

  it("returns generic message when input is an object without .error", () => {
    const err = toErrorOrNull({ status: 500 });
    expect(err).toBeInstanceOf(Error);
    expect(err?.message).toBe("Unknown error");
  });

  it("returns generic message for a non-object truthy value", () => {
    const err = toErrorOrNull("raw string error");
    expect(err).toBeInstanceOf(Error);
    expect(err?.message).toBe("Unknown error");
  });

  it("returns generic message for a thrown Error object without .error key", () => {
    const original = new Error("original");
    const err = toErrorOrNull(original);
    expect(err).toBeInstanceOf(Error);
    expect(err?.message).toBe("Unknown error");
  });
});
