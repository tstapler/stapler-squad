# UX Research: stapler-squad-website

Agent 5 of SDD Phase 2. Scope per `project_plans/stapler-squad-website/requirements.md`: landing page + curated docs site, GitHub Pages, static, no auth, no dynamic content.

## 1. Comparable UX patterns

README framing to match (`README.md:3`): "web-based mission control for running multiple AI coding agents... with a real-time dashboard, automatic approval rules, and a structured review queue." This is the hero's value prop verbatim — don't invent new copy, lift it.

**Hero section** — the dominant pattern across self-hosted/local-first dev tools (Uptime Kuma, n8n, Portainer, Immich, Gitea-style OSS landers) is a fixed stack, confirmed against this project's own README which already follows it:
1. One-line headline (what it is + who it's for)
2. One-sentence value prop (the 3 differentiators: dashboard, approval rules, review queue)
3. Install command in a copyable code block (this repo's README leads with `brew tap ...`/`curl ... | bash` — reuse verbatim, don't re-derive install steps for the site)
4. Demo media immediately below the fold line — GIF autoplay-on-view or embedded video, no click-to-play wall for the primary demo
5. Primary/secondary CTA pair: "Install" (anchor to install section or GitHub release) + "View on GitHub" (secondary, outlined)

**Feature grid** — README's own "Highlights" section (`README.md:17-38`) is already grouped into 5 categories (Visibility, Organisation, Review & Approval, History & Debugging, Infrastructure) with bolded sub-bullets. This maps directly to a 4-5 card feature grid; `docs/features.md` has longer-form per-feature copy plus screenshots (`assets/demo-recording/01-dashboard.png` etc.) already captured at the right crop/scale to drop into grid cards — reuse those PNGs rather than re-cropping from the full demo.gif.

**Docs navigation** — left-sidebar-with-sections is the dominant pattern for "deep reference content with many pages" (Docusaurus, VitePress, Starlight, Mintlify, ReadMe.io, n8n docs all default to it); top-nav-tabs suit a handful of top-level destinations only (marketing-site-style). Given scope is "curated subset" (install/getting-started + feature overview + a dozen how-to pages, per requirements' Scope section) — a 2-level left sidebar (Getting Started / Features / Troubleshooting) is the right shape, not top tabs. This is a tooling-choice-adjacent finding: if the SSG choice (Agent handling tech-stack research) lands on Starlight/Docusaurus/VitePress, sidebar nav + built-in search ships for free; a hand-rolled static site would need to build both.

**Search** — every comparable docs site (Docusaurus Algolia DocSearch, VitePress local search, Starlight's built-in Pagefind) treats docs search as table-stakes, reachable via a visible search box in the sidebar header or a `Cmd/Ctrl+K` shortcut-triggered modal. For a corpus this small (~15-20 curated pages), a client-side index (Pagefind/MiniSearch) is sufficient — no hosted search service needed, which also avoids a third-party dependency/cost on a project with "no analytics/tracking infra" in scope.

## 2. User mental models and entry points

Two entry paths per requirements' Users section, each with a different first question:

| Entry path | First question | Landing-page answer | Docs answer |
|---|---|---|---|
| GitHub README link ("see the live site") | "Is this more than what I just read?" | Same value prop, richer demo media, feature grid with screenshots the README doesn't have room for | N/A — they land on `/`, not `/docs/` |
| Search engine ("ssq agent approval rules", "claude code session manager ui") | "What is this, is it for me?" | Hero must answer in <5 seconds: product category (AI coding agent session manager) + the 3 differentiators, above the fold, no scrolling required | — |
| Existing user hit a problem | "How do I get unstuck?" | N/A — they skip the landing page entirely if they bookmark `/docs/` | Troubleshooting section must be reachable within 2 clicks from any docs page (persistent sidebar, not buried under Getting Started) |

The four-question framing from the task brief ("what is this, is it for me, how do I install it, how do I get unstuck") maps onto exactly 4 landing-page zones in order: hero (what+who) → feature grid/demo (is it for me) → install section (how) → footer/nav link to docs (unstuck). Nothing should require scrolling past install to reach a docs link — a frustrated user mid-install-failure shouldn't have to hunt.

## 3. Accessibility requirements

Target per requirements' NFRs: Lighthouse accessibility ≥ 90. Note this is a **higher bar than `web-app/lighthouserc.json`'s own current gate** (`categories:accessibility": ["warn", { "minScore": 0.8 }]`, warn-only) — the new site's CI config needs its own, stricter, blocking threshold; copying `web-app/`'s config verbatim would under-enforce.

**Color contrast** (WCAG 2.2 AA, confirmed via `web-app/src/styles/theme.css.ts`):
- Normal text ≥ 4.5:1, large text (≥24px, or ≥18.66px bold) ≥ 3:1, UI components/graphics ≥ 3:1 (criterion 1.4.11).
- The theme already has two documented near-miss fixes worth reusing as the marketing palette's baseline rather than re-deriving: light-mode `primary: #0070f3` and dark-mode `primary: #2d9cdb` (`theme.css.ts:91,201`) are the two in-use brand blues; two other theme variants in the same file carry inline contrast-fix comments (`#ff2d78`→`#cc245f` for 5.27:1, `#6366f1`→`#5457ef` for 5.27:1 — `theme.css.ts:430,667`), showing this codebase already treats "ship the WCAG-failing shade, then tune it down" as a known failure mode to avoid on a first pass. Whichever single palette the site adopts (reusing `vars.color.primary`/`textPrimary`/`background` tokens per requirements' Constraints), check its CTA-button and link text contrast against the chosen background before shipping, not after.
- `logError`/`logWarn`-style semantic color pairs in the same theme contract aren't relevant to a marketing site (no log viewer here) — don't port the full token set, just text/background/primary/border.

**Alt text**: every demo screenshot (`assets/demo-recording/01-dashboard.png` through `08-final.png`, already captured) needs descriptive alt text, not filenames — `docs/features.md`'s existing prose ("See all your AI agents at a glance...") is reusable as alt-text source material per image. Decorative images (background textures, logo marks already named in surrounding text) get `alt=""`.

**Video/GIF captions**: requirements scope new scripted Playwright-recorded demo clips (no narration — Constraints section confirms "no owner recording/narration time"). Silent, non-narrated clips need no caption track per WCAG (1.2.2 applies to audio content; these have none) — but each embedded clip still needs a text alternative nearby (the feature-grid card's own heading/description already serves this if the clip is paired with a caption, not floating alone).

**Keyboard navigation**: docs search modal (if Cmd/Ctrl+K-triggered) must trap focus correctly and be dismissible via Escape — this is the single highest-risk a11y regression class for docs-search widgets (common failure: focus escapes to page-behind, or Tab cycles through hidden off-screen results). Sidebar nav must be reachable and operable via Tab/Enter with visible focus rings — verify against the project's own theme tokens (`vars.color.inputFocusBorder` exists in `theme-contract.css.ts:31` and could inform the visible-focus-indicator color on the new site, keeping it recognizable as the same product family).

**Automated gate**: this repo already has the exact tool for this — `@axe-core/playwright` is already a dependency, used in `tests/e2e/accessibility.spec.ts` to block merges on critical/serious violations for the main app. Reuse the same library (not a different a11y scanner) for the new site's CI, scanning both the landing page and at least one docs page template.

## 4. Error states and edge cases

- **Docs search, zero results**: show a clear "No results for '{query}'" state with a link to the docs home and/or a "file a GitHub issue" link — never a blank panel. (Pagefind and MiniSearch both support a no-results slot; don't ship the library default silently.)
- **Demo GIF/video failing to load**: because media is served from GitHub Release assets per requirements' Constraints ("build-time fetch, not committed binaries"), a stale or deleted release tag breaks the `<img>`/`<video>` src with no retry. Mitigation: `<img alt="...">`'s alt text should stand in as a reasonable fallback if the image 404s (browsers render alt text in place of a broken image), and `<video>` elements need a `poster` attribute (a static frame) so a broken video source degrades to a static image rather than a blank box — pull the poster frame from the already-committed small `assets/screenshot.png` (confirmed <2MB, in-repo, exempt from the Release-asset rule per Constraints) rather than depending on the Release asset for the fallback too.
- **Docs page linking to an internal-only doc excluded from the curated subset**: the requirements explicitly flag this (Rabbit Holes: "Docs curation boundary") as a Phase 3 planning task, not a UX pattern to solve generically — but the UX principle is: a relative link inside *migrated* markdown that originally pointed at an excluded doc (e.g. `docs/how-to/run-nil-safety-analysis.md`) must be caught at build/lint time (broken-link checker in CI) and either removed or the excluded doc must simply never be linked from curated content — never shipped as a live link that 404s on the public site. This is a content-curation discipline problem, not a runtime 404-page design problem, but the site still needs *a* 404 page (GitHub Pages serves its own generic 404 by default; a branded one matching the site's own chrome is a small, cheap win worth including).
- **Mobile**: per the user's standing `feedback_mobile_desktop_ux` instinct (always consider both form factors) — the hero's install-command code block and any feature-grid screenshots need to not force horizontal scroll on a phone viewport; this is directly testable by the same Lighthouse mobile run pattern `web-app/lighthouserc.json` already demonstrates (it currently only tests desktop — the new site's Lighthouse CI config should add a mobile emulation pass, since this is a landing page where mobile traffic share is plausible, unlike the app itself which is used at a desk).

## 5. Jobs-to-be-done

- **Functional**: "Let me get this running on my machine in under 5 minutes." Served by: hero install command (copy-paste, zero scrolling to find it) + a getting-started doc page that's just the README's install section with more detail (prerequisites, troubleshooting a failed build) and nothing else gating it.
- **Emotional**: "Is this maintained? Will I waste an hour on an abandoned project?" Served by: the CI badge and GitHub Release badge already in `README.md:1` (reuse on the site header/footer, don't drop them), a visibly current demo (dated or versioned screenshots, not a 2-years-stale GIF), and a working docs search (a site with broken search reads as neglected faster than almost anything else).
- **Social**: "I want to show a teammate what this does without making them read a README." Served by: a landing page that's self-contained and demo-forward enough to screen-share or link directly mid-conversation — this is the actual reason the hero's demo media needs to be above the fold and autoplaying/scrubbable, not a click-through gallery buried below three paragraphs of text.

## Sources

- [WCAG 2.2 AA checklist — BrowserStack](https://www.browserstack.com/guide/wcag-compliance-checklist)
- [WCAG 2.2 checklist — Accesstive](https://accesstive.com/blog/wcag-2-2-checklist/)
- General docs-navigation guidance (sidebar vs. top nav, no Docusaurus/VitePress-specific source surfaced by search — pattern claims above for those tools are from existing product knowledge, not a fetched source): [Mendix navigation layouts](https://docs.mendix.com/howto10/front-end/use-navigation-layouts/), [Ohio University navigation guidance](https://www.ohio.edu/ucm/web/help/web-catalog/navigation-menu-best-practices)
- In-repo evidence: `README.md`, `docs/features.md`, `web-app/src/styles/theme.css.ts`, `web-app/src/styles/theme-contract.css.ts`, `web-app/lighthouserc.json`, `tests/e2e/accessibility.spec.ts`, `.github/workflows/demo-publish.yml`, `assets/demo-recording/*.png`
