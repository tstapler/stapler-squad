# ADR-002: Reference demo media via a hardcoded Release-asset URL convention, not a manifest or build-time API call

**Status**: Accepted
**Date**: 2026-10-14
**Context project**: stapler-squad-website

## Context

The site must embed demo GIFs/videos (`DemoClip`s) that are produced by a separate,
manually-triggered GitHub Actions workflow and published as Release assets — never committed
into the repo (per `requirements.md` Constraints, and the git-bloat incident documented in
`.github/workflows/demo-publish.yml`'s own header and `research/pitfalls.md` §1). The site's
build must only *consume* already-published assets, never regenerate or fetch-and-commit them.

`research/architecture.md` §2 identified three ways the site could know which asset URLs to
embed.

## Options considered

| Option | Key strength | Key weakness |
|---|---|---|
| **Hardcoded URL convention** (chosen) | Zero moving parts: `https://github.com/tstapler/stapler-squad/releases/download/<tag>/<clip-name>.<ext>` is a plain string, same pattern `README.md` already uses for `assets/demo.gif`; no API call, no extra workflow step, no file to keep in sync | If a clip is renamed/removed in a future recording-workflow edit, the embedded reference breaks silently (404 on that one asset) until a human or the `AssetHealthCheck` step notices |
| **Generated manifest file** checked into the site's content dir | Gives the site one place to read "what clips exist" | Adds a moving part (something has to generate and commit it) for a problem the asset-name-is-the-identity convention already solves; a hand-maintained manifest can itself drift from the workflow's real output (asset renamed, manifest not updated) — the same failure mode as the chosen option, with extra machinery |
| **Build-time GitHub API call** to list Release assets | Always reflects the live Release state, no stale references possible | Directly contradicts the constraint that the site's build must only consume already-published assets, never touch the recording pipeline's live state; adds a network dependency and rate-limit exposure to *every* deploy for data that changes only on a rare manual `workflow_dispatch` run |

## Decision

**Hardcoded URL convention**, mitigated by a build-time `AssetHealthCheck` (`website/scripts/check-asset-health.mjs`) that issues a HEAD request against every referenced `ReleaseAssetRef` URL and fails the deploy loudly on a non-2xx/3xx response, rather than shipping a silent broken-image icon. The `ReleaseAssetRef` construction itself is centralized in one small typed helper (`website/src/lib/release-asset.ts`) rather than scattered string interpolation across MDX files, so renaming a tag or filename is a one-place edit and a missing asset is a structural type (not a bare string) that the health check can enumerate.

## Consequences

- Demo clip names must stay stable across `website-demo-publish.yml` runs (the workflow already
  uses `--clobber` against a rolling `website-demo-gifs-latest` tag — see ADR-003's sibling
  decision in `research/architecture.md` §2 to use a *separate* tag from the existing
  `demo-gifs-latest`).
- Renaming a clip in the recording workflow is a two-place edit (workflow + site content) with
  no automated coupling beyond the `AssetHealthCheck` catching the resulting 404 at the next
  deploy — an accepted, documented gap (low clip count, low rename frequency), not a silent one.
- If this becomes a recurring problem, the escalation path is the generated-manifest option
  above, not a build-time API call — the API-call option is permanently ruled out by the
  "never touch the recording pipeline's live state at deploy time" constraint, not just
  deprioritized.

## Rejected

- **Generated manifest file** — adds a sync-and-commit step for a problem already solved by the
  filename-is-the-identity convention; deferred as the escalation path if silent breakage
  becomes a real, recurring problem.
- **Build-time GitHub API call** — contradicts the "consume, never regenerate/query" deploy
  constraint and adds a network dependency + rate-limit exposure to every deploy.
