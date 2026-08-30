import { protoTimestampToDate } from "@/lib/utils/timestamp";

describe("protoTimestampToDate", () => {
  it("returns null when timestamp is undefined", () => {
    expect(protoTimestampToDate(undefined)).toBeNull();
  });

  it("converts seconds and sub-second nanos precisely", () => {
    // 2026-01-01T00:00:00.500Z, with 500,000,000 nanos = 500ms.
    const date = protoTimestampToDate({
      seconds: 1767225600n,
      nanos: 500_000_000,
    } as never);
    expect(date).not.toBeNull();
    expect(date!.getTime()).toBe(1767225600 * 1000 + 500);
  });

  it("truncates sub-millisecond nanos rather than rounding up", () => {
    const date = protoTimestampToDate({
      seconds: 0n,
      nanos: 999_999, // just under 1ms
    } as never);
    expect(date!.getTime()).toBe(0);
  });
});
