# Requirements: tymux-bundled-integration

**Date**: 2026-08-25
**Type**: feature addition (existing project — stapler-squad)

## Problem Statement

stapler-squad has a `TymuxBackend` (`session/backend_tymux.go`, `session/tymux/*`) that
talks to `tymuxd` — a separate Rust daemon from the sibling repo `github.com/tstapler/tymux`
— over gRPC at `TYMUXD_ADDR` (default `http://127.0.0.1:7419`). Today the daemon is entirely
out-of-band: stapler-squad explicitly does not start or supervise `tymuxd` (Story 2.2.6,
`session/tymux/transport.go`'s `tymuxdAddr` doc comment). An operator has to `cargo build`
the sibling repo and run it themselves before the backend is usable at all. There is also no
way to select this backend except a process-wide `process_manager_backend` field in
`~/.stapler-squad/config.json`, read once at `main.go:167-174` into
`session.RegisterBackendProvider` — no per-session override is wired in production even
though `ProcessManagerOptions.Backend` exists as a field. As a result, the tymux backend is
effectively unusable and untested outside of unit tests that construct it directly.

## Users / Consumers

- **stapler-squad operators/developers** (Tyler, and any future contributor) who want to try
  or run the tymux backend instead of the tmux backend, without a manual out-of-band daemon
  build/run step.
- **CI** — needs to build and/or test the tymux integration path without silently skipping it
  forever, but without imposing a new hard toolchain requirement (cargo/rustc) on every
  contributor and every existing CI job that doesn't touch this path.
- **Individual sessions** — a single disposable/canary session should be able to opt into (or
  out of) the tymux backend independently of the process-wide default, mirroring the
  streamhub per-session override pattern.

## Success Metrics

- An operator can select the tymux backend (globally or per-session) and have `tymuxd`
  actually running and healthy without a manual `cargo build && ./tymuxd &` step —
  stapler-squad supervises the daemon lifecycle (start-if-not-running, health check, stop on
  shutdown) the way it already does for tmux (`session/tmux/tmux.go`'s
  `DoesSessionExist`/server-start logic is the closest existing precedent).
- The global on/off switch for the tymux integration follows the same rollout-safety
  mechanics already proven for streamhub: an env-var-backed default resolver, a recorded
  rollback rehearsal gate, and an audit trail — not a bare boolean flip.
- A single session can force the tymux backend (or force tmux) independent of the global
  default, mirroring `config.GetStreamHubSessionOverride`/`SetStreamHubSessionOverride`.
- Bundling approach is decided and documented (compile-from-source via a pinned submodule,
  mirroring `third_party/tmux`, vs. downloading a prebuilt binary from `tstapler/tymux`
  GitHub Releases) with the toolchain/CI cost of each explicitly weighed — not hand-waved.
- Story 2.2.6's "no supervision" decision is explicitly revisited and superseded with
  documented reasoning (not silently contradicted).

## Constraints

- The daemon binary is Rust. Building it from source requires `cargo`/`rustc` in the
  dev/CI environment — a toolchain this repo does not currently require anywhere. Research
  must surface what that costs `make ready`/CI/onboarding, and evaluate whether a
  prebuilt-binary alternative (checked-in per-platform binaries, or fetched from
  `tstapler/tymux` GitHub Releases at build time) avoids that cost the way tmux's
  submodule-compile approach does not need to (tmux has no simpler option in a
  Go-toolchain-only CI runner).
- Must follow `.claude/rules/interface-pollution-checklist.md` and
  `.claude/rules/primitive-obsession-checklist.md` when designing any new types — no
  speculative interfaces, no same-typed parameter piles.
- Must follow `.claude/rules/prefer-go-git-over-subshells.md` — process supervision
  (starting/stopping/health-checking `tymuxd`) is not a git operation, so `os/exec` is fine
  there; this rule only bites if the design reaches for a subshell where go-git already
  covers the need.
- No hard deadline. This is planning-only (phases 1–4); implementation is out of scope for
  this run.

## Scope

### In Scope

1. **Bundling mechanism** for `tymuxd`, shaped after `.claude/docs/bundling-tmux.md`'s
   submodule + `make build-tmux` + `make build-embedded` pattern, adapted for a
   cargo-built Rust binary rather than copy-pasted. Must cover: how tmux's embed mechanism
   works today (`go:embed` + extract-to-temp/data-dir, or something else) and whether that
   transfers to a Rust binary; and the supervision stapler-squad must newly take on
   (start-if-not-running, health-check, stop-on-shutdown) that Story 2.2.6 explicitly
   opted out of.
2. **Global feature flag** for the tymux integration, adapting the streamhub rollout
   mechanics (`STAPLER_SQUAD_USE_STREAM_HUB` env var + `config.ResolveGlobalStreamHubDefault`
   + `config.RollbackRehearsalCompletedAt`/`CompleteStreamHubRollbackRehearsal`, see
   `project_plans/terminal-multi-connection-streaming/implementation/plan.md` Phase 3
   Epics 3.1/3.3). Must reconcile this with the existing coarser
   `session.ProcessManagerBackend` selector (`BackendTmux`/`BackendTymux`/`BackendNative`)
   already in the codebase — decide and document whether "the global flag" means adding the
   rehearsal-gate treatment to that existing selector, or something layered on top.
3. **Per-session override**, adapting streamhub's per-session override shape
   (`config.GetStreamHubSessionOverride`/`SetStreamHubSessionOverride`,
   `session/streamhub/ownership.go`'s `SetSessionOverrideLookup`) onto the existing but
   currently-dead `ProcessManagerOptions.Backend` field — wiring a config-backed override
   map consulted at session-creation time.

### Out of Scope

- Actually implementing the code (this run stops after Phase 4 — validate).
- Any change to `tymuxd`'s own (Rust-side) behavior — this project only touches how
  stapler-squad builds, ships, supervises, and selects it.
- New backend types beyond tmux/tymux/native — no redesign of the `ProcessManagerBackend`
  enum's members.
- UI/UX work for exposing the flags in the web app, unless the research/plan phase finds
  it's required to make the per-session override reachable at all (existing streamhub
  precedent may already cover the wiring pattern needed).

## Open Questions (for research/plan phases to resolve, not the user)

- Does tmux's `go:embed` extraction mechanism transfer cleanly to a cargo-built Rust
  binary, or does cross-compilation/target-triple handling make a prebuilt-binary-download
  approach clearly better for tymuxd specifically?
- Should CI gain a new cargo/rustc-toolchain job, or can the bundling approach avoid that
  entirely (e.g., pulling a prebuilt binary from GitHub Releases at build time, similar to
  how other Go tools vendor prebuilt binaries)?
- Does "the global feature flag" (item 2 above) become the rehearsal-gated version of the
  existing `process_manager_backend` config field, or a new, separate on/off gate that then
  chooses whether `process_manager_backend: tymux` is even honored? The plan must state
  the answer and the reasoning.
- What health-check contract does `tymuxd` expose today (if any) that stapler-squad's new
  supervision code can poll, analogous to tmux's `DoesSessionExist`?
