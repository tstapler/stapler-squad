# Requirements: remote-detection-manifests

Source: backlog item `a43694c6-184f-46ff-a35a-930dfdeec632`, migrated from
`TylerStaplerAtFanatics/stapler-squad#178`.

## Status update (2026-08-13)

This item's Phase 1 "Unblock" prerequisite — re-landing `detector-plugins`' local TOML
plugin loader (`session/detection/plugins.go`, `detector_snapshot.go`, `plugin_watcher.go`)
that this document's "Critical prior-art finding" section (below) flagged as missing from
`main` on 2026-08-06 — **is now resolved**, not by this item, but by
[PR #376](https://github.com/tstapler/stapler-squad/pull/376), merged 2026-08-08. #376's own
description documents and corrects PR #307's inaccurate closure rationale directly, satisfying
the "re-land PR documents the correction" requirement without any new PR from this item.

Because of that, the backlog item's acceptance criteria (as of this date) no longer ask this
session to redo the `research`/`plan`/`validate` phases below — they're still accurate and
load-bearing for Phase 2 (the actual remote-fetch layer), which remains gated on the 90-day
demand checkpoint exactly as this document already concludes. Redoing them would have
duplicated ~2400 lines of already-committed, still-valid analysis for no benefit. What this
session (2026-08-13) actually did:

- Verified Phase 1 is complete: `go build`/`go test ./session/detection/...` pass on a
  current-`main`-based branch.
- Replaced the "prose-only" 90-day-checkpoint reference (this document's Open Question #1,
  and `project_plans/detector-plugins/requirements.md`'s Success Metric section) with a
  durable, dated tracking artifact: backlog item `3a6206c4-fe06-4ef3-b605-43d0f8f355cf`
  ("Re-evaluate detector-plugins 90-day demand checkpoint (~2026-10-31)"), which also adds a
  second demand signal (whether any built-in detector needed an out-of-band `binaries/*.go`
  hotfix release in the window) alongside the original new-agent-onboarding signal.
- Documented §3's "lower-friction built-in-detector PR review process" recommendation (below,
  and in `research/build-vs-buy.md` §3) as an actual `CONTRIBUTING.md` section, rather than
  leaving it as a described-but-unwritten near-term action.
- Made no changes to Phase 2 scope, design, or code — it remains not-implemented, gated on the
  checkpoint above, exactly as `implementation/plan.md`'s Phase 1/Phase 2 split already
  specifies. AC6-AC8 of the current backlog item (precedence order, fallback-on-failure,
  Never-Downgrade Rule) are satisfied by that existing, unmodified plan documentation — they
  describe what Phase 2 must do *if/when* it proceeds, not something to build now.

The rest of this document (below) is unmodified from 2026-08-06 and remains the operative
requirements doc for Phase 2, if/when the checkpoint resolves toward proceeding.

**Date**: 2026-08-06
**Type**: feature addition (remote distribution layer for agent-detection patterns)
**Complexity**: unresolved — see Gating Decision below; if it proceeds, ~2 (small extension) since it builds directly on `detector-plugins`' existing TOML schema and loader.

## Problem Statement

Built-in agent detection patterns (`session/detection/binaries/{claude,aider,gemini,opencode}.go`)
are hardcoded Go and can only be updated by shipping a new stapler-squad release. The
originating GitHub issue (#178) proposed following herdr's model
(`src/detect/manifest_update.rs`): ship detection rules as versioned manifests, fetch updated
manifests from a remote endpoint at startup, cache locally, and fall back to bundled versions
on fetch failure — so detection-pattern fixes (e.g. Claude Code UI changes breaking idle/busy
detection) can ship independently of a full stapler-squad release.

## Critical prior-art finding — this item's prerequisite exists only as an unmerged, closed PR

`project_plans/detector-plugins/requirements.md` is the direct predecessor to this item — it
designs and (per its commits `3c25e94f9`/`005e75827`) implements the **local half** of the herdr
pattern this issue describes: a TOML plugin schema (`id`, `binary_names`, `version`,
`[[patterns]]`), a loader/validator, hot-reload via fsnotify, and built-in-override precedence,
scoped explicitly to `~/.stapler-squad/detectors/*.toml` with **no remote fetch**.

**Verified directly against this checkout (not taken on trust from either project's docs):**
`git merge-base --is-ancestor 3c25e94f9 HEAD` → not an ancestor of `main` (HEAD=`8cbddebab`);
no `session/detection/plugins.go`/`detector_snapshot.go`/`plugin_watcher.go` exist in the working
tree; no `go-toml` dependency in `go.mod`; `gh pr view 307` → **CLOSED, `mergedAt: null`**. The
PR's closing comment ("Closing as superseded: this branch's last known commit (32f504c8) is
already present on main, so this item's work has already shipped through another path",
2026-08-02) does not hold up — `32f504c8` is also not an ancestor of `main`, and none of its
symbols exist anywhere in the tree outside a stale worktree
(`.claude/worktrees/agent-a81a7aa22827ecb09/`). **The local TOML plugin loader this item
would build on does not exist on `main`.** It was designed, implemented, reviewed (4 ADRs
exist), and then closed without merging on what appears to be an incorrect assumption.

This is a second, independent finding beyond the deferral question below: **this item cannot
be implemented today regardless of the demand question**, because its stated foundation
(`session/detection/registry.go`'s `MergedRegistry`, `detector_snapshot.go`'s
`activeSnapshot`/`rebuildSnapshot`, `plugins.go`'s parse/validate pipeline) isn't present to
build on. Re-landing PR #307 (or re-verifying and correctly closing it, if the closure turns out
right after all and this research is somehow wrong) is a hard prerequisite. The *design decision*
of re-landing that loader belongs to `detector-plugins`, not to this remote-manifest item — but
this item's plan.md scopes a narrow **Phase 1 "Unblock"** step (re-landing the already-reviewed,
already-4-ADR-accepted diff, with no new design work) as a prerequisite-recovery task executed
*by* this item, since without it Phase 2 (this item's actual subject) has nothing to build on.
**Cross-artifact override, recorded 2026-08-06 during `/sdd:4-validate`**: if `detector-plugins`'s
own plan also claims this re-land, resolve by having exactly one of the two items execute it and
the other reference it as an external blocking dependency — check before starting Phase 1 to
avoid duplicate work.

Separately, even if detector-plugins were on `main` today, its own requirements doc set an
explicit deferral gate for exactly this item:

That document is explicit and load-bearing for this item:

> **Scope (this item)**: Local, file-based plugin loading only. Remote/distributed manifest
> fetching (the herdr-style `manifest.go` / issue #178 pattern) is an explicit non-goal — this
> item is the local foundation it would build on.

And its **Risky Assumption** section (unresolved, not yet checked) states the demand for any
user-extensible detection system — local or remote — is unvalidated, and sets an explicit
90-day checkpoint plus fallback:

> If a new agent *is* onboarded in that window and it still ships as a `binaries/*.go` PR
> instead of a `.toml` file, the demand assumption below was wrong and the feature should not
> be extended further (e.g. **the deferred remote-manifest/issue-#178 work should not proceed**,
> and the cheaper alternative below should be tried instead).

As of this triage (2026-08-06), the checkpoint is 4 days into its 90-day window — effectively
unstarted. No new agent CLI has been onboarded via either path since `detector-plugins`
shipped; there is no evidence yet either way.

**This changes the shape of what "requirements" means for this item.** The requirements below
describe what remote-manifest distribution would look like *if and when* the checkpoint
validates demand for it — this document does not recommend building it now. See
`research/gating-decision.md` and the priority/task output for the triage verdict.

## Users / Consumers

Same target user as `detector-plugins`: someone running stapler-squad against an agent CLI
whose detection patterns need updating faster than a stapler-squad release cycle allows — either
a non-built-in agent (already served by local TOML plugins) or a built-in agent whose UI changed
in a way that broke existing hardcoded/bundled patterns before a stapler-squad release picks up
the fix. Solo/small-team desktop tool — no server fleet, no enterprise fleet-management use
case, no telemetry.

## Success Metrics (conditional — only if the item proceeds)

- A detection-pattern fix (e.g. a changed Claude Code idle prompt) can reach a running
  stapler-squad instance without a full release — verified by publishing a manifest update and
  confirming a running instance picks it up on next start (or background refresh) without a
  binary rebuild.
- Fetch failure (offline, endpoint down, timeout) never blocks or degrades session startup —
  bundled/cached manifests are used transparently.
- No regression to `detector-plugins`' existing guarantees: local `.toml` plugins still load,
  hot-reload, and override built-ins exactly as today.

## Appetite

Not scoped for a build appetite at this time — see Gating Decision. If revisited after the
checkpoint, this is a small extension (few days) of the existing plugin loader, not a new
appetite-worthy project: distribution is additive on top of `detector-plugins`' schema/loader/
validator, which already exist and are tested.

## Constraints

- Must not weaken `detector-plugins`' trust-boundary decision (ADR-004: plugin content is
  regex-only, no code execution, no file/network access *from plugin content*) — remote-fetched
  manifests are still just TOML parsed by the same validator, not an escalation in trust model,
  but the *fetch* itself introduces a new network trust boundary (a compromised or spoofed CDN
  endpoint could push malicious detector definitions) that `detector-plugins` never had to
  consider.
- Must preserve offline-first behavior — stapler-squad has no existing requirement to be
  network-connected to function, and detection is on the session-startup hot path.
- Any new remote endpoint/CDN is new infrastructure this personal-scale project would need to
  stand up and maintain indefinitely (availability, TLS cert renewal, abuse/cost exposure) —
  unlike `detector-plugins`, which added zero new runtime infrastructure.

## Non-functional Requirements (conditional)

- **Performance**: fetch must not add user-perceptible latency to session/app startup —
  herdr's own design (fallback after 2s) is the reference point; async background refresh
  (never blocking the current session's detection) is preferable to a startup-blocking fetch.
- **Security**: fetched manifests must be validated by the exact same schema/validator
  `detector-plugins` already built (ADR-003) before being merged into the registry; a fetch
  over plain HTTP, or without any integrity/pinning story, is not acceptable given this pattern
  is explicitly designed to auto-update detection logic that helps stapler-squad decide when an
  agent needs human approval (`needs_approval`/`input_required` categories) — a spoofed manifest
  is a plausible way to suppress approval-gate detection.
- **Availability**: no SLO — reference-implementation-only endpoint (e.g. static GitHub raw
  JSON) is consistent with the project's "no server fleet" posture from `detector-plugins`.

## Scope

### In Scope (if/when the checkpoint validates proceeding)
- A versioned manifest format compatible with the existing `detector-plugins` TOML schema
  (`id`, `binary_names`, `version`, `[[patterns]]`) — not a competing JSON format; reuse, don't
  fork the schema the sibling project just shipped and tested.
- A fetch-and-cache loader that: fetches from a configured remote source, caches under
  `~/.stapler-squad/detection-manifests/` (or reuses `~/.stapler-squad/detectors/` — TBD in
  research), compares versions, and falls back silently to the last-known-good cached or bundled
  manifest on any fetch failure/timeout.
- Merge ordering with the existing three-tier precedence (built-in → remote-fetched → user
  local `.toml`) — user-local plugins must continue to win over anything remote, since a local
  file is the user's explicit, deliberate override. *(Corrected 2026-08-06 during
  `/sdd:4-validate`: this bullet previously stated the order as built-in → local → remote, which
  literally contradicted the very next clause and plan.md's `Upsert`/`MergedRegistry`
  last-applied-wins implementation, where application order determines precedence.)*

### Out of Scope
- Building this before the `detector-plugins` 90-day demand checkpoint resolves (see Gating
  Decision) — default position pending that checkpoint or an explicit user override.
- Any new manifest *format* — must extend, not replace, the TOML schema `detector-plugins`
  already shipped with ADR-003.
- Hosting/ops for a bespoke CDN — GitHub raw content or an equivalently zero-maintenance static
  host, not a new service, per the "no server fleet" constraint already established in the
  sibling project.
- Community contribution workflow (PRs to a separate data repo) — plausible future work, not
  required for a first version.

## Rabbit Holes

- Designing a full plugin marketplace / signing infrastructure — far beyond what a personal-
  scale tool with no telemetry needs; herdr's own implementation is a single static endpoint.
- Building a generic "remote config fetch" subsystem reusable beyond detection manifests —
  YAGNI until a second remote-config use case exists.

## Open Questions

1. Has the `detector-plugins` 90-day checkpoint (target ~2026-10-31) actually been reached, and
   did it resolve toward "demand confirmed" or "demand not confirmed"? This item should not
   start implementation before that checkpoint resolves, absent an explicit user override.
2. If it proceeds, what's the actual manifest source — a raw file in the `stapler-squad` repo
   itself (simplest, zero new infra, but couples manifest updates to a git push) vs. a separate
   data repo (matches the issue's "community can contribute via PR to a separate data repo"
   idea, more infra)?
3. Should remote-fetched manifests populate `~/.stapler-squad/detectors/` (reusing the exact
   directory `detector-plugins` already watches with fsnotify) or a separate cache directory
   with its own merge step? Reusing the existing watched directory is the smaller diff.
