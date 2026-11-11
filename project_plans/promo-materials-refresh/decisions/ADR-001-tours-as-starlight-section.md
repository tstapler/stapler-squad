# ADR-001: Tours are a dedicated `/tours` Starlight section with a typed frontmatter contract

**Status**: Accepted (owner-decided input) | **Date**: 2026-11-10

## Context
The site has no place for guided tours. Research (`research/architecture.md` s4, `research/ux.md` s3) compared custom `src/pages/tours/*.astro` against Starlight content pages. The owner chose a dedicated `/tours` section built from Starlight MDX with a typed tours schema.

## Decision
- Tour pages live at `website/src/content/docs/tours/<id>.mdx` (served at `/stapler-squad/tours/<id>/`), plus `tours/index.mdx` rendering `TourGrid.astro`.
- "Tours content collection" is implemented as a **required `tour` frontmatter object on every doc under `tours/`**, validated by a Zod schema added through the docs collection's `extend` in `website/src/content.config.ts` (a separate `src/content/tours` collection was rejected, see below). Schema: `title`, `outcome`, `clip`, `features[]` (registry ids), `helpDoc` (a **website docs collection id** such as `help/getting-started`, validated at build; never an in-app `DOC_FILES` slug, which has no website page), `order` (1-3 are the Start-here tours), `category`, `audience`, `durationMin`, optional `flagNote` ("enabled in this recording"), optional `phoneFigure` (a mobile shot name), `steps[]` (`text`, `shot`, `alt`, `caption`, `narration` required; `why`, `quote`, `annotated` optional), 3-6 steps per tour. `narration` is the on-screen action for the transcript (WCAG 1.2.1/1.2.3) and must differ from `text`; `order` is unique 1..N and the single ordering for Start-here and prev/next. Clips are encoded as WebM then MP4 `<source>`s with a poster. All keys live under the nested `tour` object; the plan refers to them as `tour.<key>`.
- Sidebar: `{ label: 'Tours', autogenerate: { directory: 'tours' } }`.
- Components: `TourHeader.astro` (breadcrumb, meta row), `TourClip.astro` (wraps the existing `reduced-motion-video.ts`; owns the Install link after the clip), `TourSteps.astro` (transcript and `ol` steps), `TourShot.astro` (required `alt`, explicit width/height and aspect-ratio, lazy), `TourFooter.astro` ("Try it", prev/next, "All tours"), `TourGrid.astro`, `BuildStamp.astro` (one format: "Recorded 2026-11-10 (build <small>a1b2c3d</small>)"). Prev/next and "See the tour" links carry their full label as visible text and no `aria-label`. Media rendering has two modes (`TOURS_MEDIA_MODE`): `deploy` fails the build for any clip, shot or manifest missing; `smoke` (PR builds only) does not build a tour whose clip is unrecorded (loader glob computed from the manifest, so no page, sidebar entry, grid card or link) and omits unrecorded step shots of a built tour, with no placeholder.

## Alternatives rejected
| Option | Reason |
|---|---|
| Separate `src/content/tours` collection + custom pages | Re-implements Starlight nav, search, skip links and a11y chrome; the existing internal-link and Lighthouse gates would need new routing awareness. Revisit only if a layout Starlight cannot express is needed. |
| Gallery page + rolling release assets | Rejected by owner (requirements.md Alternatives). |
| Tour definition inside `clips.ts` only | Prose and alt text are content; Astro cannot import Playwright code at build time. |

## Consequences
Search, sidebar and a11y come free. Adding a tour = one `tours/<id>.mdx` + one `tests/e2e/website-demos/tours/<id>.ts` (requirements NFR scalability). **Open interpretation**: if the owner meant a literally separate collection, only Stories 3.1.1 and 3.1.3 change; flagged in plan.md Unresolved Questions.
