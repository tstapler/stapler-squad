# ADR-007: Embedded artifacts are staged at the paths the Go source expects; no Go source change

**Date**: 2026-11-01
**Status**: Accepted 2026-11-06 (finalised at Task 0.14d). Spike S3 (Story 0.5) is `Pass`, path `copy_directory+embedsrcs` (`implementation/spike-results.yaml`; `implementation/spike-results.md` S3 "Verdict"): rules_go 0.65.0 takes the staged tree artifact as `embedsrcs` directly, so the fallbacks below were not needed (Task 0.5b not applicable). `server/web/embed.go` is unchanged.
**Context**: `//go:embed` sites: `server/web/embed.go` (`all:dist`), `session/tmux/binary_embedded.go` (`embed/tmux`, tag `embed_tmux`), `session/tymux/binary_embedded.go` (`embed/tymuxd`, tag `embed_tymux`), `changelog_embed.go` (`CHANGELOG.md`), `server/analytics/client_metrics.go` (`client_pages.json`). Inputs come from other packages' build outputs, and a tree artifact cannot be listed file-by-file at analysis time.

## Decision

A staging rule (`copy_directory` / `copy_file` from bazel-lib) places each artifact at the exact relative path the embed directive names inside the consuming package's output tree, and passes it to `go_library(embedsrcs = ...)`. The embed tag configs (`--config=embed_tmux`, `--config=embed_tymux`) set rules_go tags and a bool `build_setting` at `//tools/bzl/flags:embed_tmux` / `:embed_tymux`, with a `select()` choosing between staged files and `[]`. The flags package lives under `tools/bzl/`, not `build/`, because `build/` is gitignored (`.gitignore:99`) and `.bazelignore`d; the tmux target is `//tools/bzl/tmux:tmux_bin`, built from the pinned release tarball, because `third_party/tmux` is a submodule gitlink that cannot hold tracked files. The submodule remains only as a version pin compared against `tools/bzl/tmux/PIN`. Go sources do not change, so `go build`, make and Bazel keep working side by side and revert stays one PR.

## Fallbacks (chosen by S3 verdict)

| Verdict | Path |
|---|---|
| rules_go rejects the staged directory | Workaround: a small custom rule that expands the tree into a declared file list (rules_go `GoSource` provider route, same technique as the ent spike) |
| Still not workable | Fork `rules_go` for directory `embedsrcs` (ADR-008 costing) **or**, last resort, embed a single `dist.tar` in both builds. The tar change is a Go source change and is only allowed after the retention window rule is amended in this ADR, because it breaks the "revert is one PR" property |

## Consequences

- `all:dist` embedding means any web change relinks the Go binary under Bazel; the dev loop keeps using `make dev` / `WEB_DEV_HMR`, not the embed edge.

## Spike outcome (Task 0.14d, evidence in `implementation/spike-results.md` S3)

- Verified for the web `dist` site only (380 files embedded, staged tree hash equals `//web-app:dist`). The tmux, tymuxd, changelog and `client_pages.json` sites are not covered by S3.
- Constraints found: a genrule cannot declare a directory output (a `declare_directory` rule is needed for fixtures), and two `copy_directory` targets in one package that stage to the same path collide, so each embed site needs a distinct staging path per package.
- S11 confirms the `embed/tmux` and `embed/tymuxd` patterns match no files in the tree, so the `select()` between staged files and `[]` stays hand-maintained as decided above.
