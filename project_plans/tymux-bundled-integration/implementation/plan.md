# Implementation Plan: tymux-bundled-integration

**Feature**: Bundle, supervise, and make selectable (globally and per-session) the `tymuxd`
backend, replacing today's out-of-band "operator runs it by hand" model.
**Date**: 2026-08-25
**Status**: Ready for implementation
**ADRs**:
- `../decisions/ADR-001-prebuilt-tymuxd-binary-download.md` — fetch prebuilt release binaries, not compile-from-submodule
- `../decisions/ADR-002-global-flag-wraps-register-backend-provider.md` — rehearsal gate wraps `RegisterBackendProvider`, evaluated once at startup
- `../decisions/ADR-003-supersede-story-2-2-6-no-supervision.md` — supersedes Story 2.2.6's "no supervision" scope decision
- `../decisions/ADR-004-defer-daemon-cold-start-to-async-backend-start.md` — moves `EnsureDaemonRunning` off the synchronous `CreateSession` RPC path and into `TymuxBackend.Start()`/`RestoreWithWorkDir()`'s existing async path; bounds the wait with `context.WithTimeout(15s)`; adds a `singleflight`-guarded spawn to close the concurrent-cold-start race

---

## Dependency Visualization

```
Phase 1: Bundling & Embedding
  Epic 1.0 (macOS Gatekeeper spike — run FIRST, before 1.1/1.2 plumbing is built out)
        │
        v
  Epic 1.1 (fetch script) ──> Epic 1.2 (go:embed + extraction) ──> Epic 1.3 (DaemonConfig)
                                                                          │
Phase 2: Daemon Supervision                                              v
  Epic 2.1 (health check + start/retry, incl. Story 2.1.3's lazy       <─ (needs DaemonConfig)
            call-before-use in TymuxBackend.Start()/RestoreWithWorkDir()
            — session/backend_tymux.go, see ADR-004)
        │
        v
  Epic 2.2 (main.go lifecycle wiring: start, keep-alive flag, OnStop) ──> Epic 2.3 (supersede 2.2.6 doc comments)
                                                                          │
Phase 3: Global Feature Flag                                             │ (independent of Phase 2's
  Epic 3.1 (config.go rehearsal gate)                                    │  runtime behavior, but the
        │                                                                │  flag must exist before
        v                                                                │  Phase 2's "is tymux needed"
  Epic 3.2 (main.go resolveStartupBackend) <────────────────────────────-┘  check can be gated by it)
        │
        v
  Epic 3.3 (TymuxRolloutService RPC surface)

Phase 4: Per-Session Override
  (Epic 4.3/4.4's InstanceOptions call sites all eventually reach instance.Start(true)'s
   async goroutine, so they get Story 2.1.3's lazy EnsureDaemonRunning wiring in
   TymuxBackend.Start()/RestoreWithWorkDir() "for free" — no new task needed here, since
   Phase 2 is already ordered before Phase 4. See ADR-004.)
  Epic 4.1 (config.go session override map, depends on Phase 3 Epic 3.1's config.go changes)
        │
        v
  Epic 4.2 (resolveSessionBackend precedence function)
        │
        v
  Epic 4.3 (proto field + session_service.go wiring) ──> Epic 4.4 (other InstanceOptions call sites)

Phase 5: Persistence & Backward Compatibility
  Epic 5.1 (InstanceData.Backend persistence) — depends on Phase 4 existing (no point
            persisting a field nothing ever sets to non-empty)

Phase 6: Security Hardening & Documentation
  Epic 6.1, 6.2, 6.3 — depend on Phases 1-5 being substantially complete (documents what was built)

Ordering: Phase 1 -> Phase 2 -> Phase 3 -> Phase 4 -> Phase 5 -> Phase 6.
Phase 3 (the flag) can be built in parallel with Phase 1/2 (the binary/supervision) since
neither has a hard code dependency on the other until Epic 2.2's "is tymux needed" check reads
the resolved backend from Phase 3 — call this out explicitly to whoever schedules work: Epic
2.2 Task 2.2.1b is the one task that blocks on Phase 3 Epic 3.2 being merged first.
```

---

## Phase 1: Bundling & Embedding

### Epic 1.0: macOS Gatekeeper pre-flight spike (do this before building out Epic 1.1/1.2's plumbing)

**Goal**: ADR-001's entire recommendation (fetch a prebuilt binary rather than compile from
source) rests on an unverified assumption — that a `tymuxd` binary downloaded and `exec`'d at
runtime doesn't get blocked by Gatekeeper the way an unsigned, freshly-extracted executable
normally would on macOS. This repo's own `.claude/docs/codesigning.md` precedent addresses
stapler-squad's *own* binary's TCC persistence via a self-signed cert — a different problem
than Gatekeeper's treatment of a *separate* binary with no signing at all. Confirm this
empirically **first**, before Epic 1.1/1.2's fetch-script/embed/Makefile plumbing is fully built
out on the unverified assumption that it will just work — reversing course (e.g. to local
compilation) is cheap now and expensive once Phase 1-6 are built on top of the download model.

#### Story 1.0.1: Confirm (or refute) Gatekeeper blocks an unsigned fetched `tymuxd`

**Acceptance Criteria**: on a real macOS machine (Intel or Apple Silicon), a `tymuxd` binary
extracted from one of the four existing `tstapler/tymux` v1.0.0 release tarballs either (a)
`exec`s successfully with no Gatekeeper prompt/block, confirming ADR-001's approach needs no
further mitigation, or (b) is blocked (`"cannot be opened because the developer cannot be
verified"` or an outright `exec` failure), in which case a fallback is chosen and recorded
before Epic 1.1 proceeds.

**Files**: none (manual verification; this task's own output feeds back into ADR-001 if the
answer is "blocked")

