// Banner kinds and their total order (ux.md 8.2, U-EDIT-15). Later stories register their banners by
// adding a kind here; the stack itself never needs to change.

export const BANNER_PRIORITY = {
  save_rejected: 1,
  save_unknown: 1,
  draft_not_stored: 1,
  deleted: 2,
  flag_off: 3,
  stale_dirty: 4,
  offline: 5,
  readonly_reason: 6,
  beyond_loaded: 7,
  stale_clean: 8,
  informational: 9,
} as const;

export type BannerKind = keyof typeof BANNER_PRIORITY;
export const BANNER_KINDS = Object.keys(BANNER_PRIORITY) as BannerKind[];

/** Priority 1: never collapsed into "+N more", `role="alert"`, no Dismiss. */
export const isBlocking = (kind: BannerKind) => BANNER_PRIORITY[kind] === 1;

export interface Banner { id: string; kind: BannerKind; /** Larger is newer. */ seq: number }

/** Highest priority first, then newest. */
export function sortBanners<T extends Banner>(banners: readonly T[]): T[] {
  return banners.slice().sort((a, b) => BANNER_PRIORITY[a.kind] - BANNER_PRIORITY[b.kind] || b.seq - a.seq);
}

/** The first `max` banners, but every blocking one regardless of `max`; the rest are hidden. */
export function pickVisible<T extends Banner>(banners: readonly T[], max: number): { visible: T[]; hidden: T[] } {
  const sorted = sortBanners(banners);
  const blocking = sorted.filter((b) => isBlocking(b.kind)).length;
  const keep = Math.max(max, blocking);
  return { visible: sorted.slice(0, keep), hidden: sorted.slice(keep) };
}
