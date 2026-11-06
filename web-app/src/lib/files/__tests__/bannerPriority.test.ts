import fc from "fast-check";
import { BANNER_KINDS, BANNER_PRIORITY, isBlocking, pickVisible, sortBanners, type Banner, type BannerKind } from "../bannerPriority";

const banner = (kind: BannerKind, seq: number, id = `${kind}-${seq}`): Banner => ({ id, kind, seq });

describe("bannerPriority", () => {
  it("bannerPriority_should_OrderKindsByTheDocumentedPriority_When_Sorted", () => {
    const order: BannerKind[] = [
      "save_rejected", "deleted", "flag_off", "stale_dirty", "offline", "readonly_reason", "beyond_loaded", "stale_clean", "informational",
    ];
    const sorted = sortBanners(order.slice().reverse().map((k, i) => banner(k, i)));
    expect(sorted.map((b) => b.kind)).toEqual(order);
    expect(BANNER_PRIORITY.save_unknown).toBe(1);
    expect(BANNER_PRIORITY.draft_not_stored).toBe(1);
  });

  it("bannerPriority_should_PutNewestFirst_When_PrioritiesTie", () => {
    const sorted = sortBanners([banner("informational", 1), banner("informational", 5), banner("informational", 3)]);
    expect(sorted.map((b) => b.seq)).toEqual([5, 3, 1]);
  });

  it("bannerPriority_should_AlwaysShowBlockingBanner_And_NeverCollapseIt_When_RandomBannerSets", () => {
    const kind = fc.constantFrom(...BANNER_KINDS);
    fc.assert(
      fc.property(fc.array(kind, { maxLength: 12 }), fc.integer({ min: 1, max: 4 }), (kinds, max) => {
        const banners = kinds.map((k, i) => banner(k, i));
        const { visible, hidden } = pickVisible(banners, max);
        const blocking = banners.filter((b) => isBlocking(b.kind));
        // every priority-1 banner is visible, never inside "+N more"
        expect(blocking.every((b) => visible.includes(b))).toBe(true);
        expect(hidden.some((b) => isBlocking(b.kind))).toBe(false);
        expect(visible.length).toBe(Math.min(banners.length, Math.max(max, blocking.length)));
        expect(visible.length + hidden.length).toBe(banners.length);
        // hidden ones are never higher priority than a visible one
        const worstVisible = Math.max(...visible.map((b) => BANNER_PRIORITY[b.kind]), 0);
        expect(hidden.every((b) => BANNER_PRIORITY[b.kind] >= worstVisible)).toBe(true);
      }),
      { numRuns: 500 },
    );
  });
});
