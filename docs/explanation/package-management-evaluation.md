# Reducing node_modules Duplication Across Worktrees

This repo accumulates dozens of git worktrees at a time (55 at the time of
this investigation: `.claude/worktrees/agent-*` from Claude Code sessions,
`~/.stapler-squad/workspaces/*/worktrees/` from stapler-squad sessions, and
ad hoc manual ones). Each one that touches `web-app/` ends up with its own
full `node_modules` tree. A btrfs ENOSPC incident (metadata chunk 98% full)
was traced partly to this; a worktree-cleanup sweep resolved the immediate
crisis. This is the preventative follow-up: what's actually duplicated, and
what — if anything — is worth changing about how `web-app/` installs its
dependencies.

## What's duplicated, measured

```
$ find web-app/node_modules | wc -l
82011
$ du -sh web-app/node_modules
1.3G
```

pnpm's own global content-addressable store (`~/.local/share/pnpm/store/v10`,
8.3G total) already dedupes package **bytes** across every project and
worktree on the machine — confirmed by inspecting the store directly. What
it does *not* dedupe is the **file/directory/symlink tree structure** pnpm
materializes fresh into every worktree's `node_modules`: roughly 82,000
filesystem entries, every single time, regardless of whether the content is
byte-identical to a tree that already exists in nine other worktrees. On
btrfs, each of those entries costs metadata-chunk space independent of the
data chunks the (deduped) file content lives in — that's the actual
resource the crisis ran out of, not disk bytes.

So the problem to solve is specifically **file/inode count**, not storage
size. That framing rules out some otherwise-obvious-looking fixes (see Bun,
below) and is why the shared-`node_modules` design below targets the tree
structure directly rather than reaching for a different package manager.

## Complication found during prototyping: local `file:` dependencies

`web-app/package.json` declares two **local, in-repo** dependencies via the
`file:` protocol:

```json
"eslint-plugin-analytics": "file:./eslint-plugin-analytics",
"eslint-plugin-rpc-lifecycle": "file:./eslint-plugin-rpc-lifecycle",
```

`pnpm-lock.yaml` records these as `{directory: eslint-plugin-analytics,
type: directory}` — **no content hash**. This matters for any
sharing/caching scheme keyed on the lockfile: if a worktree edits one of
these plugin's rule files without touching `pnpm-lock.yaml`, the lockfile
hash doesn't change, so a naive "key by `sha256(pnpm-lock.yaml)`" cache
would serve that worktree *someone else's stale copy* of the plugin through
the shared tree — a silent correctness bug, not a crash. Verified
empirically (see Evaluation 1 below): editing a rule file and recomputing a
lockfile-only hash produces an identical key; folding the plugin
directories' own file content into the key produces a different one, as it
must.

Any design that shares `node_modules` across worktrees for this repo has to
account for this, not just hash the lockfile.

## Evaluation 1: shared node_modules directory (RECOMMENDED — shipped)

**Design**, prototyped end-to-end in scratch git worktrees before writing
any production code:

- Key = `sha256(sha256(pnpm-lock.yaml) + content-hash-of-each-local-file:-dependency-directory)`.
- Shared directory: `~/.stapler-squad/shared-node-modules/<key>/`.
- First worktree to need a given key copies `package.json` + `pnpm-lock.yaml`
  + the local `file:` dependency directories into the shared dir and runs a
  real `pnpm install --frozen-lockfile` there, under an `flock` so two
  worktrees racing on a brand-new key can't corrupt it.
- Every worktree (including the first) ends up with `web-app/node_modules`
  as a **symlink** to `<shared-dir>/node_modules`.
- A worktree whose branch changed `pnpm-lock.yaml`, or edited one of the
  local plugins, gets a different key automatically — no cross-branch
  contamination, no stale-plugin bug.

