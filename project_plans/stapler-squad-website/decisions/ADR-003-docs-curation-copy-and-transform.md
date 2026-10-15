# ADR-003: Curate help docs via a committed copy-and-transform script, not a live symlink/include

**Status**: Accepted
**Date**: 2026-10-14
**Context project**: stapler-squad-website

## Context

The site publishes a curated subset of `docs/how-to/*.md` (plus `docs/features.md` and
README content) to a public audience. `requirements.md` Scope frames this explicitly as
"curated subset... adapted for a public audience," not a mirror of the full `docs/` tree.
`research/architecture.md` §1 and `research/pitfalls.md` §3 both flag the same risk: these
source files use plain relative Markdown links (including links to out-of-scope internal docs
like `project_plans/` and `CLAUDE.md`) with no front-matter and no existing staleness check.

## Options considered

| Option | Key strength | Key weakness |
|---|---|---|
| **Copy-and-transform via a committed script output** (chosen) | The sync script (`website/scripts/sync-content.mjs`) reads a version-controlled `ContentManifest`, copies + transforms each `SourceDoc` into a `CuratedDoc` (strips internal-only links, adds frontmatter), and the *output* is committed — so a content change is reviewable in a PR diff like any other change, same as the Feature Registry's `make registry-generate`/`make registry-diff` precedent already in this repo | Requires a human (or an agent) to re-run the sync script and review the diff when a source file changes — not automatic |
| **Live symlink or build-time read of `docs/` into the site's content root** | Always current with the source, no separate commit step | Definitionally a mirror, not a curated subset — a dev-facing aside added to a source file for an unrelated reason ships to the public site with no review gate; symlinks don't survive `actions/checkout` + content-collection loaders (Astro, VitePress, Docusaurus) cleanly across runners, fighting the chosen SSG's own content-loading conventions |
| **One-time manual copy-paste, no script** | Simplest possible first pass | No mechanism to catch drift — a renamed/deleted `docs/how-to/*.md` file silently orphans the copy, and there's no re-run story when the curation list changes incrementally later |

## Decision

**Copy-and-transform via a committed sync script**, with two distinct run modes:
**generate** (a human/agent runs it on-demand to create/update `CuratedDoc`s — never
automatic) and **verify** (an idempotency/diff check that runs automatically on every deploy,
per plan.md Story 6.3.1, to confirm the committed `CuratedDoc`s still match what regenerating
them would produce — it fails the build on drift rather than silently regenerating or
overwriting them). The deploy workflow only ever invokes verify mode; generate mode is never
run as part of a deploy. Concretely:

1. `website/scripts/content-manifest.json` is the version-controlled `ContentManifest`: a list
   of `{ source, slug, include, reason }` entries, one per candidate `SourceDoc`.
2. `website/scripts/sync-content.mjs` reads it, and for each `include: true` entry, copies the
   `SourceDoc`, strips/rewrites internal-only cross-references, injects Starlight frontmatter
   (title from the first H1, description, slug), and writes the result as a `CuratedDoc` under
   `website/src/content/docs/help/`. This is generate mode.
3. The script fails loudly (non-zero exit) if a manifest-referenced `source` path no longer
   exists — turning a silent stale reference into a build-time/CI-time error. This check applies
   in both modes.
4. The `CuratedDoc` output is committed to `website/`, reviewed in the PR diff like any other
   content change — not generated fresh and discarded on every deploy. The deploy workflow's
   build job instead runs the script in verify mode (Story 6.3.1), which fails the build if the
   committed output doesn't match what generate mode would produce, rather than regenerating it.

## Consequences

- A `docs/how-to/*.md` rename or deletion that affects a curated file is caught the next time
  someone runs `sync-content.mjs` in generate mode (recommended: as a `make registry-generate`-style
  habit any time `docs/how-to/` changes) — and, independently, verify mode catches any
  committed `CuratedDoc` that has drifted from its `SourceDoc` on every deploy (Story 6.3.1),
  failing the build rather than silently regenerating or shipping stale content. Generate mode
  itself deliberately never runs as part of the deploy workflow (ADR's sibling decision in
  `research/architecture.md` §1, to avoid re-introducing an unreviewed live-mirror on a cron
  instead of instantly).
- `ContentManifest` entries use an explicit `IncludeDecision` shape (`include: true/false` +
  `reason`) rather than a bare boolean, so the file-by-file judgment calls required by
  `requirements.md`'s Rabbit Holes section ("Docs curation boundary") are recorded in the
  manifest itself, not left to institutional memory.

## Rejected

- **Live symlink/build-time read** — reintroduces the exact "unreviewed mirror" problem the
  curated-subset framing exists to avoid, and fights the chosen SSG's content-loader
  conventions.
- **One-time manual copy-paste** — no drift detection, no re-run story; strictly worse than the
  chosen option for the same amount of initial effort.
