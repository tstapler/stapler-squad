# ADR-002: Extend the existing `tests/e2e/website-demos` recorder and Go seeder (build, don't buy)

**Status**: Accepted (owner-decided input) | **Date**: 2026-11-10

## Context
`research/build-vs-buy.md` evaluated Playwright-in-place, shot-scraper/Percy, Remotion, VHS/asciinema, SaaS tour builders, and a new standalone tool.

## Decision
1. Keep Playwright (video + `page.screenshot`) and `scripts/record-website-demos.sh` as the engine. Add per-tour scene files under `tests/e2e/website-demos/tours/<id>.ts`; `clips.ts` becomes an index.
2. Split by concern: **identity and prose** in `website/src/content/docs/tours/<id>.mdx` (ADR-001); **imperative scenes** in TS keyed by tour id. A generated `expected-media.json` (from frontmatter + `feature-media.json`) is the single list of names the pipeline must produce.
3. Recorder emits `media-manifest.json` (name, kind, formFactor, bytes, sha256, width, height, durationMs, commit, theme, ffmpegVersion).
4. New checkers (media guard, freshness, trademark/leak scan, docs coverage) and scenario seeding are **Go**, under `tools/promo/`, `tools/docscoverage/` and `tests/demo/scenario/`; code shared by all three (go-git helper, term/leak scanner) is in non-internal `tools/promolib/` because `tests/demo/scenario` is outside `tools/` and Go's `internal/` rule would block it. All stay in the root Go module so `make lint` (incl. `norawgitcli`), `make test` and `dupl` apply; git access is go-git via `session/git.OpenRepo`, no CLI. The one non-Go piece, `website/scripts/expected-media.mjs`, stays JS because it reads Astro content frontmatter at build time. Bash keeps only server start/stop.
5. Seeding becomes scenario-based: `go run ./tests/demo/seed --scenario <name>[,<name>] <dir>`; `default` reproduces today's `mockSessions()` unchanged.

## Alternatives rejected
Remotion (licence, doubles the toolchain, still needs footage), SaaS tour builders (hosted, breaks "generated in CI"), shot-scraper/Percy (no scripted multi-step flows or video), forking a new package (duplicates the working harness), porting orchestration to Go wholesale (rewrites working code; only new manifest/summary steps are Go).

## Consequences
Zero new runtime dependencies for recording. Optional third-party Astro plugins (image zoom, links validator) are **not adopted**; licence and Astro 7 checks stay in plan.md Unresolved Questions and only matter if someone proposes one later.
