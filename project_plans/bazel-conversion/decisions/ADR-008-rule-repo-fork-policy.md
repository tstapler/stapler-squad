# ADR-008: When to fork a Bazel rules repository versus use a workaround

**Date**: 2026-11-01
**Status**: Accepted (forking is allowed by the owner, requirements.md 2026-11-01).

## Rule

For each Epic 0 spike with a missing upstream capability, cost both paths in the spike result (cost classes, `ESTIMATION.md`) and choose by this order:

1. **Upstream already supports it** (maybe on a newer pin): bump the pin.
2. **Workaround** if it is hermetic, cacheable, and its repo-owned Starlark stays within the cap (all repo-owned `.bzl` and macros together at most 750 lines, enforced in CI by Story 11.7).
3. **Fork** when the workaround is non-hermetic, defeats caching, or its per-upgrade maintenance cost (cost class per Bazel/rules bump) exceeds the fork's rebase cost, and the patch is at most about 200 lines.
4. A fork is always paired with an upstream PR or issue link in `docs/reference/bazel-forks.md`, lives under `github.com/tstapler/<rules repo>`, is pinned by commit through `git_override`/`archive_override` (like the `tstapler/go-git` replace), and has a removal condition ("drop when upstream release X ships").
5. Custom Gazelle Go extensions are written only after directives plus a macro demonstrably fail, and then with a Go unit test (`research/build-vs-buy.md` section 3). This is a sequencing preference inside the owner's allowance (requirements.md Constraints: write custom Gazelle extensions "where a pattern repeats"), not a prohibition: spike S11 keeps the extension as its pre-decided fallback.

## Cost matrix (INFERRED; classes per ESTIMATION.md, recalibrate after the first implementer run)

| Item | Workaround | Fork |
|---|---|---|
| `bun.lock` for aspect_rules_js | Default (`Pass`): repo repository rule + Bun action, about M+M+S+S. Workaround (`FallbackWorkaround`): pinned third-party `rules_bun`, about S+M | Teach `npm_translate_lock` a `bun.lock` reader: about L+L+M+S, plus an S per rules_js bump |
| Embed from tree artifact | Custom file-list rule: about M | Directory `embedsrcs` in rules_go: about M+L, plus an S per rules_go bump |
| ent output as `go_library` srcs | Custom rule producing `GoSource` from the tree: about M+S | Directory `srcs` in rules_go: about L |
| connect-go via `go_proto_library` | `buf generate` `run_binary` with local plugins: about M | Connect compiler plus managed-mode import-path support in rules_go: about L |
| tmux on bzlmod | Genrule with host cc: about M (non-hermetic) | Patch rules_foreign_cc 0.17.0: about M if only a Bazel 9 fix |

Spike verdicts (Story 0.14) fill in measured classes and the choice.

## Consequences

- Every fork adds a pinned dependency to track in the scheduled upgrade-canary job (Story 11.8).
- The owner must create the fork repository for any Fork verdict (wall-clock blocker, listed in the plan).
- Changelog 2026-11-03: the cap was raised from 500 to 750 lines by the owner. It is our own maintainability bound, not a Bazel limit; the surface stood at 574 lines after the hermetic otel weave and the forbidden_deps test landed. Logic that does not need Starlark still belongs in scripts.
