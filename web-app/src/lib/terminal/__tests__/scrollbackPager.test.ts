import { ScrollbackPager, SCROLLBACK_NEAR_TOP_LINES, SCROLLBACK_PAGE_LINES } from "../scrollbackPager";

// kibitzer:ignore long-function -- describe callback is a list of independent cases, not one function
describe("ScrollbackPager", () => {
  const fresh = () => {
    const p = new ScrollbackPager();
    p.reset();
    return p;
  };

  it("does not page before the first snapshot has reset it", () => {
    expect(new ScrollbackPager().tryBegin()).toBeNull();
  });

  it("starts from the live bottom and asks for a full page", () => {
    expect(fresh().tryBegin()).toEqual({ fromSequence: 0, limit: SCROLLBACK_PAGE_LINES });
  });

  it("allows only one request in flight", () => {
    const p = fresh();
    expect(p.tryBegin()).not.toBeNull();
    expect(p.tryBegin()).toBeNull();
    expect(p.isInFlight).toBe(true);
  });

  it("continues where the previous page ended", () => {
    const p = fresh();
    p.tryBegin();
    p.complete({ hasMore: true, oldestSequence: 500 });
    expect(p.tryBegin()).toEqual({ fromSequence: 500, limit: SCROLLBACK_PAGE_LINES });
  });

  it("stops asking once the server reports no more history", () => {
    const p = fresh();
    p.tryBegin();
    p.complete({ hasMore: false, oldestSequence: 120 });
    expect(p.tryBegin()).toBeNull();
  });

  it("frees the claim without moving when released", () => {
    const p = fresh();
    p.tryBegin();
    p.release();
    expect(p.tryBegin()).toEqual({ fromSequence: 0, limit: SCROLLBACK_PAGE_LINES });
  });

  it("completing without metadata still frees the claim", () => {
    const p = fresh();
    p.tryBegin();
    p.complete();
    expect(p.isInFlight).toBe(false);
    expect(p.tryBegin()).not.toBeNull();
  });

  it("restarts from the bottom after a reset, dropping an in-flight claim", () => {
    const p = fresh();
    p.tryBegin();
    p.complete({ hasMore: false, oldestSequence: 900 });
    p.tryBeginIgnoringHasMore();
    p.reset();
    expect(p.tryBegin()).toEqual({ fromSequence: 0, limit: SCROLLBACK_PAGE_LINES });
  });

  it("lets a caller with its own exhaustion flag share the in-flight guard", () => {
    const p = fresh();
    p.complete({ hasMore: false, oldestSequence: 10 });
    expect(p.tryBegin()).toBeNull();
    expect(p.tryBeginIgnoringHasMore()).not.toBeNull();
    expect(p.tryBeginIgnoringHasMore()).toBeNull();
  });

  it("treats the viewport as near the top below the threshold only", () => {
    const p = fresh();
    expect(p.isNearTop(SCROLLBACK_NEAR_TOP_LINES - 1)).toBe(true);
    expect(p.isNearTop(SCROLLBACK_NEAR_TOP_LINES)).toBe(false);
  });
});
