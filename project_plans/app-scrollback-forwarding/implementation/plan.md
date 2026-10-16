# Implementation Plan: app-scrollback-forwarding

**Feature**: Forward a client scroll-up gesture into an agent CLI's self-managed
transcript view (starting with Claude Code) and stream the resulting redraw
back as a distinct "app scrollback" page, instead of the silent no-op today's
tmux-native-only pagination produces for apps that don't write their own
scrollback to the terminal.
**Date**: 2026-09-14
**Status**: Ready for implementation
**ADRs**: [ADR-001: Depend on undocumented, version-fragile agent-CLI scroll keybindings](../decisions/ADR-001-depend-on-undocumented-agent-cli-scroll-keybindings.md)

---

## Step 0.5 — Alternatives Considered (creative pass)

Three distinct approaches were weighed before committing to an architecture:

**A. Gesture-forwarding + live redraw capture** (send a synthetic scroll
keystroke into the PTY, wait for the app's redraw to settle, capture it as
the "page"). *Strength*: directly satisfies the stated need — reveals content
the app itself scrolled past, and generalizes across any adapter with a
forwardable scroll command. *Weakness*: highest technical risk — virtualized
transcripts, a redraw-capture race with live output, multi-client PTY-write
hijack, and version-fragile keybindings (`research/pitfalls.md`).

**B. Native-scrollback-dump fallback** (drive Claude Code's own `Ctrl+O`+`[`
transcript-export mechanism, then serve the result through the already-shipped
`tmux capture-pane` pagination path unchanged, per `research/build-vs-buy.md`).
*Strength*: reuses proven, already-tested plumbing with zero new response
framing. *Weakness*: Claude-Code-specific only (not a general mechanism), and
the dump keystroke is itself an extra side effect that briefly changes the
pane's real content.

**C. Structured-transcript read** (parse Claude Code's own on-disk JSONL
transcript via `ClaudeAdapter.Import`, entirely bypassing the PTY).
*Strength*: no PTY-write risk at all — no hijack risk, no capture race.
*Weakness*: explicitly rejected by requirements.md's own Alternatives
Considered section ("the user explicitly wants gesture forwarding"), and
would render structured JSON turns rather than the actual TUI pixels the user
asked to see.