##### Task 1.0.1a: Download, extract, and `exec` a v1.0.0 `tymuxd` release binary on macOS (~5 min)
- `gh release download v1.0.0 --repo tstapler/tymux --pattern 'tymux-*-apple-darwin.tar.gz'`,
  extract, `chmod +x tymuxd`, run `./tymuxd --help` (or equivalent minimal invocation) from a
  Terminal shell and via `open`/double-click-equivalent paths a downloaded-and-quarantined file
  would actually hit (Gatekeeper's quarantine attribute is set by browsers/`curl` differently
  than by `gh`/plain `curl` — test with `curl -fsSL <asset-url> -o tymuxd.tar.gz` specifically,
  since that's what `scripts/fetch-tymuxd.sh` (Task 1.1.1a) will actually do, not `gh release
  download`, which may not set the same quarantine xattr).
- Record the result (blocked or not) directly in ADR-001 (see below) and in this task's
  completion note — this is the input the rest of Phase 1 needs before proceeding.
- Files: none (manual verification)

##### Task 1.0.1b: If blocked, decide and document the fallback in ADR-001 (~4 min, conditional)
- Only needed if Task 1.0.1a finds Gatekeeper blocks the binary. Two candidate fallbacks,
  judged against ADR-001's existing reasoning (zero new toolchain requirement was the core
  win of the fetch approach): (1) ad-hoc codesign the extracted binary at install/fetch time
  (`codesign --sign - session/tymux/embed/tymuxd` in `scripts/fetch-tymuxd.sh`, no paid
  Developer ID needed for an ad-hoc signature, no notarization required for local `exec`) —
  cheapest, keeps the zero-toolchain property; or (2) fall back to local compilation on macOS
  only (reintroducing the `cargo`/`rustc` dependency ADR-001 exists to avoid, but only for that
  one platform) if ad-hoc signing turns out insufficient. Update ADR-001's Consequences section
  with whichever is chosen and why, rather than leaving the risk open past this point.
- Files: `../decisions/ADR-001-prebuilt-tymuxd-binary-download.md`

### Epic 1.1: Prebuilt-binary fetch script (ADR-001)

**Goal**: Get a real `tymuxd` binary onto disk at `session/tymux/embed/tymuxd` with no
cargo/rustc dependency anywhere, checksum-verified.

#### Story 1.1.1: `scripts/fetch-tymuxd.sh`

**As a** contributor building `stapler-squad` with the tymux backend embedded, **I want** a
script that fetches a verified prebuilt `tymuxd` binary, **so that** I never need a Rust
toolchain to produce an `embed_tymux` build.

**Acceptance Criteria**:
- Running `TYMUX_VERSION=v1.0.0 ./scripts/fetch-tymuxd.sh` on Linux x86_64/arm64 and macOS
  Intel/Apple Silicon produces `session/tymux/embed/tymuxd`, executable, matching the pinned
  checksum.
- A checksum mismatch aborts with a non-zero exit and a clear error — never silently proceeds
  with an unverified binary.
- An unsupported platform (Windows, or anything not in the four release targets) exits non-zero
  with a message naming the gap, not a confusing download failure.

**Files**: `scripts/fetch-tymuxd.sh` (new), `scripts/tymuxd-checksums.txt` (new)

##### Task 1.1.1a: Write `scripts/fetch-tymuxd.sh` (~5 min)
- Mirror `scripts/build-tmux.sh`'s shape (platform detection, `set -euo pipefail`, clear error
  messages) but replace the compile step with: map `$(uname -s)-$(uname -m)` to one of
  `aarch64-apple-darwin` / `x86_64-apple-darwin` / `x86_64-unknown-linux-musl` /
  `aarch64-unknown-linux-musl` (exit non-zero with a named-gap message for anything else,
  including any Windows shell); `curl -fsSL` the tarball from
  `https://github.com/tstapler/tymux/releases/download/${TYMUX_VERSION}/tymux-<target>.tar.gz`
  into a temp file; extract only the `tymuxd` binary (not the `tymux` CLI, which this project
  doesn't need) into `session/tymux/embed/tymuxd`; `chmod +x`.
- Files: `scripts/fetch-tymuxd.sh`

##### Task 1.1.1b: Pin checksums (~4 min)
- Create `scripts/tymuxd-checksums.txt`, one line per `<version> <target> <sha256>`, e.g.
  `v1.0.0 aarch64-apple-darwin <sha256>`. Populate the four `v1.0.0` entries by downloading each
  asset and hashing it (`gh release download v1.0.0 --repo tstapler/tymux --pattern
  'tymux-*.tar.gz' && sha256sum tymux-*.tar.gz`) — this is a one-time, implementation-time
  step, not something the script computes at runtime.
- `fetch-tymuxd.sh` looks up the expected hash for `(TYMUX_VERSION, target)` in this file and
  verifies the downloaded tarball against it (`sha256sum -c` or an equivalent portable check)
  before extracting.
- File an upstream `tstapler/tymux` issue asking `release.yml` to publish per-asset
  `.sha256` files (small, optional, not a blocker per ADR-001) — link it in the checksums
  file's header comment once filed.
- Files: `scripts/tymuxd-checksums.txt`, `scripts/fetch-tymuxd.sh` (verification logic)

#### Story 1.1.2: Makefile targets

**As a** contributor, **I want** `make` targets that mirror `build-tmux`/`build-tmux-embed`,
**so that** the tymuxd fetch step fits the existing bundling workflow without a new mental model.

**Acceptance Criteria**: `make fetch-tymuxd` populates `session/tymux/embed/tymuxd`; a second
run is a no-op unless `TYMUX_VERSION` changed (stamp-file pattern matching `TMUX_BUILD_STAMP`).

**Files**: `Makefile`

##### Task 1.1.2a: Add `fetch-tymuxd`/`build-tymuxd-embed` targets (~4 min)
- Add `TYMUX_VERSION ?= v1.0.0` and `BIN_TYMUXD := session/tymux/embed/tymuxd` near the
  existing `TMUX_BUILD_STAMP` block (`Makefile:255`).
- Add a `.tymuxd-fetch.stamp` stamp-file pair mirroring `TMUX_BUILD_STAMP`/`$(BIN_TMUX)`
  (`Makefile:255-266`): `fetch-tymuxd: ## Fetch pinned tymuxd release binary (no cargo/rustc required)` calling `./scripts/fetch-tymuxd.sh`; `build-tymuxd-embed: fetch-tymuxd` copies/confirms
  `session/tymux/embed/tymuxd` is present (the fetch script already writes directly to that
  path, so this target is mostly the `@echo "✅ ..."` confirmation step, mirroring
  `build-tmux-embed`'s structure at `Makefile:274-277`).
- Files: `Makefile`

##### Task 1.1.2b: Version-sync checklist note for `TYMUX_VERSION` vs. the `go.mod` client pin (~3 min)
- `TYMUX_VERSION` (the binary pin, this Makefile) and the `go.mod` `require` for
  `github.com/tstapler/tymux/clients/go/gen/tymux/v1` (the generated gRPC client pin) are two
  independent pins with nothing enforcing they stay in sync (ADR-001's Consequences names this
  explicitly). `checkDaemonHealthy`'s `ListSessions` probe (Epic 2.1) only proves the daemon
  answers *some* request — not that its RPC shapes match what this pinned client expects.
  A script assertion isn't viable yet (upstream `tstapler/tymux` tags aren't guaranteed 1:1 with
  the Go module's own versioning today, so there's no reliable machine-checkable invariant to
  assert against) — so the cheap mitigation for now is a doc comment, not automation: add a
  comment directly above `TYMUX_VERSION ?= v1.0.0` in the `Makefile` (Task 1.1.2a) stating
  "bump this and the `github.com/tstapler/tymux/clients/go/gen/tymux/v1` require in `go.mod`
  together — nothing enforces this automatically; see ADR-001." Revisit as a real `make
  fetch-tymuxd`-time assertion once upstream tags and the Go module version are reliably 1:1.
- Files: `Makefile`

### Epic 1.2: `go:embed` wiring + hash-verified runtime extraction

**Goal**: Reuse tmux's proven embed+extract mechanism (research/stack.md confirms it transfers
unchanged) for `tymuxd`, but close the one weakness pitfalls.md flags: length-only comparison
at extraction time is not an integrity check for a *fetched* binary the way it was an
acceptable no-op for a binary already compiled-in at `go build` time.

#### Story 1.2.1: `session/tymux/binary_embedded.go`

**As a** stapler-squad process running an `embed_tymux` build, **I want** the embedded
`tymuxd` bytes extracted to a cache dir with a real integrity check, **so that** a corrupted or
tampered cache-dir copy is never silently executed.

**Acceptance Criteria**: `TymuxdBinary()` returns a real, executable path on first call;
`TYMUXD_BIN` env var overrides it exactly like `TMUX_BIN` does for tmux; a re-run with an
unchanged embedded binary does not rewrite the cache file; a cache file whose SHA-256 doesn't
match the embedded bytes is rewritten (not silently trusted, unlike tmux's length-only check).

**Files**: `session/tymux/binary_embedded.go` (new), `session/tymux/binary.go` (new)

##### Task 1.2.1a: Write `binary_embedded.go` (~5 min)
- `//go:build embed_tymux` tag, `//go:embed embed/tymuxd` into `var tymuxdEmbedded []byte`
  (mirrors `session/tmux/binary_embedded.go:1-23` exactly).
- `TymuxdBinary() string`: `TYMUXD_BIN` env var override first (documented as a deliberate,
  unvalidated escape hatch — see Phase 6 Task 6.2.1a), else `sync.Once`-guarded
  `extractEmbeddedTymuxd()` to `$UserCacheDir/stapler-squad/tymux/$GOOS_$GOARCH/tymuxd`
  (`0755`), falling back to the bare string `"tymuxd"` (rely on `PATH`) on any extraction
  error — same graceful-degradation shape as `Binary()`.
- Extraction integrity check: unlike `binary_embedded.go`'s length-only comparison, compute
  `sha256.Sum256` of the embedded bytes and of the existing cache file (if present); skip the
  rewrite only when both hashes match. This is the pitfalls-flagged improvement — cheap (one
  hash of a few-MB file) and closes the "was the cache-dir copy tampered with or corrupted"
  gap tmux's own pattern leaves open.
- Files: `session/tymux/binary_embedded.go`

##### Task 1.2.1b: Write untagged `binary.go` fallback (~3 min)
- Mirrors `session/tmux/binary.go`'s shape (not read in full during research, but its role is
  confirmed by the tagged/untagged split): when built without `embed_tymux`, `TymuxdBinary()`
  resolves via `TYMUXD_BIN` env var, else falls back to the bare string `"tymuxd"` (PATH
  lookup) — no embed, no extraction.
- Files: `session/tymux/binary.go`

#### Story 1.2.2: Wire the embed target into the build pipeline

**Files**: `Makefile`, `.claude/docs/bundling-tmux.md`

##### Task 1.2.2a: Add `build-embedded-tymux` Makefile target (~4 min)
- New target `build-embedded-tymux: build-tmux-embed build-tymuxd-embed` building with
  `-tags "embed_tmux embed_tymuxd"` — kept **separate** from the existing `build-embedded`
  target (which stays tmux-only) rather than changing `build-embedded`'s tag set, so existing
  CI/release artifacts that depend on `-tags embed_tmux` alone are unaffected by this project.
  Mirrors `Makefile:279-288`'s Darwin `CGO_LDFLAGS`/`Info.plist` branch unchanged (tymuxd
  embedding doesn't touch TCC entitlement plumbing).
- Files: `Makefile`

##### Task 1.2.2b: Document the new build mode (~3 min)
- Rename/restructure isn't needed — add a short new section to
  `.claude/docs/bundling-tmux.md` (or a new sibling `.claude/docs/bundling-tymuxd.md`, see
  Phase 6 Task 6.3.1a for the fuller version) pointing at `make fetch-tymuxd` / `make
  build-embedded-tymux`.
- Files: `.claude/docs/bundling-tmux.md`

### Epic 1.3: `DaemonConfig` — the primitive-obsession-safe address/binary-path type

**Goal**: Per `research/architecture.md` (f)'s explicit warning, tymuxd's supervision
functions need at least two related string concepts (listen address, binary path) — bundle
them into one named type from the start rather than letting bare strings accrete across
`EnsureDaemonRunning`, the health-check function, and the spawn function.

#### Story 1.3.1: `session/tymux/daemon_config.go`

**As a** developer writing tymuxd supervision code, **I want** one named type carrying the
daemon's address and binary path, **so that** no function signature ever grows a second bare
`string` parameter that could be silently swapped with the first.

**Acceptance Criteria**: `DaemonConfig{Addr, BinaryPath}` exists; `ResolveDaemonConfig()` is
the single choke point producing one, honoring `TYMUXD_ADDR` (existing) and `TYMUXD_BIN` (new,
Task 1.2.1a) overrides, with an instance-scoped default port so two `STAPLER_SQUAD_INSTANCE`s
never collide on `127.0.0.1:7419` by default.

**Files**: `session/tymux/daemon_config.go` (new), `session/tymux/daemon_config_test.go` (new)

##### Task 1.3.1a: Define `DaemonConfig` + `ResolveDaemonConfig()` (~5 min)
- `type DaemonConfig struct { Addr string; BinaryPath string }` — every supervision function in
  Phase 2 takes a `DaemonConfig`, never separate `addr, binaryPath string` parameters.
- `ResolveDaemonConfig() DaemonConfig` (**exported** — unlike a same-package-only helper, this
  needs to be called cross-package both from `main.go` (Task 2.2.1b's startup call site) and
  from `session` (Task 2.1.3a's `TymuxBackend.Start()`/`RestoreWithWorkDir()` wiring, per
  ADR-004 — not `NewProcessManager`, which no longer calls it at all), so it cannot be
  unexported):
  - `Addr`: `TYMUXD_ADDR` env var if set (unchanged precedence from today's `tymuxdAddr()`,
    preserving backward compatibility with any existing out-of-band `TYMUXD_ADDR` usage).
    Otherwise: if `STAPLER_SQUAD_INSTANCE` is unset (the default/live instance), use
    `defaultTymuxdAddr` (`http://127.0.0.1:7419`) unchanged. If `STAPLER_SQUAD_INSTANCE` is
    set, derive a distinct port: `7420 + (crc32(instanceName) % 1000)` — mirroring the
    CRC32-based derivation `CLAUDE.md`'s manual dev port block already uses for a different
    purpose, so two manual/isolated instances never collide on tymuxd's default port the way
    `research/pitfalls.md` §1 flags as a live gap in `state-isolation.md`'s current coverage.
  - `BinaryPath`: `tymux.TymuxdBinary()` (Task 1.2.1a/1.2.1b).
- Files: `session/tymux/daemon_config.go`

##### Task 1.3.1b: Unit tests (~4 min)
- Table test: no env vars set + no instance → default `127.0.0.1:7419`; `STAPLER_SQUAD_INSTANCE`
  set to two different names → two different, deterministic, stable ports; `TYMUXD_ADDR` set →
  always wins regardless of instance name.
- Files: `session/tymux/daemon_config_test.go`

---

## Phase 2: Daemon Supervision (supersedes Story 2.2.6 — see ADR-003)

### Epic 2.1: Health check + start-with-retry primitives

**Goal**: `session/tymux/supervise.go`'s `EnsureDaemonRunning`, modeled on
`session/tmux/tmux.go`'s `EnsureServerRunning`/`ensureServerRunningWithRetry`
(tmux.go:602-708) and `daemon/daemon.go`'s `LaunchDaemon`/`StopDaemon` (daemon.go:335-420).
**Concrete package-level functions, not a `ProcessSupervisor` interface** — per
`research/architecture.md` (f), tmux's own template has never needed one, and the two
backends' health-check semantics (RPC-based vs. tmux's list-sessions-error-string parsing)
differ enough that a shared interface would immediately need type assertions.

**What "health-check" means (explicit, per ADR-003, refined by ADR-004)**: this plan implements
**call-before-use at every point tymuxd is needed** — not an ongoing background poll/restart
loop — at exactly three call sites: `main.go`'s startup phase (Task 2.2.1b); and, lazily,
`TymuxBackend.Start()` and `TymuxBackend.RestoreWithWorkDir()` (`session/backend_tymux.go`,
Story 2.1.3 below), the two `ProcessManager` methods every tymux-backed session's *async*
start/restore ultimately reaches, regardless of which of the 7+ `InstanceOptions{}` call sites
created it. **`NewProcessManager` itself is never a call site** — per ADR-004, routing the
daemon check through `NewProcessManager` would put it on `CreateSession`'s synchronous RPC path,
a latency regression the adversarial review caught (see ADR-004's Context); `EnsureDaemonRunning`
only ever runs from `main.go`'s startup phase or from inside the async goroutine that already
absorbs "starting tmux/the process" for every backend. `EnsureDaemonRunning` is idempotent and
cheap to call repeatedly (`checkDaemonHealthy` short-circuits a reuse in one RPC round-trip), so
"call it at every use site" is the supervision model, matching `daemon/daemon.go`'s existing
start-if-not-running precedent rather than introducing a new supervisor goroutine. This is a
deliberate choice, not an oversight: no `Health`/`Ping` RPC exists on tymuxd today
(`research/stack.md` (g)) to poll against, and a background loop would add a
goroutine-lifecycle/shutdown-ordering surface this plan has no other reason to need. Each of the
two `TymuxBackend` call sites bounds its own wait with `context.WithTimeout(15s)` (ADR-004) since
neither method takes a `context.Context` from its caller; concurrent cold-starts across two
sessions coalesce onto one spawn attempt via a `cfg.Addr`-keyed `singleflight.Group` inside
`EnsureDaemonRunning` itself (Task 2.1.2g, ADR-004) rather than racing to bind the port.

#### Story 2.1.1: Liveness probe via `ListSessions`

**As** stapler-squad's supervision code, **I want** a cheap, side-effect-free way to tell
whether *a* healthy tymuxd is already answering at a `DaemonConfig.Addr`, **so that** an
already-running daemon (started by this process earlier, by another local instance, or by an
operator's legacy manual workflow) is reused instead of a redundant one being spawned.

**Acceptance Criteria**: `checkDaemonHealthy` returns true only for a real gRPC `ListSessions`
success (proves process identity + protocol, not just TCP-accept); a bounded timeout so a
hung/black-holed port doesn't block startup indefinitely; a non-tymux process squatting the
port is distinguished from "tymuxd not up yet" (research/stack.md (g) confirms no dedicated
health RPC exists — `ListSessions` is the documented, adopted substitute).

**Files**: `session/tymux/supervise.go` (new)

##### Task 2.1.1a: `checkDaemonHealthy` (~5 min)
- `func checkDaemonHealthy(ctx context.Context, cfg DaemonConfig) bool` — build a transport via
  `tymux.NewRealTransport(cfg.Addr)` (already exported, reused as-is — no new transport code
  needed), call `ListSessions` with an empty filter under a short (e.g. 2s) context timeout.
  `true` only on a successful RPC response (even an empty list); `false` on any error
  (connection refused, timeout, or — notably — a non-gRPC response from a squatting process,
  which `classifyRPCError`/`ErrTymuxdUnreachable`, `session/tymux/errors.go:10-24`, already
  know how to classify as unreachable rather than a false positive).
- Files: `session/tymux/supervise.go`

#### Story 2.1.2: Start-if-not-running with retry/backoff + reuse-if-healthy

**As** stapler-squad, **I want** `EnsureDaemonRunning` to reuse an already-healthy daemon and
otherwise spawn+retry-verify a new one, **so that** the port-conflict and orphan-reuse
scenarios in `research/pitfalls.md` §2 are handled by design, not left as a race.

**Acceptance Criteria**: an already-healthy daemon short-circuits with no subprocess spawned; a
genuinely-absent daemon is spawned and polled with bounded exponential backoff (mirroring
`serverStartAttempts`/backoff constants); a port squatted by a non-tymux process fails loudly
(a new `ErrTymuxdPortSquatted`), never silently talking gRPC to an unverified process; two
concurrent callers that both observe a cold daemon for the same `DaemonConfig.Addr` coalesce
onto exactly one spawn attempt via `singleflight` (Task 2.1.2g, ADR-004) rather than racing to
bind the port.

**Files**: `session/tymux/supervise.go`, `session/tymux/supervise_test.go` (new)

##### Task 2.1.2a: `TymuxdReady` proof token + `EnsureDaemonRunning` skeleton (~5 min)
- `type TymuxdReady struct{}` — zero-size proof token, mirroring `TmuxServerReady`
  (`tmux.go:580-584`), threaded into `main.go`'s runtime phase (Epic 2.2) the same way
  `BuildRuntimeDeps` requires `TmuxServerReady`.
- `func EnsureDaemonRunning(ctx context.Context, cfg DaemonConfig) (TymuxdReady, error)`:
  step 1 — `checkDaemonHealthy(ctx, cfg)`; if true, return `TymuxdReady{}, nil` immediately
  (reuse case — the correct steady-state outcome for "start-if-not-running").
- Files: `session/tymux/supervise.go`

##### Task 2.1.2b: Spawn + PID file (~5 min)
- `func startDaemonAttempt(cfg DaemonConfig) (*os.Process, error)`: resolve `cfg.BinaryPath`,
  `safeexec.CommandContext` (mirrors `daemon.go:345` construction, `Stdin/Stdout/Stderr = nil`,
  `SysProcAttr` set to reap-preventing platform attrs on start — see Task 2.1.2d for the
  Linux-specific `Pdeathsig` addition), `cmd.Start()`, write the child PID to
  `$configDir/tymuxd.pid` (mirrors `daemon.go:361-370`'s `daemon.pid` pattern exactly, distinct
  filename), `cmd.Process.Release()` (mirrors `daemon.go:372-376`) so no zombie risk.
- Files: `session/tymux/supervise.go`

##### Task 2.1.2c: Retry/backoff after spawn (~5 min)
- `func ensureDaemonRunningWithRetry(healthy func() bool, attempts int, backoffStart, backoffMax time.Duration) bool`
  — same shape as `ensureServerRunningWithRetry` (tmux.go:647-664): poll `checkDaemonHealthy`
  after the spawn, exponential backoff, injected function args so tests can simulate
  deterministically (mirrors `startServer`/`isNotRunning` injection at tmux.go:631,647).
  Reuse the same bound constants as tmux's (`8` attempts, `100ms`→`3s`) as a documented starting
  point — not re-derived from a tymuxd-specific incident the way tmux's were, since no such
  incident history exists yet for tymuxd; revisit if real-world startup proves slower/faster.
- `EnsureDaemonRunning` step 2: if unhealthy, call `startDaemonAttempt`, then poll via the
  retry helper; if it becomes healthy, return `TymuxdReady{}, nil`.
- Files: `session/tymux/supervise.go`

##### Task 2.1.2d: Port-squat detection + Linux `Pdeathsig` (~5 min)
- After a spawn attempt exhausts retries without becoming healthy, `EnsureDaemonRunning`
  returns a new `ErrTymuxdPortSquatted` (new sentinel error, `session/tymux/errors.go`) when
  *something* is listening on `cfg.Addr` but never answers `ListSessions` correctly (i.e. the
  original `checkDaemonHealthy` at step 1 needs to distinguish "connection refused" — try to
  spawn — from "connected but didn't speak tymux's gRPC protocol" — different failure, don't
  spawn a competitor into the same address). Fail loudly here rather than silently proceeding
  with a session pointed at an unverified daemon (`research/pitfalls.md` §2/§4's shared
  mitigation).
- `startDaemonAttempt`'s parent-death handling: call `safeexec.EnsurePdeathsig(cmd)`
  (`executor/safeexec/safeexec_pdeathsig_linux.go`/`safeexec_pdeathsig_other.go`) directly on the
  daemon's `exec.Cmd`, instead of writing new `session/tymux/sysprocattr_linux.go`/
  `sysprocattr_other.go` files. `executor/safeexec.EnsurePdeathsig` already exists in this repo
  specifically for "long-running children whose parent may die unexpectedly" and is already used
  by `session/external_tmux_streamer.go`, `session/mux/multiplexer.go`, and
  `session/tmux/server_registry.go` — reuse it rather than duplicating the platform-split
  `SysProcAttr` logic `daemon/daemon.go`'s older, Pdeathsig-less `getSysProcAttr()` doesn't
  actually cover. **Adopt the existing `SIGKILL` convention, not `SIGTERM`**:
  `EnsurePdeathsig` sets `Pdeathsig: syscall.SIGKILL` (Linux only; a no-op on other platforms,
  same "known gap on macOS, no equivalent signal" caveat as before, still relying on the next
  `EnsureDaemonRunning`'s health-check-and-reap-stale-PID-file path for that case) — this plan
  has no tymuxd-specific reason to want a graceful `SIGTERM` over the repo's established
  hard-kill convention for a supervised child process, so it does not diverge.
- Files: `session/tymux/supervise.go`, `session/tymux/errors.go`

##### Task 2.1.2e: `StopDaemon`-equivalent (~4 min)
- `func StopTymuxd() error` — read `$configDir/tymuxd.pid`, `os.FindProcess` + `proc.Kill()`,
  remove the PID file, no-op (no error) if the PID file doesn't exist — mirrors
  `daemon.go:384-420`'s `StopDaemon` exactly, including its idempotent-stop contract.
- Files: `session/tymux/supervise.go`

##### Task 2.1.2f: Unit tests (~5 min)
- `TestEnsureDaemonRunning_ReusesAlreadyHealthyDaemon` (no spawn call made, injected
  `checkDaemonHealthy`-equivalent returns true immediately).
- `TestEnsureDaemonRunning_SpawnsAndRetriesUntilHealthy` (deterministic via injected
  healthy-after-N-calls function, no real subprocess or sleep — mirrors tmux's
  `TestEnsureServerRunning_NoOp`-style injection pattern).
- `TestEnsureDaemonRunning_PortSquattedFailsLoudly` (injected always-connects-but-never-tymux
  behavior → `ErrTymuxdPortSquatted`, no infinite retry).
- `TestStopTymuxd_IdempotentWhenNoPIDFile`.
- Files: `session/tymux/supervise_test.go`

##### Task 2.1.2g: Concurrent cold-start guard via `singleflight` (ADR-004) (~5 min)
- **Closes the adversarial review's third concern** ("Concurrent cold-start race... not
  discussed or tested," `adversarial-review.md:108-117`). Add a package-level
  `var spawnSF singleflight.Group` (`golang.org/x/sync/singleflight` — already a direct,
  non-`// indirect` dependency at `go.mod:208`, `v0.22.0`, with a working in-repo precedent for
  exactly this "coalesce concurrent callers onto one real attempt" shape:
  `session/tmux/tmux.go`'s `existsSF`/`noCacheSF`, used at `session/tmux/tmux.go:2589`).
- Wrap `EnsureDaemonRunning`'s unhealthy-path spawn-and-retry (Task 2.1.2b/2.1.2c's
  `startDaemonAttempt` + `ensureDaemonRunningWithRetry`) in
  `spawnSF.Do(cfg.Addr, func() (interface{}, error) { ... })` — keyed by `cfg.Addr`, **not** a
  single shared key, so two `DaemonConfig`s with different addresses (e.g. two
  `STAPLER_SQUAD_INSTANCE`s, Task 1.3.1a's port derivation) never coalesce with each other.
  Every caller that calls `EnsureDaemonRunning` while a spawn for the same `cfg.Addr` is already
  in flight blocks on `Do` and receives the same result (the coalesced spawn's `TymuxdReady{}`
  or its error) instead of independently calling `startDaemonAttempt` and racing to bind the
  port.
- Files: `session/tymux/supervise.go`

##### Task 2.1.2h: Unit test for the singleflight guard, run with `-race` (~4 min)
- `TestEnsureDaemonRunning_should_CoalesceViaSingleflight_When_ConcurrentCallersRaceOnColdStart`
  — two goroutines call `EnsureDaemonRunning` concurrently against the same `DaemonConfig` when
  nothing is running yet (injected `checkDaemonHealthy` returns false for both on their first
  call, simulating the race window). Assert the spawn function (`startDaemonAttempt`'s injection
  seam) is called **exactly once** — the coalescing guarantee `singleflight` adds beyond the
  Task 2.1.2f suite's looser "at most twice, self-heals" framing — and that both goroutines
  return the same result: both `TymuxdReady{}, nil` on success, or both the same wrapped error on
  failure. Run with `go test -race` (mirrors `session/streamhub`'s own race-focused test
  convention, e.g. `TestStreamOwnershipLock_should_NeverProduceTwoOwners_When_HubAndLegacyIntentsRaceConcurrently`).
- Files: `session/tymux/supervise_test.go`

#### Story 2.1.3: Lazy call-before-use inside the async backend-start path (closes the per-session-override gap, without blocking `CreateSession`)

**As** an operator who flips a canary session onto tymux via `SetTymuxSessionOverride` (Task
3.3.1c) without restarting the process, **I want** that session's first async start/restore to
actually start tymuxd if it isn't already running, **so that** the per-session override RPC
surface works the way its own doc comment promises (mirroring streamhub's no-restart-needed
override) instead of only ever working when the global default already started tymuxd at boot —
**and so that** getting this working doesn't cost `CreateSession` its fast-return contract (see
ADR-004; an earlier version of this story wired the check into `NewProcessManager` directly,
which the adversarial review caught as a synchronous-RPC-path latency regression — this version
supersedes that one).

**Why `TymuxBackend.Start()`/`RestoreWithWorkDir()`, not `NewProcessManager`**: `NewProcessManager`
(`session/backend_factory.go:57-79`) is the single choke point every session-creation path
funnels through to *construct* a `ProcessManager`, but that construction happens synchronously,
before the async goroutine `server/services/session_service.go`'s `CreateSession` handler defers
"starting tmux/the process" to (`s.trackCleanup(func() {...})` at
`server/services/session_service.go:2397`, which calls `instance.Start(true)` at line 2405).
Every one of the 7+ `InstanceOptions{}` call sites (Epic 4.3/4.4) that construct a
`ProcessManager` eventually reaches that same async `instance.Start(true)` path (or its
restart/resume sibling, `RestoreWithWorkDir`) via `i.pm().Start(startPath)` /
`i.pm().RestoreWithWorkDir(startPath)` (`session/instance.go`, e.g. lines 1245/1249) — so wiring
the lazy call into `TymuxBackend.Start()`/`RestoreWithWorkDir()` (`session/backend_tymux.go`)
covers every one of those call sites "for free," exactly the same reachability property the
original `NewProcessManager` placement had, but without sitting on the synchronous RPC path.
`TmuxBackend.Start()`/`RestoreWithWorkDir()` (`session/tmux_backend.go:45-46`) are the exact
same-shaped forwarding methods for the tmux backend, and they too only ever run inside this same
async path — this story makes `BackendTymux` symmetric with that existing precedent instead of
diverging from it.

**Acceptance Criteria**: `CreateSession`'s RPC response time for a `BackendTymux` session is
unaffected by tymuxd's state — identical to `BackendTmux`'s existing fast-return contract, since
`NewProcessManager` no longer touches `EnsureDaemonRunning` at all. Once the async goroutine
calls `instance.Start(true)` (or the restore/resume equivalent), `TymuxBackend.Start()`/
`RestoreWithWorkDir()` call `EnsureDaemonRunning` before delegating to the wrapped
`tymux.TymuxManager`: an already-healthy daemon adds one cheap `ListSessions` round-trip (no
spawn); a genuinely-absent daemon is spawned, bounded by an internal
`context.WithTimeout(15*time.Second)` (ADR-004 — neither method takes a `ctx` from its caller,
matching the `ProcessManager`/`TymuxManager` interfaces unchanged); and a still-unavailable
daemon returns a clear, wrapped error (not a panic, not a silent fallback to tmux) which the
async goroutine's existing failure path (`server/services/session_service.go:2405-2412`)
already converts into a `Stopped` status + a visible `creation_progress` error message — no new
error-surfacing mechanism is built for tymux specifically. Two sessions whose async start races
concurrently on a cold daemon coalesce onto one spawn attempt via the `singleflight` guard (Task
2.1.2g) instead of racing to bind the port. A session whose resolved backend is
`BackendTmux`/`BackendNative` never touches this path at all (zero footprint for non-tymux
sessions, matching Story 2.2.1's same principle at startup).

**Files**: `session/backend_tymux.go`, `session/backend_tymux_test.go`, `session/backend_factory.go` (revert only)

##### Task 2.1.3a: Wire `EnsureDaemonRunning` into `TymuxBackend.Start()`/`RestoreWithWorkDir()`, revert `NewProcessManager` (~6 min)
- **Revert** `session/backend_factory.go`'s `BackendTymux` case back to cheap construction only
  (symmetric with `BackendTmux`): `NewProcessManager`'s first parameter goes back to unused
  (`_ context.Context`), and the `case BackendTymux:` branch is just
  `return newTymuxBackendFromOpts(opts), nil` — no `EnsureDaemonRunning` call, no daemon
  round-trip, matching what this branch did before the superseded version of this task.
- In `session/backend_tymux.go`, add a package-level `const tymuxColdStartTimeout = 15 *
  time.Second` (see ADR-004 for the 15s choice: comfortably above Task 2.1.2c's ~9.1s
  worst-case retry-budget constant). Change both forwarding methods from one-line delegates to:
  ```go
  func (b *TymuxBackend) Start(dir string) error {
      ctx, cancel := context.WithTimeout(context.Background(), tymuxColdStartTimeout)
      defer cancel()
      if _, err := tymux.EnsureDaemonRunning(ctx, tymux.ResolveDaemonConfig()); err != nil {
          return fmt.Errorf("tymux backend requested but daemon unavailable: %w", err)
      }
      return b.mgr.Start(dir)
  }

  func (b *TymuxBackend) RestoreWithWorkDir(w string) error {
      ctx, cancel := context.WithTimeout(context.Background(), tymuxColdStartTimeout)
      defer cancel()
      if _, err := tymux.EnsureDaemonRunning(ctx, tymux.ResolveDaemonConfig()); err != nil {
          return fmt.Errorf("tymux backend requested but daemon unavailable: %w", err)
      }
      return b.mgr.RestoreWithWorkDir(w)
  }
  ```
  Both call sites are on the critical path of a *specific tymux-backed session's* async
  start/restore — its own resolved backend already committed to tymux, so failing loudly here
  (not silently falling back to tmux, which would violate Phase 4/5's "backend is pinned at
  creation and never silently migrated" invariant) is the correct posture, mirroring Task
  2.1.2d's port-squat "fail loudly, never silently proceed with an unverified daemon" precedent.
  This differs from Task 2.2.1b's startup call site (non-fatal, log-and-continue, because tymuxd
  failing to start at boot must never block tmux-backed sessions) precisely because this call
  site's session has already committed to `BackendTymux` — there is no tmux fallback to keep
  working here.
- Files: `session/backend_tymux.go`, `session/backend_factory.go`

##### Task 2.1.3b: Unit tests (~5 min)
- `TestTymuxBackendStart_should_CallEnsureDaemonRunning_BeforeDelegatingToManager` and
  `TestTymuxBackendRestoreWithWorkDir_should_CallEnsureDaemonRunning_BeforeDelegatingToManager`
  (`session/backend_tymux_test.go`) — inject a fake/short-circuited `EnsureDaemonRunning` (same
  injection seam as Task 2.1.2f/2.1.2h's tests) and assert it's called before the wrapped
  `mockTmuxManager.Start`/`RestoreWithWorkDir`.
- `TestTymuxBackendStart_should_ReturnWrappedError_When_DaemonUnavailable` — injected failure
  returns a non-nil, wrapped error from `Start()`, and the wrapped `TymuxManager`'s own
  `Start`/`RestoreWithWorkDir` is never called (no partial/silent proceed with an unverified
  daemon).
- `TestNewProcessManager_should_ReturnImmediately_When_BackendIsTymuxRegardlessOfDaemonState`
  (`session/backend_factory_test.go`) — the regression guard for the fix itself: construct with
  `BackendTymux` while an injected/simulated `EnsureDaemonRunning` would block or fail, and
  assert `NewProcessManager` still returns promptly with no error, proving the daemon check no
  longer runs at construction time. This directly supersedes (and should be checked against, not
  silently dropped) the now-obsolete
  `TestNewProcessManager_should_BlockForUpToRetryBudget_When_TymuxdColdStartsSynchronouslyOnCreateSessionPath`
  test from the pre-ADR-004 validation pass — see `implementation/validation.md`.
- Files: `session/backend_tymux_test.go`, `session/backend_factory_test.go`

##### Task 2.1.3c: Document the crash-mid-run behavior (verify existing error path, extend only if needed) (~4 min)
- **Crash-mid-run is explicitly out of scope for auto-recovery in this plan** — Story 2.1.3
  only closes the *no-restart-needed override* gap (a session created *after* the override is
  set), not live reconnection of a session already attached when tymuxd dies mid-run. State this
  explicitly (here and in ADR-003) rather than leaving it implied.
- What *does* happen when tymuxd crashes while a session is attached: every RPC call site in
  `session/tymux/session.go` and `stream.go` (`Start`, `RestoreWithWorkDir`, `Close`,
  `CapturePane`, `Attach`, `Attach.Send`, `ReviveSession`) already routes through
  `classifyRPCError` (`session/tymux/errors.go:33-40`), which wraps a transport-level failure as
  `ErrTymuxdUnreachable` with the underlying Connect-Go error preserved via `%w` — never
  discarded, never collapsed to a generic message. `IsAlive()` (`session/tymux/session.go:406-429`)
  already distinguishes this case in its own log line ("tymuxd unreachable, falling back to
  cached liveness") rather than reporting a flat, indistinguishable failure. This existing
  machinery already prevents a silent hang (the failure surfaces as a returned error/logged
  event, not a blocked call) and already prevents a generic, undiagnosable failure (the
  `ErrTymuxdUnreachable`-wrapped message names the daemon specifically).
- Action: confirm at implementation time that nothing between `Attach()`'s error return
  (`session/backend_tymux.go:110`) and its eventual RPC-handler/UI surfacing in
  `server/services`/`session/instance_tmux.go` flattens this wrapped error back to a generic
  string. If that confirmation finds a gap, extend it there (do not invent new error-handling
  code preemptively — the transport-level classification already does the work; this task is
  about not losing it on the way up). A crash-triggered *recovery* (auto-restart of a live
  session's daemon) is intentionally not built — the next *new* session creation will lazily
  restart tymuxd via Task 2.1.3a, but an already-attached session surfaces the error and is not
  transparently reconnected within this plan's scope.
- Files: none (verification + doc note; only touches code if the verification step finds a gap)

### Epic 2.2: `main.go` lifecycle wiring

**Goal**: Start tymuxd (only when actually needed) in the same `"runtime"` phase tmux's own
startup supervision runs in, register the first production `App.OnStop` hook, and default to
tmux's proven "outlive the process" posture unless an operator opts out.

#### Story 2.2.1: Gate supervision on actual need

**As** an operator who never opts into tymux, **I want** stapler-squad to never spawn tymuxd,
**so that** the tymux backend stays a true opt-in with zero footprint for everyone else.

**Files**: `main.go`

##### Task 2.2.1a: `tymuxNeeded` helper (~3 min)
- `func tymuxNeeded(cfg *config.Config, resolvedBackend session.ProcessManagerBackend) bool` —
  true if `resolvedBackend == session.BackendTymux` (Phase 3's resolved global default) **or**
  any entry in `cfg.TymuxSessionOverrides` (Phase 4) is `true`. Placed in `main.go` (small,
  startup-only helper — not worth its own package).
- Files: `main.go`

##### Task 2.2.1b: Call `EnsureDaemonRunning` in the runtime phase (~4 min)
- In `app.Phase("runtime", ...)` (`main.go:347-435`), immediately after the existing
  `tmux.EnsureServerRunning("")` block (`main.go:358-364`) and before `BuildRuntimeDeps`
  (line 394): if `tymuxNeeded(cfg, resolvedBackend)`, call
  `tymux.EnsureDaemonRunning(ctx, tymux.ResolveDaemonConfig())`. On error: `log.Warn` and continue (same non-fatal posture as tmux's own
  `EnsureServerRunning` failure handling at `main.go:359-364`, respecting
  `STAPLER_SQUAD_STRICT_STARTUP` the same way) — a tymux daemon that fails to start should not
  prevent stapler-squad from serving tmux-backed sessions.
- Depends on Phase 3 Epic 3.2's `resolveStartupBackend` existing so `resolvedBackend` is
  available here — the one hard cross-phase dependency called out in the Dependency
  Visualization above.
- **This is one of three `EnsureDaemonRunning` call sites, not the only one** — this covers
  "tymux needed at process startup" (global default, or a session override already set before
  this process started); Story 2.1.3's `TymuxBackend.Start()`/`RestoreWithWorkDir()` wiring
  (`session/backend_tymux.go`) covers "tymux needed by a session created after this process is
  already running" (e.g. a fresh per-session override via `SetTymuxSessionOverride` with no
  restart), via the async goroutine every session's start/restore already runs through — not
  `NewProcessManager`, which stays synchronous-and-cheap for every backend (ADR-004). All three
  are call-before-use, not a background poll — see Epic 2.1's Goal note, ADR-003, and ADR-004.
- Files: `main.go`

#### Story 2.2.2: Keep-alive-by-default + first production `App.OnStop`

**As** an operator restarting/upgrading stapler-squad, **I want** tymuxd to survive the
restart by default (matching tmux's behavior), **so that** the exact
`tmux-keep-server-on-restart.md` incident does not recur one process down the tree.

**Acceptance Criteria**: default behavior (no flag) leaves tymuxd running across a
`make install-service` restart; `--tymuxd-keep-server=false` (explicit opt-out) stops it via
the new `App.OnStop` hook.

**Files**: `main.go`

##### Task 2.2.2a: `--tymuxd-keep-server` flag (~3 min)
- New cobra flag `tymuxdKeepServerFlag bool`, default `true` — mirrors `tmuxKeepServerFlag`'s
  existing declaration and default (`main.go:59`, default set where `rootCmd.Flags()` are
  registered).
- Files: `main.go`

##### Task 2.2.2b: Register `OnStop("tymuxd", ...)` (~5 min)
- Immediately after Task 2.2.1b's `EnsureDaemonRunning` call, when tymux was actually started
  by this process (not merely reused — track this via `EnsureDaemonRunning`'s return or a
  parallel bool, since stopping a daemon this process didn't start would kill another
  instance's or an operator's manual daemon out from under them): if `!tymuxdKeepServerFlag`,
  register `a.OnStop("tymuxd", func(ctx context.Context) error { return tymux.StopTymuxd() })`.
  When the flag is true (default), skip registration entirely and log at Info level that
  tymuxd will remain running across this shutdown — this is the **first production call site**
  of `pkg/warren/App.OnStop` in the codebase (confirmed zero prior call sites,
  `research/architecture.md` (e)).
- Files: `main.go`

### Epic 2.3: Supersede Story 2.2.6's doc comments (ADR-003)

#### Story 2.3.1: Update the two canonical citations

**Files**: `session/tymux/transport.go`, `session/tymux/errors.go`

##### Task 2.3.1a: Update `transport.go:108-119` (~3 min)
- `tymuxdAddr()`'s doc comment currently states stapler-squad "does not start or supervise
  tymuxd itself (Story 2.2.6's documented scope decision)." Replace with a note that this
  changed — reference ADR-003 by path, state the address is now supervision-aware (still
  overridable via `TYMUXD_ADDR`, matching `DaemonConfig.Addr`'s resolution in Task 1.3.1a).
- Files: `session/tymux/transport.go`

##### Task 2.3.1b: Update `errors.go:16-19` (~2 min)
- Update `ErrTymuxdUnreachable`'s doc comment to reflect that unreachability can now also mean
  "supervision failed to start it" (in addition to "an out-of-band daemon isn't running"),
  referencing ADR-003.
- Files: `session/tymux/errors.go`

### Epic 2.4: `install-service` audit (avoid the Linux/macOS drift incident recurring)

#### Story 2.4.1: Confirm both platforms pass the new flag consistently

**As** the person who shipped `--tmux-keep-server`'s original incident (Linux systemd unit
omitted it while macOS's LaunchAgent had it), **I want** `--tymuxd-keep-server`'s *default*
(not an explicit flag pass-through) verified safe on both platforms before this ships, **so
that** the exact drift documented in `.claude/docs/tmux-keep-server-on-restart.md` doesn't
repeat for tymuxd.

##### Task 2.4.1a: Audit `scripts/install-service.sh` (~4 min)
- Since `--tymuxd-keep-server` defaults to `true`, no `ExecStart`/`ProgramArguments` change is
  strictly required on either platform for the *default* (safe) behavior — but confirm neither
  script's `ExecStart` line passes an unexpected `--tymuxd-keep-server=false` or equivalent
  override, and add a one-line comment in both the systemd unit template and the LaunchAgent
  plist template noting the default is intentional (preventing a future edit from silently
  flipping it, the same class of drift that caused the original tmux incident).
- Files: `scripts/install-service.sh` (and whatever unit/plist templates it writes — confirm
  exact paths during implementation via `grep -n tmux-keep-server scripts/install-service.sh`)

---

## Phase 3: Global Feature Flag & Rollout Safety (ADR-002)

### Epic 3.1: `config.go` rehearsal gate

#### Story 3.1.1: Rehearsal-completion field + resolver

**As** an operator, **I want** the tymux global default gated by a rollback rehearsal exactly
like streamhub's, **so that** flipping it on is a conscious, audited action, not a bare boolean.

**Acceptance Criteria**: `ResolveGlobalTymuxDefault(cfg, false)` always returns `(false, nil)`;
`ResolveGlobalTymuxDefault(cfg, true)` returns `ErrTymuxRollbackRehearsalNotCompleted` unless
`TymuxRollbackRehearsalCompletedAt` is set; `RecordTymuxRollbackRehearsalCompleted` persists
`time.Now()` and saves config.

**Files**: `config/config.go`, `config/config_test.go`

##### Task 3.1.1a: Add `TymuxRollbackRehearsalCompletedAt` field (~3 min)
- `TymuxRollbackRehearsalCompletedAt *time.Time \`json:"tymux_rollback_rehearsal_completed_at,omitempty"\``
  next to the existing `RollbackRehearsalCompletedAt` (`config.go:435-443`) — a **distinct**
  field, not a shared one, because the two rehearsals verify different things (ADR-003's
  rollback-semantics note: tymux's rollback cannot mean "reconnect under the legacy path" the
  way streamhub's does).
- Files: `config/config.go`

##### Task 3.1.1b: `ResolveGlobalTymuxDefault` + sentinel error (~4 min)
- `var ErrTymuxRollbackRehearsalNotCompleted = errors.New(...)` (mirrors
  `ErrRollbackRehearsalNotCompleted`, `config.go:446-450`, message text citing this project's
  rollback-rehearsal procedure — see ADR-003).
- `func ResolveGlobalTymuxDefault(cfg *Config, requested bool) (bool, error)` — byte-for-byte
  same control flow as `ResolveGlobalStreamHubDefault` (`config.go:463-471`), substituting the
  tymux field.
- Files: `config/config.go`

##### Task 3.1.1c: `RecordTymuxRollbackRehearsalCompleted` (~3 min)
- `func (c *Config) RecordTymuxRollbackRehearsalCompleted() error` — mirrors
  `RecordRollbackRehearsalCompleted` (`config.go:480-484`) exactly.
- Files: `config/config.go`

##### Task 3.1.1d: Unit tests (~5 min)
- `TestResolveGlobalTymuxDefault_AlwaysAllowsFalse`
- `TestResolveGlobalTymuxDefault_RefusesTrueWithoutRehearsal`
- `TestResolveGlobalTymuxDefault_AllowsTrueAfterRehearsal`
- `TestRecordTymuxRollbackRehearsalCompleted_PersistsTimestamp`
- Files: `config/config_test.go`

### Epic 3.2: `main.go` — single source of truth resolution (ADR-002's concrete shape)

#### Story 3.2.1: `resolveStartupBackend`

**As** the codebase, **I want** exactly one function deciding the effective startup backend,
**so that** hand-editing `process_manager_backend: "tymux"` in `config.json` can never bypass
the rehearsal gate (closing the risk named in `research/pitfalls.md` §3).

**Acceptance Criteria**: `process_manager_backend: "tymux"` in config with no rehearsal
completed and no env var resolves to `BackendTmux` (not an error — a loud `log.Warn`, matching
existing non-fatal-startup-warning posture); `STAPLER_SQUAD_USE_TYMUX=true` with rehearsal
completed resolves to `BackendTymux`; `process_manager_backend: "native"` is unaffected
(passes through unchanged — the gate only intercepts the tymux case).

**Files**: `main.go`, `main_test.go`

##### Task 3.2.1a: Extract `resolveStartupBackend` (~5 min)
- Replace the inline block at `main.go:167-175` with a call to a new, independently testable
  function:
  ```go
  func resolveStartupBackend(cfg *config.Config, tymuxEnvRequested bool) (session.ProcessManagerBackend, error) {
      backend := session.ProcessManagerBackend(cfg.ProcessManagerBackend)
      if backend == "" {
          backend = session.BackendTmux
      }
      tymuxRequested := backend == session.BackendTymux || tymuxEnvRequested
      tymuxEffective, err := config.ResolveGlobalTymuxDefault(cfg, tymuxRequested)
      if tymuxEffective {
          backend = session.BackendTymux
      } else if backend == session.BackendTymux {
          backend = session.BackendTmux
      }
      return backend, err
  }
  ```
  Call site: `backend, err := resolveStartupBackend(cfg, os.Getenv("STAPLER_SQUAD_USE_TYMUX") == "true"); if err != nil { log.Warn("tymux: global default requested but rollback rehearsal not completed; falling back to tmux", "err", err) }; session.RegisterBackendProvider(backend)`.
- Files: `main.go`

##### Task 3.2.1b: Unit tests (~5 min)
- `TestResolveStartupBackend_EmptyConfigDefaultsToTmux`
- `TestResolveStartupBackend_TymuxConfigValueWithoutRehearsalFallsBackToTmux` (the bypass-guard
  regression test — this is the one that would have caught the pitfalls.md §3 risk)
- `TestResolveStartupBackend_TymuxConfigValueWithRehearsalCompletes`
- `TestResolveStartupBackend_EnvVarWithRehearsalCompletes`
- `TestResolveStartupBackend_NativeBackendPassesThroughUnaffected`
- Files: `main_test.go`

### Epic 3.3: `TymuxRolloutService` RPC surface

**Goal**: Requirements.md's Out-of-Scope carve-out allows UI/UX work "if research/plan finds
it's required to make the per-session override reachable at all." Without an RPC surface, the
only way to set a per-session override or record the rehearsal is hand-editing `config.json` —
worse UX than streamhub's own precedent and inconsistent with it. Scope is the **RPC handler
only** (mirrors `stream_hub_rollout_service.go`'s concrete-type shape, no interface — a
config-backed handler with no second implementation) — no new React components; reachability
via `grpcurl`/a future UI pass is sufficient to satisfy "reachable at all."

#### Story 3.3.1: Proto + handler

**Files**: `proto/session/v1/session.proto`, `server/services/tymux_rollout_service.go` (new),
`server/server.go`

##### Task 3.3.1a: Proto RPCs (~5 min)
- Add a `TymuxRolloutService` (new proto file `proto/session/v1/tymux_rollout.proto`, mirroring
  wherever `StreamHubRolloutService` is declared) with three RPCs:
  `GetTymuxRolloutStatus` (env var set?, rehearsal timestamp, session overrides list),
  `CompleteTymuxRollbackRehearsal` (calls `RecordTymuxRollbackRehearsalCompleted`),
  `SetTymuxSessionOverride` (see Phase 4 Epic 4.1 for the underlying config accessor). The
  global env var itself is **not** settable via RPC, mirroring streamhub's explicit doc-comment
  reasoning (`stream_hub_rollout_service.go:21-28`) — restart-only by design.
- Files: `proto/session/v1/tymux_rollout.proto` (new)

##### Task 3.3.1b: `make proto-gen` (~2 min)
- Mechanical — regenerate, confirm `go build ./...` succeeds. Generated output is gitignored
  per this repo's convention (do not commit `gen/`).
- Files: none committed (generated)

##### Task 3.3.1c: Handler implementation (~5 min)
- `server/services/tymux_rollout_service.go`: concrete `TymuxRolloutService` type (config-backed,
  no interface — same doc-comment justification as `stream_hub_rollout_service.go`'s own file
  header), implementing the three RPCs against `config.LoadConfig()`/`SaveConfig`.
- Files: `server/services/tymux_rollout_service.go`

##### Task 3.3.1d: Register in `server/server.go` (~3 min)
- Wire the new service's Connect handler into the existing mux, mirroring how
  `StreamHubRolloutService` is registered.
- Files: `server/server.go`

##### Task 3.3.1e: Unit tests (~5 min)
- `TestGetTymuxRolloutStatus_ReportsRehearsalState`
- `TestCompleteTymuxRollbackRehearsal_PersistsTimestamp`
- `TestSetTymuxSessionOverride_RoundTrips`
- Files: `server/services/tymux_rollout_service_test.go` (new)

---

## Phase 4: Per-Session Override (wiring the dead field)

### Epic 4.1: `config.go` session override map

**Goal**: `TymuxSessionOverrides`, shaped exactly like `StreamHubSessionOverrides` — but the
consultation logic in Phase 4 Epic 4.2 must **not** copy `streamhub/ownership.go`'s
`resolveLocked` bug (documented in `research/features.md` (b).5: `ok && forceHub` only ever
pushes `effective` toward `true`, never back to `false`, contradicting
`SetStreamHubSessionOverride`'s own doc comment that a `false` override "explicitly pins the
session to the legacy path"). This project's per-session override must genuinely work in both
directions — force tymux, or force tmux — regardless of the global default.

#### Story 4.1.1: Config storage + accessors

**Files**: `config/config.go`, `config/config_test.go`

##### Task 4.1.1a: `TymuxSessionOverrides` map + accessors (~4 min)
- `TymuxSessionOverrides map[string]bool \`json:"tymux_session_overrides,omitempty"\`` (mirrors
  `StreamHubSessionOverrides`, `config.go:426-434`).
- `func (c *Config) GetTymuxSessionOverride(sessionName string) (forceTymux bool, ok bool)` and
  `func (c *Config) SetTymuxSessionOverride(sessionName string, forceTymux *bool) error` —
  identical tri-state `*bool` convention to `SetStreamHubSessionOverride`
  (`config.go:498-517`): `nil` deletes (fall back to global), non-nil `true` forces tymux,
  non-nil `false` forces tmux.
- Files: `config/config.go`

##### Task 4.1.1b: Unit tests including the both-directions regression guard (~5 min)
- `TestGetTymuxSessionOverride_NilConfigIsNilSafe`
- `TestSetTymuxSessionOverride_ForceTrueThenForceFalse` — explicitly exercises **both**
  directions in sequence on the same session name (the test streamhub's own suite never wrote,
  per `research/features.md` (b).5) to prove the accessor itself has no directional bias. (The
  actual bug-class guard lives in Epic 4.2's consultation-logic test, since the accessor here
  is a plain map write with no combinator logic to get wrong.)
- Files: `config/config_test.go`

### Epic 4.2: `resolveSessionBackend` — the precedence function

**Goal**: Per `research/features.md` (b).6, this is **not** a per-connection sticky-resolve
lookup like streamhub's `sessionOverrideLookup` — `Instance.Backend` is already a per-session,
persisted field, and `NewProcessManager` already resolves it once at construction. The correct
shape is a plain, direct precedence function consulted once at `InstanceOptions` construction
time, returning the effective backend outright (never an incremental "OR into true"
combinator) — this is what structurally prevents the streamhub `resolveLocked` bug class from
recurring here.

#### Story 4.2.1: `session/backend_resolution.go`

**As** the `CreateSession` RPC handler (and any other `InstanceOptions{}` construction site),
**I want** one function that resolves the effective backend for a new session, **so that** the
precedence (per-request override → per-session config map → rehearsed global default →
`BackendTmux`) is defined exactly once and testable in isolation.

**Acceptance Criteria**: an explicit per-request override always wins; absent that, a
config-map override (in either direction) wins over the global; absent both, the already-gated
global default (`getSelectedBackend()`, set once by Phase 3's `resolveStartupBackend`) applies;
absent all three, `BackendTmux`.

**Files**: `session/backend_resolution.go` (new), `session/backend_resolution_test.go` (new)

##### Task 4.2.1a: `resolveSessionBackend` (~5 min)
```go
// resolveSessionBackend resolves the effective ProcessManagerBackend for a
// new session, in precedence order: an explicit per-request override, this
// session's config-backed override (in either direction — see
// research/features.md (b).5's resolveLocked bug this deliberately does not
// replicate), the already-gated process-wide default, then BackendTmux.
func resolveSessionBackend(cfg *config.Config, sessionName string, requestOverride ProcessManagerBackend) ProcessManagerBackend {
    if requestOverride != "" {
        return requestOverride
    }
    if forceTymux, ok := cfg.GetTymuxSessionOverride(sessionName); ok {
        if forceTymux {
            return BackendTymux
        }
        return BackendTmux
    }
    if backend := getSelectedBackend(); backend != "" {
        return backend
    }
    return BackendTmux
}
```
- Note this lives in `package session` (not a sub-package) — `package session` already imports
  `package config` extensively (confirmed via repo grep: `instance.go`, `storage.go`, etc.), so
  none of streamhub's one-way-dependency constraint (`session/streamhub` cannot import
  `config`) applies here; no function-pointer indirection needed.
- Files: `session/backend_resolution.go`

##### Task 4.2.1b: Unit tests (~5 min)
- `TestResolveSessionBackend_RequestOverrideWinsOverEverything`
- `TestResolveSessionBackend_SessionOverrideForcesTymuxOverGlobalTmux`
- `TestResolveSessionBackend_SessionOverrideForcesTmuxOverGlobalTymux` — the direct regression
  guard for the streamhub bug class: proves a `false` override actually overrides an
  `effective == true` global, which `resolveLocked` never could.
- `TestResolveSessionBackend_FallsBackToGlobalWhenNoOverrides`
- `TestResolveSessionBackend_FallsBackToTmuxWhenNothingSet`
- Files: `session/backend_resolution_test.go`

### Epic 4.3: Wire into `CreateSession`

#### Story 4.3.1: Proto field for an explicit per-request override

**Files**: `proto/session/v1/session.proto`

##### Task 4.3.1a: Add `backend_override` field (~3 min)
- `CreateSessionRequest` currently uses field numbers 1-33: `remote = 31`,
  `restart_from_session_id = 32`, and `confirm_restart_with_live_source = 33` are the highest
  observed (already claimed since this field-number research was last done — `auto_approve = 29`
  /`extra_args = 30` are no longer the top of the range). `34` is the next free number. Add:
  ```proto
  // Optional: force a specific ProcessManager backend for this session
  // ("tmux", "tymux"), overriding both the per-session config override and
  // the process-wide default. Empty means no per-request override — normal
  // precedence applies (see session.resolveSessionBackend).
  optional string backend_override = 34;
  ```
  (confirm the actual next-free number via `grep -n '= 3[0-9];' proto/session/v1/session.proto`
  at implementation time before finalizing 34, in case another in-flight change has claimed it
  first since this plan was written).
- Files: `proto/session/v1/session.proto`

##### Task 4.3.1b: `make proto-gen` (~2 min)
- Files: none committed (generated)

#### Story 4.3.2: `session_service.go` wiring

**Files**: `server/services/session_service.go`, `server/services/session_service_test.go`

##### Task 4.3.2a: Populate `instanceOpts.Backend` (~5 min)
- At the `instanceOpts := session.InstanceOptions{...}` construction (`session_service.go:2291`
  onward), add:
  ```go
  Backend: session.ResolveSessionBackend(config.LoadConfig(), req.Msg.Title, session.ProcessManagerBackend(req.Msg.BackendOverride)),
  ```
  (exporting `resolveSessionBackend` as `ResolveSessionBackend` for cross-package use from
  `server/services`, or keeping it unexported and adding a thin exported wrapper — match
  whichever convention `session`'s other exported-for-services helpers already use; confirm at
  implementation time). Use the session's `Title` (not yet a persisted session name at this
  point in the request) — confirm against how `TymuxSessionOverrides`/`StreamHubSessionOverrides`
  key their maps (tmux session *name*, which is derived from `Title`, not `Title` itself) and
  adjust the lookup key accordingly so it matches whatever key
  `initTmuxSession`/`ProcessManagerOptions.SessionName` actually uses.
- Files: `server/services/session_service.go`

##### Task 4.3.2b: Unit tests (~5 min)
- `TestCreateSession_HonorsExplicitBackendOverride`
- `TestCreateSession_HonorsSessionNameOverrideMap`
- `TestCreateSession_FallsBackToGlobalDefaultWhenNoOverrides`
- Files: `server/services/session_service_test.go`

### Epic 4.4: Other `InstanceOptions{}` call sites

**Goal**: `research/architecture.md` (c) lists 7 non-test `InstanceOptions{}` literals; only
the primary `CreateSession` RPC handler has a natural place for an explicit per-request
override (Epic 4.3). The other six should still consult the session-name override map (tier 2
of `resolveSessionBackend`, passing `requestOverride = ""`) so a canary override applies
consistently regardless of which entry point created the session.

#### Story 4.4.1: MCP tool call sites

**Files**: `server/mcp/tools_github.go`, `server/mcp/tools_lifecycle.go`

##### Task 4.4.1a: Wire `tools_github.go:213` and `tools_lifecycle.go:162` (~5 min)
- Same pattern as Task 4.3.2a, with `requestOverride = ""` (no per-request field on these MCP
  tool schemas — out of scope to add one here; the session-name override map is sufficient
  reachability for canary use through these paths).
- Files: `server/mcp/tools_github.go`, `server/mcp/tools_lifecycle.go`

#### Story 4.4.2: Directory/worktree session creation

**Files**: `server/services/session_service.go`

##### Task 4.4.2a: Wire `CreateDirectorySession` (line ~1310) and `CreateWorktreeSession` (line ~1362) (~5 min)
- Same pattern, `requestOverride = ""`.
- Files: `server/services/session_service.go`

#### Story 4.4.3: Checkpoint restore + commit import

**Files**: `session/instance_checkpoint.go`, `session/import_commit.go`

##### Task 4.4.3a: Wire `instance_checkpoint.go:189` and `import_commit.go:138` (~4 min)
- Same pattern, `requestOverride = ""`. These paths reconstruct an `Instance` from existing
  state, so the session-name override map is the only relevant tier (no meaningful
  "per-request" concept here).
- Files: `session/instance_checkpoint.go`, `session/import_commit.go`

---

## Phase 5: Persistence & Backward Compatibility

**Why this phase is required, not optional**: `research/architecture.md` (d) and
`research/pitfalls.md`'s Summary item 2 both establish that pinning the backend at
session-creation time and keeping it immutable for the session's lifetime is the load-bearing
safety property preventing an in-flight session from being silently migrated between backends.
`session/instance_serialization.go:331-334`'s doc comment already confirms `instance.Backend`
is **not currently persisted** in `InstanceData`. Once Phase 4 makes `Instance.Backend`
non-empty for real tymux-backed sessions, leaving it unpersisted means every process restart
silently drops that pin: a restored tymux-backed session would fall through to
`getSelectedBackend()` (today's global, which may by then be `BackendTmux`), reconstructing its
`ProcessManager` against the wrong backend entirely — not just a UX regression but a
correctness bug (the pane/session identity `TymuxBackend` expects doesn't exist under tmux).
**Decision: this plan adds persistence.** Documenting it as a "known gap" instead would ship a
safety property (Phase 4's whole point) that silently stops holding on the very first restart.

### Epic 5.1: `InstanceData.Backend`

#### Story 5.1.1: Persist and restore

**Files**: `session/storage.go`, `session/instance_serialization.go`,
`session/instance_serialization_test.go`, `session/ent/schema/session.go`

##### Task 5.1.1a: Add `Backend` to `InstanceData` (~3 min)
- `Backend ProcessManagerBackend \`json:"backend,omitempty"\`` — add next to `InstanceData`'s
  other simple fields in `session/storage.go` (exact location: wherever `Title`/`Path`-adjacent
  fields are declared; confirm struct layout during implementation).
- Files: `session/storage.go`

##### Task 5.1.1b: Set it in `ToInstanceData` (~3 min)
- In `instance_serialization.go`'s `ToInstanceData` (the `data := InstanceData{...}` literal
  starting at line 51), add `Backend: snap.Backend` (or the equivalent field from whatever the
  snapshot struct exposes — confirm `InstanceSnapshot` carries `Backend` already or needs its
  own addition alongside this task).
- Files: `session/instance_serialization.go`

##### Task 5.1.1c: Read it on restore (~4 min)
- At the restore path (`instance_serialization.go:328-337`, the block immediately preceding
  `NewProcessManager(..., ProcessManagerOptions{Backend: instance.Backend})`), ensure
  `instance.Backend` is set from `data.Backend` *before* this call (today `instance.Backend`
  presumably comes from a struct-literal copy earlier in the same restore function — confirm
  and add the field to that literal, mirroring how every other `InstanceData` field already
  round-trips).
- Files: `session/instance_serialization.go`

##### Task 5.1.1d: Update the doc comment (~3 min)
- Replace the `instance.Backend is not currently persisted... out of Epic 2.1's scope`
  sentence (`instance_serialization.go:331-333`) with a note that this now persists, and state
  the backward-compatibility behavior explicitly (not left implicit, per
  `research/features.md` (d).5's warning): an old `sessions.json`/`InstanceData` entry with no
  `"backend"` key unmarshals to the Go zero value `""` automatically — identical to today's
  implicit behavior, falls through to whatever the process-wide global resolves to. No explicit
  migration code is needed for this case; state that explicitly in the comment so a future
  reader doesn't have to re-derive it.
- Files: `session/instance_serialization.go`

##### Task 5.1.1e: Round-trip + backward-compat tests (~5 min)
- `TestToInstanceData_PreservesBackend` — create an instance with `Backend: BackendTymux`,
  round-trip through `ToInstanceData`/`FromInstanceData`, assert it survives.
- `TestFromInstanceData_OldJSONWithoutBackendFieldDefaultsEmpty` — hand-construct a legacy JSON
  blob lacking a `"backend"` key, confirm it unmarshals to `""` and restores under the current
  global default (the explicit backward-compat regression test the doc comment update names).
- Files: `session/instance_serialization_test.go`

##### Task 5.1.1f: Add `backend` column to `session/ent/schema/session.go` + regenerate (~5 min)
- `session/ent/schema/session.go` is a **fully columnar** schema — every other simple
  `InstanceData` field (`title`, `program`, `category`, `hidden`, `workflow_id`, etc.) is its
  own discrete `field.String`/`field.Bool`/`field.Time` entry (confirmed by direct inspection,
  `session/ent/schema/session.go:20-138`); `Tags` is a many-to-many `edge.To(Tag.Type)`, not
  JSON at all. The only genuine "opaque JSON blob" precedent in this schema is the
  purpose-built `session_artifacts` field
  (`field.String("session_artifacts").Optional().Default("").Comment("JSON-encoded
  SessionArtifactsBlob...")`, `session/ent/schema/session.go:134-137`) — a one-off composite
  field, not the norm. `InstanceData.Backend` (Task 5.1.1a) needs its own discrete column,
  the same way every other simple field does: no verification step is needed to know this, and
  no contingent "if a discrete column is required" framing applies.
- Add `field.String("backend").Optional().Default("").Comment("Per-session ProcessManager
  backend pin (e.g. \"tymux\"); empty means no pin, falls through to the process-wide
  default.")` next to the other simple `field.String` entries in `session/ent/schema/session.go`.
- Regenerate with the exact command `CLAUDE.md` requires (the `--feature sql/upsert` flag is
  not optional — omitting it breaks `UpsertRule` and similar generated methods):
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`.
  Confirm `go build ./...` succeeds afterward. **Do not commit the generated output** —
  `.gitignore` excludes `session/ent/*.go`/`session/ent/*/` outside `schema/`; commit only the
  `session/ent/schema/session.go` diff itself.
- Files: `session/ent/schema/session.go`

---

## Phase 6: Security Hardening & Documentation

### Epic 6.1: Transport binding decision

#### Story 6.1.1: Document the accepted TCP-loopback risk

**Files**: `.claude/docs/bundling-tymuxd.md` (new)

##### Task 6.1.1a: Write the security section (~4 min)
- Document, per `research/pitfalls.md` §4: tymuxd binds TCP loopback (`127.0.0.1`) with no
  TLS/auth, weaker than a Unix-socket's filesystem-permission boundary (any local user on a
  shared machine can reach it, not just the process owner) — accepted risk on a single-user
  dev machine, matching this repo's existing precedent for `localhost:8543`'s own HTTP server;
  explicitly flagged as a gap on shared/multi-user machines. A Unix-socket alternative would
  require a Rust-side change to `tymuxd` itself — out of this project's scope (requirements.md
  explicitly excludes changes to tymuxd's own behavior) — note it as a natural upstream
  follow-up, not something this plan resolves.
- Files: `.claude/docs/bundling-tymuxd.md`

### Epic 6.2: `TYMUXD_BIN` escape-hatch documentation

##### Task 6.2.1a: Doc comment (~2 min)
- On `TymuxdBinary()` (Task 1.2.1a), state explicitly that `TYMUXD_BIN` is a deliberate,
  unvalidated escape hatch (anyone who can set env vars for the process can point it at an
  arbitrary binary) — an accepted risk for local dev/testing, mirroring `TMUX_BIN`'s identical,
  already-accepted shape, not a new gap introduced by this project.
- Files: `session/tymux/binary_embedded.go`

### Epic 6.3: User-facing documentation

#### Story 6.3.1: `bundling-tymuxd.md` + index entry

##### Task 6.3.1a: Write `.claude/docs/bundling-tymuxd.md` (~4 min)
- Mirror `.claude/docs/bundling-tmux.md`'s shape: `make fetch-tymuxd`, `make
  build-embedded-tymux`, the `STAPLER_SQUAD_USE_TYMUX`/`TymuxSessionOverrides`/rehearsal-gate
  flow (link to ADR-002/ADR-003), the `--tymuxd-keep-server` default, and the Task 6.1.1a
  security note.
- Files: `.claude/docs/bundling-tymuxd.md`

##### Task 6.3.1b: Add row to `CLAUDE.md`'s Reference Documents Index (~2 min)
- One new row: `| Bundling tymuxd (single-binary, supervised) | \`.claude/docs/bundling-tymuxd.md\` |`.
- Files: `CLAUDE.md`

---

## Summary of New Public Surface (for reviewers)

| Symbol | Package | Purpose |
|---|---|---|
| `DaemonConfig`, `ResolveDaemonConfig` | `session/tymux` | Addr+BinaryPath bundle (primitive-obsession fix) |
| `TymuxdBinary()` | `session/tymux` | Embedded/PATH binary resolution, mirrors `tmux.Binary()` |
| `EnsureDaemonRunning`, `TymuxdReady`, `StopTymuxd` | `session/tymux` | Concrete supervision functions (no interface); internally singleflight-guarded against concurrent cold-starts (Task 2.1.2g, ADR-004); called from three sites — `main.go` startup, and `session.TymuxBackend.Start()`/`RestoreWithWorkDir()`'s lazy per-session async path (Story 2.1.3) — never from `session.NewProcessManager`, which stays synchronous-and-cheap (ADR-004) |
| `ErrTymuxdPortSquatted` | `session/tymux` | New sentinel for the port-conflict-with-non-tymux-process case |
| `TymuxRollbackRehearsalCompletedAt`, `ResolveGlobalTymuxDefault`, `RecordTymuxRollbackRehearsalCompleted`, `TymuxSessionOverrides`, `Get/SetTymuxSessionOverride` | `config` | Rollout-safety mechanics, mirroring streamhub's shape |
| `resolveStartupBackend` | `main` (package `main`) | Single source of truth for the startup backend, closes the config-field-bypass gap |
| `resolveSessionBackend` | `session` | Per-session precedence function, direction-safe (no `resolveLocked`-style bug) |
| `TymuxRolloutService` | `server/services` | Minimal operator-facing RPC surface |
| `InstanceData.Backend` | `session` | Persistence for the per-session pin (Phase 5) |

## Risks Not Fully Resolved by This Plan (flagged for implementation time)

1. **macOS codesigning/notarization for the fetched `tymuxd` binary** (ADR-001's Consequences)
   — this repo's `.claude/docs/codesigning.md` precedent (self-signed `StaplerSquadDev` cert)
   addresses stapler-squad's *own* binary's TCC persistence, not Gatekeeper's treatment of a
   *separate*, freshly-downloaded-and-extracted executable. **Moved from "deferred to
   implementation time" to Epic 1.0's explicit pre-flight spike task** (Task 1.0.1a) — the
   Gatekeeper outcome is still genuinely unknown until that spike runs on a real Mac, but the
   plan no longer leaves discovering it until Phase 1's fetch-script/embed/Makefile plumbing is
   already built out; see Epic 1.0 below for the fallback path if it does block.
2. **Version skew between the pinned `tymuxd` binary and the generated Go gRPC client**
   (`github.com/tstapler/tymux/clients/go/gen/tymux/v1`, pinned independently via `go.mod`) —
   `checkDaemonHealthy`'s `ListSessions` probe proves protocol identity, not RPC-shape
   compatibility. No dedicated version-reporting RPC exists upstream (confirmed by proto
   inspection in `research/stack.md` (g)); closing this fully requires an upstream
   `tstapler/tymux` change, out of this project's scope. Task 1.1.2b adds a cheap, non-enforcing
   mitigation (a checklist doc comment) for the meantime.
3. **Exact `Title`-vs-tmux-session-name key used for `TymuxSessionOverrides`/the
   `CreateSession` wiring** (Task 4.3.2a) — the plan states the intent (key by whatever name
   `StreamHubSessionOverrides` already keys by) but the precise derivation (`Title` vs. a
   generated tmux session name) needs confirming against `initTmuxSession`'s actual naming
   logic during implementation, not re-derived here.
