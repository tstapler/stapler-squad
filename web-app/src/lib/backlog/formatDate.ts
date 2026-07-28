/**
 * Formats an ISO date string for display, e.g. "Jul 21, 2026, 03:45 PM".
 * Returns "—" when `iso` is missing.
 *
 * Deliberately distinct from `lib/utils/datetime.ts` (takes `Date | number`)
 * and `lib/utils/timestamp.ts`'s `formatDate` (takes a proto `Timestamp`) —
 * this one takes the raw ISO string BacklogItem fields carry and owns the
 * "—" empty-state fallback those two don't have.
 *
 * Extracted from BacklogItemDetail.tsx so it can be shared with the
 * detail/*Section.tsx components that were split out from it (Epic 3), which
 * had each been copy-pasting this same helper.
 */
export function formatDate(iso?: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