**Chosen: A, with B wired in as Claude Code's adapter-internal fallback
strategy** (see ADR-001), validated by an explicit feasibility spike
(Story 1.2.1) before any downstream work depends on which one ships. C is
rejected outright per requirements and recorded in the Pattern Decisions table
below.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `AltScreenTracker` | Stateful scanner (`pkg/ansi`) that observes PTY output bytes for DECSET 1049 (`ESC[?1049h`/`ESC[?1049l`) and reports whether the alternate screen buffer is currently active. | Mirrors `pkg/ansi/csi.go`'s `StripCSI` style; net-new, no existing alt-screen tracking exists in the repo (`research/build-vs-buy.md` §3). |
| `AltScreenActive` | New boolean field on `Instance`/`InstanceSnapshot` recording the live alt-screen state for a session's pane, updated by `AltScreenTracker` observations. | Added to `InstanceSnapshot` per `.claude/rules/instance-lock-free-reads.md`; read via `GetAltScreenActive()`, never the raw field. |
| `ScrollAdapter` | Interface mapping a program name to per-CLI scroll-forwarding capability: `CanHandle(program string) bool`, `Capability() ScrollForwardCapability`, `KeySequences(ScrollDirection) [][]byte` (an *ordered list* of separate writes, not one), `CaptureVia() ScrollCaptureMode`. | Deliberately separate from `HistoryAdapter` (transcript file conversion) — see Pattern Decisions. Widened from an earlier single-write `ScrollGesture([]byte, bool)` shape during architecture review (BLOCKER: `NativeDumpFallbackStrategy`'s two-write, different-capture-mechanism behavior wasn't representable — see Pattern Decisions row "Capture mechanism selection"). |
| `ScrollCaptureMode` | Sum type declared by `ScrollAdapter.CaptureVia()`: `RedrawQuiescenceCapture` (send keys, wait for `RedrawQuiescence`, capture via `CapturePaneContentPriority`) \| `NativeScrollbackDelegateCapture` (send keys, then defer entirely to the existing `GetScrollbackHistory`/tmux-capture pagination path — no quiescence wait, no fresh pane capture). | Lets `Instance.ForwardScroll` (Story 1.3.1) branch on the interface's own declared contract instead of type-switching on concrete strategy type or relying on undeclared per-strategy protocol knowledge. |
| `ScrollForwardCapability` | Value object recording, per adapter: the confirmed keystroke byte sequence, the CLI version it was verified against, and known failure modes. | The durable per-adapter feasibility record `research/ux.md` §6 recommends instead of an undocumented assumption. |
| `ScrollDirection` | Sum type: `ScrollUp` \| `ScrollDown`. | Phase 1 only forwards `ScrollUp` (requirements' explicit scope); `ScrollDown` is reserved for the lease-release/return-to-live flow, not a user-facing gesture. |
| `GestureForwardStrategy` | `ScrollAdapter` sub-implementation that sends the CLI's own scroll keybinding directly (e.g. `PageUp` bytes to Claude Code's fullscreen conversation view): `KeySequences` returns a single-element `[][]byte`, `CaptureVia` returns `RedrawQuiescenceCapture`. | GoF Strategy; the "A" approach above. |
| `NativeDumpFallbackStrategy` | `ScrollAdapter` sub-implementation that drives a CLI's transcript-to-native-scrollback export (Claude Code: `Ctrl+O` then `[`) and defers to the existing tmux-capture pagination path: `KeySequences` returns `[][]byte{{0x0F}, {0x5B}}` (two separate writes, per BUG-031's separate-write discipline), `CaptureVia` returns `NativeScrollbackDelegateCapture`. | GoF Strategy; the "B" approach above, wired as Claude's fallback per ADR-001. |
| `AppScrollGate` | Composite safety predicate combining: (1) `ScrollAdapter.CanHandle(GetProgram())`, (2) `GetAltScreenActive()`, (3) `DetectedStatus == StatusIdle` (an allowlist of exactly one status, not a denylist of unsafe ones — see Task 1.1.3a), (4) single-client connection check, before a scroll keystroke may be sent. | A plain guard-clause composition, not a new state-machine value — see Pattern Decisions for why `DetectedStatus` itself isn't extended. |
| `ScrollLease` | Per-`Instance`, single-flight exclusive claim (atomic CAS, bounded lifetime) held by the one client currently forwarding a scroll gesture for a session. | Serializes concurrent forward attempts. Distinct from — but held alongside — the new `StreamHub` attach barrier below; the lease guards *one Instance's* concurrent `ForwardScroll` calls, the barrier guards *new subscribers joining* during an in-flight forward. |
| `ScrollForwardAttachBarrier` | New `sync.Mutex` field on `StreamHub` (`session/streamhub/hub.go`), held for the full duration of an in-flight `ForwardScroll` (PathHubOwned sessions only) via `BeginScrollForward()`/`EndScrollForward()`, and acquired by `AttachSubscriber` before its existing `h.mu.Lock()` critical section. | Closes the check-then-act race in the multi-client mitigation (architecture review BLOCKER) — see Risk Control's "Concrete resolution" section below. Bounded by `RedrawQuiescence`'s own deadline, so a concurrent connect is delayed by at most that constant, never blocked indefinitely. |
| `AltScreenGestureTrigger` | New client-side logic (`XtermTerminal.tsx`) that recognizes a scroll-up wheel/touch gesture while the client's own alt-screen tracking says the pane is alt-screen-active, and invokes the existing `requestScrollback` call instead of relying on the `viewportY`-based `.xterm-viewport` `scroll` listener (which never fires for an alt-screen pane — see Epic 1.4's new Story 1.4.0). | Reuses `TerminalStreamManager.ts:370`'s existing `\x1b[?1049h`/`l` substring detection (today only checked for the exit/`needsRefresh` case) rather than inventing a second detector or round-tripping the server's `AltScreenActive` over the wire. |
| `ScrollForwardOutcome` | Sum type: `Delivered` \| `AtTop` \| `NoCapability` \| `Blocked`. | The four UX states `research/ux.md` §4 identifies as needing distinct treatment. |
| `ScrollBlockedReason` | Sum type carried alongside `Blocked`, distinguishing *why*: `MultipleViewers` (real 2+ subscriber contention on a `PathHubOwned` session), `LeaseContention` (this client's own rapid repeat scroll racing itself), `UnsupportedStreamingPath` (any session on `PathLegacyPerConnection`, unconditionally, via the `-1` sentinel — see Risk Control). | Closes the adversarial-review BLOCKER that a single hardcoded `Blocked` toast conflated three structurally different causes, one of which (`UnsupportedStreamingPath`) is factually false to render as "another viewer is connected" for a solo user on the legacy path. |
| `AppScrollbackResponse` | New `TerminalData` oneof message (field 20) carrying a captured redraw's content, its `ScrollForwardOutcome`, the adapter's program name, and a `forward_id` correlation token. | Deliberately not a reuse of `ScrollbackResponse`'s sequence-number contract or `ANSI_SNAPSHOT_PREFIX` — see Pattern Decisions and `research/architecture.md` §5-6. |
| `RedrawQuiescence` | The settle-detection wait (reusing the `resizeSettling`/`ResizeQuiescence` pattern) between sending a forwarded keystroke and capturing the resulting pane content. | Reuses `server/services/connectrpc_websocket.go`'s existing `resizeSettling *atomic.Bool` precedent, not a new sleep-then-capture. |
| `ScrollSourceIndicator` | Client-side banner component communicating "you're viewing `<Program>`'s own history" while a `ScrollLease` is held by this client. | Resolves the deferred UX open question — see Risk Control. Reuses the `visuallyHidden`+`aria-live="polite"` pattern already in `XtermTerminal.tsx:1325`/`web-app/src/styles/a11y.css.ts`. |
| `scrollForwardKeybindingCanary` | Live drift-detection check: after a forwarded keystroke, verify the captured redraw changed in a way consistent with "scrolled," not just "any redraw happened." | Mirrors `compactingCanary`/`shellMonitorWordingCanary` (`session/detection/detector.go:361-413`). Volume-gated at the alert layer (3+/hour, see Observability Plan) — see `scrollForwardVersionMismatchCheck` below for the low-traffic-safe complement. |
| `scrollForwardVersionMismatchCheck` | Immediate (not volume-gated), once-per-binary-path drift detector: shells out to `claude --version` and logs a `log.Error` the first time the result differs from `ScrollForwardCapability.VerifiedAgainstVersion`. | Mirrors `session/tmux/version_check.go`'s `checkControlModeVersionMatchOnce`/`normalizeTmuxVersion`/`GetVersionMismatches` pattern (an existing, already-shipped precedent for exactly this "installed binary version differs from what this process expected" check) — added to close pre-mortem P1 #1: `scrollForwardKeybindingCanary` alone can't surface a regression for a session with too little scroll traffic to trip its 3+/hour threshold, but a version mismatch is detectable the moment a session with a differing `claude` binary starts, independent of how much that session's user happens to scroll. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Per-CLI scroll capability lookup | New `ScrollAdapter` interface + registry (`resolveScrollAdapter`), mirroring `HistoryAdapter`/`resolveHistoryAdapter` | GoF Strategy; PoEAA-adjacent Registry | Bolt `ScrollGesture`/`Capability` methods onto the existing `HistoryAdapter` interface | Interface pollution — `HistoryAdapter.Import`/`Export` solve an unrelated problem (transcript file conversion) that has nothing to do with live PTY writes; this repo has already shipped one drift bug (`isClaudeAntigravityFamily` vs. `AgyAdapter.CanHandle`) from two checks that were supposed to agree quietly diverging, and conflating two unrelated capability concepts in one interface is how that class of bug happens again |
| Claude Code's forwarding mechanism | Two `ScrollAdapter` sub-implementations (`GestureForwardStrategy`, `NativeDumpFallbackStrategy`) selected by the Phase 1 spike's outcome, both behind the same interface | GoF Strategy | Runtime auto-fallback (try gesture-forward; if the captured redraw looks unchanged, silently retry via native dump) | `research/ux.md` §4a explicitly warns against inferring capability live from output diffing — a redraw that happens to look identical is indistinguishable from "no capability" and "already at the top," so a runtime auto-switch would misfire; a statically-known, spike-confirmed choice per adapter is safer and testable |
| Safety gate before sending a scroll keystroke | `AppScrollGate`: a plain composed predicate function, not a new enum state | Guard-clause composition (no formal GoF name — deliberately lightweight) | Extend `DetectedStatus` with a new `StatusScrollSafe` value | `DetectedStatus` is a general session-status concept read by many consumers (status badges, backlog triage); overloading it with a scroll-specific safety concept couples two unrelated concerns — `research/pitfalls.md` §1 notes explicitly that none of the existing states "were designed to answer this," so it's a new invariant, not a variant of an existing one |
| Concurrent-forward serialization | `ScrollLease`: per-`Instance` atomic CAS, single-flight, bounded lifetime | Concurrency primitive (`sync/atomic`), not a queue | A request queue that serializes multiple pending forward requests | `research/pitfalls.md` §1.3 flags that queued writes racing live PTY output have no defined interleaving guarantee today; rejecting a concurrent request outright (client retries) is simpler and avoids introducing a second write-ordering problem on top of the one already being solved |
| Capture mechanism selection inside `ForwardScroll` | `ForwardScroll` branches on `ScrollAdapter.CaptureVia()`'s declared `ScrollCaptureMode`, sending each of `KeySequences()`'s writes in order first | Interface declares its own contract (Liskov-safe Strategy) | Type-switch on the concrete strategy type (`switch s := adapter.(type) { case *GestureForwardStrategy: ...; case *NativeDumpFallbackStrategy: ... }`) | Architecture review BLOCKER: the original single-write `ScrollGesture([]byte, bool)` shape couldn't represent `NativeDumpFallbackStrategy`'s two-write, different-capture-path behavior, so the two "interchangeable" strategies weren't actually substitutable — a type-switch would "fix" the symptom but defeats the point of the interface (callers back to needing per-strategy knowledge); widening the interface itself keeps `ForwardScroll` strategy-agnostic |
| Newly-attaching subscriber during an in-flight forward | `ScrollForwardAttachBarrier`: `StreamHub.AttachSubscriber` blocks (bounded by `RedrawQuiescence`'s deadline) on a dedicated mutex held for the full `ForwardScroll` call, PathHubOwned only | Hold-for-duration mutex, not a repeated/re-checked point-in-time count | Re-check `SubscriberCount()` at multiple points during `ForwardScroll` (poll-based) | Architecture review BLOCKER: a re-check is still check-then-act with a narrower window, not a closed one — a subscriber attaching between the last re-check and the capture completing still gets hijacked; a mutex held for the operation's actual duration closes the window structurally rather than shrinking it. Deliberately *not* the previously-rejected "shield other clients' live view" primitive (that requires new per-subscriber paused state in `session/streamhub/subscriber.go`, touching every existing subscriber) — this barrier only delays a *new* attach's `AttachSubscriber` call, adding one `sync.Mutex` to `hub.go` and no changes to existing subscriber state |
| Response framing for a captured redraw | New `AppScrollbackResponse` message, own discriminator + `forward_id`, not reusing `ScrollbackResponse` or `ANSI_SNAPSHOT_PREFIX` | PoEAA-adjacent: distinct DTO per distinct semantics | Reuse `TerminalOutput`/`ANSI_SNAPSHOT_PREFIX` full-pane-replacement framing | `research/architecture.md` §6 names this "the single highest-risk implementation shortcut this project could take" — the client's prefix-sniffing dispatch (`TerminalStreamManager.ts:268`) cannot distinguish a routine resize resync from a forwarded-scroll page if they share the same prefix, risking clobber/staleness races |
| Data-fetch strategy inside `handleScrollbackRequest`/`handleShellScrollbackRequest` | Transaction Script: a thin per-request orchestration function (`Instance.ForwardScroll`) that calls adapter → gate → lease → send → quiescence → capture in sequence | PoEAA: Transaction Script | Domain Model (a richer `ScrollSession` aggregate object with its own lifecycle) | The operation is a single linear sequence with no persisted state beyond the in-memory `ScrollLease` and no business rules complex enough to warrant a domain object graph — Transaction Script matches the complexity level, consistent with how `handleScrollbackRequest` itself is already structured |
| Structured-transcript read (Approach C) | Rejected outright, not built | — | Parse `ClaudeAdapter.Import`'s JSONL transcript instead of forwarding keystrokes | Explicitly out of scope per requirements.md's own Alternatives Considered ("the user explicitly wants gesture forwarding"); would also render structured turns rather than the TUI's actual visual output |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `server/services/connectrpc_websocket.go` (3965 lines) | Large file; `onScrollbackRequest`'s signature (`func(startLine, endLine string) (string, error)`) is tmux-capture-range-shaped and has no way to express "forward a gesture" | **Extend as-is via the existing callback seam** — formalizing `research/architecture.md` §7's verdict, confirmed correct on independent reading | The callback field is already isolated behind a narrow function-typed field precisely so behavior can be swapped per call site (`streamViaControlMode` at :1343, `streamViaHub` at :2053) — exactly two call sites to update, not a scattered set. Widening its signature to `func(ctx, ScrollbackForwardRequest) (ScrollbackResult, error)` is the seam doing its job, not a sign it needs restructuring first. Confirmed by direct read of both call sites (`server/services/connectrpc_websocket.go:1343-1355`, `:2053-2059`) during this planning pass — both remain a single closure assignment. |
| `session/history_adapter.go` | None — file is small (29 lines), single-purpose, already has the "one resolution point" property this project wants to mirror | **Extend by imitation, not modification** | `resolveHistoryAdapter` itself is untouched by this project; `resolveScrollAdapter` is a new, parallel file following the same shape (see Pattern Decisions row 1) |
| `session/instance_program.go:12-20` (`isClaudeAntigravityFamily`) | Documented historical drift bug between this family-gate check and `AgyAdapter.CanHandle` (`session/instance_program_test.go:202-211`) | **Isolate via seam — do not add a third independent check** | `ScrollAdapter.CanHandle` must derive from (or at minimum be tested against) the same program-string matching this repo already has (`isClaude`/`isPi` at `session/instance_tmux.go:148-183`), per Task 1.1.2c's explicit cross-check test, rather than writing a fourth string-matching implementation that can drift from the other three |

---

## Migration Plan

Omitted — no persisted data schema changes. The new `TerminalData` oneof field
(20) and `Instance`/`InstanceSnapshot` field (`AltScreenActive`) are additive;
existing clients/sessions are unaffected until the feature flag is enabled.
`config.json`'s `feature_flags` map gains new keys on first write, same as
every other flag in `server/services/feature_flag_service.go`'s
`knownFeatureFlags` — no migration script needed.

## Observability Plan

- **Logs**: structured log line at every `Instance.ForwardScroll` entry/exit
  (`server/services/connectrpc_websocket.go`'s existing `log.Warn`/`log.Error`
  convention), recording which path served a given scroll-up request
  (`tmux_native` vs. `app_forwarded`) per the requirements' Observability
  Requirements section. A `log.Warn` when a session's program matches no
  registered `ScrollAdapter` (coverage-gap visibility, not silent
  degradation), and a `log.Warn` from `scrollForwardKeybindingCanary` when a
  forwarded keystroke's captured redraw doesn't change as expected.
- **Metrics**: `scroll_forward_attempts_total{adapter,outcome,blocked_reason}`
  (counter) — one increment per `Instance.ForwardScroll` call, labeled by
  adapter name, `ScrollForwardOutcome`, and (only when `outcome=BLOCKED`)
  `ScrollBlockedReason` (`blocked_reason="n/a"` for every other outcome, a
  bounded fourth value, not an open string); satisfies the requirements'
  "forwarded-scroll attempts vs. successes, labeled per adapter" ask and
  additionally answers the adversarial-review Concern that the
  `PathLegacyPerConnection` population's zero-benefit was "silently
  unmeasured" — `blocked_reason="unsupported_streaming_path"`'s share of
  total attempts is now a directly queryable ratio, not a gap.
  `scroll_forward_capture_duration_ms{adapter}` (histogram) measuring the
  `RedrawQuiescence` wait, since this is the one new operation plausibly
  exceeding 100ms.
- **Alerts**: the canary log line (`scrollForwardKeybindingCanary`) is wired
  into this repo's existing log-pattern-clustering tool
  (`docs/how-to/debug-with-logs.md`'s pattern-clustering reference) with a
  threshold rule — 3+ canary-warning lines for the same adapter within a
  rolling 1-hour window pages the same low-urgency channel `compactingCanary`
  drift already uses, rather than a new alert integration. Below that
  threshold, the flag/log combination remains the first response mechanism
  (flip the flag, don't page — still a UX-affecting, non-data-loss feature).
  This closes the adversarial-review Concern that "a log line nobody is
  watching is functionally still silent" — a single canary firing stays
  quiet (avoids paging on one flaky redraw), but a *trend* now surfaces
  without a human needing to think to go look for it, which was the entire
  point of building the canary in the first place.
  **`scrollForwardVersionMismatchCheck` (Story 1.5.3) is the deliberate
  exception to this volume-gating**: it fires `log.Error` immediately, once
  per distinct `claude` binary path, with no 1-hour/3-line threshold — closes
  pre-mortem P1 #1, that the organic canary's volume gate could let a
  keybinding regression on a low-traffic session go undetected indefinitely.
  A version mismatch is a strong enough signal (a confirmed input to the
  scroll-forwarding contract has changed) to warrant immediate visibility
  regardless of how many sessions have hit it yet.

## Risk Control

**Appetite reconciliation**: the stated Medium (1-2 week) appetite is tight
against Phase 1's actual surface (a new `StreamHub` synchronization
primitive, three feature flags, dual drift-detection mechanisms) — Phase 1
alone may stretch it. This is a real factor, not just a technical one, in the
Story 1.2.1 spike's strategy choice: if the spike shows `NativeDumpFallbackStrategy`
(reusing the already-shipped tmux-capture pagination path) recovers comparable
history depth to `GestureForwardStrategy` (new `RedrawQuiescenceCapture`
machinery), prefer the cheaper strategy partly *because* it fits the appetite
better, not purely because it's technically safer.

- **Feature flags**: `terminal:app-scrollback-forwarding:claude` (default
  off), `terminal:app-scrollback-forwarding:pi` (default off, Phase 2),
  `terminal:app-scrollback-forwarding:agy` (default off, Phase 3, not wired
  until agy's keybindings are researched) — live-settable via
  `config.FeatureFlags`/`FeatureFlagService`, never an env var, per this
  repo's convention (`config/config.go`'s `FeaturePiSupport` precedent) and
  this session's own memory (`rollout flags: live-settable, no env vars`).
  Each CLI's forwarding path can be disabled independently without affecting
  the others, addressing `research/pitfalls.md` §3's requirement that the
  flag be scoped per-adapter, not a single top-level toggle.
- **Rollback procedure**: flip the relevant flag off. The existing
  tmux-native `GetScrollbackHistory` pagination path is completely unchanged
  and remains the fallback for every session this feature doesn't cover
  (requirements' own Risk Control section, unmodified by this plan).
- **Staged rollout**: full rollout on merge, flag defaulting to off; the
  flag itself is the staging mechanism — no percentage/cohort rollout
  infrastructure exists in this repo for this flag type, consistent with
  `FeaturePiSupport`'s precedent (binary opt-in, not staged).

### Concrete resolution — multi-client PTY-write-hijack risk (deferred by requirements.md, resolved here)

**Decision: block forwarding outright whenever more than one client is
currently connected to the session, *and* close the check-then-act window
between that check and the send+capture it guards — never attempt to shield
or pause other clients' live view.**

Mechanism: `Instance.ForwardScroll` checks the session's live subscriber
count as part of `AppScrollGate` — `hub.SubscriberCount()` for `PathHubOwned`
sessions (the count already surfaced to clients via the existing
`connection_count` field on `TerminalOutput`, `proto/session/v1/events.proto:171`
and `ConnectionCountIndicator`). Forwarding is available only when that count
is exactly 1 (the requester's own connection); if 2+, the gate returns
`ScrollForwardOutcome.Blocked` immediately, before any PTY write happens.
`PathLegacyPerConnection` sessions get the same `Blocked` outcome by
construction, not by a path-aware branch inside `AppScrollGate` itself — its
signature (Task 1.1.3a) takes only `subscriberCount int`, no path parameter.
Instead, the `streamViaControlMode` closure (Task 1.3.3b) passes a fixed
sentinel `subscriberCount: -1` for that path, rather than deriving a count
from `activeControlModeStreams`, whose *stream-generation* semantics (not
concurrent-viewer count — `server/services/connectrpc_websocket.go:331`)
can't support this gate accurately. `-1` never equals 1, so `AppScrollGate`'s
check (4) always fails for `PathLegacyPerConnection`, producing the same
`Blocked` outcome without the call site pretending `activeControlModeStreams`
measures something it doesn't (architecture-review iteration-2 Concern: the
plan previously asserted this path was "already unconditionally Blocked"
without stating what value made that true).

**The `-1` sentinel and a genuine `subscriberCount >= 2` must not collapse
into the same client-visible reason** (adversarial-review BLOCKER: an
earlier draft's single hardcoded `Blocked` toast — "another viewer is
connected" — is factually false for a solo user whose session happens to be
on `PathLegacyPerConnection`, since that path is blocked unconditionally
regardless of actual viewer count). `AppScrollGate`'s check (4) therefore
returns a distinct reason string per case — `subscriberCount < 0` →
`"unsupported streaming path"`, `subscriberCount > 1` → `"multiple viewers
connected"` — and `Instance.ForwardScroll` (Task 1.3.1c) maps each to its own
`ScrollBlockedReason` (`UnsupportedStreamingPath` vs. `MultipleViewers`) on
`AppScrollbackResponse`, plus a third (`LeaseContention`) when
`scrollLease.tryAcquire()` itself fails rather than the gate. Story 1.4.3's
client-side toast branches on this field so a `PathLegacyPerConnection`
session's solo user sees "scroll-forwarding isn't available for this session
yet," not a claim about a second viewer that isn't there. This also converts
the previously-unmeasured `PathLegacyPerConnection` population (adversarial
review: "neither the plan's Unresolved Questions nor its Risk Control
tradeoff discussion mentions this population at all") into a measured
quantity for free — `scroll_forward_attempts_total`'s `outcome` label already
fires once per attempt (Epic 1.5's Observability Plan), and adding
`blocked_reason` as a second label on that same counter (Task 1.5's metric
definition, below) directly answers "what fraction of blocked attempts are
solo users on the unsupported path" without a separate ad hoc query.

**This single entry-time check, by itself, is check-then-act**: a second
client attaching *during* the send+`RedrawQuiescence` window (confirmed by
reading `AttachSubscriber`'s actual body, `session/streamhub/hub.go:295-336`
— it's a single `h.mu`-locked critical section with no re-check against an
in-flight forward) would still get the mid-scroll pane as its catch-up
snapshot, reproducing the exact hijack this section claims to eliminate — an
architecture-review BLOCKER on an earlier draft of this plan, which described
the entry-time check alone as "fully eliminating" the risk. It does not; the
check must be held for the operation's full duration, not just evaluated once
at the start. `ScrollForwardAttachBarrier` (Story 1.3.1, Task 1.3.1b) closes
this: a new `sync.Mutex` on `StreamHub`, acquired by `BeginScrollForward()`
for the entire `ForwardScroll` call and consulted by `AttachSubscriber` as
its very first statement (`PathHubOwned` only — `PathLegacyPerConnection` is
already unconditionally `Blocked` above regardless of this addition, so it
needs no barrier of its own). A concurrent `AttachSubscriber` call blocks,
bounded by `waitForRedrawQuiescence`'s own deadline (the same constant this
plan already commits to elsewhere), rather than proceeding and observing
stale/mid-scroll content. This is a hold-for-duration lock, not a repeated
point-in-time re-check — the distinction matters because a re-check narrows
the race window without closing it (a subscriber could still attach between
the last re-check and capture completing), while a lock held for the actual
operation closes it structurally.

Why this — entry-time block plus a hold-for-duration attach barrier, *not* a
richer shielding mechanism — over the alternatives requirements.md sketched
(serialize vs. leader/follower arbitration vs. notify-and-accept): a true
"shield other clients' live view and resync them after" mechanism would
require a new per-subscriber selective-pause primitive inside
`session/streamhub` (no such primitive exists today — `subscriber.go` has no
paused/held state) built across *two* parallel streaming architectures
(`PathLegacyPerConnection` and `PathHubOwned` are mid-migration, per the
`connection_count`'s own proto doc comment). That is materially more new
surface area than this Medium-appetite, Phase-1-is-Claude-Code-only project
should take on, and — per `research/pitfalls.md`'s own framing — inventing a
new PTY-adjacent synchronization primitive is exactly the class of thing this
repo has been burned by before. `ScrollForwardAttachBarrier` is not that
primitive: it touches no existing subscriber's delivery path, only delays a
*new* `AttachSubscriber` call for one bounded window — one `sync.Mutex`
field and a lock/unlock at the top of an existing method. Blocking outright
plus this barrier is structurally simple (there is no second client to
hijack for the barrier's duration, and none can attach mid-operation to
become one), fully
eliminates the risk rather than mitigating it, and degrades honestly: a
blocked attempt tells the requester why (Story 1.4.3), rather than silently
doing nothing (today's baseline) or corrupting a second client's view (the
naive-forwarding regression). The tradeoff — forwarded scroll unavailable
during paired/shared-viewing sessions — is recorded as an Unresolved Question
below, not silently accepted, and revisitable once single-client usage data
exists.

The `ScrollLease` additionally serializes a *single* client's own overlapping
requests (e.g. rapid repeated scroll gestures on mobile, per
`research/pitfalls.md` §5.3's "compounding" risk) — this applies even in the
single-client case and is not conditional on the connection-count gate.

#### Decision — per-connection, not per-user identity (pre-mortem P1 #5)

**Decision: `AppScrollGate`'s check stays keyed on raw connection count
(`hub.SubscriberCount()`). Phase 1 does not attempt to dedup by user
identity.** The same person viewing one session from two devices (laptop +
phone, or two browser tabs) is therefore indistinguishable from two different
people, and sees the same `MULTIPLE_VIEWERS` `Blocked` outcome either way —
accepted as a known Phase-1 limitation, not silently absent.

This was a real choice, not a default: option (a) — key the gate on distinct
authenticated user identity instead of raw connection count — was checked
against what this codebase's auth layer actually carries, not assumed.
`server/middleware/auth.go`'s `Auth` middleware calls a single
`AuthValidator.ValidateAuthSession(token string) bool` — it returns whether
the request is authenticated at all, not a user or credential identifier, and
nothing downstream of it (the ConnectRPC WebSocket handler in
`server/services/connectrpc_websocket.go`, `StreamHub`/`subscriber` in
`session/streamhub/hub.go`+`subscriber.go`) carries any per-connection
identity value today — confirmed by reading all three files during this
planning pass; `session/streamhub/subscriber.go`'s `subscriber` struct holds
only `id SubscriberID`, `transport`, and `capability`, no owner/identity
field. stapler-squad is also a single-user personal tool (WebAuthn
credentials in `server/auth/webauthn.go` authenticate *a* device to *the one*
account, not one of several distinct users), so "distinct authenticated
user" isn't even a concept the system currently models — building option (a)
would mean inventing a new per-connection identity primitive (e.g. a stable
per-device/session credential ID threaded from the WebAuthn layer through the
HTTP handshake into `StreamHub.AttachSubscriber` and `AppScrollGate`) purely
to serve this one gate. That is materially more new surface area than this
Medium-appetite, Phase-1-is-Claude-Code-only project should take on — the
same proportionality argument this plan already makes in the sibling
"multi-client PTY-write-hijack risk" decision above against building a
richer shielding primitive. Option (b) — accept the limitation explicitly —
costs nothing new and is honest about what the toast (`design/ux.md` Surface
3's `MULTIPLE_VIEWERS` copy) already says: "another viewer is connected,"
which is technically true (another *connection*) even when it's the same
person, and the instruction ("close other tabs or sessions") is equally
correct and actionable regardless of who owns the other connection —
`design/ux.md`'s own "Both clients belong to the same person" edge case
(Surface 3) already documents this, this decision just makes it the
project's explicit committed default rather than an unstated one.

This is recorded as a **committed Phase-1 limitation**, not merely deferred:
the Unresolved Questions entry below on whether the single-client-only
restriction should become a richer mechanism remains open (that's a larger
question, gated on real usage data), but *this specific sub-question* —
per-connection vs. per-user — is resolved now. A same-user-multiple-devices
test scenario asserting this accepted (not fixed) behavior is added to
`validation.md`.

### Concrete resolution — UI indicator for app-managed scrollback (deferred by requirements.md, resolved here)

**Decision: yes, build `ScrollSourceIndicator` — a persistent, accessible
banner shown while a `ScrollLease` is held by the viewing client.**

Content: "Viewing `<Program>`'s own history" (e.g. "Viewing Claude Code's own
history"), auto-dismissed when the lease releases and the view returns to
live. Implementation reuses the existing `visuallyHidden`+`aria-live="polite"`
announcer pattern already used in this exact component
(`XtermTerminal.tsx:1325`, `web-app/src/styles/a11y.css.ts`) rather than
inventing a new one. Justification: `research/ux.md` found no existing
terminal product that solves this ambiguity for free (iTerm2's alt-screen
scrollback suppression is the closest analog, and it's still a documented
source of user confusion even there), identified it as "not optional polish"
given three states a user can't otherwise distinguish (no capability / hit
top / still loading), and named a direct precedent already shipped in this
codebase for the same kind of persistent status affordance (commit
`0ce32e531`, "fork-pressure hysteresis + system status banner"). Designed as
Story 1.4.2, with the three failure-mode UX states (`AtTop`, `NoCapability`,
`Blocked`) as Story 1.4.4/1.4.3 sharing the same indicator component rather
than three bespoke banners.

## Unresolved Questions

- [ ] Does sending literal `PageUp` bytes (`\x1b[5~`) to a Claude Code session
  in fullscreen rendering actually scroll its transcript, confirmed via a
  captured-pane diff — or does `GestureForwardStrategy` need to fall back to
  `NativeDumpFallbackStrategy` per ADR-001? — blocks Story 1.3.1 (and
  everything downstream of it in Epic 1.3/1.4) — owner: whoever runs
  Story 1.2.1's spike, decision recorded in
  `session/scroll_forward_capability.go`'s doc comment before Epic 1.3 starts.
- [ ] agy's specific alt-screen scroll-keybinding IDs were not located by
  Phase 2 research (`research/stack.md` §3) — blocks all of Phase 3 (Epic 3.1)
  — owner: whoever picks up Phase 3; requires either fetching agy's CLI
  Reference doc page or empirically testing a live `agy --sandbox` session's
  `?`/help listing, per `research/stack.md`'s own next-step note.
- [ ] Is the single-client-only restriction (Risk Control, above) an
  acceptable permanent constraint, or should a follow-up epic build the
  richer live-view-shielding mechanism for multi-client sessions? (Note: the
  narrower per-connection-vs-per-user question is *not* part of this open
  item any more — Risk Control's "Decision — per-connection, not per-user
  identity" resolves it as a committed Phase-1 default. What remains open
  here is only whether the coarser single-client restriction itself, of
  whichever kind, should eventually get a richer shielding mechanism.) — does
  not block Phase 1 shipping (the restriction is this plan's committed
  default) — owner: product decision after Phase 1 ships and real usage data
  shows whether paired/shared-viewing sessions commonly hit the block.
- [ ] Does pi's `tui.altScreen.pageUp`/`pageDown` respect a user's *remapped*
  keybindings, or only the documented defaults? (`research/stack.md` §4 flags
  this as relevant if stapler-squad ever needs to detect a customized keymap)
  — blocks Story 2.1.1's capability-confirmation task — owner: whoever runs
  the Phase 2 spike; if remapping can't be detected, `ScrollForwardCapability`
  for pi should document defaults-only as a known limitation, not block
  shipping.

## Dependency Visualization

```
Epic 1.1 (Detection Foundation)
  AltScreenTracker ─┐
  ScrollAdapter registry ─┼─→ AppScrollGate ─┐
  ScrollForwardCapability ┘                  │
                                              ▼
Epic 1.2 (Feasibility Spike) ──decision gate──→ Epic 1.3 (Server pipeline)
  Story 1.2.1: confirm Gesture vs.              ScrollLease
  NativeDump strategy for Claude                ScrollForwardAttachBarrier
  (KeySequences()/CaptureVia() shape             (StreamHub, new)
  chosen per strategy)                          RedrawQuiescence (Redraw-
                                                 QuiescenceCapture mode only)
                                                 AppScrollbackResponse wiring
                                                              │
                                                              ▼
                                              Epic 1.4 (Client rendering)
                                                 Story 1.4.0: alt-screen
                                                 scroll-up trigger (wheel +
                                                 touch) — no code dependency
                                                 on 1.3, can start once
                                                 Epic 1.1's client-visible
                                                 escape-sequence convention
                                                 is settled
                                                 Story 1.4.5: loading pill
                                                 (Surface 1, shared w/ the
                                                 tmux-native path) — ships no
                                                 later than 1.4.2
                                                 AppScrollbackResponse handler
                                                 ScrollSourceIndicator
                                                 Blocked/AtTop/NoCapability UX
                                                              │
                                                              ▼
                                              Epic 1.5 (Observability + flag)
                                                 (can start in parallel with
                                                 1.3/1.4 — flag registration
                                                 has no code dependency on them)
                                                              │
                    ┌─────────────────────────────────────────┘
                    ▼
        Phase 1 SHIPPABLE (Claude Code complete)
                    │
        ┌───────────┴───────────┐
        ▼                       ▼
  Epic 2.1 (pi adapter)   Epic 3.1 (agy research spike,
  reuses Epic 1.1/1.3/1.4  blocked on Unresolved Question
  scaffolding, new          above — may not produce an
  ScrollAdapter impl only   implementation epic at all)
```

---

## Phase 1: Claude Code (shippable MVP)

### Epic 1.1: Detection Foundation
**Goal**: A session's live scroll-forwarding eligibility (program + alt-screen
state + safety) can be queried without any of Epic 1.3's send/capture logic
existing yet — this epic is entirely read-side and independently testable.

#### Story 1.1.1: Live alt-screen state tracking per instance
**As a** scroll-forwarding gate, **I want** to know whether a session's pane
is currently in the alternate screen buffer, **so that** forwarding is never
attempted against a plain shell a user has dropped to inside an agent-CLI
session.
**Acceptance Criteria**:
- `AltScreenTracker.Observe` correctly detects entry and exit.
  - *Given* a fresh `AltScreenTracker`, *When* `Observe` is called with bytes
    containing `"\x1b[?1049h"`, *Then* it returns `(active=true, changed=true)`;
    a second `Observe` call with plain text `"hello\n"` returns
    `(active=true, changed=false)`.
- `Instance.GetAltScreenActive()` reflects the tracker's last observation via
  the lock-free snapshot path.
  - *Given* an `Instance` whose PTY output stream has just delivered bytes
    containing `"\x1b[?1049l"`, *When* `GetAltScreenActive()` is called from a
    different goroutine, *Then* it returns `false` without a data race
    (verified by `go test -race`).
**Files**: `pkg/ansi/altscreen.go` (new), `pkg/ansi/altscreen_test.go` (new),
`session/instance_snapshot.go`, `session/instance_actor_setters.go`,
`session/instance_terminal.go`

##### Task 1.1.1a: Implement `AltScreenTracker` (~4 min)
- Add `pkg/ansi/altscreen.go`: a small stateful struct (mirrors
  `pkg/ansi/csi.go`'s `StripCSI` style — regex-based, no new dependency) with
  `type AltScreenTracker struct{ active bool }` and
  `func (t *AltScreenTracker) Observe(data string) (active, changed bool)`
  scanning for `\x1b[?1049h` (sets `active=true`) and `\x1b[?1049l`
  (`active=false`), returning whether the call changed the state.
- Files: `pkg/ansi/altscreen.go`

##### Task 1.1.1b: Unit tests for `AltScreenTracker` (~3 min)
- Table-driven test covering: enter-only, exit-only, enter-then-exit in one
  call, no markers present (no change), and markers split across two
  `Observe` calls (state must persist between calls).
- Files: `pkg/ansi/altscreen_test.go`

##### Task 1.1.1c: Add `AltScreenActive` to `InstanceSnapshot` (~3 min)
- Add `AltScreenActive bool` field to `InstanceSnapshot` struct
  (`session/instance_snapshot.go`) and its `buildSnapshot` population, per
  this file's own header comment ("single authoritative place").
- Files: `session/instance_snapshot.go`

##### Task 1.1.1d: Add setter + lock-free accessor (~4 min)
- Add `setAltScreenActiveLocked(active bool)` to
  `session/instance_actor_setters.go`, following the existing
  `setGitHubResolutionLocked` shape (mutate under `i.mu.Lock()`, republish
  snapshot).
- Add `GetAltScreenActive() bool` to `session/instance_terminal.go`, mirroring
  `GetProgram()`'s `return i.Snapshot().AltScreenActive` shape.
- Files: `session/instance_actor_setters.go`, `session/instance_terminal.go`

##### Task 1.1.1e: Wire tracker into the output-streaming hot path (~5 min)
- In `server/services/connectrpc_websocket.go`'s per-chunk PTY-output
  processing (the same code path that already exists for both
  `streamViaControlMode` and `streamViaHub`), call
  `instance.ObserveAltScreenTransition(chunk)` (new thin `Instance` method
  wrapping the tracker + `setAltScreenActiveLocked`) once per output chunk.
- One `AltScreenTracker` instance lives on `Instance` itself (new unexported
  field, not snapshot-tracked — it's a stateful scanner, not observable state
  itself, matching `instance_snapshot.go`'s documented exclusion list for
  manager/dependency objects).
- Files: `server/services/connectrpc_websocket.go`, `session/instance.go`
  (new field + `ObserveAltScreenTransition` method)

---

#### Story 1.1.2: `ScrollAdapter` registry
**As a** developer wiring up forwarding, **I want** a single resolution point
mapping a program string to its scroll-forwarding capability, **so that** a
future adapter can't drift from the existing `resolveHistoryAdapter`-style
pattern the way `isClaudeAntigravityFamily` already has once.
**Acceptance Criteria**:
- `resolveScrollAdapter` returns the correct adapter for a known program and
  `nil` for an unknown one.
  - *Given* the registry has `ClaudeScrollAdapter` registered, *When*
    `resolveScrollAdapter("claude")` is called, *Then* it returns a
    non-nil `*ClaudeScrollAdapter` whose `CanHandle("claude")` is `true`.
  - *Given* the same registry, *When* `resolveScrollAdapter("bash")` is
    called, *Then* it returns `nil`.
- `ClaudeScrollAdapter.CanHandle` agrees with `isClaude` (the existing,
  hardened program-matching helper) on a shared test-vector set, closing the
  drift risk named in Tech Debt Disposition.
  - *Given* the test vectors `["claude", "CLAUDE", "env -u VAR claude",
    "claude-squad"]`, *When* both `isClaude(v)` and
    `ClaudeScrollAdapter{}.CanHandle(v)` are evaluated for each, *Then* they
    agree on every vector (`claude-squad` → both `false`).
**Files**: `session/scroll_adapter.go` (new), `session/scroll_adapter_test.go`
(new), `session/claude_scroll_adapter.go` (new)

##### Task 1.1.2a: Define `ScrollAdapter` interface + registry (~5 min)
- New file `session/scroll_adapter.go`: `type ScrollAdapter interface { Name()
  string; CanHandle(program string) bool; Capability()
  ScrollForwardCapability; KeySequences(dir ScrollDirection) [][]byte;
  CaptureVia() ScrollCaptureMode }`, `type ScrollDirection int` (`ScrollUp`,
  `ScrollDown` consts), `type ScrollCaptureMode int`
  (`RedrawQuiescenceCapture`, `NativeScrollbackDelegateCapture` consts), and
  `func resolveScrollAdapter(program string) ScrollAdapter` mirroring
  `session/history_adapter.go:21-29`'s structure exactly (doc comment
  explaining it's the single resolution point, same as that file's own
  comment).
  `KeySequences` returns an *ordered* slice — one element for a single-write
  strategy, two for a two-write one — so `Instance.ForwardScroll` (Story
  1.3.1) always sends every element in order rather than assuming exactly one
  write; `CaptureVia` tells `ForwardScroll` which capture path to run
  afterward. This shape replaced an earlier single-write
  `ScrollGesture(dir) ([]byte, bool)` during architecture review: that shape
  could not represent `NativeDumpFallbackStrategy`'s two-write,
  different-capture-mechanism behavior, breaking Liskov substitutability
  between the two Epic 1.2 strategies (see Pattern Decisions, "Capture
  mechanism selection inside `ForwardScroll`").
- Files: `session/scroll_adapter.go`

##### Task 1.1.2b: `ScrollForwardCapability` value type (~3 min)
- New file `session/scroll_forward_capability.go`:
  `type ScrollForwardCapability struct { KeySequencesUp [][]byte;
  VerifiedAgainstVersion string; KnownFailureModes []string }`. No logic —
  pure value object, populated by each adapter's constructor.
- Files: `session/scroll_forward_capability.go`

##### Task 1.1.2c: `ClaudeScrollAdapter` stub + `CanHandle` drift test (~4 min)
- New file `session/claude_scroll_adapter.go`: `ClaudeScrollAdapter` struct
  wrapping a `strategy ScrollAdapter` field (populated in Epic 1.2 once the
  spike resolves which concrete strategy to use — this task only wires
  `CanHandle`/`Name`, delegating `KeySequences`/`CaptureVia`/`Capability` to a
  not-yet-implemented `strategy` that panics with a clear "not yet
  implemented, see Story 1.2.1" message so any accidental early call site
  fails loudly, not silently).
- `CanHandle` delegates to `strings.Contains(strings.ToLower(program),
  "claude")` (same rule `ClaudeAdapter.CanHandle` uses,
  `session/claude_adapter.go:27-29`).
- Register `ClaudeScrollAdapter` in `resolveScrollAdapter`.
- Add the drift test comparing `isClaude`/`ClaudeScrollAdapter.CanHandle`
  vector-by-vector (per this story's Given-When-Then).
- Files: `session/claude_scroll_adapter.go`, `session/scroll_adapter_test.go`

---

#### Story 1.1.3: `AppScrollGate` composite safety predicate
**As a** forwarding pipeline, **I want** one function that says "yes, it is
currently safe to send a scroll keystroke to this session," **so that** the
keystroke never lands on a plain shell, a modal picker, or a session another
client is already forwarding for.
**Acceptance Criteria**:
- Gate returns `false` (with a reason) when any one of its four checks fails,
  and `true` only when all four pass.
  - *Given* an `Instance` with `Program="claude"`, `AltScreenActive=true`,
    `DetectedStatus=StatusIdle`, and exactly 1 connected subscriber, *When*
    `AppScrollGate(inst)` is evaluated, *Then* it returns `(true, "")`.
  - *Given* the same `Instance` but with `DetectedStatus=StatusNeedsApproval`,
    *When* `AppScrollGate(inst)` is evaluated, *Then* it returns
    `(false, "unsafe detected status: needs_approval")`.
  - *Given* the same `Instance` but with `DetectedStatus=StatusExecuting`
    (agent actively mid-turn), *When* `AppScrollGate(inst)` is evaluated,
    *Then* it returns `(false, "unsafe detected status: executing")` —
    regression guard for the architecture-review blocker that this gate must
    never treat mid-turn as safe (`research/pitfalls.md` §1,
    `research/ux.md` §4c).
  - *Given* the same `Instance` but with 2 connected subscribers, *When*
    `AppScrollGate(inst)` is evaluated, *Then* it returns
    `(false, "multiple viewers connected")`.
**Files**: `session/scroll_gate.go` (new), `session/scroll_gate_test.go` (new)

##### Task 1.1.3a: Implement `AppScrollGate` (~5 min)
- New file `session/scroll_gate.go`: `func AppScrollGate(inst *Instance,
  subscriberCount int) (ok bool, reason string)` composing, in order: (1)
  `resolveScrollAdapter(inst.GetProgram()) != nil`, (2)
  `inst.GetAltScreenActive()`, (3) a status-safety check requiring
  `DetectedStatus == StatusIdle` exactly — an allowlist of one, not a
  denylist of the statuses considered unsafe. `StatusExecuting` ("actively
  executing commands", `session/detection/detector.go:98`) is **unsafe**,
  same as `StatusNeedsApproval`/`StatusInputRequired`: forwarding a scroll
  keystroke while the agent is mid-turn is the highest-severity concurrency
  race identified in `research/pitfalls.md` §1 ("scroll keystroke lands
  mid-agent-turn... the client has no way to distinguish 'this frame is the
  scroll result' from... normal streaming output") and the explicit
  opposite of what `research/ux.md` §4c recommends ("bias toward *not*
  forwarding while output is actively streaming... and only offering
  forwarded-scroll once the session is idle"). An earlier draft of this task
  cited this session's memory note on gating *unattended* PTY writes
  (`StatusIdle` + a `StatusContext` allowlist) as precedent for treating
  `StatusExecuting` as safe — that precedent doesn't transfer: it answers
  "is it safe for the driver to inject a *new turn's prompt* while the agent
  is otherwise idle-ish," a different question from "is it safe to inject a
  *scroll keystroke* while the agent is actively streaming output," and
  applying it here produced the opposite of what both of this feature's own
  research passes concluded. Every other `DetectedStatus` value (including
  `StatusUnknown`, `StatusCompacting`, `StatusWaitingForAgent`, etc.) is also
  unsafe by construction, since the check is `== StatusIdle`, not a maintained
  exclusion list — a new status added later is unsafe by default rather than
  silently safe. (4) `subscriberCount == 1`. `subscriberCount` is passed in
  rather than queried internally, since the caller (Epic 1.3) already has it
  from the hub/legacy path and this keeps `scroll_gate.go` free of a
  `streamhub` import. `PathLegacyPerConnection` callers pass a `-1` sentinel
  to guarantee check (4) fails (see Risk Control's "Concrete resolution —
  multi-client PTY-write-hijack risk" section for why
  `activeControlModeStreams` can't supply a real count).
- Files: `session/scroll_gate.go`

##### Task 1.1.3b: Unit tests for all four failure branches + the pass case
(~4 min)
- Table-driven test with one case per Given-When-Then above plus a
  no-capability case (`Program="bash"`).
- Files: `session/scroll_gate_test.go`

---

### Epic 1.2: Claude Code Feasibility Spike
**Goal**: Resolve, with evidence (not assumption), whether
`GestureForwardStrategy` or `NativeDumpFallbackStrategy` ships for Claude Code
in Phase 1 — this is the decision gate everything in Epic 1.3 depends on
(per ADR-001 and the task brief's explicit instruction not to assume gesture
forwarding works before it's prototyped).

#### Story 1.2.1: Confirm Claude Code's scroll mechanism against a live session
**As a** planner, **I want** empirical confirmation of which strategy works,
**so that** Epic 1.3 is built against a fact, not a hope.
**Acceptance Criteria**:
- The spike produces a checked-in decision, not just a manual finding.
  - *Given* a manually-launched `claude` session inside a throwaway tmux pane
    at the installed version confirmed in `research/stack.md` (`2.1.270`),
    *When* the spike sends `\x1b[5~` (`PageUp`) via `tmux send-keys -H` and
    captures the pane before/after, *Then* either the captured content
    visibly differs in a way consistent with scrolled transcript (→
    `GestureForwardStrategy` chosen) or it doesn't (→
    `NativeDumpFallbackStrategy` chosen, confirmed by then sending `0x0F`
    then `0x5B` and verifying the dumped transcript appears in
    `tmux capture-pane` history).
  - *Given* the spike's outcome, *When* `ClaudeScrollAdapter`'s constructor is
    written (Task 1.2.1b), *Then* `session/scroll_forward_capability.go`'s
    `ScrollForwardCapability{VerifiedAgainstVersion: "2.1.270", ...}` for
    Claude records the confirmed byte sequence and outcome, per ADR-001.
- The spike also measures recoverable history depth, not just pass/fail
  (pre-mortem P2 finding #4).
  - *Given* the same long-running spike session, *When* the chosen strategy is
    scrolled repeatedly until it stops revealing new content, *Then* the
    approximate number of pages/lines actually recovered is recorded as a
    string in `ScrollForwardCapability.KnownFailureModes` (e.g. "recovers
    ~N pages before hitting AtTop on a long-running session"), so a bounded
    recovery limit is a documented, known limitation rather than a surprise
    discovered later by users.
**Files**: `session/claude_scroll_adapter.go`,
`session/scroll_forward_capability.go` (doc comment update)

##### Task 1.2.1a: Manual spike, not automated (~5 min to record findings;
the live-session testing itself is exploratory and untimed)
- Launch `claude` manually in a throwaway tmux pane (not through
  stapler-squad, to isolate the CLI's own behavior first). Send `PageUp`
  bytes, capture before/after via `tmux capture-pane -p`, diff. Record the
  raw finding (pass/fail + captured output excerpt) in this task's PR
  description or a scratch note — not committed to the repo (per this
  session's `comment proportionality` memory: investigation detail belongs
  outside the artifact).
- Also scroll repeatedly on a long-running session until no further content
  is revealed, and record the approximate recoverable depth into
  `ScrollForwardCapability.KnownFailureModes` (Task 1.2.1b) — see this
  Story's updated Acceptance Criteria (pre-mortem P2 finding #4).
- **Golden fixture (adversarial-review Concern)**: separately from the
  scratch-note finding above, save the raw before/after pane-content bytes
  this spike captures as a checked-in fixture pair (e.g.
  `session/testdata/scroll_forward_claude_before.txt` /
  `..._after.txt`) — this is data, not investigation narrative, so it's
  exempt from the proportionality note above. This gives
  `waitForRedrawQuiescence`'s comparison logic (Task 1.3.2a) and
  `scrollForwardKeybindingCanary` (Task 1.5.2a) something real to regression-
  test against, addressing the adversarial review's observation that,
  without it, "nothing in the test suite re-verifies the byte-sequence-to-
  behavior claim after the initial spike." This does **not** add live-CLI
  coverage to CI — no automated test drives a real `claude` binary — that
  tradeoff (manual spike + committed fixture + production canary, no live-CLI
  CI) is a deliberate, stated choice, not an oversight left implicit.
- No other file changes — this task is pure investigation beyond the fixture
  above; the strategy decision itself is recorded in Task 1.2.1b.

##### Task 1.2.1b: Wire the confirmed strategy into `ClaudeScrollAdapter`
(~5 min)
- Replace Task 1.1.2c's panic-stub `strategy` field with the concrete
  `GestureForwardStrategy{KeySequences: [][]byte{[]byte("\x1b[5~")}}` or
  `NativeDumpFallbackStrategy{}` per Task 1.2.1a's finding.
- Update `session/scroll_forward_capability.go`'s Claude entry with the
  verified version and byte sequence(s).
- Files: `session/claude_scroll_adapter.go`,
  `session/scroll_forward_capability.go`

##### Task 1.2.1c: Implement the chosen strategy type (~5 min)
- If `GestureForwardStrategy` won: new file
  `session/gesture_forward_strategy.go` — `KeySequences` returns the
  single-element `[][]byte{keySeq}`, `CaptureVia` returns
  `RedrawQuiescenceCapture`.
- If `NativeDumpFallbackStrategy` won: new file
  `session/native_dump_fallback_strategy.go` — `KeySequences` returns the
  two-element `[][]byte{{0x0F}, {0x5B}}` (`Ctrl+O` then `[`, per BUG-031's
  separate-write discipline, `session/pane_submit.go:19-48` — `ForwardScroll`
  sends each element as its own write, never concatenated), `CaptureVia`
  returns `NativeScrollbackDelegateCapture`, telling `ForwardScroll` to defer
  capture to the existing `GetScrollbackHistory` call instead of a fresh pane
  capture (see Story 1.3.1/1.3.2's updated capture-mode branch).
- Files: `session/gesture_forward_strategy.go` or
  `session/native_dump_fallback_strategy.go` (whichever applies)

---

### Epic 1.3: Server-side forwarding pipeline
**Goal**: A `ScrollbackRequest` for an app-scrollback-eligible session sends
the adapter's confirmed keystroke, waits for the redraw to settle, captures
it, and returns it — end to end, server-side only (no client changes yet).

#### Story 1.3.1: `Instance.ForwardScroll` orchestration + `ScrollLease` +
`ScrollForwardAttachBarrier`
**As a** `handleScrollbackRequest` caller, **I want** one method that does
gate-check → lease-acquire → attach-barrier-acquire → send → capture →
release, **so that** the existing two call sites (`streamViaControlMode`,
`streamViaHub`) each need only a single new branch, not duplicated
orchestration logic, and no client can attach mid-forward and have its view
hijacked.
**Acceptance Criteria**:
- A single in-flight forward blocks a concurrent one for the same instance.
  - *Given* an `Instance` with no active `ScrollLease`, *When*
    `ForwardScroll` is called twice concurrently from two goroutines, *Then*
    exactly one call proceeds to send a keystroke and the other returns
    `ScrollForwardOutcome.Blocked` immediately (verified via a test that
    counts `SendInputViaControlMode` invocations).
- The gate's `Blocked` verdict (multi-client) short-circuits before any PTY
  write.
  - *Given* `AppScrollGate` returns `(false, "multiple viewers connected")`
    for the calling `Instance`, *When* `ForwardScroll` is called, *Then* it
    returns `ScrollForwardOutcome.Blocked` and `instance.SendInputViaControlMode`
    is never called (mock assertion).
- A subscriber attaching mid-forward cannot observe the mid-scroll pane
  (closes the architecture-review BLOCKER: the gate was previously checked
  only once, at entry, leaving a check-then-act race for the whole
  send+quiescence window).
  - *Given* a `PathHubOwned` session with exactly 1 subscriber and an
    in-flight `ForwardScroll` call currently inside its
    `waitForRedrawQuiescence` wait, *When* a second client calls
    `hub.AttachSubscriber`, *Then* that call blocks until `ForwardScroll`
    returns (bounded by `waitForRedrawQuiescence`'s own deadline) before
    running its existing `h.mu.Lock()`/`sendCatchUpSnapshot` body, so the
    newly-attached subscriber's catch-up snapshot is never the mid-scroll
    pane content (verified with a test that starts `AttachSubscriber` in a
    goroutine during a slow fake `ForwardScroll` and asserts it does not
    return until after `ForwardScroll` completes).
- A mid-forward error releases the lease and the attach barrier on every
  error branch, not just the happy path (pre-mortem P1 #3: an untested
  `defer`-ordering bug here could leave a session's scroll-forwarding
  permanently disabled, or hang every future `AttachSubscriber` call for it).
  - *Given* a fake `SendInputViaControlMode` that returns an error, *When*
    `ForwardScroll` is called (and returns its non-nil `err` per Task 1.3.1c's
    error-path contract), *Then* a subsequent `ForwardScroll` call for the
    same `Instance` is not immediately rejected as lease-contended (i.e.
    `scrollLease.release()` fired), and a subsequent `hub.AttachSubscriber`
    call for the same hub returns without blocking (i.e. the attach barrier's
    `release()` fired).
  - *Given* the same setup but the error instead comes from a fake
    `waitForRedrawQuiescence` poll function (mid-quiescence failure, not a
    PTY-write failure), *When* `ForwardScroll` is called, *Then* the same two
    assertions hold — the lease and barrier release regardless of which of
    the two error points (send vs. capture) failed.
**Files**: `session/instance_scroll_forward.go` (new),
`session/instance_scroll_forward_test.go` (new), `session/scroll_lease.go`
(new), `session/streamhub/hub.go` (extend), `session/streamhub/hub_test.go`
(extend)

##### Task 1.3.1a: `ScrollLease` (~4 min)
- New file `session/scroll_lease.go`: `type scrollLease struct{ held
  atomic.Bool; lastCaptured []byte }` as an unexported field added to
  `Instance` (not snapshot-tracked, per the manager/dependency exclusion —
  it's transient orchestration state, not observable session state).
  `tryAcquire()`/`release()` via `CompareAndSwap`.
- Files: `session/scroll_lease.go`, `session/instance.go` (new field)

##### Task 1.3.1b: `ScrollForwardAttachBarrier` on `StreamHub` (~5 min)
- Add a new unexported `scrollForwardMu sync.Mutex` field to `StreamHub`
  (`session/streamhub/hub.go`), separate from the existing `h.mu` (so it's
  held only across the rare, multi-hundred-ms forward call, never across the
  hot broadcast/attach path in the common case).
- Add `func (h *StreamHub) BeginScrollForward() (release func())` —
  `h.scrollForwardMu.Lock()`, returns `h.scrollForwardMu.Unlock`.
- In `AttachSubscriber` (`hub.go:295`), acquire and immediately release
  `h.scrollForwardMu` as the very first statement, *before* the existing
  `h.mu.Lock()` critical section at line 299 — this makes a concurrent
  `AttachSubscriber` call wait for any in-flight `BeginScrollForward` holder
  to release before proceeding to `h.mu.Lock()`/`sendCatchUpSnapshot`,
  closing the window where a new subscriber's catch-up snapshot (or a
  subsequent broadcast) could be the mid-scroll pane content. Verified by
  reading `AttachSubscriber`'s actual body (`hub.go:295-336`) during this
  planning pass — it is a single locked critical section with no other entry
  point, so gating its start is sufficient; no other method needs the same
  treatment.
- This is deliberately *not* the previously-rejected "shield other clients'
  live view" mechanism (a new per-subscriber paused state in
  `session/streamhub/subscriber.go`) — it adds one mutex to `hub.go` and
  changes nothing about existing subscribers' delivery path; it only delays
  a *new* attach for the bounded duration of one forward.
- Files: `session/streamhub/hub.go`

##### Task 1.3.1c: `Instance.ForwardScroll` orchestration (~6 min)
- New file `session/instance_scroll_forward.go`:
  `func (i *Instance) ForwardScroll(ctx context.Context, subscriberCount int,
  hub *streamhub.StreamHub) (outcome ScrollForwardOutcome, blockedReason
  ScrollBlockedReason, content []byte, err error)` — checks `AppScrollGate`
  first: a gate failure whose reason string is `"unsupported streaming path"`
  (i.e. `subscriberCount < 0`) returns `(Blocked, UnsupportedStreamingPath,
  nil, nil)`; a gate failure whose reason is `"multiple viewers connected"`
  returns `(Blocked, MultipleViewers, nil, nil)`; any other gate failure
  (capability/alt-screen/status) returns `(Blocked,
  ScrollBlockedReasonUnspecified, nil, nil)` — the client never surfaces
  those non-viewer gate failures as `Blocked` in practice (Epic 1.4 only
  attempts the request when the client already believes forwarding is
  eligible), but the zero value keeps the mapping total rather than panicking
  on an unmapped reason string. Then tries `scrollLease.tryAcquire()` —
  failure returns `(Blocked, LeaseContention, nil, nil)`. On to the send/
  capture path: calls `hub.BeginScrollForward()` (`hub` is `nil` on
  `PathLegacyPerConnection`, which is already unconditionally `Blocked` by
  `AppScrollGate` per Risk Control, so this call is skipped there — no nil
  check needed on the happy path), defers both the barrier's `release()` and
  `scrollLease.release()` (barrier releases *last*, i.e. its `defer` is
  registered first, so a newly-unblocked `AttachSubscriber` never races the
  lease's own cleanup), resolves the adapter, sends every element of
  `KeySequences(ScrollUp)` as its own separate `SendInputViaControlMode` call
  in order (reusing the existing input path per `research/architecture.md`
  §4 — no new PTY-write plumbing), then branches on `CaptureVia()`:
  `RedrawQuiescenceCapture` calls `waitForRedrawQuiescence`/
  `CapturePaneContentPriority` (Task 1.3.2); `NativeScrollbackDelegateCapture`
  skips quiescence entirely and calls the existing
  `i.GetScrollbackHistory(startLine, endLine)`, wrapping its result instead
  of a fresh pane capture.
- **Error path (adversarial-review Concern: previously unspecified)**: if any
  `SendInputViaControlMode` call in the ordered sequence returns an error
  (PTY write failure — e.g. the underlying tmux pane died mid-call), or the
  subsequent capture call itself errors (`waitForRedrawQuiescence`'s poll
  function — see Story 1.3.2's added poll-error case — or
  `GetScrollbackHistory` for the delegate-capture mode), `ForwardScroll`
  returns immediately with a non-nil `err` and every other return value at
  its zero value — the call site (Task 1.3.3b) must treat a non-nil `error`
  return as a distinct case from every `ScrollForwardOutcome`, not squash it
  into `Blocked`. The two deferred
  releases (`scrollLease.release()`, `hub`'s barrier release) still run via
  `defer`, so a mid-forward error never leaves the lease held or the attach
  barrier locked. Client-visible behavior: the call site logs the error
  (existing `log.Error` convention) and falls through to the unchanged
  tmux-native `GetScrollbackHistory`/`ScrollbackResponse` path for that one
  request — the same silent-degrade-to-baseline behavior `NO_CAPABILITY`
  already uses client-side (Story 1.4.4), so a transient PTY error produces
  "scrolling behaves like today," not a new error surface the client has to
  handle. This does not apply to `waitForRedrawQuiescence`'s own
  `deadlineExceeded` case (Story 1.3.2) — that's an honest partial success,
  not an error, and still returns a real `ScrollForwardOutcome`.
- Files: `session/instance_scroll_forward.go`

##### Task 1.3.1d: Unit tests for lease contention + gate short-circuit +
attach barrier + mid-forward error release (~8 min)
- Cover the three Given-When-Then cases above using a fake `ScrollAdapter`, a
  mock/stub `SendInputViaControlMode` call counter, and a real `StreamHub`
  (from `session/streamhub`'s existing test helpers) for the attach-barrier
  case.
- **Mid-forward error release (pre-mortem P1 #3)**: add the two additional
  Given-When-Then cases from Story 1.3.1's Acceptance Criteria above — a fake
  `SendInputViaControlMode` erroring, and a fake `waitForRedrawQuiescence`
  poll erroring — each followed by a second `ForwardScroll` call (must not
  see `LeaseContention`) and a concurrent `AttachSubscriber` call on the same
  hub (must not block). This is a regression test, not a code-reading
  argument: it forces the actual `defer` chain in Task 1.3.1c to run under
  an error and asserts both cleanups fired, rather than asserting the code
  merely reads as correct.
- Files: `session/instance_scroll_forward_test.go`,
  `session/streamhub/hub_test.go`

---

#### Story 1.3.2: `RedrawQuiescence` capture (`RedrawQuiescenceCapture` mode
only)
**As a** `ForwardScroll` caller whose adapter declares
`RedrawQuiescenceCapture`, **I want** to wait for the app's redraw to settle
before capturing, **so that** the captured "page" isn't a partial or
mid-repaint frame. (This story does not apply to a `NativeScrollbackDelegateCapture`
adapter — see Task 1.3.1c — which defers capture to the existing
`GetScrollbackHistory` path instead.)
**Acceptance Criteria**:
- Capture happens only after two consecutive pane snapshots are
  byte-identical, or a deadline elapses.
  - *Given* a fake pane-content source that returns `"partial"` on the first
    two polls and `"settled"` on the third and fourth, *When*
    `waitForRedrawQuiescence` is called with a 200ms poll interval and 2s
    deadline, *Then* it returns `"settled"` after the third+fourth poll pair
    matches, without waiting for the full deadline.
  - *Given* a fake source that never stabilizes, *When*
    `waitForRedrawQuiescence` is called with a 500ms deadline, *Then* it
    returns the last-seen content and a `deadlineExceeded` flag, not an
    error (a partial-but-honest capture beats no capture).
  - *Given* a fake pane-content source whose poll function returns an error
    on its first call (adversarial-review Concern: previously unspecified —
    Task 1.3.3b's branch only defined behavior for a gated attempt, not one
    that fails partway through), *When* `waitForRedrawQuiescence` is called,
    *Then* it returns immediately with that error (not retried, not treated
    as `deadlineExceeded`), which `ForwardScroll` propagates per its
    Task 1.3.1c error-path contract above.
**Files**: `session/instance_scroll_forward.go` (extend),
`session/instance_scroll_forward_test.go` (extend)

##### Task 1.3.2a: Implement `waitForRedrawQuiescence` (~5 min)
- Add to `session/instance_scroll_forward.go`. This is a *new, independently
  stated* poll-and-compare with its own state — modeled directly on
  `waitForPaneSettle`'s (`session/autonomous_driver.go:449`) shape, not
  literal shared state with either `waitForPaneSettle` or
  `connectrpc_websocket.go`'s `resizeSettling *atomic.Bool` (a concurrent
  resize and a concurrent scroll-forward must not corrupt each other's
  quiescence state). Parameterized by a `paneContent func() (string, error)`
  closure (reuses `CapturePaneContentPriority`, `session/instance_tmux.go:942`)
  instead of `autonomous_driver.go`'s own checker interface, since this call
  site doesn't need the rest of `paneSettleChecker`. If `paneContent()`
  itself returns an error on any poll, `waitForRedrawQuiescence` returns
  immediately with that error rather than treating it as a non-stabilizing
  poll result (see Story 1.3.2's added error-case acceptance criterion).
- Files: `session/instance_scroll_forward.go`

##### Task 1.3.2b: Wire into `ForwardScroll` + tests (~4 min)
- After sending the gesture, call `waitForRedrawQuiescence`; compare the
  settled content against `scrollLease.lastCaptured` to compute
  `ScrollForwardOutcome` (`AtTop` if byte-identical, `Delivered` otherwise);
  store the new content as `lastCaptured` for the next call in this lease's
  lifetime.
- Files: `session/instance_scroll_forward.go`,
  `session/instance_scroll_forward_test.go`

---

#### Story 1.3.3: `AppScrollbackResponse` proto + call-site wiring
**As a** client, **I want** a response message that can't be confused with a
routine resync or the existing sequence-numbered scrollback contract,
**so that** `TerminalStreamManager.ts`'s prefix-sniffing dispatch routes it
correctly (per `research/architecture.md` §6).
**Acceptance Criteria**:
- The new message round-trips through `TerminalData`'s oneof without
  colliding with an existing field number.
  - *Given* `TerminalData`'s oneof currently reserves fields 2-7, 9-11, 16,
    18-19 (verified against `proto/session/v1/events.proto` at plan time),
    *When* `app_scrollback_response` is added, *Then* it is assigned field
    `20` and `make proto-gen` succeeds with no field-collision error.
- Both call sites branch to the new path only when the gate would pass.
  - *Given* a `ScrollbackRequest` arrives for a session where
    `AppScrollGate` returns `true`, *When* `handleScrollbackRequest` (or its
    shell-tab analog) processes it, *Then* it calls `Instance.ForwardScroll`
    and sends an `AppScrollbackResponse`, not a `ScrollbackResponse`.
  - *Given* a `ScrollbackRequest` arrives for a plain-shell session
    (`AppScrollGate` returns `false`, no capability), *When* the same handler
    processes it, *Then* it falls through to the existing
    `GetScrollbackHistory`/`ScrollbackResponse` path unchanged.
**Files**: `proto/session/v1/events.proto`,
`server/services/connectrpc_websocket.go`

##### Task 1.3.3a: Add `AppScrollbackResponse` + `ScrollForwardOutcome` +
`ScrollBlockedReason` enums to proto (~5 min)
- Add `enum ScrollForwardOutcome { SCROLL_FORWARD_OUTCOME_UNSPECIFIED = 0;
  DELIVERED = 1; AT_TOP = 2; NO_CAPABILITY = 3; BLOCKED = 4; }`,
  `enum ScrollBlockedReason { SCROLL_BLOCKED_REASON_UNSPECIFIED = 0;
  MULTIPLE_VIEWERS = 1; LEASE_CONTENTION = 2; UNSUPPORTED_STREAMING_PATH = 3;
  }` (adversarial-review BLOCKER fix — see Risk Control's "Concrete
  resolution" section above: distinguishes a real 2+ subscriber block from
  the unconditional `PathLegacyPerConnection` block, which the plan's
  previous single-reason `Blocked` couldn't represent), and
  `message AppScrollbackResponse { bytes content = 1; ScrollForwardOutcome
  outcome = 2; string program = 3; string forward_id = 4; ScrollBlockedReason
  blocked_reason = 5; }` (`blocked_reason` is meaningful only when
  `outcome == BLOCKED`; `SCROLL_BLOCKED_REASON_UNSPECIFIED` for every other
  outcome) to `proto/session/v1/events.proto`, and add `AppScrollbackResponse
  app_scrollback_response = 20;` to `TerminalData`'s oneof.
- Run `make proto-gen` (per `session/ent/generate.go`-adjacent convention —
  do not hand-edit generated output; `gen/` is gitignored).
- Files: `proto/session/v1/events.proto`

##### Task 1.3.3b: Widen `onScrollbackRequest`'s return type + branch inside
it (~5 min)
- `handleScrollbackRequest` (`connectrpc_websocket.go:3116`) itself has no
  `instance` in scope — only `stream`, `sessionID`, `scrollbackReq`, and the
  `onScrollbackRequest` closure do; `instance` is closed over only at the two
  places the closure is *defined* (:1343, :2053). So the gate check and
  `ForwardScroll` call must live **inside** those two closures, not inside
  `handleScrollbackRequest` itself — widen `onScrollbackRequest`'s type from
  `func(startLine, endLine string) (string, error)` to `func(startLine,
  endLine string) (ScrollbackResult, error)`, where
  `type ScrollbackResult struct { Content string; AppScroll
  *AppScrollResult }` and `type AppScrollResult struct { Content []byte;
  Outcome ScrollForwardOutcome; BlockedReason ScrollBlockedReason; Program
  string; ForwardID string }` (new small types in `connectrpc_websocket.go`,
  `AppScroll` nil = unchanged tmux-native path). Each closure creates its own
  bounded context internally (mirroring the existing `onInput` closure's
  `context.WithTimeout(context.Background(), 2*time.Second)` pattern at
  line 2035 — no new `ctx` parameter needed on `handleScrollbackRequest`),
  checks the flag (Task 1.5.1c) + `AppScrollGate`, and either calls
  `instance.ForwardScroll(ctx, subscriberCount, hub)` and returns a populated
  `AppScroll` (propagating `ForwardScroll`'s `outcome`/`blockedReason`
  straight into `AppScrollResult.Outcome`/`BlockedReason`, which the response
  builder then copies onto the proto's `outcome`/`blocked_reason` fields
  unchanged), or falls through to the existing
  `instance.GetScrollbackHistory(startLine, endLine)` call unchanged. The two
  closures pass different values for `hub` (Task 1.3.1c): `streamViaHub`'s
  closure (:2053) already has a `hub` local in scope (used by its `onResize`
  at :2049) and passes it directly; `streamViaControlMode`'s closure (:1343)
  has no `hub` (it's the `PathLegacyPerConnection` path) and passes `nil` for
  it, and passes a fixed `subscriberCount: -1` sentinel (never equal to 1, so
  `AppScrollGate`'s check (4) always fails — see Risk Control section and
  Task 1.1.3a) rather than deriving a count from `activeControlModeStreams`.
  `ForwardScroll` therefore returns `Blocked` before ever touching `hub`, so
  the `nil` is harmless.
- Update `handleScrollbackRequest`'s body to branch on `result.AppScroll !=
  nil`: build and send `AppScrollbackResponse` when set, otherwise keep the
  existing `buildScrollbackResponse`/`ScrollbackResponse` path verbatim.
- Files: `server/services/connectrpc_websocket.go`

##### Task 1.3.3c: Same widening in `handleShellScrollbackRequest` (~4 min)
- `handleShellScrollbackRequest` (`connectrpc_websocket.go:2415`) is a
  structurally separate function (its own `shellStreamParams`, not routed
  through `handleScrollbackRequest`) — apply the identical
  `ScrollbackResult`-widening + branch to its own `onScrollbackRequest`-style
  field and body. This is the second of the two call sites that need
  updating (the shared `handleScrollbackRequest` covers both
  `streamViaControlMode` and `streamViaHub` in one edit per Task 1.3.3b,
  since both wire the same shared function; this task covers the shell-tab
  analog separately).
- Files: `server/services/connectrpc_websocket.go`

##### Task 1.3.3d: Integration test for both branches (~5 min)
- Table test with two cases (app-forwarded path taken; tmux-native path
  taken, unchanged) against a fake `Instance`/gate, asserting the correct
  response message type is sent on the mock stream.
- Files: `server/services/connectrpc_websocket_test.go`

---

### Epic 1.4: Client-side rendering + indicator
**Goal**: An alt-screen-aware client actually *sends* a scroll-up request in
the case this project exists to fix (Story 1.4.0), shows a loading affordance
for the wait (Story 1.4.5), and `AppScrollbackResponse` renders distinctly
from a live resync once one arrives, with a visible source indicator and
honest treatment of the three non-`Delivered` outcomes.

#### Story 1.4.0: Client-side alt-screen scroll-up trigger
**As a** user viewing an alt-screen agent-CLI session, **I want** a
scroll-up gesture (mouse wheel or touch swipe) to request older content even
though xterm's own scrollback buffer is empty, **so that** scrolling up
actually does something instead of the silent no-op this project exists to
fix — on desktop *and* mobile (architecture-review BLOCKER: the plan
previously specified a server pipeline and a response handler with no
defined client-side condition that ever sends the request for the alt-screen
case; the existing `.xterm-viewport` `scroll` listener depends on
`terminal.buffer.active.viewportY`, which never moves for an alt-screen pane
since nothing is ever written to xterm's own scrollback,
`research/architecture.md` §3).
**Decisions this story makes** (all three required by the review; the third
was added in re-review after Task 1.4.0c's first draft was found to conflict
with existing client gesture handling):
1. **Reuses the existing `ScrollbackRequest`/`requestScrollback()` call**,
   not a new request message — Story 1.3.3's server-side branch already
   decides tmux-native vs. app-forwarded based on `AppScrollGate`, not on
   anything in the request payload, so the client only needs a new *trigger
   condition* for calling the same function, not a new wire shape.
2. **Alt-screen state reaches the client by local detection, not a
   server round-trip.** `AltScreenActive` (Story 1.1.1) is deliberately
   server-only (Domain Glossary) — round-tripping it over the wire for a
   purely local trigger decision would add a request/response hop before the
   client can even decide to fire. Instead, extend
   `TerminalStreamManager.ts`'s existing `\x1b[?1049h`/`\x1b[?1049l`
   substring scan (today only checked one-way, for the exit/`needsRefresh`
   case at `TerminalStreamManager.ts:370`) into a small persisted
   `altScreenActive` boolean plus an `onAltScreenChange` callback — the same
   escape sequences the server's `AltScreenTracker` (Story 1.1.1) scans for,
   applied to the same byte stream the client already receives, so the two
   trackers can't drift on *what* they match, only observe it independently
   (client-local vs. server-local), which is correct here since each side
   needs its own copy for a different purpose (client: trigger; server:
   safety gate).
3. **The touch trigger integrates into `useTerminalGestures`'s existing
   gesture state machine (`web-app/src/lib/hooks/useTerminalGestures.ts`)
   instead of adding a second `touchstart`/`touchmove` listener pair on
   `scrollEl`.** That hook already owns single-finger vertical-drag
   recognition on the terminal container via its `SCROLLING` state
   (`useTerminalGestures.ts:253-256`), built specifically to eliminate a
   documented double-`touchmove`-handler conflict (its own header comment
   cites R4.3/ADR-012, `useTerminalGestures.ts:9-15`). `scrollEl`
   (`.xterm-viewport`) is a descendant of the container `useTerminalGestures`
   listens on, so a second independent `touchstart`/`touchmove` pair there
   would reintroduce exactly that class of bug for the alt-screen case
   (architecture-review iteration-2 BLOCKER — the earlier draft's citation of
   `XtermTerminal.tsx:258`'s comment as scoping precedent was a misread: that
   comment documents `useTerminalGestures` itself, not an independent
   precedent for a third handler). Task 1.4.0c instead adds an optional
   `onAltScreenScrollUp` callback + `isAltScreenActive` accessor to the
   hook's options, consulted only inside the existing `SCROLLING` state's
   per-frame throttled callback.
**Acceptance Criteria**:
- A wheel scroll-up gesture while alt-screen-active calls `requestScrollback`
  even when `viewportY` is 0 and there is no native scrollback.
  - *Given* `TerminalStreamManager`'s `altScreenActive` is `true`, *When* the
    user generates a `wheel` event with negative `deltaY` (scroll up) over
    the terminal viewport, *Then* `requestScrollback` is called (subject to
    the same in-flight/`hasMoreScrollbackRef` guards the existing tmux-native
    trigger already uses at `TerminalOutput.tsx:1050-1057`), regardless of
    `terminal.buffer.active.viewportY`'s value.
  - *Given* `altScreenActive` is `false` (plain shell), *When* the same
    `wheel` event fires, *Then* the existing `viewportY`-gated listener's
    behavior is unchanged — this story adds a second trigger path, it does
    not replace or alter the first.
- A touch drag gesture does the same, by extending `useTerminalGestures`'s
  existing `SCROLLING` state (`useTerminalGestures.ts:253-256`) rather than
  adding a second touch listener pair — see Decision 3.
  - *Given* the terminal container, *When* a single-finger drag crosses
    `useTerminalGestures`'s existing 15px `PENDING`→`SCROLLING` threshold
    (`useTerminalGestures.ts:228`) in the downward (content-scroll-up)
    direction, *and* `isAltScreenActive()` returns `true`, *Then* the
    `SCROLLING` state's per-frame throttled callback calls
    `onAltScreenScrollUp` instead of `terminal.scrollLines()`, subject to the
    same `isFetchingScrollbackRef`/`hasMoreScrollbackRef`/`isConnected`
    guards Task 1.4.0b's wheel trigger already applies (the guards live
    inside the callback `TerminalOutput.tsx` passes down, not inside the
    hook itself).
  - *Given* the same drag but `isAltScreenActive()` is `false` (plain shell,
    native scrollback), *When* `SCROLLING`'s throttled callback fires, *Then*
    it calls `terminal.scrollLines()` exactly as it does today — this story
    adds a second *outcome* inside the existing state machine, not a second
    recognizer.
  - *Given* a touch sequence that instead promotes to `SELECTING` (long
    press) or resolves as `TAPPING`, *When* it moves, *Then*
    `onAltScreenScrollUp` is never called — those states are mutually
    exclusive with `SCROLLING` in the existing machine, so no new scoping
    logic against the selection-handle (`XtermTerminal.tsx:813-886`) or
    scrollbar-thumb (`XtermTerminal.tsx:932-1008`) touch handlers is needed
    beyond what the hook already provides.
**Files**: `web-app/src/lib/terminal/TerminalStreamManager.ts` (extend),
`web-app/src/lib/hooks/useTerminalGestures.ts` (extend),
`web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` (extend),
`web-app/src/components/sessions/XtermTerminal.tsx` (extend),
`web-app/src/components/sessions/TerminalOutput.tsx` (extend),
`web-app/src/components/sessions/TerminalOutput.test.tsx` (extend)

##### Task 1.4.0a: `altScreenActive` tracking + `onAltScreenChange` in
`TerminalStreamManager` (~5 min)
- Extend the existing substring checks at `TerminalStreamManager.ts:370` to
  also match `\x1b[?1049h`/`\x1b[?47h` (entry), add a private
  `altScreenActive: boolean` field updated on both entry and exit, and a
  `setOnAltScreenChange(cb: (active: boolean) => void)` setter mirroring
  `setOnFullSnapshot`'s shape (`TerminalStreamManager.ts:179`), called only
  when the value actually changes (mirrors the existing `changed` return
  convention from the server-side `AltScreenTracker.Observe`, Task 1.1.1a,
  kept consistent for anyone reading both).
- Files: `web-app/src/lib/terminal/TerminalStreamManager.ts`

##### Task 1.4.0b: Wheel listener (desktop) (~4 min)
- In the same `useEffect` block that attaches the existing `scroll` listener
  (`TerminalOutput.tsx:1037-1063`), add a `wheel` listener on `scrollEl`
  (`{ passive: true }`) that, when `altScreenActive` (from Task 1.4.0a's
  callback, held in a ref) is `true` and `event.deltaY < 0`, applies the same
  `isFetchingScrollbackRef`/`hasMoreScrollbackRef`/`isConnected` guards as
  the existing listener and calls `requestScrollback`. No conflict with
  existing handlers — nothing in `XtermTerminal.tsx` registers a `wheel`
  listener today (confirmed by grep: only `touchstart`/`touchmove`/`touchend`/
  `touchcancel` and DOM `scroll`).
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.4.0c: Alt-screen scroll-up via `useTerminalGestures`'s existing
`SCROLLING` state (mobile) (~6 min)
- Extend `UseTerminalGesturesOptions` (`useTerminalGestures.ts:30-37`) with
  two new optional fields: `isAltScreenActive?: () => boolean` and
  `onAltScreenScrollUp?: (lines: number) => void`, threaded through via the
  same stable-ref pattern already used for `onSendData`/`longPressMs`
  (`onSendDataRef`/`longPressMsRef`, `useTerminalGestures.ts:51-60`) so a
  changing callback identity doesn't retrigger the listener-attach
  `useEffect`.
- Inside the `SCROLLING` state's throttled callback set up in `onTouchMove`
  (`useTerminalGestures.ts:240-247`), after computing `lines`: if `lines < 0`
  (scroll-up direction, matching Task 1.4.0b's `deltaY < 0` desktop check)
  and `isAltScreenActiveRef.current?.()` is `true` and
  `onAltScreenScrollUpRef.current` is set, call
  `onAltScreenScrollUpRef.current(-lines)` and `return`, instead of falling
  through to `terminal.scrollLines(lines)`. Plain-shell scrolling
  (`isAltScreenActive` absent or `false`) and downward drags (`lines >= 0`)
  are unaffected — same branch, same call, unchanged behavior.
- Add matching `isAltScreenActive`/`onAltScreenScrollUp` props to
  `XtermTerminalProps` (`XtermTerminal.tsx:136-167`) and pass them into the
  existing `useTerminalGestures({...})` call (`XtermTerminal.tsx:262-266`).
  `TerminalOutput.tsx` supplies both: `isAltScreenActive` reads the same ref
  Task 1.4.0b's wheel listener already holds (Task 1.4.0a's
  `altScreenActive`); `onAltScreenScrollUp` is a closure applying the
  identical `isFetchingScrollbackRef`/`hasMoreScrollbackRef`/`isConnected`
  guards and calling `requestScrollback` — factor this guard-and-call body
  into one helper shared with Task 1.4.0b's wheel handler so the two trigger
  paths can't drift apart.
- No new listener is registered anywhere, and no `closest()`-based scoping
  against the selection-handle/scrollbar-thumb elements is needed: those are
  driven into `useTerminalGestures`'s `SELECTING` state (long press), which
  is already mutually exclusive with `SCROLLING` in the existing state
  machine — the exact conflict this task's earlier draft was trying to avoid
  by hand is already solved by the machine it now extends.
- Files: `web-app/src/lib/hooks/useTerminalGestures.ts`,
  `web-app/src/components/sessions/XtermTerminal.tsx`,
  `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.4.0d: Tests (~4 min)
- Cover both Acceptance-Criteria groups: wheel-trigger-fires,
  wheel-trigger-inert-when-not-alt-screen (`TerminalOutput.test.tsx`);
  `SCROLLING`-state fires `onAltScreenScrollUp` instead of `scrollLines` when
  `isAltScreenActive` is true, falls through to `scrollLines` when false or
  the drag is downward, and is never invoked from `SELECTING`/`TAPPING`
  (regression guard for the state machine's existing mutual exclusion —
  `useTerminalGestures.test.ts`).
- Files: `web-app/src/components/sessions/TerminalOutput.test.tsx`,
  `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts`

---

#### Story 1.4.5: Scroll-forward loading indicator (`design/ux.md` Surface 1)
**Sequencing note**: numbered 1.4.5 (not inserted as 1.4.1) to avoid
renumbering Stories 1.4.1-1.4.4, which `design/ux.md` and `validation.md`
already cross-reference by number — but it belongs in the same
implementation wave as Story 1.4.0-1.4.2, since 1.4.2's `ScrollSourceIndicator`
banner (and the `BLOCKED`/`AT_TOP` surfaces in 1.4.3/1.4.4) all replace this
story's loading pill once an outcome arrives; ship this story no later than
1.4.2. Placed here, immediately after 1.4.0, for reading order — trigger
fires (1.4.0) → loading shown (this story) → response dispatched (1.4.1) →
outcome rendered (1.4.2/1.4.3/1.4.4).
**Product Triad Review BLOCKER fix**: the original plan built Epic 1.4's
`DELIVERED`/`BLOCKED`/`AT_TOP` outcome-rendering stories (1.4.2-1.4.4) but
never built the in-progress/loading affordance itself, even though
`design/ux.md` Surface 1 specifies one in detail and `research/ux.md` §0
warns that the pre-existing tmux-native scrollback fetch already has *no*
loading indicator (`isFetchingScrollbackRef` is set but never read by any
render path) — shipping the new, strictly longer (up to `waitForRedrawQuiescence`'s
2s deadline, occasionally 8s+ before a Cancel path appears) forwarded-scroll
wait with zero loading affordance would make that pre-existing gap actively
worse, for the most common-path (`DELIVERED`) outcome specifically.
**As a** user who has just scrolled up, **I want** a loading indicator during
the wait for a forwarded (or tmux-native) scrollback fetch, **so that** the
wait never reads as the same silent no-op this whole feature exists to fix.
**Acceptance Criteria** (per `design/ux.md` Surface 1's interaction table):
- The pill appears only after a 150ms delay, not instantly — avoids a
  flash-of-spinner for the common fast-settling case.
  - *Given* a scroll-up request (tmux-native or app-forwarded) that resolves
    in under 150ms, *When* the response arrives, *Then* no loading pill is
    ever rendered.
  - *Given* a scroll-up request still pending at the 150ms mark, *When* that
    mark passes with no response, *Then* a pill renders with `role="status"`
    + `aria-live="polite"`, reusing `reconnectingBanner`'s exact shape
    (absolutely positioned, top-centered, `background: vars.color.modalBackground`,
    `color: vars.color.textPrimary`) — text `Loading <Program>'s history…`
    for the app-forwarded path (`<Program>` from `AppScrollbackResponse.program`)
    or `Loading more…` for the unchanged tmux-native path.
- After 8s total with no response, the pill offers a Cancel action
  (backs UX-AC-5 — "no state in this design leaves the user staring at an
  indefinite spinner with no way out").
  - *Given* the pill has been showing for 8s with still no server response
    (network stall, not an app-level `deadlineExceeded` result — that case
    already returns a real outcome per Story 1.3.2), *When* the 8s mark
    passes, *Then* pill copy changes to `Still trying to load <Program>'s
    history…` and a keyboard-focusable Cancel button appears.
  - *Given* the stalled pill with its Cancel button visible, *When* the user
    activates Cancel (click or Enter/Space), *Then* the pill clears, the
    client's fetch-in-flight state (`isFetchingScrollbackRef`-equivalent) is
    reset, and the view returns to its pre-scroll state without waiting for
    the request further — the in-flight server request itself is not
    cancelled (per `design/ux.md`'s edge case: nothing to gain from racing a
    cancel against a request already in the pipe; a late response is simply
    ignored if the pill/fetch-state has already been reset).
- The pill never persists past outcome delivery, for any outcome.
  - *Given* any response arrives — `AppScrollbackResponse` with
    `outcome ∈ {DELIVERED, AT_TOP, NO_CAPABILITY, BLOCKED}`, or the unchanged
    tmux-native `ScrollbackResponse` — *When* `TerminalOutput` handles it,
    *Then* the loading pill (and its 150ms/8s timers) is unconditionally
    cleared before the outcome-specific surface (banner, toast, "no more
    history" line, or silent no-op) renders.
**Files**: `web-app/src/components/sessions/ScrollLoadingPill.tsx` (new),
`web-app/src/components/sessions/ScrollLoadingPill.test.tsx` (new),
`web-app/src/components/sessions/TerminalOutput.tsx`,
`web-app/src/components/sessions/TerminalOutput.test.tsx`

##### Task 1.4.5a: `ScrollLoadingPill` component (~4 min)
- New file: presentational component, props `{ visible: boolean, stalled:
  boolean, program?: string, onCancel: () => void }`. Renders `null` when
  `!visible`; otherwise the `reconnectingBanner`-shaped pill described above,
  `role="status"` + `aria-live="polite"`, with the Cancel button (rendered
  only when `stalled`) following the existing `Scroll to bottom` button's
  keyboard-focusability precedent (`XtermTerminal.tsx:1573-1578`).
- Files: `web-app/src/components/sessions/ScrollLoadingPill.tsx`

##### Task 1.4.5b: Wire the 150ms show-delay + 8s stalled timer + Cancel into
`TerminalOutput.tsx` (~5 min)
- At the same call site(s) that invoke `requestScrollback` (the existing
  tmux-native trigger and Story 1.4.0's new alt-screen wheel/touch triggers
  both funnel through this one function), start a 150ms `setTimeout` that
  flips local `pillVisible` state true if the request is still in flight, and
  an 8s `setTimeout` (from the same request-start instant) that flips
  `pillStalled` true. The Cancel handler clears both timers, resets
  `pillVisible`/`pillStalled`, and resets the same fetch-in-flight guard
  (`isFetchingScrollbackRef`-equivalent) Task 1.4.0's triggers already check
  before allowing a new request.
- Clear both timers and reset pill state unconditionally in every response
  handler this component already has: the existing tmux-native
  `ScrollbackResponse` handler, and Task 1.4.1a's new `onAppScrollback`
  callback (for all four `ScrollForwardOutcome` values) — a single shared
  `clearScrollLoadingState()` helper used by both, so the two paths can't
  drift on when the pill gets cleared.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.4.5c: Tests (~4 min)
- Cover every Given-When-Then above: sub-150ms no pill, pill appears at
  150ms, stalled copy + Cancel button at 8s, Cancel clears pill and resets
  fetch state without cancelling the in-flight request, and pill cleared
  unconditionally for each of `DELIVERED`/`AT_TOP`/`NO_CAPABILITY`/`BLOCKED`
  and the plain tmux-native response.
- Files: `web-app/src/components/sessions/TerminalOutput.test.tsx`,
  `web-app/src/components/sessions/ScrollLoadingPill.test.tsx`

---

#### Story 1.4.1: Dispatch `AppScrollbackResponse` to a new handler
**As a** `TerminalStreamManager`, **I want** a discriminator that routes an
app-scrollback frame away from `onFullSnapshot`, **so that** a concurrent
resize resync can't interleave with or clobber it (per
`research/architecture.md` §6).
**Acceptance Criteria**:
- An `AppScrollbackResponse` message is routed to a new `onAppScrollback`
  callback, never to `onFullSnapshot`.
  - *Given* a `TerminalStreamManager` instance with both `onFullSnapshot` and
    a new `onAppScrollback` callback registered, *When* a `TerminalData`
    frame with `app_scrollback_response` set (outcome `DELIVERED`, content
    `"...transcript..."`) arrives, *Then* `onAppScrollback` is called with
    the content and outcome, and `onFullSnapshot` is not called.
  - *Given* the same setup, *When* a normal `TerminalOutput` frame starting
    with `ANSI_SNAPSHOT_PREFIX` arrives immediately after, *Then*
    `onFullSnapshot` is called as before, unaffected by the prior
    app-scrollback frame.
**Files**: `web-app/src/lib/terminal/TerminalStreamManager.ts`,
`web-app/src/lib/terminal/TerminalStreamManager.test.ts`

##### Task 1.4.1a: Add `onAppScrollback` registration + dispatch (~5 min)
- Add `private onAppScrollback: ((content: string, outcome:
  ScrollForwardOutcome) => void) | null = null;` and a
  `setOnAppScrollback(cb)` setter, mirroring `setOnFullSnapshot`'s shape
  (`TerminalStreamManager.ts:179`). Dispatch on `app_scrollback_response`
  presence in the incoming `TerminalData`, checked before the existing
  `ANSI_SNAPSHOT_PREFIX` sniff (this message type never carries that prefix,
  so ordering is safe either way, but checking first keeps the two paths
  visibly separate in the code).
- Files: `web-app/src/lib/terminal/TerminalStreamManager.ts`

##### Task 1.4.1b: Tests for dispatch isolation (~4 min)
- Cover both Given-When-Then cases above.
- Files: `web-app/src/lib/terminal/TerminalStreamManager.test.ts`

---

#### Story 1.4.2: `ScrollSourceIndicator` banner
**As a** user who has scrolled into an agent CLI's own history, **I want** a
visible, accessible indicator saying so, **so that** I don't confuse a
forwarded page with the live session view (Risk Control decision above).
**Acceptance Criteria**:
- The banner appears exactly while a lease-holding scroll is active and
  disappears when it isn't.
  - *Given* `TerminalOutput` receives an `onAppScrollback` callback with
    `outcome=DELIVERED` and `program="claude"`, *When* the component
    re-renders, *Then* a `ScrollSourceIndicator` renders the text "Viewing
    Claude Code's own history" and an `aria-live="polite"` region announces
    it once.
  - *Given* the same component, *When* the user scrolls back down past the
    lease-release threshold (server sends a follow-up live frame with no
    `app_scrollback_response` set), *Then* the banner unmounts.
**Files**: `web-app/src/components/sessions/ScrollSourceIndicator.tsx` (new),
`web-app/src/components/sessions/ScrollSourceIndicator.test.tsx` (new),
`web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.4.2a: `ScrollSourceIndicator` component (~5 min)
- New file: a small presentational component taking `{ program: string,
  visible: boolean }`, rendering a themed banner (reusing existing style
  tokens per `web-app/src/styles`) plus a `visuallyHidden` +
  `aria-live="polite"` span for the announcement, matching
  `XtermTerminal.tsx:1325`'s existing pattern in this codebase.
- Files: `web-app/src/components/sessions/ScrollSourceIndicator.tsx`

##### Task 1.4.2b: Wire into `TerminalOutput.tsx` state (~5 min)
- Add `appScrollbackState` (`{ active: boolean, program: string }`) local
  state, set from Task 1.4.1a's `onAppScrollback` callback, cleared on the
  next non-app-scrollback frame. Render `<ScrollSourceIndicator>` above the
  terminal viewport when active.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.4.2c: Component + integration tests (~4 min)
- Snapshot/render test for the banner's visible+hidden states; integration
  test asserting the banner appears/disappears per the callback sequence.
- Files: `web-app/src/components/sessions/ScrollSourceIndicator.test.tsx`

---

#### Story 1.4.3: `Blocked` outcome UX (multi-client / lease contention /
unsupported streaming path)
**As a** user whose scroll-up was blocked, **I want** a clear, non-error
explanation that's actually true for *my* situation, **so that** I don't
think the feature is broken or get told about a second viewer who doesn't
exist.
**Acceptance Criteria** (adversarial-review BLOCKER fix: a single hardcoded
"another viewer is connected" toast is factually false for a solo user on
`PathLegacyPerConnection`, which is unconditionally `Blocked` regardless of
actual viewer count — see Risk Control's "Concrete resolution" section and
the new `ScrollBlockedReason` enum, Task 1.3.3a):
- A `Blocked` outcome with `blocked_reason=MULTIPLE_VIEWERS` renders the
  viewer-contention toast.
  - *Given* `onAppScrollback` fires with `outcome=BLOCKED,
    blocked_reason=MULTIPLE_VIEWERS`, *When* `TerminalOutput` handles it,
    *Then* it shows the toast "Can't browse Claude Code's history right now —
    another viewer is connected. Close other tabs or sessions viewing this
    session to enable it." (program-name-interpolated) instead of attempting
    to render `content` (which is empty for `Blocked`) — this is `design/ux.md`
    Surface 3's canonical copy verbatim (Task 1.4.3a's implementation must
    match this exact string, not the shorter form an earlier draft of this
    plan quoted).
- A `Blocked` outcome with `blocked_reason=UNSUPPORTED_STREAMING_PATH`
  renders a *different*, honest toast — never the "another viewer" copy.
  - *Given* `onAppScrollback` fires with `outcome=BLOCKED,
    blocked_reason=UNSUPPORTED_STREAMING_PATH`, *When* `TerminalOutput`
    handles it, *Then* it shows the toast "Scroll-forwarding isn't available
    for this session yet" — no viewer-count claim, since none is true here.
- A `Blocked` outcome with `blocked_reason=LEASE_CONTENTION` (this client's
  own rapid repeat scroll) renders a third, distinct toast.
  - *Given* `onAppScrollback` fires with `outcome=BLOCKED,
    blocked_reason=LEASE_CONTENTION`, *When* `TerminalOutput` handles it,
    *Then* it shows the toast "Still loading — try again in a moment" rather
    than either of the above, since neither "another viewer" nor "not
    supported yet" is true for this case.
- The `MULTIPLE_VIEWERS` toast is paired with a brief `ConnectionCountIndicator`
  pulse (Task 1.4.3c); the other two reasons are not.
  - *Given* `onAppScrollback` fires with `outcome=BLOCKED,
    blocked_reason=MULTIPLE_VIEWERS`, *When* the toast renders, *Then*
    `ConnectionCountIndicator` also receives a ~600ms `pulse`; the same check
    for `UNSUPPORTED_STREAMING_PATH`/`LEASE_CONTENTION` asserts no pulse
    fires.
**Files**: `web-app/src/components/sessions/TerminalOutput.tsx`,
`web-app/src/components/sessions/ConnectionCountIndicator.tsx`

##### Task 1.4.3a: Handle `BLOCKED` outcome branch, keyed on `blocked_reason`
(~5 min)
- Extend Task 1.4.2b's callback handling: `outcome === 'BLOCKED'` switches on
  `blocked_reason` to pick one of the three copy strings above, then shows
  the existing toast mechanism already used elsewhere in this component
  (reuse, don't invent a new one) instead of setting
  `appScrollbackState.active`. `blocked_reason` unset/`UNSPECIFIED` (a gate
  failure the client should never actually attempt a request for, per Epic
  1.1's eligibility check) falls back to the `MULTIPLE_VIEWERS` copy as the
  least-wrong default rather than showing no message.
  - **Deliberate P2 tradeoff, not an oversight**: pre-mortem.md's Failure #2
    recommended a generic "unavailable" string instead of this fallback;
    accepted as-is since `UNSPECIFIED` should be unreachable per the note
    above, not fixed.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.4.3b: Tests (~4 min)
- Assert the toast path fires (and the banner path doesn't) for a `BLOCKED`
  outcome, once per `blocked_reason` value, asserting the correct copy
  string is shown for each — regression guard for the adversarial-review
  BLOCKER that a solo legacy-path user must never see the "another viewer"
  message.
- Files: `web-app/src/components/sessions/TerminalOutput.test.tsx`

##### Task 1.4.3c: `ConnectionCountIndicator` pulse on `MULTIPLE_VIEWERS` (~3
min)
**Product Triad Review gap fix**: `design/ux.md` Surface 3 pairs the
`MULTIPLE_VIEWERS` toast with a brief `pulse` on the existing
`ConnectionCountIndicator` badge — "a lightweight visual link between 'here's
why' (toast) and 'here's the standing state that caused it' (badge)" — but
this had no implementation task.
- When Task 1.4.3a's toast renders specifically for
  `blocked_reason=MULTIPLE_VIEWERS` (not `UNSUPPORTED_STREAMING_PATH` or
  `LEASE_CONTENTION` — per `design/ux.md`, "the other two reasons have no
  connected-viewer state to point to"), also add a `pulse` CSS class to the
  existing `ConnectionCountIndicator` badge for ~600ms, reusing its existing
  `expanded`/tooltip visual language (no new component). The pulse is
  `aria-hidden="true"` — purely visual reinforcement, since the toast text
  already carries the full explanation (`design/ux.md`'s Accessibility
  summary table).
- Files: `web-app/src/components/sessions/ConnectionCountIndicator.tsx`,
  `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.4.3d: Test for the pulse pairing (~3 min)
- Assert the `pulse` class is applied to `ConnectionCountIndicator` when (and
  only when) a `MULTIPLE_VIEWERS` toast renders, and not for
  `UNSUPPORTED_STREAMING_PATH`/`LEASE_CONTENTION`.
- Files: `web-app/src/components/sessions/TerminalOutput.test.tsx`

---

#### Story 1.4.4: `AtTop` / `NoCapability` outcome UX
**As a** user who has reached the top of an app's own history, or whose
session's program has no known scroll mechanism, **I want** an honest,
distinct signal for each case, **so that** I don't mistake either for the
feature being broken.
**Scope correction (cross-artifact-consistency BLOCKER, resolved here)**: an
earlier draft of this story had Task 1.4.4b render the "no more history"
affordance identically whether reached via `AT_TOP` (this feature) or the
pre-existing tmux-native exhaustion case (`metadata.hasMore === false`) —
i.e. unconditionally, for every session, including ones with this feature's
flag off or running a non-Claude program. That directly contradicts
requirements.md's own Risk Control section ("the existing tmux-native
pagination path is unchanged and remains the fallback for every session this
feature doesn't cover") and crosses its Out-of-Scope line ("Rebuilding
tmux-native scrollback delivery... out of scope"). **Phase 1 of this project
scopes the affordance to the `AT_TOP` (app-forwarded) outcome only** — the
tmux-native exhaustion case's behavior (today: no affordance at all) is
unchanged by this project. Unifying the two into one shared "no more
history" component remains a good idea (`research/ux.md` §4a/b's
recommendation still holds on its merits), but it is out of scope for *this*
feature and is recorded as its own follow-up story below, explicitly outside
this feature's flag and scope, not bundled into Phase 1.
**Acceptance Criteria**:
- `AtTop` shows a "reached the top" affordance, scoped to the app-forwarded
  outcome only — it does not change behavior for the pre-existing
  tmux-native exhaustion case.
  - *Given* `onAppScrollback` fires with `outcome=AT_TOP`, *When*
    `TerminalOutput` handles it, *Then* it sets a new
    `hasMoreAppScrollbackRef` flag to `false` (not the pre-existing
    `hasMoreScrollbackRef`, which the tmux-native path alone continues to
    own) and shows a small "No more history available" affordance near the
    top of the viewport.
  - *Given* a plain-shell (or flag-off, or non-Claude) session whose
    tmux-native `GetScrollbackHistory` call returns `metadata.hasMore ===
    false`, *When* `TerminalOutput` handles that response, *Then* its
    behavior is byte-for-byte unchanged from today — no "No more history
    available" affordance renders, since this project does not touch that
    path in Phase 1.
- `NoCapability` is never actually reached at runtime for a known-registered
  adapter (Epic 1.1's gate returns `false`/no-attempt before any request is
  sent for an unregistered program), but the client still handles the enum
  value defensively.
  - *Given* a `ScrollForwardOutcome.NO_CAPABILITY` value (reserved for a
    future adapter registered server-side with `Capability()` explicitly
    marked unconfirmed), *When* the client receives it, *Then* it falls back
    to the pre-existing silent-no-op behavior (today's baseline, not a
    regression) rather than crashing on an unhandled enum case.
**Files**: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.4.4a: Handle `AT_TOP` and `NO_CAPABILITY` branches (~4 min)
- Extend the callback handling from Story 1.4.3 with the two remaining
  outcome branches per the acceptance criteria.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.4.4b: `AT_TOP`-only "reached the top" affordance (~5 min)
- Add a small "no more history" visual affordance (a static line, not a
  spinner), rendered only when `outcome=AT_TOP` sets the new
  `hasMoreAppScrollbackRef` flag from Story 1.4.4's Acceptance Criteria
  above. **Does not read or write `hasMoreScrollbackRef`** — that ref remains
  exclusively owned by the pre-existing tmux-native path, unmodified by this
  task, so a plain-shell/flag-off/non-Claude session's behavior cannot be
  affected by this change even indirectly through shared state.
- **Cross-reference for Story 1.4.0's trigger guards**: Tasks 1.4.0b/1.4.0c's
  wheel/touch trigger for alt-screen sessions cites "the same
  `isFetchingScrollbackRef`/`hasMoreScrollbackRef`/`isConnected` guards the
  existing tmux-native trigger already uses" — for an alt-screen session,
  that guard must consult `hasMoreAppScrollbackRef` (this task's new flag),
  not `hasMoreScrollbackRef` (which a pure alt-screen session's tmux-native
  path never touches and so would never flip false, silently defeating the
  guard). `isFetchingScrollbackRef`/`isConnected` are genuinely
  path-agnostic and stay shared as originally described.
- **Scope boundary (cross-artifact-consistency BLOCKER, resolved — see Story
  1.4.4's Scope correction above)**: this task deliberately does *not* touch
  `requirements.md`'s Out-of-Scope-named tmux-native pagination path at all
  — no shared component, no shared ref, no shared render branch. Unifying
  the two "no more history" affordances into one component is real, cheap
  future work (see the follow-up story immediately below), but it is not
  this task's job, and this task must not be implemented in a way that
  silently reintroduces the coupling by reusing `hasMoreScrollbackRef` for
  convenience.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Follow-up (out of scope for this feature): unify the two "no more history" affordances
`research/ux.md` §4a/b's recommendation — that the app-forwarded `AT_TOP`
affordance and the tmux-native exhaustion case should look and read
identically to the user — is worth doing, but as its own change against the
already-shipped tmux-native pagination path, scoped and reviewed on its own
terms rather than riding in as a side effect of this feature's flag. That
follow-up is not numbered as a Phase 1/2/3 epic here (per requirements.md's
own Out-of-Scope line, it isn't part of this project at all) — file it as a
separate backlog item once Task 1.4.4b ships, referencing this note and
`research/ux.md` §4b.

##### Task 1.4.4c: Tests for both branches (~4 min)
- Files: `web-app/src/components/sessions/TerminalOutput.test.tsx`

##### Task 1.4.4d: Verify `textMuted`-on-terminal-background contrast
(UX-AC-11) (~3 min)
**Product Triad Review gap fix**: `design/ux.md`'s Accessibility summary
flags the "No more history available" line's `color: vars.color.textMuted`
as a *new* usage context for that token — previously only used inside a pill
with its own background (e.g. `reconnectingBanner`), never directly against
the plain terminal background — and calls it out as "unverified pre-ship,"
but no task owned checking it.
- Before ship, verify `vars.color.textMuted` against the terminal
  background's actual rendered color meets ≥4.5:1 contrast in both light and
  dark theme (`web-app/src/styles/theme-contract.css.ts`'s token values).
  Reuses this repo's existing `@axe-core/playwright` check already wired for
  `web-app/src/` PR checks — `validation.md`'s
  `tests/e2e/scroll-forward-contrast.spec.ts` (UX-AC-11) already scopes an
  `AxeBuilder` pass to this line's container in both themes; this task is
  what makes that check meaningful rather than an unowned note. If contrast
  fails, swap to a token that passes (e.g. `textSecondary`) rather than
  shipping the unverified value.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx` (token swap if
  the check fails), `tests/e2e/scroll-forward-contrast.spec.ts` (already
  exists per `validation.md` — no new file)

---

### Epic 1.5: Observability + feature flag + canary
**Goal**: The feature ships gated, measurable, and self-detecting when its
core assumption (the keybinding) drifts — can proceed in parallel with Epic
1.3/1.4 once Epic 1.1 lands, since the flag/canary have no code dependency on
the response-framing work.

#### Story 1.5.1: Feature flag registration
**As an** operator, **I want** `terminal:app-scrollback-forwarding:claude` to
be a live-settable, off-by-default flag, **so that** the feature ships dark
and can be enabled/disabled without a deploy.
**Acceptance Criteria**:
- The flag defaults to off and is toggleable via the existing flag RPC.
  - *Given* a fresh `config.Config` with no `feature_flags` entries, *When*
    `cfg.GetFeatureFlag("terminal:app-scrollback-forwarding:claude")` is
    called, *Then* it returns `false`.
  - *Given* the flag is set to `true` via `UpdateFeatureFlag`, *When*
    `Instance.ForwardScroll`'s call site checks it, *Then* forwarding is
    attempted; when `false`, the existing tmux-native path is used
    unconditionally regardless of `AppScrollGate`'s verdict.
**Files**: `config/config.go`, `server/services/feature_flag_service.go`

##### Task 1.5.1a: Declare the flag constant (~3 min)
- Add `const FeatureAppScrollForwardingClaude =
  "terminal:app-scrollback-forwarding:claude"` to `config/config.go`,
  following `FeaturePiSupport`'s exact doc-comment shape
  (`config/config.go:1673-1676`), and add it to the `GetFeatureFlag` doc
  comment's recognized-flags list.
- Files: `config/config.go`

##### Task 1.5.1b: Register in `knownFeatureFlags` (~3 min)
- Add a `terminalAppScrollForwardingClaudeFlagName =
  config.FeatureAppScrollForwardingClaude` shared-constant + registry entry
  in `server/services/feature_flag_service.go`, mirroring
  `piSupportFlagName`'s pattern (`feature_flag_service.go:15-18`).
- Files: `server/services/feature_flag_service.go`

##### Task 1.5.1c: Gate the call site on the flag (~4 min)
- In Task 1.3.3b's branch, check `config.LoadConfig().GetFeatureFlag(...)`
  before calling `AppScrollGate` at all — flag-off short-circuits to the
  existing tmux-native path with zero new behavior, satisfying the
  Acceptance Criteria's second bullet.
- Files: `server/services/connectrpc_websocket.go`

##### Task 1.5.1d: Tests (~3 min)
- Cover flag-off (no gate call) and flag-on (gate evaluated) cases.
- Files: `server/services/connectrpc_websocket_test.go`

---

#### Story 1.5.2: `scrollForwardKeybindingCanary`
**As a** maintainer, **I want** a log line when a forwarded keystroke doesn't
produce a scroll-consistent redraw, **so that** a silent CLI-version
regression is discoverable instead of only reported by a confused user
(ADR-001).
**Acceptance Criteria**:
- The canary logs a warning exactly when the outcome is ambiguous (redraw
  happened but doesn't look like a scroll), not on every `AtTop`.
  - *Given* a forwarded gesture whose `RedrawQuiescence` capture changed but
    the change doesn't match any expected scroll-consistent pattern (e.g. the
    pane content is byte-identical except for a spinner/timestamp that
    updates on every redraw regardless of scroll — a known false-redraw
    signature), *When* `scrollForwardKeybindingCanary` runs, *Then* it logs a
    `log.Warn` with the adapter name and a truncated content diff, once per
    session (mirroring `compactingCanaryLogged`'s once-per-session guard,
    `session/detection/detector.go:126-128`).
**Files**: `session/detection/detector.go`,
`session/instance_scroll_forward.go`

##### Task 1.5.2a: Implement the canary check (~5 min)
- Add `scrollForwardKeybindingCanary(outcome ScrollForwardOutcome, before,
  after []byte) bool` (returns whether it fired) to
  `session/detection/detector.go`, following `compactingCanary`'s structure
  (`detector.go:361-413`) — a heuristic false-redraw check, not a strict
  proof, logged at `log.Warn` when it fires.
- Add a `scrollForwardCanaryLogged atomic.Bool` field mirroring
  `compactingCanaryLogged`.
- Files: `session/detection/detector.go`

##### Task 1.5.2b: Call the canary from `ForwardScroll` + test (~4 min)
- Call after `waitForRedrawQuiescence` resolves an ambiguous case; unit test
  asserting the once-per-session guard and the log line's content.
- Files: `session/instance_scroll_forward.go`,
  `session/detection/detector_test.go`

---

#### Story 1.5.3: Immediate Claude Code version-mismatch warning
**As a** maintainer, **I want** to know the moment a session's `claude`
binary no longer matches the version `GestureForwardStrategy`/
`NativeDumpFallbackStrategy` was confirmed against, **so that** a
low-traffic-session regression is still visible even though
`scrollForwardKeybindingCanary` (Story 1.5.2) alone can't surface it until
enough scroll attempts accumulate (pre-mortem P1 #1 — sparse per-session
scroll usage means the canary's 3+/hour alert threshold may never trip even
as the feature silently regresses to a no-op).
**Acceptance Criteria**:
- The check fires at most once per distinct `claude` binary path, not once
  per session or per scroll attempt (mirrors `versionCheckedSockets`'s
  memoization in `session/tmux/version_check.go` — re-checking on every
  session for a binary path that hasn't changed would be pure waste, same
  rationale that file's own doc comment gives).
  - *Given* `resolveScrollAdapter("claude")` returns a `ClaudeScrollAdapter`
    with `Capability().VerifiedAgainstVersion == "2.1.270"`, *When* a session
    starts against a `claude` binary at path `/usr/local/bin/claude` whose
    `claude --version` reports `"2.2.0"`, *Then*
    `scrollForwardVersionMismatchCheck` logs exactly one `log.Error` line
    (fields: binary path, expected version, actual version) the first time
    that path is checked, and logs nothing on a second session against the
    same binary path.
  - *Given* the same setup but `claude --version` reports `"2.1.270"`
    (matches), *When* the check runs, *Then* it logs nothing.
  - *Given* `claude --version` itself fails (binary not found, non-zero
    exit), *When* the check runs, *Then* it logs a `log.Warn` (not `Error`)
    and does not claim a mismatch — mirrors
    `checkControlModeVersionMatchOnce`'s treatment of its own client-version
    lookup failing (`session/tmux/version_check.go:70-73`): a lookup failure
    is a different, lower-severity condition than a confirmed mismatch, not
    the same thing reported the same way.
- This check is independent of `scrollForwardKeybindingCanary` — it is not
  gated on scroll volume, and firing (or not) for one has no bearing on
  whether the other fires for the same session.
  - *Given* a session that never sends a single scroll-up gesture,
    *When* that session starts against a mismatched `claude` binary,
    *Then* `scrollForwardVersionMismatchCheck` still fires — unlike
    `scrollForwardKeybindingCanary`, which requires at least one forwarded
    scroll attempt to have anything to evaluate.
**Files**: `session/claude_version_check.go` (new),
`session/claude_version_check_test.go` (new), `session/claude_scroll_adapter.go`

##### Task 1.5.3a: Implement `scrollForwardVersionMismatchCheck` (~5 min)
- New file `session/claude_version_check.go`, structured directly on
  `session/tmux/version_check.go`'s existing shape: a package-level
  `claudeVersionCheckedPaths sync.Map` (memoization key: the resolved
  `claude` binary's absolute path, mirroring `versionCheckedSockets`'s
  per-socket key), and
  `func scrollForwardVersionMismatchCheck(ctx context.Context, binaryPath,
  verifiedAgainstVersion string, runner CommandRunner)` that: skips (via
  `LoadOrStore`) if `binaryPath` was already checked; runs `<binaryPath>
  --version`; on a run error, `log.Warn`s and returns (lookup failure, not a
  confirmed mismatch — see acceptance criteria); on success, normalizes the
  output the same defensive way `normalizeTmuxVersion` does (trim
  whitespace; `claude --version`'s output format is confirmed in
  `research/stack.md` as a plain version string, no `"claude "` prefix to
  strip, so normalization here is narrower than `version_check.go`'s) and
  compares to `verifiedAgainstVersion`; `log.Error`s the mismatch case with
  both versions and the binary path, matching
  `checkControlModeVersionMatchOnce`'s field-naming convention
  (`client_version`/`server_version` → here, `expected_version`/
  `actual_version`).
- `CommandRunner` is a small interface (`Output(cmd) ([]byte, error)`) so the
  test suite can fake `claude --version` without a real binary — same
  pattern `TmuxSession.cmdExec` already uses for its own version check
  (`session/tmux/version_check.go:69`), reused rather than reinvented.
- Files: `session/claude_version_check.go`

##### Task 1.5.3b: Call the check once per session at adapter resolution (~4 min)
- Call `scrollForwardVersionMismatchCheck` from `ClaudeScrollAdapter`'s
  construction path (the same point Task 1.2.1b wires the confirmed
  `VerifiedAgainstVersion` in), passing the session's resolved `claude`
  binary path (already available wherever the session first resolves its
  launch command — reuse that value, do not re-derive it) and
  `Capability().VerifiedAgainstVersion`. This runs at most once per unique
  binary path regardless of how many sessions launch against it (Task
  1.5.3a's memoization), so it adds no meaningful per-session-start latency
  beyond the first.
- Files: `session/claude_scroll_adapter.go`

##### Task 1.5.3c: Tests (~4 min)
- Cover the three Given-When-Then cases above using a fake `CommandRunner`:
  mismatch (fires once, not twice for two sessions on the same path), match
  (silent), and lookup failure (`Warn`, not `Error`, no mismatch claimed).
- Files: `session/claude_version_check_test.go`

---

## Phase 2: pi adapter (droppable without reworking Phase 1)

Structurally, this phase adds one `ScrollAdapter` implementation and one
feature flag — it reuses every piece of Epic 1.1/1.3/1.4's scaffolding
unchanged, which is the whole point of the Strategy-pattern seam chosen in
Pattern Decisions. If time/feasibility don't allow this phase, Phase 1 ships
complete on its own.

### Epic 2.1: pi `ScrollAdapter`
**Goal**: pi sessions get the same forwarding behavior as Claude Code, using
pi's documented `tui.altScreen.pageUp`/`pageDown` keybindings.

#### Story 2.1.1: Confirm and implement `PiScrollAdapter`
**As a** pi user, **I want** scroll-up to forward into pi's transcript,
**so that** the same fix Claude Code got applies here too.
**Acceptance Criteria**:
- pi's default `pageUp` keybinding is confirmed against a live session before
  being hardcoded.
  - *Given* a manually-launched `pi --tui-mode fullscreen` session, *When*
    the spike sends the default `pageUp` key's byte sequence and captures
    before/after, *Then* the captured content differs in a way consistent
    with scrolled transcript (per `research/stack.md` §4's documented
    keybinding table), confirming `GestureForwardStrategy` applies directly
    with no fallback needed for pi.
  - *Given* the confirmed sequence, *When* `resolveScrollAdapter("pi")` is
    called, *Then* it returns a `PiScrollAdapter` whose `CanHandle` agrees
    with the existing `isPi` helper (`session/instance_tmux.go:162-183`) on
    the same drift-test-vector discipline as Task 1.1.2c.
**Files**: `session/pi_scroll_adapter.go` (new),
`session/pi_scroll_adapter_test.go` (new), `config/config.go`,
`server/services/feature_flag_service.go`

##### Task 2.1.1a: Spike pi's default keybinding (~5 min to record)
- Same shape as Task 1.2.1a, targeting pi instead of Claude Code.
- No file changes.

##### Task 2.1.1b: `PiScrollAdapter` + registration (~5 min)
- New file `session/pi_scroll_adapter.go`, `CanHandle` delegating to `isPi`
  directly (reuse, not reimplement — pi has no separate `HistoryAdapter`
  string-match to duplicate, per `research/features.md` §4's note that
  `pi_adapter.go` isn't a `HistoryAdapter` at all) plus a
  `GestureForwardStrategy` populated with the spike's confirmed byte
  sequence. Register in `resolveScrollAdapter`.
- Files: `session/pi_scroll_adapter.go`

##### Task 2.1.1c: `terminal:app-scrollback-forwarding:pi` flag (~4 min)
- Same shape as Story 1.5.1, new flag constant + registration.
- Files: `config/config.go`, `server/services/feature_flag_service.go`

##### Task 2.1.1d: Tests (~4 min)
- `CanHandle`/`isPi` drift test; flag-gating test.
- Files: `session/pi_scroll_adapter_test.go`

---

## Phase 3: agy adapter (research-gated, may not become an implementation
phase at all)

**Not started until the Unresolved Question above closes.** agy's specific
scroll-keybinding IDs were not located during Phase 2 research
(`research/stack.md` §3) — this phase begins with a research task, not an
implementation task, and produces either an Epic 3.1 identical in shape to
Epic 2.1, or a documented decision that agy stays out of scope if no
forwardable command is found (consistent with the Rabbit Holes section of
requirements.md: "if infeasible for one, that adapter should drop out of
scope rather than blocking the whole project").

### Epic 3.1 (provisional): agy `ScrollAdapter`
Blocked on: agy CLI Reference doc page or live `agy --sandbox` session's
`?`/help listing confirming specific alt-screen scroll-keybinding action IDs.
Once unblocked, this epic's shape mirrors Epic 2.1 exactly (spike →
`AgyScrollAdapter` → flag → tests), reusing the same `ScrollAdapter`
interface with no changes to Epic 1.1/1.3/1.4.