**Implementation**: [`scripts/shared-node-modules.sh`](../../scripts/shared-node-modules.sh),
wired into the Makefile's `web-app/node_modules/.modules.yaml` target (the
existing `pnpm install --frozen-lockfile` call site) and its re-install
guard in the `web-app/out` target. Escape hatch:
`STAPLER_SQUAD_SHARED_NODE_MODULES=0` forces a plain local install, e.g. to
rule the shared mechanism in/out while debugging something unrelated.
Regression tests: [`scripts/shared-node-modules.test.sh`](../../scripts/shared-node-modules.test.sh)
(stubs `pnpm` so the suite is fast/offline/deterministic per the
`deterministic-fast-tests` skill — it's testing this script's own
hashing/locking/symlinking logic, not pnpm's install behavior).

**What was actually tested, not just read about** (three scratch git
worktrees via `git worktree add --detach`, plus one plain directory copy
used only as a second concurrent actor for the race test below; all
discarded after):

| Check | Result |
|---|---|
| Two worktrees, identical lockfile + plugin content → same shared install, reused (no reinstall) | PASS — confirmed via `stat -c %i` inode comparison on `node_modules/next/package.json`, identical inode number across both worktrees |
| A worktree with a *different* lockfile gets its own tree, no contamination | PASS — distinct hash, distinct shared directory |
| Editing a local `file:` plugin's rule file (lockfile untouched) invalidates the key | PASS — hash changed, fresh install triggered, confirmed this is exactly the bug a naive lockfile-only key would have |
| Two worktrees racing on a brand-new key simultaneously (parallel `flock`) | PASS — one did the real install, the other blocked on the lock and then reused it; final symlinks inode-identical, no corruption |
| `next build` (full static export, via `make web-build` in a real worktree, not just the scratch ones) against the symlinked tree | PASS — including the custom `webpack()` config (extensionAlias remapping, CSS splitChunks cache group) and `@vanilla-extract/next-plugin`'s build-time CSS-extraction webpack loader. Failed only on legitimately-missing `web-app/src/gen/*` protobuf output until `make proto-gen` ran — unrelated to node_modules. |
| `jest` | PASS — one targeted file (21/21 tests) in a scratch worktree, then the **full suite** re-run against this change's actual implementation in a real (non-scratch) worktree |
| `next lint` (ESLint, including the two local `file:` plugins loading through the symlinked tree) | PASS — zero errors (pre-existing warnings only) |
| `stylelint` + `check-css-vars.mjs` | PASS |
| Playwright (`npx playwright --version` resolves; browser binaries live in `~/.cache/ms-playwright`, independent of `node_modules` either way) | PASS |
| `make build`, `make test` (Go suite), `make lint`, `make web-build`, `make test-shell` (new `shared-node-modules.test.sh`), `make lint-shell` — all run against this change's actual implementation, not just the prototype | PASS (all) |

**Known limitation, flagged rather than silently accepted**: once linked,
`web-app/node_modules` for a given hash is the *same physical directory* for
every worktree sharing that hash — there is no per-worktree overlay. This is
fine for the local `file:` plugins specifically because their content is
folded into the cache key (above), so "different content → different
directory" already guarantees correctness without needing an overlay. It
would **not** be fine for a future local dependency that's `file:`-mounted
as mutable/dev-linked state rather than a committed plugin — if one is ever
added, it must go through the same content-hashing treatment or be excluded
from sharing entirely.

**Scope note**: this change only covers `web-app/`. `tests/e2e/`,
`tools/scanner/frontend/`, `tools/ci-gates/`, and `tools/coverage/` each have
their own separate `package.json`/lockfile/`node_modules` and are **not**
covered — a smaller secondary duplication source worth a follow-up if it
ever matters, out of scope here per the task that commissioned this
investigation.

**GC story**: folded into the existing `worktree-disk-cleanup` skill
(`.claude/skills/worktree-disk-cleanup/SKILL.md`, new Step 5) rather than a
second cleanup mechanism — a shared-dir whose hash no worktree's
`node_modules` symlink currently resolves to, and which is older than an
hour (to avoid racing an install in progress), is safe to delete.

## Evaluation 2: Yarn Berry PnP

**Verdict: technically viable, real but manageable migration cost — presented
for a human decision, not shipped here (per this investigation's scope: only
the clearly-low-risk option ships as a PR).** Prototyped in a separate
worktree by migrating `web-app/` to Yarn 4.18.1 with `nodeLinker: pnp`,
running the real build/test/lint/Playwright pipeline against it, and fixing
whatever broke for real rather than theorizing.

**The two highest-risk areas flagged going in had zero friction**:
- `@vanilla-extract/next-plugin`'s build-time CSS-extraction webpack loader —
  `next build` compiled and statically exported successfully
  (`✓ Compiled successfully in 70s`, `✓ Generating static pages (44/44)`,
  `✓ Exporting (3/3)`), CSS files generated correctly.
- The two local `eslint-plugin-analytics`/`eslint-plugin-rpc-lifecycle`
  `file:` dependencies — resolved and fired correctly through PnP's
  require-hook resolution with zero code changes, confirmed via 903 matching
  rule-ID hits (`analytics/require-on-click` etc.) in a real `next lint` run.

`jest`, `stylelint`, `check-css-vars.mjs`, and Playwright (`@playwright/test`
as a direct `web-app` devDependency, separate from `tests/e2e/`'s own
pnpm-managed package) all passed unmodified.

**Real breakages found, each with a confirmed fix**:
1. **Phantom peer dependencies — the recurring cost, not a one-time fix.**
   pnpm auto-installs undeclared-but-needed peers from anywhere in the tree;
   PnP refuses this outright and hard-fails. Hit three times
   (`recharts`→`react-is`, `ts-jest`→`jest-util`,
   `@testing-library/react`→`@testing-library/dom`), each fixed by adding
   the dependency explicitly. This recurs on every future dependency bump
   that happens to surface a new undeclared transitive peer — an ongoing
   tax, not a migration-day cost.
2. **`buf.gen.yaml` hardcodes `web-app/node_modules/.bin/protoc-gen-es`** —
   there is no `node_modules/.bin` under PnP. Confirmed failure
   (`no such file or directory`), fixed by invoking the binary through
   `yarn exec` instead. `Makefile`'s own mtime staleness check for the same
   binary path would also silently lose its cache-invalidation signal
   (fails `-nt` silently rather than erroring).
3. **`enableScripts` is one global on/off switch under Yarn, not pnpm's
   per-package `onlyBuiltDependencies` allowlist** — a real loss of
   expressiveness if a future dependency needs a scoped, trusted install
   script (didn't matter today: none of the three allowlisted packages
   actually need their install script to function).
4. **Unevaluated follow-on cost**: 17 `pnpm` references in the `Makefile`,
   10 CI workflow files, and the editor/IDE TypeScript SDK shim setup
   (`yarn dlx @yarnpkg/sdks vscode`) that PnP requires — flagged as a known,
   documented, required step, not verified live (no VS Code instance in the
   prototype environment).

**Recalibrates the premise, usefully**: the prototype independently found
this repo's own shared-`node_modules` tool (Evaluation 1, already live in a
sibling worktree at prototyping time) already collapses the common case — a
new worktree with an *unchanged* lockfile costs ~0 marginal inodes today,
not 82k. PnP's per-worktree footprint (`.pnp.cjs`, ~1.5MB; `.yarn/unplugged/`
for packages needing real on-disk files, ~265MB/327 files in this repo) is
real but **narrower than the "50+ worktrees" framing suggests** given that.
Its distinct advantage is finer-grained, individual-package-version caching
(`~/.yarn/berry/cache`) that would also help a worktree whose lockfile has
*diverged* from main — a case Evaluation 1's lockfile-hash-keyed scheme does
not help with (a diverging lockfile there means a brand-new full tree). If
ever revisited, the case for PnP is "retire the custom shared-node-modules
tool entirely," not "inodes saved per identical-lockfile worktree," which
Evaluation 1 already covers.

