# Implementation Plan: remote-detection-manifests

**Feature (if/when it proceeds)**: Fetch versioned agent-detection-pattern manifests from a
remote HTTPS source, cache them locally, and merge them into the detector registry between
built-ins and user-authored local plugins — so a detection-pattern fix can reach a running
instance without a full stapler-squad release.
**Date**: 2026-08-06
**Status**: **Phase 1 ready for implementation. Phase 2 BLOCKED — do not start.**
**ADRs**:
- [ADR-001](../decisions/ADR-001-manifest-version-comparison-mechanism.md) — manifest-content version comparison mechanism (Phase 2, provisional)
- [ADR-002](../decisions/ADR-002-remote-source-trust-and-pinning.md) — remote source trust/pinning (Phase 2, provisional)
- Reused, not re-litigated: `project_plans/detector-plugins/decisions/ADR-001` (go-toml v2) through `ADR-004` (trust boundary/resource caps) — see Pattern Decisions below.

---

## Why this plan has two phases, and why they gate differently

Research surfaced two **independent** findings that make "just build the remote-fetch feature"
the wrong shape for this plan:

1. **The stated foundation isn't on `main`.** `detector-plugins`' local TOML plugin loader
   (`session/detection/plugins.go`, `detector_snapshot.go`, `plugin_watcher.go`,
   `registry.go`'s `MergedRegistry`) does not exist in this working tree. Its PR
   (`gh pr view 307 --repo tstapler/stapler-squad` → `state: CLOSED`, `mergedAt: null`) was
   closed 2026-08-02 with the rationale "this branch's last known commit (`32f504c8`) is
   already present on main" — verified false: `git merge-base --is-ancestor 3c25e94f9 HEAD`
   exits 1 (not an ancestor), and none of the three plugin-loader files exist anywhere in
   `main`'s tree. **This item cannot be built on top of code that isn't there.**
2. **Independently, even if it were there**, `detector-plugins/requirements.md` set an
   explicit 90-day demand-validation checkpoint (started 2026-08-02, target ~2026-10-31,
   ~4 days elapsed as of this plan) before "the deferred remote-manifest/issue-#178 work" —
   i.e. this item — should proceed. `research/pitfalls.md` §5 and `research/build-vs-buy.md`
   §5 both independently concluded the same thing and named a cheaper, checkpoint-orthogonal
   alternative (lower-friction `binaries/*.go` PR review process, `build-vs-buy.md` §3) worth
   pursuing regardless of how the checkpoint resolves.

These two findings are orthogonal, so they gate different work differently:

| Phase | What it is | Gated by finding #1? | Gated by finding #2 (checkpoint)? |
|---|---|---|---|
| **Phase 1 — Unblock** | Investigate why PR #307 was closed incorrectly; re-land the already-reviewed local TOML plugin loader | N/A — this IS the fix for #1 | **No.** Re-landing previously-reviewed, already-designed work is not "building new user-extensible-detection infrastructure to serve unvalidated demand" — it restores something 4 ADRs and a full review cycle already accepted. |
| **Phase 2 — Remote-fetch layer** | The actual herdr-style fetch+cache+version-compare+fallback feature | Yes — needs Phase 1's files to exist | **Yes.** This is exactly the work `detector-plugins/requirements.md` named as gated on the checkpoint. |

**Phase 2 is fully planned below** (concrete files, tasks, ~2-5 min granularity) because a
future implementer picking this up after the checkpoint resolves needs a real plan, not a
placeholder — but every story and task in Phase 2 carries an explicit
**BLOCKED — do not start until: (a) Phase 1 lands, AND (b) the detector-plugins 90-day demand
checkpoint (target ~2026-10-31) resolves toward "demand confirmed," OR a user/owner explicitly
overrides the checkpoint** annotation. Do not begin any Phase 2 task without first confirming
both conditions with the user.

---

## Domain Glossary

### Phase 1 terms (recovery)

| Term | Definition | Notes |
|------|-----------|-------|
| PR #307 | `feat(detection): user-extensible TOML agent-detector plugins`, GitHub PR against `tstapler/stapler-squad`, head branch `backlog/stapler-squad-detector-plugins`. | `state: CLOSED`, `mergedAt: null`. Verified via `gh pr view 307 --repo tstapler/stapler-squad`. |
| Closure Rationale | The 2026-08-02 closing comment: *"Closing as superseded: this branch's last known commit (32f504c8...) is already present on main, so this item's work has already shipped through another path."* | Contradicted by `git merge-base --is-ancestor 32f504c8 HEAD` (also not an ancestor) and by the absence of any of the PR's symbols in `main`'s tree. Root-causing *why* this comment was written in error is in scope for Phase 1 Story 1.1. |
| Feature Commits | The two commits carrying the actual diff on the PR's remote head: `36a951acb` (`feat(detection): ...`) and `c64d94cf8` (`fix(detection): add missing hardening tests...`), on `origin/backlog/stapler-squad-detector-plugins`. | `git diff 32f504c8 c64d94cf8 -- session/detection main.go session/claude_controller.go` — 12 files, 2617 insertions / 19 deletions. This is the diff to re-land, not the whole branch (the branch also carries unrelated `chore(sdd)` planning-artifact commits per the PR body). |
| Recovery Branch(es) | `backlog/stapler-squad-detector-plugins-recovery` / `-recovery2` (local only, not on `origin`) | `-recovery2`'s tip commits `3c25e94f9`/`005e75827` are a re-authored equivalent of the same feature diff, based on a different point in `main`'s history (`git merge-base backlog/stapler-squad-detector-plugins-recovery2 main` → `789775f4d`, vs. the origin branch's merge-base `14e26b7ba`). Either lineage is an acceptable source to re-land from; Task 1.2.1 picks whichever rebases cleaner onto current `main` (`8cbddebab` at plan time). |
| Re-land | The act of getting the Feature Commits' diff merged into `main` — via rebase-and-open-new-PR, not by reopening PR #307 (GitHub does not allow reopening a PR whose head branch may have moved/been force-pushed since; a fresh PR against current `main` is the standard path). | Phase 1's deliverable. |

### Phase 2 terms (remote-fetch layer — carried through from `research/architecture.md`)

| Term | Definition | Notes |
|------|-----------|-------|
| `RemoteManifest` | A fetched TOML document using the exact `detector-plugins` schema (ADR-003: `id`, `version`, `binary_names`, `[[patterns]]`) — no new format. | Not a new type in code; same `pluginFile`/`patternEntry` DTOs `plugins.go` already parses into. |
| `RemoteCacheDir` | `config.GetConfigDir()/detectors-remote-cache/` — a sibling of, not a subdirectory of, `detectors/` (`plugins.go:370-376`'s `PluginDir()`). | Recommendation from `architecture.md` §5, for four independently-justified reasons (trust-boundary visibility, avoiding fsnotify double-triggers, simple per-directory collision detection, free `STAPLER_SQUAD_TEST_DIR`/`STAPLER_SQUAD_INSTANCE` isolation via `config.GetConfigDir()`). |
| `RemoteFetcher` | New unexported struct owning the `*http.Client` + configured source URL, mirroring `AnthropicLimitsClient`'s mutex-guarded-cache shape (`server/services/anthropic_limits_client.go`). | Package/struct-scoped client with fixed `Timeout`, matching `github/http_client.go:17`'s convention (`&http.Client{Timeout: 30 * time.Second}`), not a per-call client. |
| `ManifestVersion` | The manifest-**content** version used to decide "is what I fetched newer than what's cached" — a different axis from ADR-003's schema `version` field (`"1"` vs a future `"2"`, a parser-selection concern). **A value type** (`func ParseManifestVersion(s string) (ManifestVersion, error)`, `func (v ManifestVersion) Compare(other ManifestVersion) int`), not a raw `string` — added 2026-08-06 per architecture-review.md's Blocker (parse-once at manifest-load time, not re-validated on every compare call; eliminates swappable-bare-string-parameter risk between `cachedVersion`/`fetchedVersion`). | Flagged unresolved by `architecture.md` §7; provisional decision in `ADR-001-manifest-version-comparison-mechanism.md`. |
| `AcceptDecision` | Sum type replacing `shouldAcceptManifest`'s free-form `(accept bool, reason string, err error)` return — variants `accepted{}`, `rejectedDowngrade{}`, `rejectedContentMismatch{}`, `rejectedValidationFailed{err}`. Added 2026-08-06 per architecture-review.md's Blocker: the Observability Plan's log-line reason strings and Story 2.2.2's acceptance-criteria reason strings had already drifted into two different free-form vocabularies for the same outcomes; a closed sum type makes the log-line mapping an exhaustive, compiler-checked switch instead. | `architecture-review.md` Blocker #1, resolved via type-driven-design remediation. |
| `refreshRemoteManifests` | `func(ctx context.Context, cacheDir string)` — the background goroutine body: fetch → validate → version-compare → atomic cache write → `rebuildSnapshot`. Never returns a value to a caller (nothing in scope consumes one). | Launched via `go refreshRemoteManifests(ctx, remoteCacheDir)` from `InitRemoteManifests`, per `architecture.md` §3 step 2. |
| `InitRemoteManifests` | `func(ctx context.Context) error` — sibling of `InitPlugins(ctx)` (`plugins.go:486`), called right after it from `main.go`. Does one **synchronous, local-disk-only** scan of `RemoteCacheDir` (no network), then launches `refreshRemoteManifests` in a goroutine. | This is what makes "must not block startup" a structural property, not a race against a timeout — see `architecture.md` §3. |
| `remoteFetchTimeout` | A `context.WithTimeout` bound on the *entire* fetch-plus-parse operation (not just the HTTP round trip), in the 5-15s range. | `research/pitfalls.md` §2 recommends this range from herdr's verified `--connect-timeout 5`/`--max-time 15` and this repo's own 30s convention; final value is an implementation-time constant, not re-litigated here. |
| Three-Layer Precedence | built-in → remote-fetched → local user `.toml`, local always wins. | Requirement from `requirements.md` §Scope; achieved by calling `MergedRegistry` twice (`architecture.md` §2.1) with **no signature change**. |
| `DetectorProvenance` (extended) | The existing `map[string]string` diagnostic (`detector_snapshot.go:142-148`) gains the ability to distinguish `""` (built-in) / local `.toml` path / remote cache file path. | A field decision at plan time, not an architecture one, per `architecture.md` §2.2. |
| Pinned-SHA Fetch | Fetching a manifest from a specific commit SHA (e.g. `raw.githubusercontent.com/.../<sha>/manifest.toml`) rather than a floating `main`/`latest` ref. | `research/pitfalls.md` §1's proportionate middle tier — cheap given a version identifier is needed anyway; provisional decision in `ADR-002-remote-source-trust-and-pinning.md`. |
| Semantic Spoofing | The threat a structurally-valid manifest can still carry: a `needs_approval`/`input_required` pattern quietly deleted, narrowed, or retagged `success` by a compromised/malicious source — passes every schema/regex validator, caught by none of them. | Named in `requirements.md` §Non-functional Requirements (Security) and `research/pitfalls.md` §3; the residual risk pinning/review exists to cover, not validation. |
| Never-Downgrade Rule | A fetched manifest with a `ManifestVersion` lower than, or equal-but-different-content to, the cached one is rejected, not silently accepted. | Adopted directly from herdr's verified behavior (`research/pitfalls.md` §3) as a Phase 2 acceptance criterion. |
| Atomic Cache Write | Write to `<name>.tmp` (or `os.CreateTemp` in `RemoteCacheDir`) then `os.Rename` — this repo's existing convention (`config/state.go:293-304`, `config/config.go:833`, `config/claude.go:211`, `config/workspace_meta.go:57,89`), not a new pattern. | No third-party atomic-write library exists in `go.mod`; none is added. |

**23 terms.**

---

## Pattern Decisions

### Step 0.5 creative pass — Phase 2 approach selection

| # | Approach | Strength | Weakness |
|---|---|---|---|
| (a) | **[CHOSEN]** Separate `detectors-remote-cache/` dir + background goroutine + reuse `MergedRegistry` called twice (`architecture.md`'s recommendation) | Structurally isolates the new network trust boundary from the local-authorship trust boundary; startup latency is provably zero because the synchronous step at `InitRemoteManifests` is disk-only; reuses `LoadPluginDir`/`rebuildSnapshot`/`MergedRegistry`/the validation pipeline entirely unchanged. | More moving parts than (b) or (c): a new directory, a new writer goroutine, a `rebuildSnapshot` signature generalization touching code three call sites depend on. |
| (b) | Fetch replaces bundled `embed.FS` content directly, no separate cache dir — merge remote content into the same directory/registry structure `detectors/` already uses, with no independent persistence step | Fewer directories and files to reason about; smaller initial diff. | Directly contradicts `architecture.md` §5's four independently-justified reasons for a separate directory — mixes remote content into the fsnotify-watched local directory, causing spurious double-rebuilds on every cache write (§5.2) and complicating same-directory collision detection with a remote/local distinction it doesn't currently need (§5.3); also erodes the trust-boundary visibility `requirements.md`'s own Security NFR asks for. Rejected. |
| (c) | Manual refresh only — a `stapler-squad detectors refresh` CLI command, no auto-fetch, no startup/background-goroutine complexity at all | Zero startup-latency risk by construction (no automatic network call ever); simplest to build, test, and reason about; fully sidesteps the three-writer concurrency question in `architecture.md` §4. | Defeats the feature's actual value proposition — herdr's model, and this item's own Success Metrics (`requirements.md`: "reach a running instance ... without a binary rebuild"), depend on *not* requiring the user to remember to run a command on a machine they may not be actively watching. A manual-only command is closer to "`git pull && go build`" (already available today per `research/pitfalls.md` §4) than to the fix this item is meant to add. Rejected as the primary design, though nothing here precludes *also* offering a manual-trigger command as an addition once (a) is built — out of scope for this plan. |

**(a) is carried through the rest of this plan**, matching `architecture.md`'s own conclusion; (b) and (c) are recorded here, not developed further.

### Technology validation + pattern selection

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Manifest-content version comparison | **Flagged, not finalized here — see ADR-001.** Provisional: plain dotted-numeric monotonic string compare (no semver library), reusing the pattern herdr verified (`research/pitfalls.md` §3). | `architecture.md` §7 explicitly declines to decide this; `go.mod` has no `semver`/`Masterminds`/`hashicorp/go-version` dependency today. | A semver library (`Masterminds/semver`) | Would be the *first* semver dependency in this repo for a single narrow comparison a ~20-line dotted-integer compare already covers; inconsistent with the repo's demonstrated zero-extra-dep convention for this exact class of problem (`research/stack.md` §2, `research/build-vs-buy.md` §1). ADR-001 records the final call once Phase 2 unblocks. |
| Remote source trust/pinning | **Flagged, not finalized here — see ADR-002.** Provisional: HTTPS-only + pinned-commit-SHA fetch against `raw.githubusercontent.com` on this same repo. | `research/pitfalls.md` §1's proportionate-mitigation table. | Full cryptographic signing (GPG/sigstore) | `requirements.md`'s own "Rabbit Holes" section rules this out explicitly ("far beyond what a personal-scale tool with no telemetry needs"); the pinned-SHA approach and "edit the local `.toml` by hand" (already shipped) share the *same* actual trust root (Tyler's own GitHub push access), so signing would defend against a threat that doesn't materially differ from one already accepted elsewhere in this repo's release process. ADR-002 records the final call. |
| `MergedRegistry` reuse for 3-layer precedence | Call `MergedRegistry` twice: `withRemote := MergedRegistry(builtins, remoteDetectors)`, then `final := MergedRegistry(withRemote, localDetectors)` — **no signature or body change**. | `detector-plugins` ADR-002 (registry-level snapshot); `architecture.md` §2.1 confirms `Upsert`-based construction composes transitively. | A three-way merge function, or a `[]Layer` parameter | `Upsert`'s last-write-wins semantics already compose correctly through repeated application; a new merge function would duplicate logic `MergedRegistry` already has and tests already cover. |
| `rebuildSnapshot` — one dir to two | Generalize `rebuildSnapshot(ctx, dir)` → `rebuildSnapshot(ctx, localDir, remoteCacheDir string)`, calling the existing `LoadPluginDir` once per directory. | `architecture.md` §2.2; `LoadPluginDir` (`plugins.go:517`) is already a pure "scan this directory" function with no assumption about *why* files are there. | A parallel `rebuildRemoteSnapshot` function | Would duplicate the entire failure-isolation/provenance-building logic `rebuildSnapshot` already implements (directory-scan-failure vs. per-file-skip, `detector_snapshot.go:182-198`) for no behavioral difference — the remote directory needs *exactly* the same semantics, just a second source. |
| Concurrency for the three writers (initial rebuild, fsnotify-triggered local rebuild, network-triggered remote rebuild) | Reuse the existing `activeSnapshot atomic.Pointer[detectorSnapshot]` + `snapshotWriteMu sync.Mutex` pair unchanged (`detector_snapshot.go:34-37`). | `architecture.md` §4; `detector-plugins` ADR-002's whole-sequence-mutex rationale generalizes unchanged from two writers to three. | A per-source lock, or a separate "remote snapshot" pointer reconciled against the local one | The lost-update risk (a slower-started-but-slower-finishing writer landing stale data after a faster one) is identical in shape regardless of writer count; the existing whole-sequence mutex already closes it. A second pointer would need its own reconciliation logic that doesn't otherwise exist. |
| Cache location | New sibling directory `detectors-remote-cache/`, NOT inside `detectors/`, NOT fsnotify-watched. | `architecture.md` §5 (four independently-justified reasons). | Reuse `detectors/` directly (creative alt (b) above) | Rejected in the Step 0.5 pass above — trust-boundary visibility, fsnotify double-trigger avoidance, per-directory collision-detection simplicity. |
| HTTP client shape | Package/struct-scoped `*http.Client{Timeout: N}` + `http.NewRequestWithContext`, matching the repo's only existing convention. | `github/http_client.go:17`, `server/services/anthropic_client.go:37`, `server/services/anthropic_limits_client.go`'s mutex-guarded-cache-with-fallback shape. | A retry/backoff library (`cenkalti/backoff`, `hashicorp/go-retryablehttp`) | Not used anywhere else in this codebase (`research/stack.md` §1); every existing network caller fails fast and falls back to cache rather than retrying — matching that convention keeps this feature's failure semantics consistent with the rest of the repo. |
| Atomic cache write | Temp-file-in-same-dir + `os.Rename`, this repo's existing convention. | `config/state.go:293-304`, `config/config.go:833`, `config/claude.go:211`, `config/workspace_meta.go:57,89`. | A third-party atomic-write library (`google/renameio`, `natefinch/atomic`) | None exists in `go.mod`; the hand-rolled idiom is already used in 20+ files in this repo and needs no new dependency. |
| Remote-fetch dependency (overall build-vs-buy) | Hand-rolled fetcher on stdlib `net/http` + the existing (once re-landed) TOML validator. | `research/build-vs-buy.md` §1 — no OSS library (`go-github`, `go-getter`, Viper/koanf remote providers) fits the narrow "fetch one small versioned file, cache, fall back silently" shape; §2 rules out SaaS config-distribution (no fleet to target). | Any of the above | Every candidate either solves a bigger problem (multi-source config merge, artifact unpacking) or a narrower one (raw HTTP GET, already stdlib) than this feature needs. |

---

## Migration Plan

**Not applicable.** No database schema change — no `ent` schema edit, no migration. Phase 1
restores files that were already fully designed with zero schema impact (`detector-plugins`
plan.md's own Migration Plan: "not applicable"). Phase 2 adds a new on-disk cache directory
(`detectors-remote-cache/`), created empty on first run, with no data to migrate for existing
installations — identical shape to how `detectors/` itself was introduced.

---

## Observability Plan

### Phase 1
No new logging design needed — this is a re-land of already-designed, already-implemented
logging (see `detector-plugins/implementation/plan.md`'s own Observability Plan, unchanged).
The only Phase-1-specific observability need is around the re-land process itself:
- `git log`/`gh pr view` output captured in Story 1.1's investigation, so the root-cause
  finding is reproducible by a future reader, not just asserted.

### Phase 2 (BLOCKED — planned for when unblocked)
Extends the logging `detector-plugins` already established (`log.Info("detector plugins
loaded", ...)`, `log.Warn("detector plugin rejected", ...)`, both at `detector_snapshot.go:214`
and `plugins.go`'s per-`PluginLoadError` sites) with:
- `log.Info("remote manifest fetched", "source", url, "version", newVersion, "detectors", n)` —
  once per successful fetch-and-cache-write.
- `log.Warn("remote manifest fetch failed", "source", url, "err", err)` — network error, timeout,
  or non-2xx; previous cache/snapshot untouched.
- `log.Warn("remote manifest rejected", "source", url, "reason", "stale version"|"validation
  failed"|"downgrade rejected")` — Never-Downgrade Rule and validation-pipeline rejections,
  reusing the exact `PluginLoadError`-shaped reporting the local loader already has.
- `log.Info("detector plugin overrides remote manifest", "binary", name, "file", localPath)` —
  the second `MergedRegistry` call's collision-log path, mirroring the existing "overrides
  built-in" log line (`registry.go:42-48`) one layer up.
- **Metrics**: none added, matching `detector-plugins`' own decision (no counter/gauge facility
  in `session/detection`; cardinality doesn't justify introducing one for a single-maintainer
  desktop tool).
- **Alerts**: none — same rationale as `detector-plugins`' plan (no alerting pipeline for a
  local, single-user desktop daemon).

---

## Risk Control

### Phase 1
- **Rollback**: a fresh PR is easier to revert than reopening a closed one — if re-landing
  surfaces a real regression (not just a bad closure comment), close the new PR and no code
  has touched `main`.
- **No feature flag needed**: Phase 1 restores exactly what 4 ADRs and a full review cycle
  already accepted (per PR #307's own test-plan checklist: `go build`, `go vet`, `gofmt`,
  the repo's custom linters, `go test -race`, and all 8 acceptance criteria independently
  re-verified). `detector-plugins`' own `STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS=1` kill switch
  (ADR-004 §4) ships as part of the re-land itself and is Phase 1's rollback lever if a
  post-merge issue surfaces in production.

### Phase 2 (BLOCKED — planned for when unblocked)
- **Feature flag**: a **second**, independent kill switch,
  `STAPLER_SQUAD_DISABLE_REMOTE_MANIFESTS=1`, checked at the top of `InitRemoteManifests` —
  deliberately separate from `STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS` so a user can disable
  remote fetch while keeping local `.toml` plugins working (and vice versa). Not a
  `config.GetFeatureFlag` entry, for the same reason ADR-004 gives for the local flag
  (`GetFeatureFlag` defaults unset keys to `false`, which would silently disable the feature
  the flag is meant to let users *opt out* of, inverting the intended default).
- **Rollback procedure**, escalating:
  1. Delete `RemoteCacheDir`'s contents — next `InitRemoteManifests` synchronous scan finds
     nothing, falls back to built-in/local-only behavior, no restart needed for the *cache*
     itself, though the in-memory snapshot won't drop remote entries until the next rebuild.
  2. Set `STAPLER_SQUAD_DISABLE_REMOTE_MANIFESTS=1` and restart (confirm `--tmux-keep-server`
     per `.claude/rules/tmux-keep-server-on-restart.md` first).
  3. Revert the commit — isolated to `InitRemoteManifests`, the `rebuildSnapshot` two-dir
     generalization, and the new `RemoteFetcher` type; the local-only path (Phase 1) is
     unaffected by a Phase 2 revert because `rebuildSnapshot`'s remote-dir argument degrades
     to "always empty" if the second call site is removed.
- **Staged rollout**: not applicable (no fleet) — same reasoning as `detector-plugins`' plan.
  An empty `RemoteCacheDir` (state of every installation before this ships) is byte-identical
  behavior to Phase 1 alone.
- **Standing operational risk, named explicitly**: `research/pitfalls.md` §4 — the remote
  endpoint is new infrastructure (DNS, TLS cert, availability) this personal-scale project
  would maintain indefinitely, for a benefit (`research/pitfalls.md` §4: "fix a machine Tyler
  isn't at, without `git pull`") that is narrow once the already-shipped local-edit path
  (Phase 1) is priced in as the real counterfactual. This is not a risk Phase 2's design can
  engineer away — it is the core reason Phase 2 is gated on the demand checkpoint at all.

---

## Unresolved Questions

1. **Which recovery branch to rebase from** (Phase 1, Task 1.2.1) — `origin/backlog/
   stapler-squad-detector-plugins` (PR #307's actual head, commits `36a951acb`/`c64d94cf8`)
   vs. local-only `backlog/stapler-squad-detector-plugins-recovery2` (commits `3c25e94f9`/
   `005e75827`, re-authored, different merge-base with `main`). **Not resolved here** —
   Task 1.2.1 picks whichever rebases cleaner onto current `main` at implementation time; both
   carry the identical reviewed design.
2. **Manifest-content version comparison mechanism** (Phase 2) — flagged by `architecture.md`
   §7, provisional answer in ADR-001, final decision deferred to Phase 2 unblock time since no
   code will be written against it until then.
3. **Remote source trust/pinning mechanism** (Phase 2) — flagged by `architecture.md` §6,
   provisional answer in ADR-002, same deferral reasoning.
4. **Whether the manifest source is this repo itself vs. a separate data repo**
   (`requirements.md` Open Question #2) — genuinely unresolved, not addressed by research
   because it doesn't affect the technical design (both are a plain HTTPS GET); a decision for
   whoever picks up Phase 2, informed by whichever the pinned-SHA ADR-002 decision favors.
5. **Whether the 90-day checkpoint itself is being tracked anywhere durable** — `research/
   build-vs-buy.md` §5 flags a process risk that the checkpoint could lapse unchecked over 90
   days without anyone re-evaluating it. Out of scope for this plan to fix (it belongs to
   `detector-plugins`'s own tracking, not this item), but worth naming so Phase 2's "BLOCKED"
   annotations don't silently expire into "forgotten" rather than "deliberately re-evaluated."
   **Confirmed P1 by `pre-mortem.md` #1 (2026-08-06)**: a markdown paragraph is not a durable
   trigger. Before Phase 1 starts, create an actual scheduled artifact for the ~2026-10-31
   re-evaluation (a `create_backlog_item` due on that date, or a `schedule`-skill cron entry) —
   do not rely on anyone re-reading this file.
6. **Whether the checkpoint's demand signal actually measures Phase 2's problem.**
   **Confirmed P1 by `pre-mortem.md` #2 (2026-08-06)**: `detector-plugins`'s checkpoint tracks
   only "was a *new* agent onboarded via `.toml` instead of `binaries/*.go`" — it would resolve
   "no demand" even if a *built-in* agent's detector broke and needed an out-of-band release in
   the same window, which is this item's actual Problem Statement scenario. Before the
   checkpoint date, `detector-plugins/requirements.md`'s checkpoint criteria should gain a
   second, independent signal — "did any built-in detector require an out-of-band `binaries/
   *.go` hotfix release in the window" — tracked alongside new-agent-onboarding, not instead of
   it. This edit belongs to `detector-plugins`, not this plan; recorded here so it isn't lost.

---

## Dependency Visualization

```
PHASE 1 (unblock — not gated by checkpoint)

  Story 1.1: Investigate PR #307 closure
       │  (gh pr view, git merge-base, symbol grep — already done in this plan's
       │   own research; Story 1.1 re-verifies at implementation time in case
       │   main has moved, and documents the finding for the PR description)
       ▼
  Story 1.2: Re-land the feature diff
       │
       ├── Task 1.2.1: Pick source branch, rebase onto current main
       ├── Task 1.2.2: Resolve conflicts (if any) against main's current
       │                session/detection/*, main.go, session/claude_controller.go
       ├── Task 1.2.3: Re-run full verification (build/vet/gofmt/lint/test -race)
       ├── Task 1.2.4: Open a fresh PR against current main
       └── Task 1.2.5: Merge
              │
              ▼
       session/detection/{plugins.go, detector_snapshot.go, plugin_watcher.go,
       registry.go's MergedRegistry} NOW EXIST ON MAIN
              │
              ▼  (this is what unblocks Phase 2 — condition (a) of its gate)
════════════════════════════ CHECKPOINT GATE ════════════════════════════
              │  (condition (b): 90-day demand checkpoint resolves
              │   "demand confirmed", ~2026-10-31, OR explicit override)
              ▼
PHASE 2 (remote-fetch layer — BLOCKED on both conditions above)

  Epic 2.1: RemoteCacheDir + RemoteFetcher            [BLOCKED]
       │
  Epic 2.2: Version compare + Never-Downgrade Rule    [BLOCKED]  (needs ADR-001 finalized)
       │
  Epic 2.3: rebuildSnapshot generalized to two dirs   [BLOCKED]  (needs Epic 2.1, 2.2)
       │        + MergedRegistry called twice
       │
  Epic 2.4: InitRemoteManifests + background goroutine [BLOCKED] (needs Epic 2.3)
       │        wiring into main.go
       │
  Epic 2.5: Trust/pinning + kill switch + logging      [BLOCKED] (needs ADR-002 finalized;
                                                                    can start in parallel with
                                                                    2.1-2.4's non-trust pieces)
```

Critical path (Phase 1): 1.1 → 1.2.1 → 1.2.2 → 1.2.3 → 1.2.4 → 1.2.5.
Critical path (Phase 2, once unblocked): 2.1 → 2.2 → 2.3 → 2.4, with 2.5's trust/pinning work
parallelizable against 2.1-2.3 (it only needs the `RemoteFetcher` HTTP-call site to exist, not
the full snapshot-integration chain) but must land before 2.4's goroutine is ever enabled by
default.

---

## Phase 1: Unblock — Investigate and Re-land `detector-plugins`

**Gating**: none beyond normal review. This phase is **not** gated by the 90-day demand
checkpoint — it restores previously-reviewed, already-designed work (4 ADRs, a passing test
plan, 8 independently-verified acceptance criteria per the PR body) rather than building new
user-extensible-detection infrastructure to serve unvalidated demand. Start immediately.

### Epic 1.1: Root-cause the incorrect PR closure
**Goal**: Establish, on the record, why PR #307 was closed with a rationale that verification
contradicts — so the re-land PR's description can state the correction plainly and a future
reader doesn't have to re-derive it.

#### Story 1.1: Verify and document the closure discrepancy
**As a** future maintainer looking at PR #307's closed state, **I want** a clear, re-verified
record of why the closure rationale was wrong, **so that** I don't have to redo the git
archaeology myself before trusting that re-landing is warranted.
**Acceptance Criteria**:
- The closure claim is independently re-checked against current `main`, not just cited from
  this plan's own research (which may be stale by implementation time).
  - *Given* current `main` HEAD at implementation time,
    *When* `git merge-base --is-ancestor 3c25e94f9 HEAD` and
    `git merge-base --is-ancestor 32f504c8 HEAD` are both run,
    *Then* both exit non-zero (not-an-ancestor), reproducing this plan's finding.
  - *Given* current `main`,
    *When* `grep -rln "InitPlugins\|EnsurePluginDir\|ResolveDetectorForProgram" --include=*.go .`
    is run (excluding `.claude/worktrees/`),
    *Then* it returns no matches, confirming the plugin-loader symbols are absent.
- The finding is written into the re-land PR's description (Story 1.2, Task 1.2.4), not left
  only in this plan.
**Files**: none (investigation only); output feeds Task 1.2.4's PR body.

##### Task 1.1a: Re-run the verification commands against current `main` (~3 min)
- `gh pr view 307 --repo tstapler/stapler-squad --json state,mergedAt,closedAt,comments`
- `git merge-base --is-ancestor 3c25e94f9 HEAD; echo $?` and same for `32f504c8`
- `grep -rln "InitPlugins\|EnsurePluginDir\|ResolveDetectorForProgram" --include=*.go . | grep -v .claude/worktrees`
- Record output verbatim (paste into the task's own scratch notes, not into any repo file).

##### Task 1.1b: Draft the correction paragraph for the re-land PR (~3 min)
- One paragraph: what the closure comment claimed, what verification shows, and that this PR
  re-lands the same reviewed diff PR #307 carried. No repo file changes — this becomes part of
  Task 1.2.4's PR body draft.

---

### Epic 1.2: Re-land the feature diff onto current `main`
**Goal**: Get the already-reviewed `session/detection/{plugins.go, detector_snapshot.go,
plugin_watcher.go}` + `registry.go`'s `MergedRegistry`/`Upsert` + the `main.go`/
`session/claude_controller.go` wiring merged into `main`, unchanged in design from what PR #307
already carried and 4 ADRs already accepted.

**If, during Task 1.2.1, re-verification finds the closure WAS actually correct after all**
(i.e., this plan's own research was wrong) — stop, do not proceed with Epic 1.2, and instead
write a short correction note explaining what the earlier research got wrong and why the
closure stands. That outcome is possible in principle even though every check performed so far
points the other way; Task 1.2.1 is the last chance to catch it before spending effort re-landing
something that shouldn't be re-landed.

#### Story 1.2: Rebase, verify, and merge the feature diff
**As** the project owner, **I want** the local TOML plugin loader actually merged into `main`,
**so that** `detector-plugins`' shipped-and-tested design is real infrastructure other work
(including this item's own Phase 2, later) can build on.
**Acceptance Criteria**:
- The re-landed diff matches the reviewed design with no unexplained deviation.
  - *Given* the rebased branch,
    *When* `git diff <rebase-base> HEAD -- session/detection main.go session/claude_controller.go
    session/instance_terminal.go session/claude_controller_test.go` (widened 2026-08-06 per
    adversarial-review.md Blocker #2 to include the two previously-omitted files) is compared
    against the original `git diff 32f504c8 c64d94cf8` (or the `recovery2` equivalent),
    *Then* the only differences are mechanical conflict-resolution hunks against files that
    changed on `main` since the PR was authored (documented in Task 1.2.2), not new design
    decisions.
- All of PR #307's own test-plan items pass again post-rebase.
  - *Given* the rebased branch,
    *When* `go build ./session/... ./.`, `go vet ./session/...`, `gofmt -l` (on touched files),
    the repo's custom linters (`hotpolllog`/`nocommandpattern`/`norawexec`/`tmuxsocketscope`),
    and `go test ./session/... -count=1` are run,
    *Then* all pass with zero failures, matching the original PR body's checklist.
  - *Given* the rebased branch,
    *When* `go test -race ./session/detection/... -count=1` is run,
    *Then* it passes clean (no race detected), reproducing the original PR's own claim.
- `make lint` and `make quick-check` (this repo's actual gate, per `CLAUDE.md`) both pass.
- The new PR's description states the closure correction from Story 1.1.
**Files**: `session/detection/plugins.go` (new), `session/detection/detector_snapshot.go` (new),
`session/detection/plugin_watcher.go` (new), `session/detection/{plugins,detector_snapshot,
plugin_watcher}_test.go` (new), `session/detection/registry.go` (modified — adds
`MergedRegistry`), `session/detection/registry_test.go` (modified), `session/detection/
binary_detector.go` (modified — adds `Upsert`), `session/detection/detector.go` (modified —
`lookupBinaryDetector` repoints `DetectForProgram`), `main.go` (modified — wires `InitPlugins`),
`session/claude_controller.go` (modified — resolves via `ResolveDetectorForProgram`),
**`session/instance_terminal.go` (modified — adds `Instance.GetProgram()`, the accessor
`ResolveDetectorForProgram`'s call site depends on) and `session/claude_controller_test.go`
(modified) — both added 2026-08-06 per adversarial-review.md Blocker #2: `git diff 32f504c80
c64d94cf8 --diff-filter=M --name-only` shows both files carry real, substantive changes that
the original Files list and re-verification diff command omitted**, `go.mod`/
`go.sum` (adds `pelletier/go-toml/v2`).

##### Task 1.2.1: Pick the source branch and start the rebase (~5 min)
- Compare `origin/backlog/stapler-squad-detector-plugins` (commits `36a951acb`/`c64d94cf8`,
  merge-base with `main` at `14e26b7ba`) against local `backlog/stapler-squad-detector-plugins-
  recovery2` (commits `3c25e94f9`/`005e75827`, merge-base with `main` at `789775f4d`) — check
  which has a smaller/cleaner diff to rebase against current `main`
  (`git rebase --onto main <merge-base> <branch>` dry-run via `git rebase -i` abort-if-conflicts
  check, or simply attempt both and compare conflict counts).
- Pick the cleaner one; abort and discard the other rebase attempt.
- If Story 1.1's re-check (Task 1.1a) found the closure was actually correct, stop here — do
  not proceed.
- Files: none yet (branch/rebase state only).

##### Task 1.2.2: Resolve conflicts against current `main` (~5-15 min, size varies)
- `main` has moved since either source branch was authored — expect conflicts limited to import
  ordering, adjacent unrelated changes in `main.go`'s `RunE` (other features wired in since
  2026-08-01/02), and possibly `session/detection/detector.go` if `DetectForProgram` was touched
  by unrelated work.
- Resolve preserving the reviewed design exactly (no new logic beyond what's needed to compile
  against `main`'s current state) — every substantive resolution gets a one-line comment in the
  commit message explaining what changed and why, per Story 1.2's acceptance criterion.
- Files: whichever files conflict — expected candidates `main.go`, `session/detection/
  detector.go`, `session/claude_controller.go`.

##### Task 1.2.3: Re-run the full verification suite (~5 min)
- `go build ./session/... ./.`
- `go vet ./session/...`
- `gofmt -l session/detection/*.go main.go session/claude_controller.go` — must be empty output
- `make lint`
- `go test ./session/... -count=1`
- `go test -race ./session/detection/... -count=1`
- `make quick-check`
- Fix anything that regresses relative to PR #307's own passing checklist; do not weaken any
  existing test to make it pass.
- Files: none (verification only) unless a genuine regression is found, in which case fix it in
  the already-touched files above.

##### Task 1.2.4: Open the re-land PR (~3 min)
- `gh pr create` against `main`, title `feat(detection): user-extensible TOML agent-detector
  plugins (re-land of #307)`.
- Body: reuse PR #307's original description (it's still accurate — same feature, same design),
  prepended with Story 1.1's correction paragraph (Task 1.1b) explaining that this re-lands
  work incorrectly closed as "already shipped," with the verification commands and their output
  from Task 1.1a.
- Per `.claude/rules/gh-pr-merge-repo-flag.md`, remember `--repo` on any later `gh pr merge`
  call for this PR.
- Files: none (GitHub PR object only).

##### Task 1.2.5: Merge (~2 min, after CI + any requested review)
- Once CI is green and review (if requested) is satisfied, merge per the project's normal PR
  process — `make ready`/CI gates, not a special path for being a "re-land."
- Confirm post-merge: `grep -rln "InitPlugins" --include=*.go main.go` now matches, and
  `go build ./...` on fresh `main` succeeds.
- Files: none (merge only).

---

## Phase 2: Remote-Fetch Layer — Fetch, Cache, Version-Compare, Fallback

> **BLOCKED — do not start any story or task in this phase until: (a) Phase 1 lands on `main`,
> AND (b) the `detector-plugins` 90-day demand checkpoint (target ~2026-10-31) resolves toward
> "demand confirmed," OR a user/owner explicitly overrides the checkpoint.** This phase is
> planned in full now so a future implementer has a real plan to execute, not a placeholder —
> but planning it is not authorization to start it. Every story below repeats this annotation.

**File:line anchors below** are written as they will exist once Phase 1 merges (i.e.
`session/detection/plugins.go:370-376` etc.) — these files do not exist on `main` as of this
plan's writing; see Phase 1.

### Epic 2.1: Remote cache directory + fetch client
**BLOCKED — see phase-level gate above.**
**Goal**: A `RemoteFetcher` that can GET a manifest over HTTPS, and a `RemoteCacheDir` helper
mirroring `PluginDir()`'s derivation, with no network call wired into any startup path yet.

#### Story 2.1.1: `RemoteCacheDir()` helper
**As a** developer, **I want** the remote cache directory derived the same way `PluginDir()` is,
**so that** it inherits `STAPLER_SQUAD_TEST_DIR`/`STAPLER_SQUAD_INSTANCE` isolation for free.
**Acceptance Criteria**:
- Returns a sibling of, not a child of, the plugin directory.
  - *Given* `config.GetConfigDir()` returns `/tmp/x/.stapler-squad`,
    *When* `RemoteCacheDir()` is called,
    *Then* it returns `/tmp/x/.stapler-squad/detectors-remote-cache`, distinct from
    `PluginDir()`'s `/tmp/x/.stapler-squad/detectors`.
- Inherits test isolation.
  - *Given* `STAPLER_SQUAD_TEST_DIR` is set,
    *When* `RemoteCacheDir()` is called,
    *Then* it resolves under that test dir, matching `PluginDir()`'s existing behavior for the
    same env var.
**Files**: `session/detection/remote_manifests.go` (new), `session/detection/remote_manifests_test.go` (new)

##### Task 2.1.1a: Implement `RemoteCacheDir()` (~2 min) — BLOCKED
- `func RemoteCacheDir() (string, error)` — `config.GetConfigDir()` then
  `filepath.Join(cfgDir, "detectors-remote-cache")`, mirroring `PluginDir()` (`plugins.go:370-
  376`) exactly, including its comment about never using raw `os.UserHomeDir()`.
- Files: `session/detection/remote_manifests.go`

##### Task 2.1.1b: Test `RemoteCacheDir()` (~2 min) — BLOCKED
- Two cases per the acceptance criteria above, using `t.Setenv("STAPLER_SQUAD_TEST_DIR", ...)`.
- Files: `session/detection/remote_manifests_test.go`

---

#### Story 2.1.2: `RemoteFetcher` — bounded-timeout HTTPS GET
**As a** background refresh goroutine, **I want** a single call that fetches and returns raw
manifest bytes or a typed error, **so that** the caller doesn't need to know about HTTP
plumbing.
**Acceptance Criteria**:
- A successful fetch returns the response body bytes.
  - *Given* a test HTTP server (`httptest.NewTLSServer`) returning 200 with a valid TOML body,
    *When* `RemoteFetcher.Fetch(ctx, sourceURL)` is called,
    *Then* it returns the body bytes and a nil error.
- The whole operation is bounded by `remoteFetchTimeout`, not just the HTTP round trip.
  - *Given* a test server that accepts the TCP connection but never writes a response,
    *When* `Fetch(ctx, sourceURL)` is called with `remoteFetchTimeout = 100 * time.Millisecond`,
    *Then* it returns within ~150ms (not hangs), with a context-deadline-exceeded-wrapped error.
- A plain-`http://` URL is rejected before any network call.
  - *Given* `sourceURL = "http://example.com/manifest.toml"`,
    *When* `Fetch(ctx, sourceURL)` is called,
    *Then* it returns an error containing `https` without attempting a connection (verified by
    a test server that would fail the test if hit).
- A non-2xx response is a typed failure, not a silent empty success.
  - *Given* a test server returning 404,
    *When* `Fetch` is called,
    *Then* it returns an error containing `404` and no bytes.
**Files**: `session/detection/remote_manifests.go`, `session/detection/remote_manifests_test.go`

##### Task 2.1.2a: Define `RemoteFetcher` struct and constructor (~3 min) — BLOCKED
- `type RemoteFetcher struct { client *http.Client }`, constructor `NewRemoteFetcher() *RemoteFetcher`
  setting `client: &http.Client{Timeout: remoteFetchTimeout}`, matching `github/http_client.go:17`'s
  package-scoped-client convention.
- `const remoteFetchTimeout = 15 * time.Second` (upper end of the 5-15s range `research/
  pitfalls.md` §2 recommends) — comment citing herdr's verified `--max-time 15`.
- Files: `session/detection/remote_manifests.go`

##### Task 2.1.2b: Implement the HTTPS-only guard (~2 min) — BLOCKED
- At the top of `Fetch`, `if !strings.HasPrefix(sourceURL, "https://") { return nil,
  fmt.Errorf("remote manifest source must use https, got %q", sourceURL) }` before any request
  construction — near-zero-cost mitigation from `research/pitfalls.md` §1's table.
- Files: `session/detection/remote_manifests.go`

##### Task 2.1.2c: Implement `Fetch` (~5 min) — BLOCKED
- `func (f *RemoteFetcher) Fetch(ctx context.Context, sourceURL string) ([]byte, error)`.
- `ctx, cancel := context.WithTimeout(ctx, remoteFetchTimeout); defer cancel()` — bounds the
  *whole* operation per `architecture.md` §3 step 2, not just relying on `client.Timeout`.
- `http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)`, `f.client.Do(req)`.
- Non-2xx status → typed error naming the status code.
- Success → `io.ReadAll` the body, respecting `maxPluginFileSize` (ADR-004's 256 KiB cap,
  reused unchanged per `architecture.md` §6) via `io.LimitReader(resp.Body,
  maxPluginFileSize+1)` and rejecting if the limit is hit — same cap, same rationale, applied at
  the network layer this time instead of the filesystem layer.
- Files: `session/detection/remote_manifests.go`

##### Task 2.1.2d: Tests for `Fetch` (~6 min) — BLOCKED
- One test per acceptance criterion above using `httptest.NewTLSServer` (note: `httptest`'s TLS
  server uses a self-signed cert — either `client.Transport` gets its `CertPool`, or tests use
  `server.Client()` as the fetcher's underlying client for the TLS-trust path; do not disable
  cert verification in the shipped code, only in the test's client construction).
- One oversized-body test asserting the `maxPluginFileSize` limit rejects a response with a
  clear error before the caller ever sees the bytes.
- Files: `session/detection/remote_manifests_test.go`

---

### Epic 2.2: Version compare + Never-Downgrade Rule
**BLOCKED — see phase-level gate above. Also depends on ADR-001 being finalized before Task
2.2.1a is implemented** (this plan's ADR-001 records a provisional recommendation; confirm it
still holds at Phase 2 unblock time).

#### Story 2.2.1: `compareManifestVersion`
**As a** refresh goroutine, **I want** to know whether a fetched manifest's version is strictly
newer than the cached one, **so that** the Never-Downgrade Rule can be enforced.
**Acceptance Criteria**:
- A strictly higher dotted-numeric version compares greater.
  - *Given* cached `"1.2.0"` and fetched `"1.3.0"`,
    *When* `compareManifestVersion(fetched, cached)` is called,
    *Then* it returns `1` (fetched is newer).
- An equal version compares equal.
  - *Given* cached `"1.2"` and fetched `"1.2.0"` (missing segment treated as zero, per herdr's
    verified behavior in `research/pitfalls.md` §3),
    *When* `compareManifestVersion` is called,
    *Then* it returns `0`.
- A lower version compares less, and the caller (Task 2.2.2) rejects it (Never-Downgrade Rule).
  - *Given* cached `"1.3.0"` and fetched `"1.2.0"`,
    *When* `compareManifestVersion` is called,
    *Then* it returns `-1`.
- A malformed version string is a typed error, not a panic or silent zero.
  - *Given* fetched `"not-a-version"`,
    *When* `compareManifestVersion` is called,
    *Then* it returns an error naming the malformed string.
**Files**: `session/detection/remote_manifests.go`, `session/detection/remote_manifests_test.go`

##### Task 2.2.1a: Implement `ParseManifestVersion`/`ManifestVersion.Compare` (~4 min) — BLOCKED (confirm ADR-001 first)
- **Updated 2026-08-06 per architecture-review.md Blocker #1** — replaces the originally-specced
  bare-string `compareManifestVersion(a, b string) (int, error)` with a value type:
  `func ParseManifestVersion(s string) (ManifestVersion, error)` — split on `.`, parse each
  segment as `int`, pad the shorter with zeros; called once, at manifest-parse time. Reject
  non-numeric segments with a typed error.
- `func (v ManifestVersion) Compare(other ManifestVersion) int` — no error return; construction
  already proved both operands valid.
- Doc comment citing ADR-001 for why this is a hand-rolled compare rather than a semver
  dependency.
- Files: `session/detection/remote_manifests.go`

##### Task 2.2.1b: Tests for `compareManifestVersion` (~4 min) — BLOCKED
- Table test covering all four acceptance criteria plus edge cases (`"0"` vs `""`, three-segment
  vs two-segment).
- Files: `session/detection/remote_manifests_test.go`

---

#### Story 2.2.2: Enforce the Never-Downgrade Rule + content-must-match-version check
**As an** operator, **I want** a fetched manifest that isn't a genuine version bump rejected,
**so that** a broken or malicious manifest with an unbumped or lower version number can't
silently replace a good cached one.
**Acceptance Criteria**:
- A fetched manifest with a lower version than cached is rejected and the cache is untouched.
  - *Given* a cached manifest at version `"1.3.0"` and a fetch returning version `"1.2.0"`,
    *When* the refresh flow runs,
    *Then* the cache file's contents and mtime are unchanged, and a
    `log.Warn("remote manifest rejected", "reason", "downgrade rejected", ...)` line is emitted.
- A fetched manifest with the same version but different byte content is rejected (content-must-
  match-version, herdr's verified rule per `research/pitfalls.md` §3).
  - *Given* a cached manifest at version `"1.2.0"` with content hash `H1`, and a fetch returning
    version `"1.2.0"` with content hash `H2 != H1`,
    *When* the refresh flow runs,
    *Then* the fetch is rejected with `reason: "version unchanged but content differs"` and the
    cache is untouched.
- A genuinely newer version replaces the cache.
  - *Given* cached `"1.2.0"` and fetched `"1.3.0"` with valid, schema-passing content,
    *When* the refresh flow runs,
    *Then* the cache file is atomically replaced (Epic 2.3) and a
    `log.Info("remote manifest fetched", ...)` line is emitted.
**Files**: `session/detection/remote_manifests.go`, `session/detection/remote_manifests_test.go`

##### Task 2.2.2a: Implement the version-gate + content-hash check (~5 min) — BLOCKED
- **Updated 2026-08-06 per architecture-review.md Blocker #1**: `func shouldAcceptManifest(cached
  *ManifestVersion, cachedContent []byte, fetched ManifestVersion, fetchedContent []byte)
  AcceptDecision` — `cached == nil` means no cached manifest exists (bootstrap case, see below);
  returns the `AcceptDecision` sum type (variants `accepted{}`, `rejectedDowngrade{}`,
  `rejectedContentMismatch{}`, `rejectedValidationFailed{err}`) instead of a free-form
  `(bool, string, error)`, so the Observability Plan's log-line mapping is an exhaustive switch.
- **Bootstrap case (added 2026-08-06 per adversarial-review.md Blocker #1)**: if `cached == nil`
  (no cached manifest exists yet — every fresh install before its first successful fetch, per
  the Migration Plan's "created empty on first run" note), short-circuit to `accepted{}` without
  calling `.Compare` at all.
- Otherwise calls `fetched.Compare(*cached)`; `< 0` → `rejectedDowngrade{}`; `== 0` → compare
  `sha256(fetchedContent)` against `sha256(cachedContent)`, `rejectedContentMismatch{}` if they
  differ, otherwise `accepted{}` as a noop (nothing changed, no write needed); `> 0` →
  `accepted{}`.
- Files: `session/detection/remote_manifests.go`

##### Task 2.2.2b: Tests for `shouldAcceptManifest` (~5 min) — BLOCKED
- One test per acceptance criterion above, including
  `TestShouldAcceptManifest_should_AcceptWithoutComparing_When_NoCachedManifestExists`
  (bootstrap case — the fresh-install path adversarial-review.md flagged as previously untested).
- Files: `session/detection/remote_manifests_test.go`

---

### Epic 2.3: `rebuildSnapshot` generalized to two directories + `MergedRegistry` called twice
**BLOCKED — see phase-level gate above. Depends on Epic 2.1 (needs `RemoteCacheDir`) but not on
Epic 2.2 (version-compare only matters at write time, not read/merge time).**

#### Story 2.3.1: Generalize `rebuildSnapshot`'s signature and merge logic
**As a** loader, **I want** one rebuild function that scans both the local and remote-cache
directories and merges them with local winning, **so that** every reload path (startup,
fsnotify, network-triggered) produces a consistent three-layer result.
**Acceptance Criteria**:
- With an empty remote cache dir, behavior is byte-identical to Phase 1 alone.
  - *Given* a populated local `detectors/` dir and an empty (or non-existent)
    `detectors-remote-cache/`,
    *When* `rebuildSnapshot(ctx, localDir, remoteCacheDir)` runs,
    *Then* the resulting snapshot is identical to what `rebuildSnapshot(ctx, localDir)` (Phase
    1's one-arg version) would have produced.
- A remote-only detector (no local override) is present and resolvable.
  - *Given* `detectors-remote-cache/my-agent.toml` declaring binary `my-agent`, and no local
    file claiming that name,
    *When* `rebuildSnapshot` runs,
    *Then* `DetectForProgram(..., "my-agent")` resolves via the remote-sourced detector, and
    `DetectorProvenance()["my-agent"]` names the remote cache file's path.
- A local file overriding a remote-sourced binary name wins (Three-Layer Precedence).
  - *Given* `detectors-remote-cache/claude.toml` claiming `claude`, AND
    `detectors/claude-local.toml` also claiming `claude`,
    *When* `rebuildSnapshot` runs,
    *Then* `DetectorProvenance()["claude"]` names the **local** file's path, not the remote
    cache file's.
- A remote-sourced detector overriding a built-in (no local override) wins over the built-in.
  - *Given* `detectors-remote-cache/claude.toml` claiming `claude`, and no local override,
    *When* `rebuildSnapshot` runs,
    *Then* `DetectorProvenance()["claude"]` names the remote cache file's path, not `""`
    (built-in).
- A remote-directory-level scan failure does not affect local-directory detectors.
  - *Given* a valid local `detectors/` dir and `detectors-remote-cache/` replaced by a regular
    file (making its `os.ReadDir` fail),
    *When* `rebuildSnapshot` runs,
    *Then* local detectors still load correctly, and the failure is logged distinctly from a
    local-directory failure (distinguishable `dir` field in the log line).
**Files**: `session/detection/detector_snapshot.go` (modified)

##### Task 2.3.1a: Change `rebuildSnapshot`'s signature and call `LoadPluginDir` twice (~5 min) — BLOCKED
- `func rebuildSnapshot(ctx context.Context, localDir, remoteCacheDir string) error`.
- Call `LoadPluginDir(localDir)` and `LoadPluginDir(remoteCacheDir)` — both already tolerate a
  non-existent directory (`plugins.go:517`'s existing `os.IsNotExist` → `(nil, nil)` handling),
  so an unpopulated `remoteCacheDir` needs no special-case branch.
- A `Field == "directory"` error from *either* scan is logged distinctly (tag which directory
  failed) but only a **local**-directory failure trips the existing "leave previous snapshot
  untouched" fatal path — per Story 2.3.1's last acceptance criterion, a remote-scan failure
  alone should not prevent local-only detectors from loading. (This is a deliberate asymmetry
  from Phase 1's single-directory behavior; document it in a comment.)
- Files: `session/detection/detector_snapshot.go`

##### Task 2.3.1b: Build the three-source provenance map and call `MergedRegistry` twice (~5 min) — BLOCKED
- `builtins := DefaultRegistry()`; `withRemote := MergedRegistry(builtins,
  asBinaryDetectors(remoteDetectors))`; `final := MergedRegistry(withRemote,
  asBinaryDetectors(localDetectors))` — exactly `architecture.md` §2.1's snippet, no
  `MergedRegistry` signature change.
- Provenance loop gains a third pass (built-in → `""`, remote → remote file path, local → local
  file path — local pass runs last so it wins on the same key, matching the merge order above).
- Files: `session/detection/detector_snapshot.go`

##### Task 2.3.1c: Tests for the two-directory merge (~8 min) — BLOCKED
- One test per acceptance criterion above, using two separate `t.TempDir()`s for local and
  remote-cache.
- Files: `session/detection/detector_snapshot_test.go`

---

### Epic 2.4: `InitRemoteManifests` + background refresh goroutine
**BLOCKED — see phase-level gate above. Depends on Epic 2.1, 2.2, and 2.3.**

#### Story 2.4.1: Synchronous local-only startup step (no network)
**As a** session-startup path, **I want** the remote-cache directory scanned exactly like the
local one at startup, **so that** whatever was cached from the last successful fetch (or nothing,
on first run) is available immediately with zero network I/O in the startup path.
**Acceptance Criteria**:
- Startup never blocks on the network.
  - *Given* `InitRemoteManifests(ctx)` is called with no network access available at all (e.g.
    a firewall rule blocking outbound, or a nil-routed DNS),
    *When* it returns,
    *Then* it returns within the same order of magnitude as `InitPlugins`'s existing local-only
    scan (milliseconds, not seconds) — provable by a test that points `RemoteFetcher` at an
    unroutable address and asserts `InitRemoteManifests` itself still returns fast, because the
    network attempt happens only in the *goroutine* it launches, not before returning.
- A populated cache from a prior run is visible immediately.
  - *Given* `detectors-remote-cache/my-agent.toml` already exists from a previous successful
    fetch,
    *When* `InitRemoteManifests(ctx)` runs,
    *Then* `DetectorProvenance()["my-agent"]` is populated before the function returns — no
    need to wait for the background goroutine.
**Files**: `session/detection/remote_manifests.go`

##### Task 2.4.1a: Implement `InitRemoteManifests`'s synchronous half (~4 min) — BLOCKED
- `func InitRemoteManifests(ctx context.Context) error` — checks
  `STAPLER_SQUAD_DISABLE_REMOTE_MANIFESTS` kill switch first (return nil immediately if set,
  per Risk Control); otherwise `os.MkdirAll(remoteCacheDir, 0o755)` (mirroring `EnsurePluginDir`'s
  mode), then calls the Epic 2.3 two-dir `rebuildSnapshot(ctx, localDir, remoteCacheDir)`
  synchronously — this is disk-only, same cost profile as `InitPlugins`'s existing scan.
- Files: `session/detection/remote_manifests.go`

##### Task 2.4.1b: Test the no-network-in-the-synchronous-path guarantee (~5 min) — BLOCKED
- Point the (unused, in this synchronous half) `RemoteFetcher`'s configured source at an
  unroutable test address (`10.255.255.1` or similar RFC 5737/6890 reserved-and-unroutable
  range) and assert `InitRemoteManifests` still returns in well under a second — proving no
  network call happens before the function returns.
- Files: `session/detection/remote_manifests_test.go`

---

#### Story 2.4.2: Background `refreshRemoteManifests` goroutine
**As a** running instance, **I want** the actual network fetch to happen off the startup path,
**so that** a slow, unreachable, or degraded network never delays a session or the app itself.
**Acceptance Criteria**:
- A successful background fetch updates the live snapshot without any caller polling for it.
  - *Given* a reachable test HTTPS server serving a newer, valid manifest,
    *When* `refreshRemoteManifests(ctx, remoteCacheDir)` completes,
    *Then* `DetectorProvenance()` reflects the new remote detector on the very next call — no
    fsnotify round-trip needed (the fetcher calls `rebuildSnapshot` directly per `architecture.md`
    §3 step 3).
- A failed background fetch leaves everything as it was.
  - *Given* an unreachable test server (connection refused),
    *When* `refreshRemoteManifests` runs,
    *Then* `activeSnapshot` and `remoteCacheDir`'s on-disk contents are both unchanged, and a
    `log.Warn("remote manifest fetch failed", ...)` line is the only observable effect.
- The goroutine is launched exactly once per `InitRemoteManifests` call and does not leak across
  repeated calls (mirrors `InitPlugins`'s `sync.Once` re-entrancy guard, Hardening Addendum item
  5 from `detector-plugins`'s own plan).
  - *Given* `InitRemoteManifests(ctx)` is called twice in the same process,
    *When* both calls return,
    *Then* exactly one background goroutine is running (verified via a call counter injected
    into a test double of `RemoteFetcher.Fetch`), not two.
**Files**: `session/detection/remote_manifests.go`, `session/detection/remote_manifests_test.go`, `main.go` (modified)

##### Task 2.4.2a: Implement `refreshRemoteManifests` (~6 min) — BLOCKED
- `func refreshRemoteManifests(ctx context.Context, remoteCacheDir string)` — no return value
  (nothing consumes one, per `architecture.md` §3's carve-out from the double-checked-locking
  rule in `.claude/rules/go-double-checked-locking.md`, which only binds a caller that needs the
  result back).
- Steps: `RemoteFetcher.Fetch` → parse/validate via the **same** `parsePluginFile`/
  `validatePluginFile` pipeline Phase 1 already built (`plugins.go:50,204` — reuse unchanged,
  per `architecture.md` §6's "reuse, don't rebuild, the validation gate") → `shouldAcceptManifest`
  (Epic 2.2) → on accept, atomic temp-then-rename write into `remoteCacheDir` (Task 2.4.2b) →
  call the two-dir `rebuildSnapshot(ctx, localDir, remoteCacheDir)` directly.
- On any failure at any step, `log.Warn` with a reason and return — no retry (matches this
  repo's fail-fast-and-fall-back-to-cache convention, `research/stack.md` §1/§4).
- Files: `session/detection/remote_manifests.go`

##### Task 2.4.2b: Implement the atomic cache write (~4 min) — BLOCKED
- `os.CreateTemp(remoteCacheDir, "*.toml.tmp")`, write the validated bytes, `os.Rename` over the
  final `<id>.toml` path — this repo's existing convention (`config/state.go:293-304`), not a
  new pattern.
- Files: `session/detection/remote_manifests.go`

##### Task 2.4.2c: Wire the `sync.Once` re-entrancy guard and launch the goroutine from `InitRemoteManifests` (~4 min) — BLOCKED
- Add `var initRemoteManifestsOnce sync.Once` at package scope; `InitRemoteManifests` wraps its
  body (Task 2.4.1a's synchronous half + `go refreshRemoteManifests(ctx, remoteCacheDir)`) in
  `initRemoteManifestsOnce.Do(...)`, mirroring `InitPlugins`'s existing guard exactly.
- Files: `session/detection/remote_manifests.go`

##### Task 2.4.2d: Tests for the background goroutine (~8 min) — BLOCKED
- One test per acceptance criterion above; the "exactly one goroutine" test needs a way to
  observe fetch-call-count without a real race — inject a counting `RemoteFetcher` double and
  use a `sync.WaitGroup` or channel-based synchronization to await goroutine completion instead
  of a `time.Sleep` (per this repo's own testing conventions against flaky sleep-based tests,
  `.claude/rules/fix-flaky-tests-dont-defer.md`'s spirit).
- Files: `session/detection/remote_manifests_test.go`

##### Task 2.4.2e: Wire `InitRemoteManifests` into `main.go` (~3 min) — BLOCKED
- Call `detection.InitRemoteManifests(ctx)` immediately after the existing `detection.
  InitPlugins(ctx)` call site (`plugins.go:486`'s call site in `main.go`'s `RunE`), same
  location Phase 1 wires `InitPlugins` — after logging init, before the daemon/web-server split.
- Files: `main.go`

---

### Epic 2.5: Trust/pinning + kill switch + full logging
**BLOCKED — see phase-level gate above. Depends on ADR-002 being finalized (Task 2.5.1a).
Parallelizable against Epics 2.1-2.4's non-trust pieces (only needs `RemoteFetcher`'s call site
to exist, per the Dependency Visualization), but must land before Epic 2.4's goroutine is
enabled by default in any release.**

#### Story 2.5.1: Pinned-source configuration
**As an** operator, **I want** the fetch source to be a pinned commit SHA by default, not a
floating branch ref, **so that** no single push to whatever repo hosts the manifest can change
detection behavior for every running instance on next fetch without the pin also being bumped.
**Acceptance Criteria**:
- The default configured source URL embeds a specific commit SHA, not `main`/`latest`.
  - *Given* no explicit source URL override in config,
    *When* the default source URL is resolved,
    *Then* it matches the pattern `https://raw.githubusercontent.com/.../<40-hex-char-sha>/...`,
    not `.../main/...`.
- Bumping the pin is a documented, single-line config change.
  - *Given* a new manifest version is published at a new commit SHA,
    *When* an operator wants to adopt it,
    *Then* the only required change is updating the pinned SHA in config — verified by a
    doc comment or README section (not a repo file this plan writes, but referenced) pointing
    at exactly which config key to change.
**Files**: `session/detection/remote_manifests.go`, `config/config.go` (modified — new config
key), depends on final ADR-002 decision for the exact source repo/path.

##### Task 2.5.1a: Confirm ADR-002's decision still holds, or record why it changed (~3 min) — BLOCKED
- Re-read `ADR-002-remote-source-trust-and-pinning.md`'s provisional decision against whatever
  is true at Phase 2 unblock time (e.g., has `requirements.md` Open Question #2 — same-repo vs.
  separate data repo — been answered elsewhere by then?); update the ADR's Status field to
  `Accepted` if it still holds, or write a superseding ADR if not.
- Files: `project_plans/remote-detection-manifests/decisions/ADR-002-remote-source-trust-and-pinning.md`

##### Task 2.5.1b: Add the pinned-source config key (~4 min) — BLOCKED
- New field on whatever config struct `config/config.go` uses for feature-level settings (follow
  the existing pattern for a similar single-string setting; exact field name/location decided at
  implementation time against ADR-002's final source choice).
- Files: `config/config.go`

##### Task 2.5.1c: Tests for source URL resolution (~4 min) — BLOCKED
- Assert the default embeds a SHA pattern; assert an explicit override is honored.
- Files: `config/config_test.go` or `session/detection/remote_manifests_test.go` (whichever
  owns the resolution function).

---

#### Story 2.5.2: `STAPLER_SQUAD_DISABLE_REMOTE_MANIFESTS` kill switch + full logging
**As an** operator, **I want** a single environment variable to fully disable remote fetch
independent of local plugins, and a complete, greppable log trail for every fetch outcome,
**so that** I can turn this off without losing local `.toml` plugin support, and can debug a
fetch problem from logs alone.
**Acceptance Criteria**:
- The kill switch disables `InitRemoteManifests` entirely, leaving `InitPlugins`/local plugins
  unaffected.
  - *Given* `STAPLER_SQUAD_DISABLE_REMOTE_MANIFESTS=1`,
    *When* `InitRemoteManifests(ctx)` is called,
    *Then* it returns `nil` immediately, `RemoteCacheDir()` is never created, and no goroutine
    is launched — while a separately-called `InitPlugins(ctx)` (Phase 1) behaves exactly as if
    Phase 2 didn't exist.
- Every fetch outcome (success, network failure, validation rejection, downgrade rejection,
  content-mismatch rejection) produces exactly one log line, per the Observability Plan.
  - *Given* each of the five outcomes above, individually,
    *When* `refreshRemoteManifests` completes,
    *Then* exactly one log line matching the Observability Plan's format for that outcome is
    emitted (verified via a test log sink/hook, not by parsing stdout).
**Files**: `session/detection/remote_manifests.go`, `session/detection/remote_manifests_test.go`

##### Task 2.5.2a: Implement the kill switch check (~2 min) — BLOCKED
- At the top of `InitRemoteManifests`, `if os.Getenv("STAPLER_SQUAD_DISABLE_REMOTE_MANIFESTS")
  != "" { return nil }`, mirroring ADR-004 §4's existing `STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS`
  check exactly (same env-var-not-config-flag reasoning: `GetFeatureFlag` defaults unset keys to
  `false`, which would invert the intended default).
- Files: `session/detection/remote_manifests.go`

##### Task 2.5.2b: Wire every log line from the Observability Plan (~5 min) — BLOCKED
- Add the five `log.Info`/`log.Warn` call sites listed in this plan's Observability Plan
  (Phase 2 section) at their respective points in `refreshRemoteManifests` and
  `shouldAcceptManifest`'s caller.
- Files: `session/detection/remote_manifests.go`

##### Task 2.5.2c: Tests for the kill switch and full logging coverage (~6 min) — BLOCKED
- One test for the kill switch (asserting no directory creation, no goroutine — reuse the
  counting-double technique from Task 2.4.2d).
- One test per logging outcome, using `t.Setenv` for the kill switch tests and a log-capturing
  hook for the logging tests (check this repo's `log` package for an existing test-capture
  helper before writing a new one).
- Files: `session/detection/remote_manifests_test.go`

---

## Summary of what "done" means for each phase

**Phase 1 is done when**: a PR re-landing PR #307's diff (or the `recovery2` equivalent) is
merged to `main`, `session/detection/plugins.go`/`detector_snapshot.go`/`plugin_watcher.go`
exist and pass `make ci`, and `grep -rln InitPlugins main.go` matches.

**Phase 2 is done when** (not before the gate above resolves): `InitRemoteManifests` is wired
into `main.go` alongside `InitPlugins`, a background fetch against a pinned-SHA HTTPS source
updates the live detector snapshot without blocking startup, the Never-Downgrade Rule and
content-hash check are enforced, `STAPLER_SQUAD_DISABLE_REMOTE_MANIFESTS` fully disables it, and
every acceptance criterion above passes under `make ci`.
