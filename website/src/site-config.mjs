// Single source of truth for the deployed site's origin and GitHub Pages base
// path. astro.config.mjs, check-internal-links.mjs, and their tests all need
// these same two values; before this module existed each one hardcoded its
// own copy, so a Pages/domain change could update astro.config.mjs (and its
// co-located test) while check-internal-links.mjs silently kept checking
// links against a stale base.
export const SITE_ORIGIN = 'https://tstapler.github.io';
export const SITE_BASE = '/stapler-squad';