No prototype branch was pushed (the local diff is small — `buf.gen.yaml` and
`web-app/package.json` — but `.yarn/unplugged/` is 265MB of binary cache
content not worth pushing for a non-shipping evaluation); the full diff is
reproducible from the steps above if revisited.

## Evaluation 3: Bazel (`aspect-build/rules_js`)

**Verdict: skip — it doesn't solve this problem, and plausibly makes it
worse.**

- `rules_js`'s own `contrib/nextjs` macros
  ([docs](https://aspect.build/docs/bazel/javascript/aspect_rules_js/contrib_nextjs_defs.md))
  wrap `next build` as a single `nextjs_build` target consuming all sources
  and the config file — consistent with `next build` running as one opaque
  Bazel action (not decomposed into Bazel's fine-grained target graph), not
  a fine-grained incremental Next.js build. No doc found stating this
  outright either way; this is an inference from the macro's shape.
- **Critically, it doesn't reduce file count — it adds a second
  materialization layer.** `npm_translate_lock` still produces a pnpm-style
  symlink forest under `bazel-bin/node_modules/.aspect_rules_js` +
  per-package `bazel-bin/packages/<pkg>/node_modules`
  ([npm_translate_lock docs](https://docs.aspect.build/rules/aspect_rules_js/docs/npm_translate_lock)).
  Bazel's `output_base` (where `bazel-bin` lives) is an MD5 hash of the
  **absolute workspace directory path**
  ([Bazel output directory layout docs](https://bazel.googlesource.com/bazel/+/49c9475aa7db252c19157ee059c40862a412df5d/site/docs/output_directories.md)) —
  since every git worktree is a distinct absolute path, each one gets its
  own `output_base` and therefore its own full symlink-forest
  materialization, by default. Forcing a shared `output_base` via
  `--output_base` is possible but Bazel invocations hold a lock on their
  output base, serializing concurrent builds against it — exactly the kind
  of contention this repo's dozens-of-parallel-worktrees workflow can't
  tolerate.
- **Cost**: `BUILD.bazel` authoring/upkeep on every `package.json`/
  `next.config.ts` change, `MODULE.bazel`/bzlmod maintenance, CI integration
  — all before getting to whether it helps, which (above) it doesn't for
  this specific problem. Not worth it at this project's scale (a
  single-developer tool), independent of the file-count question.

## Evaluation 4: Bun as a drop-in pnpm replacement

**Verdict: lateral move on the actual problem (file count); real but
orthogonal compatibility risk if used as more than an installer.**

Bun's isolated linker (default for workspace/monorepo installs since 1.3.2,
[docs](https://bun.com/docs/pm/isolated-installs)) is architecturally the
same idea as pnpm: a central store with symlinks into each project's
top-level `node_modules`. It dedupes package *bytes* the same way pnpm's
store already does — Bun's "hundreds of MB → a few MB" marketing claim is
about disk bytes via global-store hardlinks, not file/inode count
([global-cache docs](https://bun.com/docs/pm/global-cache)). The
per-project symlink-tree *entry count* — the actual problem here — stays
comparable to pnpm's, since that's inherent to isolated/strict linking
generally, not something Bun does differently. (Bun's other mode, the
hoisted/flat linker, is the classic npm/yarn flat tree — not smaller either.)
Switching package managers is not a lever for the file-count problem
specifically.

Compatibility: no direct report found for `@vanilla-extract/next-plugin`
under Bun specifically, but that plugin is a webpack loader consumed by
Next's own bundler — irrelevant if Bun is used only as the installer (`bun
install`) while Node still runs `next build`. The real, separate risk shows
up if Bun is used as the **runtime** for the build itself: a reported
Vercel-community case has a Next.js 16.3 build failing on one pinned Bun
runtime patch version while succeeding on a newer one
([community thread](https://community.vercel.com/t/next-js-16-3-bun-runtime-1-3-12-build-failure-on-vercel-while-bun-1-3-14-works-locally/47139)) —
genuine Bun-version sensitivity for *running* Next's build, distinct from
installing with it. Speed (which Bun genuinely wins at) was never the
problem this investigation was commissioned to solve.

## Comparison

| Option | Reduces file-count/inode overhead? | Migration cost | Compatibility risk (vanilla-extract, native builds, custom webpack config, Playwright) | Verdict |
|---|---|---|---|---|
| **Shared node_modules dir (this PR)** | Yes — measured: ~82k entries → 1 shared copy + N symlinks | Low (one script, one Makefile hook, one skill update) | None found — full `next build`/`jest`/`eslint`/`stylelint`/Playwright all verified working against it | **Ship it** |
| Yarn Berry PnP | Partial — ~0 marginal cost already matched by Evaluation 1 for an unchanged lockfile; PnP's real edge is only for a *diverging* lockfile per worktree | Moderate (3 confirmed peer-dep fixes, 1 `buf.gen.yaml` rewrite, 17 Makefile + 10 CI references unevaluated, untested IDE SDK step) | None found on the two highest-risk items (vanilla-extract, local `file:` plugins); real but fixable breaks elsewhere (phantom peers, `.bin` path assumption) | Viable — present for human decision |
| Bazel (`rules_js`) | No — adds a second per-worktree materialization layer via per-path `output_base` | High (BUILD files, bzlmod, CI) | Unresolved/unneeded given no file-count benefit | Skip |
| Bun (installer only) | No — same isolated-linker file-count shape as pnpm | Low if install-only | Build-runtime version sensitivity if used beyond install | Not worth it for this problem; pnpm's store already does the byte-dedup Bun would offer |

## Recommendation

Ship the shared-`node_modules` directory design (this PR). It directly
targets the measured root cause (duplicated directory/symlink tree
structure, ~82k filesystem entries per worktree), was validated against the
repo's actual build/test/lint tooling rather than assumed to work, has a
narrow and well-tested blast radius (one script + one Makefile hook, with an
env-var escape hatch), and folds its garbage collection into the existing
`worktree-disk-cleanup` skill instead of inventing a second mechanism.

Bazel and Bun are not recommended for this problem at this project's scale —
see Evaluations 3 and 4 for the specific reasons, not a speed/ecosystem
discussion.

Yarn Berry PnP is genuinely viable — the prototype found zero friction on
both items flagged highest-risk going in (vanilla-extract's build-time CSS
extraction, the two local `file:` ESLint plugins) — but its case is weaker
than it looked before prototyping, precisely because this PR's
shared-`node_modules` design already neutralizes the common case (an
unchanged lockfile costs ~0 marginal inodes either way). PnP's distinct,
real edge — finer-grained per-package-version caching that also helps a
worktree whose lockfile has *diverged* from main, which Evaluation 1 does
not help with — plus retiring a from-scratch tool in favor of an
ecosystem-standard one, has to be weighed against a moderate, real migration
cost (3 confirmed peer-dependency fixes with more likely on future bumps, a
`buf.gen.yaml` rewrite, 17 Makefile + 10 CI workflow references not yet
touched, and an untested required IDE SDK setup step). Per the task that
commissioned this investigation, that's a human decision, not shipped here.
