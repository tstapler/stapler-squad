/**
 * Legacy theme ids that still resolve to a current theme.
 *
 * Remove after 2027-03-01 (at least two releases after first shipping), once dormant users have
 * had time to return. Tracking issue: https://github.com/tstapler/stapler-squad/issues/1547
 * Removal also drops the matching allow-list entries and the server-side base normalization.
 */
export const LEGACY_THEME_IDS: Readonly<Record<string, string>> = Object.freeze({
  wh40k: "grimdark",
});

/**
 * Maps a stored/typed theme id to its canonical id. Unknown ids pass through unchanged; falling
 * back to `clean` is the caller's job (app boundary), so this stays a pure alias lookup.
 */
export function parseThemeId(id: string): string {
  return Object.hasOwn(LEGACY_THEME_IDS, id) ? LEGACY_THEME_IDS[id] : id;
}
