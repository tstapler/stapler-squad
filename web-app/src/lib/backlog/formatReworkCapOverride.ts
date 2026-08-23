/**
 * A rework-cap override value is tri-state: `undefined` means no override is
 * set (global default applies), `0` explicitly means unlimited retries for
 * this item, and any other value is this item's own cap. Resolving that
 * tri-state must use `=== undefined` / `=== 0` checks, never `||`/truthy
 * coercion (which would misreport an explicit 0 as unset) — this type and
 * resolver centralize that check so StuckItemDetail.tsx (the stuck-items
 * panel, which spells out a full sentence) and LifecycleSummary.tsx (the
 * backlog item detail page's compact badge) can't drift into inconsistent
 * tri-state handling even though they render different strings from it.
 */
export type ReworkCapOverrideState =
  | { kind: "unset" }
  | { kind: "unlimited" }
  | { kind: "capped"; rounds: number };

export function resolveReworkCapOverride(value: number | undefined): ReworkCapOverrideState {
  if (value === undefined) return { kind: "unset" };
  if (value === 0) return { kind: "unlimited" };
  return { kind: "capped", rounds: value };
}
