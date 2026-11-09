# Implementation Plan: mobile-input-layer

**Feature**: Renderer-agnostic Android soft-keyboard input layer (hidden textarea + pure reducer), disconnected-input handling, server/client input observability, diagnostics overlay and compose bar, behind a live-settable kill-switch flag.
**Date**: 2026-11-06
**Status**: Ready for implementation (Phase 3 plan-repair applied; Phase 4 plan-repair applied 2026-11-08 from `pre-mortem.md`, `consistency-review.md`, `validation.md`; triad-review repair applied 2026-11-08 (UX, product and engineering gaps: Stories 0.1.5, 1.2.5, 1.4.3, 1.4.4, Tasks 0.1.3c, 1.1.4f/g, 3.1.5b, 4.1.2c, 4.2.2c, 5.1.1e, 5.1.2d, 5.1.4d/e); reviews: `adversarial-review.md`, `architecture-review.md`)
**ADRs**: [ADR-001](../decisions/ADR-001-renderer-agnostic-input-layer-no-new-deps.md), [ADR-002](../decisions/ADR-002-live-send-composition-bounded-atomic-rewrite.md), [ADR-003](../decisions/ADR-003-mobile-input-layer-flag-and-renderer-policy.md), [ADR-004](../decisions/ADR-004-disconnected-input-visible-drop-and-bounded-queue.md), [ADR-005](../decisions/ADR-005-server-input-observability.md), [ADR-006](../decisions/ADR-006-textarea-primary-editcontext-sub-flag.md), [ADR-007](../decisions/ADR-007-replay-corpus-test-strategy.md), [ADR-008](../decisions/ADR-008-secret-entry-limitation-and-fixture-privacy.md)

**Owner-visible behavior change (call out in the PR description of PR-B)**: input typed while the stream is disconnected is no longer silently dropped. Plain text only (no Enter, no control bytes) is held as atomic units (<= 4 KiB, <= 30 s) and replayed on reconnect; Enter, Esc, arrows and Ctrl typed offline are never queued and show "not sent"; a visible chip says what happened (ADR-004). It is behind sub-flag `mobile_input_queue`; flag off restores the old drop.

**Owner-visible risk (ADR-008)**: the layer keeps Gboard autocorrect and the suggestion strip ON and cannot detect echo-off prompts, so text typed at `sudo`/`ssh`/token prompts can be seen by the suggestion strip and learned into Gboard's personal dictionary. Mitigation is `autocomplete="off"` only; the limitation is named in the reference docs.

**Platform scope (v1)**: only the Android Chrome-class touch-primary device attaches the layer (`shouldAttachMobileInput()`; Chrome, Firefox and Samsung Internet on Android all match the Android UA test, and Chrome's "Request desktop site" drops Android from the UA, which opts that tab out; document both). iOS Safari and touch laptops with hardware keyboards keep today's behavior, including the generic coarse-pointer UI (link-tap menu, selection handles, gestures) via a separate `isTouchPrimary()`. The `mobile_input_layer` flag is a rollback switch (operator-facing, not a user setting).

**Flag default (PR-A1 description requirement; conditional)**: `mobile_input_layer` merges default ON for the Android touch-primary class ONLY IF the pre-merge phone check on the isolated manual instance passes (Task 5.1.4c, owner wall-clock blocker, pre-mortem P1-1); otherwise it merges default OFF with a per-scope override for the owner. It affects only the Android class (`shouldAttachMobileInput()` true); desktop and iOS are unchanged whatever the flag says. Rollback: flip the flag off in the feature-flag panel. The client reads the persisted value at preload, so it takes ONE page load for the off value to be persisted to `ssq:flag:` and a SECOND page load for it to apply from the panel; the immediate same-load escape is the `?mobile_input=0|1` query param (Task 1.3.2e), which persists per origin (`localStorage` is per origin, so `:8543`, `:8444`, the Tailscale host and the LAN IP each need it once). Flag off on the Android class shows a visible one-line hint, never a silent landing on weak xterm. Task 5.1.4b is the owner's post-redeploy acceptance check, not a default flip.

**Safe on a link flap from PR-A**: the layer never emits a DEL against text it sent before a transport boundary (disconnect, reconnect, `dropped`, thrown send, aborted chunked paste) or into a different session. This holds by construction: DELs are only computed from units emitted in the CURRENT delivery epoch, and `controller.invalidate(reason)` bumps the epoch or abandons the region at every boundary (Story 1.2.4, AC `no-DEL-across-boundary`). Residual, stated honestly: `sent` means handed to the transport, not delivered (a half-open socket can lose a frame silently), so after a boundary a lost frame can leave a truncated or missing word (insert-only, never an erased prompt). **PR-A has no visible drop notice** (the chip is PR-B): the user sees only the existing disconnected state (stream-status UI and the Reconnect button after 5 s, unchanged) and the text they typed while offline simply does not appear; the only new signal is the `cm.input{event=dropped_disconnected}` counter.

Root cause carried from requirements (UNVERIFIED on this worktree until Story 0.1.1): ghostty-web 0.4.0 ignores keydowns with `keyCode === 229` and `preventDefault()`s every `beforeinput`, so Android IME text never reaches `term.onData`.

---

## Delivery Slicing (first PR is the critical path)

"PR-A" below means PR-A1 and PR-A2 together unless a row says otherwise. The first PR (PR-A1) is shippable with ONLY the core layer, the minimal diagnostics, the flag gate and the pre-merge phone check; server and client observability (PR-A2) is independent and may merge before or after it. Everything else is a later PR behind its own sub-flag and is not a prerequisite for merging PR-A.

| PR | Contents | Flag | Depends on |
|---|---|---|---|
| Wave 0 (before any feature code) | Story 0.1.4: bun install, `make build`, jest and go smoke, `make ready` baseline on a stub branch, `.jscpd.json` fixture exclusion, per-worker worktree rules | none | none |
| PR-A1 (critical path, the minimum that makes the phone type) | Phase 0 spikes, Phase 1 (reducer, adapter, platform gate, focus ownership with a gesture-safe tap target, flag + `?mobile_input` query param, renderer wiring, Story 1.2.4 delivery ledger and invalidate with a stop rule, `useMobileInputChrome` shell incl. flag-off hint), Story 4.1.1 (content-free ring buffer and report, pulled forward from PR-C), Story 2.3.1 (minimal read-only `?inputdebug=1` panel with Copy report), Story 0.1.5 (Android keyboard-resize spike) and Task 0.1.3c (baseline), Story 1.2.5 (rewrite flicker budget, deferred-commit feedback), Story 1.4.3 (chrome stack budget and minimum terminal rows), Story 1.4.4 (live-region contract and TalkBack typed-character announcement), Stories 5.1.1 (a, b, d, e, flag-off part of c), 5.1.2 (reference page + how-to core), 5.1.3, Tasks 5.1.4c/5.1.4d/5.1.4e (pre-merge phone gate, TalkBack check, loss and latency re-measure) | `mobile_input_layer` (default ON only if Task 5.1.4c passes, else OFF + per-scope owner override; affects only the Android class) | Wave 0 |
| PR-A2 (independent of A1) | Phase 2: server `terminal_input_*` metrics, input-site instrumentation, actuator endpoint, `cm.input`/`cm.ime_anomaly` telemetry, dashboard row, alert rule, how-to diagnostic section | none (always on) | Wave 0 (A1 for the client telemetry hookup, Task 2.2.2b) |
| (post-redeploy) | Task 5.1.4b owner device acceptance on the deployed build, including remote-origin, PWA, plain-HTTP, hardware-keyboard and Ctrl-W rows; Task 5.1.2d timed "diagnosable in under a minute" drill (needs PR-A2 merged too) | n/a | PR-A1 merged and redeployed (5.1.2d also PR-A2) |
| PR-B | Phase 3 (chip, atomic-unit queue with strict FIFO, `onQueueDiscard` delivered reporting, chip states) and the disconnect e2e case | `mobile_input_queue` (+ requires layer); includes Task 3.1.5b (stuck-state copy) | PR-A1 (includes the hook shell and `SendResult` plumbing) |
| PR-C | Phase 4.1 remainder (Stories 4.1.2 full diagnostics sheet, 4.1.3 capture-for-fixture, device fixtures; Story 4.1.1 already shipped in A1) | `mobile_input_diagnostics`; includes Task 4.1.2c (settings entry) | PR-A1 (extends the chrome hook shell and the A1 ring buffer) |
| PR-D | Phase 4.2 (compose bar, fallback detector and notice) | `mobile_input_compose`; includes Task 4.2.2c (anomaly-based compose offer) | PR-A1 (chrome hook shell), Story 0.1.1 |
| PR-E | Phase 6 (EditContext, follow-up issues; Story 6.1.2 issue filing is not a PR and is unconditional) | `mobile_input_edit_context` | PR-C corpus |

**Stop rule for delivery-ledger complexity (pre-mortem P2-10)**: if two review rounds on Story 1.2.4 each produce new blockers in the ledger or invalidate logic, simplify to "abandon on any boundary" (zero-tail: every transport or session boundary abandons the region with zero bytes) and drop the insert-only tail logic; do not patch a third time.

**Flag default policy**: `mobile_input_layer` merges ON by default only if the pre-merge phone gate (Task 5.1.4c) passes on the owner's phone against the isolated manual instance (ports 62871/62872, reachable over LAN/Tailscale), covering: soft keyboard opens on tap, plain letters, autocorrect, swipe, Backspace right after a commit (Story 0.1.2/0.1.3 result), Enter. If the gate fails, A1 merges with the server default OFF and the owner gets a per-scope override, and the failure is recorded as a fixture-capture task using the `?inputdebug=1` Copy report. "On" affects only the Android class; desktop and iOS are unchanged. Safety also rests on the replay corpus, the Pixel 7 e2e, the delivered-prefix diff base (Story 1.2.4) and a live rollback (panel: one load persists, a second applies; `?mobile_input=0` applies at once). Sub-flags stay default off.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `MobileInputEvent` | Discriminated union on `kind`, each variant carrying only its own required fields: `keydown{key,keyCode,isComposing,t}`, `beforeinput{inputType,data,value,selStart,selEnd,t}`, `input{value,selStart,selEnd,t}`, `compositionstart{t}`, `compositionupdate{data,t}`, `compositionend{data,t}`, `selectionchange{selStart,selEnd,t}`, `blur{t}`, `tick{t}` | Compiler-checked exhaustive switch (including `tick`); no DOM references, no optional-field bags |
| `MobileInputState` | `Idle` \| `Composing{region, sent}` \| `Degraded` | Sum type, exhaustive `switch`; `Rewriting` is not a state, it is a pure step |
| `CompositionRegion` | Text of the current composition as the browser reports it (the textarea value while composing) | Newtype over string |
| `SentText` | Exact string this layer already emitted for the current `CompositionRegion` | Newtype over string; the only text a rewrite may delete |
| `Grapheme` | One user-perceived character from `Intl.Segmenter({granularity:"grapheme"})` | Unit for DEL counts; never UTF-16 units |
| `RewriteEdit` | `{backspaces: number, insert: string}` computed from `SentText -> CompositionRegion` | `backspaces <= graphemes(SentText)`, 0 when `SentText` empty |
| `InputCommand` | Output of one reducer step: `Emit{bytes}` \| `ResetTextarea` \| `Anomaly{kind}` \| `EnterDeferred` | Sum type |
| `TerminalBytes` | String handed to `onData`; may contain `\x7f` and text | Branded string, never logged |
| `AtomicEdit` / queue unit | One `TerminalBytes` containing DELs plus text, delivered as one `sendInput`; the `InputQueue` stores these original chunks as indivisible units | Never chunk-split below `ATOMIC_EDIT_MAX_BYTES`, never split or merged in the queue |
| `MAX_REGION` | 64 graphemes: max composition length rewritten live | Const in `policy.ts`; above it the region goes deferred |
| `ATOMIC_EDIT_MAX_BYTES` | 1024 UTF-8 bytes ceiling for an atomic edit; larger is treated as paste/dictation | Const in `policy.ts` |
| `DeferredCommit` | Region mode that holds text until `compositionend` | Fallback for cap overflow, newline in region, selection change |
| `ImeAnomaly` | Enum: `orphaned_input`, `rewrite_bound_exceeded`, `unrecognized_sequence`, `stuck_composition`, `selection_in_region`, `duplicate_suppressed` | Fixed enum; used as telemetry label; class only, never content |
| `InputSource` | Interface the adapter reads events from: `TextareaSource` now, `EditContextSource` later | ADR-006 |
| `MobileInputOptions` | `{onData: OnData = (bytes: TerminalBytes, seq: UnitSeq) => SendResult \| void, getModifiers, consumeModifiers?, isReadOnly, onAnomaly, onDiagnostic?, onQueueDiscard?: (info: DeliveryReport) => void}` | Seam contract; `SendResult` return is part of it from Task 1.2.2b |
| `UnitSeq` | Monotonic integer the layer assigns to each emitted atomic unit | Lets the sink report delivery per unit |
| `DeliveryLedger` | Layer-internal list of `{seq, epoch, baseBefore, baseAfter}` for units emitted in the current delivery epoch | The DEL base is always `baseAfter` of the last unit of the CURRENT epoch known delivered (Story 1.2.4) |
| `DeliveryReport` | `{deliveredThroughSeq: UnitSeq, discardedFromSeq: UnitSeq \| null, chars: number}` (`chars` = code points of the discarded text, the number the chip prints) | Reported by the sink on a drop or queue discard; no content. If `deliveredThroughSeq` is not in the ledger the layer abandons the region |
| `AbandonedRegion` | Layer state flag set by `invalidate` with a session-class reason or by a queue discard: the in-flight region emits NOTHING further (no DELs, no tail), the ledger is cleared, and the next composition starts from an empty base | Safe fallback of Story 1.2.4 |
| `DeliveryEpoch` | Integer bumped at every transport boundary (`invalidate('transport')`: `isConnected` true->false, reconnect, any `dropped`, thrown send, aborted chunked paste). Text emitted in an earlier epoch is insert-only reference: never deleted | Story 1.2.4; makes `sent` honest (handed to transport, not proof of delivery) |
| `MobileInputController` | `{focus, blur, dispose, active, bindTapTarget(el), invalidate(reason)}`; `focus` must run synchronously inside a user gesture; `bindTapTarget` is the layer-owned tap-to-focus entry for both renderers; `invalidate(reason: 'session'\|'pool_reuse'\|'visibility'\|'queue_discard'\|'transport')` per Story 1.2.4 | |
| `MobileLayerFlag` | Client view of `mobile_input_layer` (default per the Task 5.1.4c gate; persisted `ssq:flag:` as `"1"`/`"0"`; overridable per load by `?mobile_input=0\|1`) | Rollback switch; affects the Android class only |
| `shouldAttachMobileInput` | Platform predicate for layer attach and `resolveRenderer` ONLY: Android Chrome-class touch-primary (`(pointer: coarse) && !(hover: hover) && Android UA/UA-CH`) | `platform.ts` |
| `isTouchPrimary` | Generic coarse-pointer predicate (`(pointer: coarse)`, today's semantics) for link-tap menu, selection handles and gestures on every coarse-pointer device | `platform.ts`; `XtermTerminal.tsx` imports it, behavior unchanged on iOS and touch laptops |
| `RendererKind` | `"ghostty" \| "xterm"` | Return type of `resolveRenderer` |
| `resolveRenderer` | Pure `({ghosttyFlag, mobileLayerFlag, attachable}) -> RendererKind`, `attachable = shouldAttachMobileInput()` | In `TerminalPool.tsx`'s new sibling `rendererPolicy.ts` |
| `SendResult` | `"sent" \| "queued" \| "dropped"` returned by `sendInput` per atomic unit | Sum type; replaces void; produced from PR-A (disconnected = `dropped`); `sendInput` is strictly FIFO: while the queue is non-empty or flushing it enqueues behind, never sends ahead |
| `InputDropInfo` | `{reason: "disconnected"\|"read_only"\|"control"\|"enter"\|"overflow"\|"expired"\|"session_changed", chars: number}` | No content |
| `InputQueue` | Bounded FIFO of atomic plain-text units (<= 4 KiB, per-unit <= 30 s), whole-queue discard on overflow/expiry/session change, injectable clock | Pure class in `InputQueue.ts` |
| `InputClass` | `text` \| `escape` \| `other` (server classifier result) | Go enum type, not string constants |
| `InputPath` | `cm` \| `shell` \| `capture` \| `stream` (server input entry point; four labels, five tested sites because the `:1653`/`:2489` closures share `cm`) | Go enum type; metric label |
| `ListenerKind` | `local` \| `remote` | Existing server constant |
| `TerminalInputTally` | Per-session `{frames, bytes, lastAt, byKind}` in a 256-entry LRU | Served at `/actuator/terminal-input` |
| `ReplayTrace` | Fixture `{id, source, keyboard, events[], expectedBytes, finalText, scrubbed?, signoff?, link?}` | JSON in `__fixtures__/`; `source:"device"` requires `scrubbed:true` and a non-empty `signoff` |
| `FixtureSource` | `"synthetic" \| "device"` | Required on every trace; counts reported separately |
| `ModelTerminal` | Test-only string buffer that applies DEL and text to check rewrite maths | In `replay.test.ts` helper |
| `DiagnosticsEvent` | Redacted ring-buffer entry `{t, source, inputType, dataClass, len, composing, action, bytesOut}` | No content by default. "Reveal text" content is held in a separate in-memory-only `revealBuffer` inside the sheet component, never in `DiagnosticsEvent`, never serialized, cleared on close |
| `DiagnosticsReport` | Artifact 1: content-free, shareable JSON of the ring buffer plus header; NOT convertible to a replayable trace | Triage only |
| `FixtureCapture` | Artifact 2: opt-in, local-only recording of event data for one short user-initiated session, never auto-sent, converted by `captureToTrace` | Feeds device corpus after the scrub checklist |
| `ComposeBar` | Opt-in visible native textarea + Send / Send+Enter, writes the string as one unit | Story 4.2, PR-D |
| `useMobileInputChrome` | The one hook owning flag-off hint, minimal diagnostics panel, chip, queue-flush, diagnostics sheet, compose toggle and fallback-detector state for `TerminalOutput.tsx` | Shell created in PR-A1 (Task 1.4.1e); PR-B/C/D only extend it |

---

## Pattern Decisions

Step 0.5 alternatives considered: (A) fix inside each renderer; strength: smallest diff per renderer, weakness: two copies, regression returns on next swap. (B) Wrapper layer that owns a hidden textarea and a pure reducer, renderer only gets a byte sink; strength: renderer-agnostic and replay-testable, weakness: two text inputs per terminal must be reconciled. (C) Compose-first UX only (visible text field, send as unit); strength: perfect IME fidelity, weakness: breaks per-key flows (y/n, menus, TUIs). Chosen: B, with C as an opt-in secondary mode.

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Input pipeline | Functional core, imperative shell (pure `reduce`, DOM adapter) | type-driven-design | Logic in event handlers | Replayable in Jest with no DOM |
| `MobileInputState` | Sum type + exhaustive switch | type-driven-design | Booleans (`isComposing`, `isDegraded`) | Illegal state combos unrepresentable |
| `CompositionRegion` / `SentText` / `TerminalBytes` | Newtypes (branded strings) | type-driven-design | Raw `string` | Prevents passing region where sent text belongs |
| `InputClass`, `InputPath` (Go) | Typed enums with `String()` | primitive-obsession-checklist | String constants | Bounded metric labels, exhaustive switches |
| `attachMobileInput` | Adapter / Facade (GoF) | GoF | Per-renderer inline effects | One seam; ~10 lines per renderer |
| Event sources | Strategy (`InputSource`) | GoF | Hard-coded textarea | EditContext slots in later (ADR-006) |
| `resolveRenderer` | Pure policy function | Fowler (Special Case/Policy) | Inline ternary at `TerminalPool.tsx:359` | Testable 8-combination matrix |
| `InputQueue` | Bounded queue with injectable clock | deterministic-fast-tests | `setTimeout` inside component | Deterministic tests |
| Diagnostics ring buffer | Observer on `onDiagnostic` | GoF | Logging in renderers | Zero cost when closed |
| `recordTerminalInput` | Single choke-point helper | PoEAA Gateway | Counter code at each handler | One test enumerates entry points |
| Rewrite diff | Hand-written common-prefix/suffix edit | build-vs-buy.md 1g | `diff` / `fast-diff` | Single contiguous edit; terminal-safe |
| Per-session tally | Bounded LRU + atomics | golang-concurrency | Prometheus session label | Cardinality |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `web-app/src/lib/terminal/TerminalPool.tsx` | Renderer choice is an inline ternary at :359, untestable | Refactor-first | Story 1.3.2 extracts `resolveRenderer` before the gate uses it |
| `web-app/src/components/sessions/XtermTerminal.tsx` (1,857 lines) | Hotspot, touch handlers call `preventDefault` | Isolate via seam | One shared `useMobileInput` hook; no logic moves in |
| `web-app/src/components/sessions/GhosttyTerminal.tsx` (188 lines) | Small | Extend as-is | Same hook call |
| `web-app/src/components/sessions/TerminalOutput.tsx` (2,456 lines) | God component | Isolate via seam | All chrome state lives in the single `useMobileInputChrome` hook (shell in PR-A Task 1.4.1e, extended by PR-B/C/D); TerminalOutput gains one hook call, one fragment, prop pass-throughs, the `SendResult` return plumbing (Task 1.2.4b, PR-A), the read-only hint `role="status"`, and the `:1518` focus rerouting (Task 1.2.3b); no inline chip/queue/diagnostics/compose logic |
| `web-app/src/lib/hooks/useTerminalFlowControl.ts` | `sendInput` returns void, silent drop at :144 | Extend as-is (return type widened) | Stable; one new return value, chunking untouched |
| `web-app/src/lib/hooks/useTerminalGestures.ts` | Android focus/blur workaround at :683-687; `t.focus()` at :640/:660 | Isolate via seam | Receive a `focusTerminal` callback instead of `t.focus()`; skip workaround when layer active |
| `server/services/connectrpc_websocket.go` (large) | Closures at :1653, :2489 duplicate input handling | Isolate via seam | Two call sites in this file (shared read loop :3131, capture path near :3849); the :1653/:2489 closures are only touched to thread the `forwarded` result back, nothing else; new file holds logic |
| `connectrpc_websocket_shell.go`, `session_service_stream_terminal.go` | Separate input handlers | Extend as-is | One helper call each (`received`; `forwarded` per Story 2.1.2 site table) |
| `server/analytics/client_metrics.go` | Event allow-list | Extend as-is | Two events following the existing pattern |
| `web-app/src/lib/features/flagStore.ts` | Flags special-cased by name | Extend as-is | One constant, one branch |

---

## Migration Plan

Not applicable: no schema or persisted-data changes. The flag value persists via the existing feature-flag store and the `ssq:flag:` localStorage key; the actuator tally is in-memory.

## Observability Plan
- **Logs**: server Info on first input per stream and a summary at most 1/min/stream, fields = path, listener, frames, bytes, kind counts; never content. Client `console.error` once per anomaly kind, never including typed data.
- **Metrics (server)**: `terminal_input_frames_total{kind,listener,path,stage}`, `terminal_input_bytes_total{...}` (class-only labels, no session labels). `stage` is `received` (recorded at the loop entry, always) or `forwarded` (recorded only where the writer actually forwards after the read-only and `CanSendCommand` checks); an `accepted` label is NOT used because the entry-side call cannot know the permission result. Actuator `GET /actuator/terminal-input` for per-session detail (256-entry LRU).
- **Metrics (client, via `cm.*` beacon)**: `cm.input{event: emitted|dropped_disconnected|queued_flushed|queue_discarded, renderer}`, `cm.ime_anomaly{kind}` rate limited (max 1 per kind per 10 s, burst 5).
- **Name mapping to requirements.md** (names stay as the plan's; the requirement names are these series): `input_dropped_disconnected_total` = `cm.input{event="dropped_disconnected"}`; `ime_event_sequence_unrecognized_total` = `cm.ime_anomaly{kind="unrecognized_sequence"}`; `terminal_input_frames_total{kind}` and `terminal_input_bytes_total` as in the requirements, plus the `listener`, `path`, `stage` labels. The docs page (Story 2.2.3) states this mapping.
- **Dashboard**: extend `docs/observability/grafana/dashboards/client-network.json` with an input row.
- **Alerts**: no paging alert. Documented diagnostic query: client `cm.input emitted` rate > 0 while `terminal_input_frames_total{kind="text",listener="remote",stage="received"}` rate == 0 for 5 minutes = the regression symptom (client typed, server received nothing); `received` flat-positive with `forwarded` zero = read-only or permission denial. This difference is only meaningful for paths that record `forwarded` (see the per-site table in Story 2.1.2) and the query is scoped to `listener="remote"` and those `path` values.

## Risk Control
- **Feature flags**: `mobile_input_layer` (rollback-only switch, live-settable in the feature-flag panel with per-scope override, no env var, read from the flag store only (test: Task 1.3.1b); server default ON only if the pre-merge phone gate Task 5.1.4c passes, otherwise OFF with a per-scope owner override; blast radius limited to the Android class because desktop and iOS never attach). Sub-flags, each default OFF and each owned by its own later PR: `mobile_input_queue` (Phase 3), `mobile_input_diagnostics` (full sheet, Phase 4.1; the A1 `?inputdebug=1` panel is not behind it) and `mobile_input_compose` (Phase 4.2), `mobile_input_edit_context` (Phase 6). The layer attaches only where `shouldAttachMobileInput()` is true.
- **Rollback**: flip `mobile_input_layer` off in the panel (first load persists `"0"`, SECOND load applies), or open the app with `?mobile_input=0` (applies in the same load and persists for that origin only). On the Android touch class, flag off also forces xterm even if `ghostty_renderer` is on and shows a visible one-line hint (not silent). Known-good fallback state for the owner's phone (which renderer and flag combination typed correctly) is recorded by Story 0.1.3. Document in the PR-A1 description and `debug-phone-typing.md`. Revert the PR if needed.
- **Rollout**: PR-A1 is first exercised on the isolated manual instance (ports 62871/62872, private tmux socket, per CLAUDE.md, never `make install-service`) by the owner's phone BEFORE merge (Task 5.1.4c). Merge with the flag ON only if that passes. After merge the owner redeploys locally and runs Task 5.1.4b. `make install-service` restarts the server and drops every WebSocket (tmux sessions survive): that is a transport boundary (`invalidate('transport')`), so redeploy only when no phone is mid-composition, and the phone may still run a cached bundle, so acceptance starts with a build-id check (diagnostics header, hard reload). Before the redeploy check `~/.stapler-squad/workspaces/*/config.json` for an explicit `mobile_input_layer` that would pin the value.

## Unresolved Questions
- [x] Does turning `ghostty_renderer` off alone restore plain-letter typing on the phone? — NO LONGER A BLOCKER: the layer owns input under both renderers, so the fix does not depend on the answer. The result is still recorded by Story 0.1.3 for the how-to wording only — owner: Tyler
- [ ] Are ghostty-web `dist/ghostty-web.js` lines 851 (keyCode 229 early return) and 2330 (blanket `beforeinput` preventDefault) true for the installed copy? They were read from the main checkout's `web-app/node_modules` earlier; this worktree has no `node_modules/ghostty-web` — blocks Story 1.4.1 (helper-textarea suppression design) — owner: worker agent in Story 0.1.1
- [ ] Exact Gboard (suggestions on/off, swipe, symbol keyboard) and Samsung event sequences — blocks Story 5.1.4 only; synthetic corpus ships first — owner: Tyler (device recording via Story 4.1 report)
- [ ] Does CDP `Input.imeSetComposition` produce `beforeinput insertCompositionText` like Android? — blocks Story 5.1.1 helper design — owner: worker agent in Story 0.1.2
- [ ] On Android Chrome (default `interactive-widget` behavior), Samsung Keyboard (split, floating, one-handed) and floating Gboard, does the soft keyboard resize the layout viewport, only the visual viewport, or overlay the page, and does the cursor row stay visible? The existing keyboard-avoidance layout (`mobile-ux-improvements` ADR-003, `--viewport-height` written by `ViewportProvider`) was designed around iOS Safari's fixed-vs-layout-viewport bug; its Android behavior is UNVERIFIED — blocks Story 1.4.3 (chrome stack budget) and the `100dvh` fallback — owner: worker agent (Task 0.1.5b) and Tyler (device, Task 0.1.5a)
- [ ] Does TalkBack announce typed characters correctly when the textarea value is only the in-flight composition and committed text goes to a polite live region (research/ux.md TalkBack note, INFERRED)? — blocks nothing in code; a failing device check (Task 5.1.4d) reopens Story 1.4.4 design — owner: Tyler (device, TalkBack)
- [ ] OBS-4 alert rule (Task 2.2.3b): if the repo has no alert-provisioning convention, the stop path records an OWNER DECISION in `requirements.md` that the dashboard panel plus the documented query satisfy the requirement. OPEN until the owner accepts it; it does not block PR-A2 merge, and the alert rule file is delivered immediately if a convention is found — owner: Tyler
- [ ] Is a Ctrl-W (`\x17`) for `deleteWordBackward` honored by Claude Code's input box? — hard input to Task 1.1.2a keymap row: if it fails, map `deleteWordBackward` to repeated `\x7f` bounded by the word length from the textarea value; also an acceptance row in Task 5.1.4b — owner: worker agent in Story 0.1.3
- [ ] Does Gboard/Samsung emit `beforeinput deleteContentBackward` (or any event) for Backspace on the textarea after it was reset to `""` at a commit boundary? — blocks Task 1.1.2a design (sentinel character vs keydown-8 fallback) — owner: worker in Story 0.1.2 (CDP) and Tyler (device, Task 0.1.3b)
- [ ] Does `focus()` from a tap open the real soft keyboard on a `1px`, `pointer-events:none` textarea with the gesture handlers active? — blocks Task 1.2.3c design (tap target is the visible terminal surface; `touchend`/`click` fallback) — owner: Tyler (device, Task 0.1.3b)
- [ ] Is bracketed-paste state (DECSET 2004) readable from both renderers, so the compose bar and `insertFromPaste` can wrap correctly? — blocks Story 1.1.3 (paste handling) and Task 4.2.1b — owner: worker agent in Story 0.1.1
- [ ] Is Chrome Android EditContext behavior better with Gboard? — blocks Story 6.1 only — owner: Tyler (device test)
- [ ] Is there any reliable echo-off signal to turn `autocorrect="off"` on at password prompts? — blocks nothing (v1 documents the limitation, ADR-008) — owner: worker agent in Task 1.2.1c

## Dependency Visualization

```
Wave 0: S0.1.4 smoke + make ready baseline + .jscpd.json fixture exclusion + per-worker worktrees ─► gates everything below
PR-A1 (critical path)
Phase 0 spikes
 S0.1.1 verify ghostty-web + DECSET 2004 ──┬─► blocks 1.1.3 (paste), 1.4.1c (mount), 4.2.1b
 S0.1.2 CDP IME probe (+0.1.2b backspace-on-empty) ─┼─► blocks 5.1.1a; 0.1.2b blocks 1.1.2a design
 S0.1.3 owner protocol (+0.1.3b device: backspace-on-empty, tap-opens-keyboard, origins; +0.1.3c baseline loss/latency) ─┘ blocks 1.1.2a, 1.2.3c; 0.1.3c feeds 5.1.4e
 S0.1.5 Android keyboard-resize spike (0.1.5a device, 0.1.5b worker) ─► blocks 1.4.3 (chrome stack budget)
Phase 1
 1.1.1 types+rewrite ─► 1.1.2 reducer ─► 1.1.3 keymap/paste ─► 1.1.4 corpus+replay (1.1.4b/f/g fixture batches, +1.1.4d fixture lint, +1.1.4e backspace-on-empty and hardware-keyboard fixtures)
                              └──────────────► 1.2.1 policy/platform(+isTouchPrimary, +1.2.1d import-graph test) ─► 1.2.2 adapter (SendResult-typed onData, +1.2.2d latency budget) ─┬► 1.2.3 focus ownership (+bindTapTarget on visible surface)
                                                                                                                                  └► 1.2.4 delivery-aware diff base + invalidate(reason) [stop rule: 2 review rounds] (1.4.1a calls invalidate; 1.2.4e needs 1.4.1a wiring)
                                                                                                                                                  └► 1.2.5 rewrite flicker budget (1.2.5a) + DeferredCommit feedback (1.2.5b, needs 1.4.1e chrome shell)
 1.3.1 server flag (default per 5.1.4c) (+1.3.1b no-env test) ─► 1.3.2 flagStore(persist 0/1)+resolveRenderer+isTouchPrimary split (+1.3.2e ?mobile_input=0|1) ─► 1.4.1 hook+renderers (attach on visible/dock; 1.4.1e chrome shell + flag-off hint; 1.4.1f read-only extra-keys test) ─► 1.4.2 integration test (fake ghostty) (+1.4.2b ghostty-web version pin)
 1.4.1e chrome shell ─► 1.4.3 chrome stack budget + minimum terminal rows (needs 0.1.5) ; 1.4.1e ─► 1.4.4 live-region contract (polite + assertive) + TalkBack typed-character announcement
 4.1.1 ring buffer + content-free report ─► 2.3.1 minimal ?inputdebug=1 panel (via chrome shell 1.4.1e)
Phase 5 (PR-A1 part): 5.1.1 e2e (needs 1.4.2) (+5.1.1d required CI check on dependency change, +5.1.1e rewrite-frame sampler) ─► 5.1.2 docs ─► 5.1.3 gates ─► 5.1.4c OWNER pre-merge phone gate on the isolated manual instance (same session: 5.1.4d TalkBack check, 5.1.4e loss and latency re-measure vs 0.1.3c baseline) ─► MERGE (default ON only if it passes)
PR-A2 (parallel, independent of A1): 2.1.1 ─► 2.1.2 ─► 2.1.3 ; 2.2.1 ─► 2.2.2 ─► 2.2.3 ─► 2.2.3b alert rule
Post-redeploy: 5.1.4b OWNER acceptance on the deployed build (build-id check, remote :8444, PWA, plain-HTTP, hardware keyboard, Ctrl-W rows) ; 5.1.2d timed diagnosis drill (needs PR-A1 + PR-A2 merged)

PR-B (needs PR-A1: 1.2.4, 1.4.1e)  3.1.1 onInputDropped+atomic send ─► 3.1.2 chip (extends chrome hook) ─► 3.1.3 atomic-unit queue + strict FIFO ─► 3.1.4 queue delivery reports + replay ─► 3.1.5 chip states (+3.1.5b stuck-state copy) ; 5.1.1c disconnect case
PR-C (needs PR-A1: 1.2.2, 1.4.1e, 4.1.1)  4.1.2 full sheet (via chrome hook; registers mobile_input_diagnostics; +4.1.2c settings entry) ─► 4.1.3 capture-for-fixture ─► device fixtures (5.1.4a)
PR-D (needs PR-A1: 1.4.1e, 0.1.1)  4.2.1 compose bar ─► 4.2.2 fallback detector + notice (+4.2.2c anomaly-based offer)
PR-E (conditional)          6.1.1 EditContext (needs PR-C corpus) ; 6.1.2 follow-up issues (unconditional, not a PR)
```

---

## Phase 0: Spikes (riskiest unknowns first)

### Epic 0.1: Verify assumptions before building
**Goal**: Close the factual gaps the design depends on.

#### Story 0.1.1: Re-verify ghostty-web citations and DECSET 2004 access
**As a** layer author, **I want** the ghostty-web line citations and renderer capabilities confirmed against the installed copy, **so that** the helper-textarea suppression and paste handling rest on facts.
**Acceptance Criteria**:
- Lines `dist/ghostty-web.js:851` (keyCode 229 early return) and `:2330` (blanket `beforeinput` preventDefault) are confirmed or corrected against the installed 0.4.0 in this worktree.
  - *Given* `bun install` has run in `web-app/` of this worktree, *When* the worker greps `web-app/node_modules/ghostty-web/dist/ghostty-web.js` for `keyCode === 229` and `beforeinput`, *Then* `spike-findings.md` lists the actual line numbers (or the corrected ones) and the exact condition text.
- The full listener set ghostty-web 0.4.0 registers on the element passed to `open()` and on its own hidden textarea is recorded (keydown/beforeinput/input/composition handlers), and whether UA-CH `navigator.userAgentData.platform` is available in the target Chrome.
  - *Given* the installed copy, *When* the worker greps `addEventListener` near `open()`, *Then* the findings list each event type, its target element and the condition text (cited by condition, not line number).
- Whether bracketed-paste mode state is readable from each renderer is recorded; this result BLOCKS Story 1.1.3 (paste handling) and Task 4.2.1b.
  - *Given* `XtermTerminal` exposing `terminal.modes.bracketedPasteMode` and `GhosttyTerminal` wrapping ghostty-web, *When* the worker inspects both APIs, *Then* the findings file states for each renderer `readable: yes/no` with the property or function name.
**Files**: `project_plans/mobile-input-layer/implementation/spike-findings.md`

##### Task 0.1.1a: Install deps and re-verify ghostty-web lines (XS)
- `cd web-app && unset npm_config_user_agent && bun install`; grep the installed `ghostty-web/dist/ghostty-web.js`; record corrections (the plan cites 851/2330 from the main checkout).
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`

##### Task 0.1.1b: Record bracketed-paste access per renderer (XS)
- Read `web-app/src/components/sessions/XtermTerminal.tsx`, `GhosttyTerminal.tsx`, `web-app/src/lib/terminal/ghosttyAdapter.ts`; append findings.
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`

#### Story 0.1.2: CDP IME helper feasibility
**As a** test author, **I want** to know what `Input.imeSetComposition` actually fires in Chromium, **so that** the e2e helper is designed on evidence.
**Acceptance Criteria**:
- The event order produced by CDP IME calls on a textarea is recorded, and so is Backspace on the textarea after a commit reset (Task 0.1.2b).
  - *Given* a scratch page with a focused `<textarea>` logging `keydown`, `beforeinput`, `input` and `composition*`, *When* the probe calls `Input.imeSetComposition({text:"hel",selectionStart:3,selectionEnd:3})` then `Input.insertText({text:"hello"})`, *Then* the findings list the exact event sequence including `inputType` values and `isComposing`.
**Files**: `project_plans/mobile-input-layer/implementation/spike-findings.md` (probe script kept in the scratchpad, not committed)

##### Task 0.1.2a: Run the CDP probe via Playwright in a scratch dir (S)
- Use `devices['Pixel 7']`; record sequence; note whether `insertReplacementText` can be produced at all.
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`

##### Task 0.1.2b: CDP probe: Backspace on the textarea after it was reset to `""` (XS)
- Pre-mortem P1-3. The layer resets the textarea to `""` at every commit boundary, so on a real Android keyboard Backspace may emit only `keydown`/`keyup` (keyCode 8 or 229) with no `beforeinput`. Record in the scratch page, with the textarea empty and then holding a sentinel character, whether `beforeinput deleteContentBackward` fires, what `keydown` reports, and whether CDP can model it at all.
  - *Given* a scratch textarea reset to `""` after a commit, *When* the probe dispatches Backspace (CDP `Input.dispatchKeyEvent` key 8), *Then* `spike-findings.md` lists which of `keydown{keyCode}`, `beforeinput{deleteContentBackward}`, `input` fired, for both the empty and the sentinel-character variants.
- The result plus Task 0.1.3b chooses the reducer design for Task 1.1.2a: a sentinel character kept in the textarea (as xterm.js and CodeMirror do), or a `keydown` keyCode-8 fallback path. Record the choice.
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`

#### Story 0.1.3: Owner device protocol and Ctrl-W check
**As the** owner, **I want** a short checklist for the phone, **so that** the root-cause split and corpus recording are quick.
**Acceptance Criteria**:
- A checklist exists for: (1) ghostty off, type "abc" in `stelekit`; (2) ghostty on, same; (3) note keyboard and suggestion-strip setting.
  - *Given* the owner's phone on the remote listener, *When* the owner follows step 1 with `ghostty_renderer` off, *Then* the results table in `spike-findings.md` has a yes/no for "plain letters arrive" for each renderer.
- Backspace-on-empty and tap-opens-keyboard results from the physical phone are recorded (Task 0.1.3b), together with the origins tested.
- Baseline measurements exist BEFORE any feature code (Task 0.1.3c), so the Success Metrics have measured "before" numbers: exact-match count out of the scripted phrase list (Story 5.1.4) with `ghostty_renderer` off and on, keystroke loss on the phone and on desktop, and the desktop keystroke-to-echo latency p50/p95.
  - *Given* the current main build on the isolated manual instance, *When* the owner types the scripted phrase list on the phone with ghostty off and then on, and a Playwright script types a 200-character string with a hardware keyboard on desktop, *Then* `spike-findings.md` has a baseline table: `exact N/22` per renderer on the phone, `chars received / chars typed` per run, and desktop `p50/p95 ms` keydown-to-echo.
- Ctrl-W behavior in Claude Code's input box is recorded from the manual instance.
  - *Given* a manual instance session with `ssqtest hello` typed at the prompt, *When* `\x17` is sent via `write_to_session`, *Then* the findings state whether `hello` was removed.
**Files**: `project_plans/mobile-input-layer/implementation/spike-findings.md`

##### Task 0.1.3a: Write owner checklist and run Ctrl-W check (XS)
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`
- The Ctrl-W result is a hard input to Task 1.1.2a: if `\x17` fails in Claude Code's input box, map `deleteWordBackward` to repeated `\x7f` bounded by the word length from the textarea value.

##### Task 0.1.3b: Owner-device spike: Backspace-on-empty and tap-opens-keyboard, per origin (XS, owner wall-clock blocker)
- Pre-mortem P1-3 and P1-4. On the owner's phone, against a scratch probe page served from the isolated manual instance port block (62876-62880) or a throwaway static server, with the existing `useTerminalGestures`-style touch handlers (`preventDefault`, `pointercancel` on scroll) active on the surface: (1) record Backspace-on-empty after a commit and after a suggestion pick, Gboard and Samsung if available; (2) check that `focus()` called synchronously from `pointerup`, from `touchend` and from `click` on the VISIBLE terminal surface (not the 1px `pointer-events:none` textarea) opens the soft keyboard, including after a scroll gesture; (3) record which origins were tested (`:62872` private-CA HTTPS remote listener, LAN or Tailscale URL, plain-HTTP LAN origin, installed PWA window) and which renderer and flag combination types correctly today (the known-good fallback).
  - *Given* the probe page on the owner's phone, *When* the owner follows the checklist, *Then* `spike-findings.md` has a results table with yes/no per (event path, keyboard, origin) and the chosen tap-binding events and Backspace design.
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`

##### Task 0.1.3c: Baseline: phrase-list exact-match, keystroke loss and desktop latency on the current build (XS, owner wall-clock blocker for the phone half)
- Phone half (owner, runs in the same session as Task 0.1.3b): type the scripted phrase list (Story 5.1.4 table) on the current main build, ghostty off then on; count exact matches by reading the pane with `read_session_output`; count characters that reached the pane versus typed. Desktop half (worker): a Playwright script on desktop Chromium types a fixed 200-character string with `page.keyboard.type` and records keydown-to-echo p50/p95 by sampling the terminal buffer each animation frame. Latency figures are RECORDED only and never asserted in CI (no wall-clock thresholds in e2e); Task 5.1.4e compares against them.
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`

#### Story 0.1.4: Wave-0 smoke, baseline gates and parallel-worker rules
**As the** lead agent, **I want** the worktree proven buildable and the CI gate baseline known before any feature code, **so that** wave 1 spike workers can run grep and jest, and `make ready` does not fail at the end of the critical path (pre-mortem P2-8, P2-9).
**Acceptance Criteria**:
- The worktree builds and tests run.
  - *Given* the worktree under `~/.cache/`, *When* the worker runs `cd web-app && unset npm_config_user_agent && bun install`, `make build`, one `go test` on a small package and one `bunx jest --no-coverage` smoke, *Then* all exit 0 and the commands plus outputs are recorded in `spike-findings.md`.
- The `make ready` baseline is known on a stub branch.
  - *Given* a stub branch with no feature code, *When* `make ready` runs, *Then* the baseline result is recorded, and `web-app/.jscpd.json` `format`/`ignore` is checked and `__fixtures__` (JSON fixtures) is excluded so 21 near-identical JSON files do not count toward the 0.14% ratchet.
- Parallel-worker rules are fixed before wave 1.
  - *Given* the wave plan, *Then* each parallel worker uses its OWN worktree created from the feature branch, scratch stays on disk (below 1 GB, `free -g` checked first, never `/tmp`, which is RAM tmpfs), `GOTMPDIR` is a short non-git path, and no unscoped `pkill` is used.
**Files**: `project_plans/mobile-input-layer/implementation/spike-findings.md`, `web-app/.jscpd.json`

##### Task 0.1.4a: Install, build, smoke tests and `make ready` baseline on a stub branch (S)
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`, `web-app/.jscpd.json`

##### Task 0.1.4b: CI gate pre-checks and worker rules recorded (XS)
- Record in `spike-findings.md`: (1) the `connectrpc_websocket.go` change is extracted to a new file and only a returned `bool` is threaded through the `:1653`/`:2489` closures, because gocognit/funlen/dupl apply to modified existing functions; (2) new hooks pass `react-hooks/exhaustive-deps` and the zero-warning web lint (`no-restricted-imports` on `next/*`); (3) the registry check, `config_surface_proof_test.go` and BUG-087 (no "gate" in Go test filenames) run in the same task as the flag (1.3.1a); (4) the per-worker worktree and scratch rules above.
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`

#### Story 0.1.5: Android soft-keyboard resize and viewport behavior
**As the** owner, **I want** the real Android keyboard-resize behavior recorded, **so that** the chrome stack (toolbar, compose bar, chip) provably stays above the keyboard and the cursor row stays visible instead of relying on the iOS-derived keyboard-avoidance assumption.
**Acceptance Criteria**:
- Chrome on Android is measured, not assumed: with the app's current viewport meta, opening the soft keyboard on a probe page records `window.innerHeight`, `visualViewport.height`, `visualViewport.offsetTop`, `document.documentElement.clientHeight` and the computed `100dvh` before and after, and states which of `resizes-content`, `resizes-visual` or `overlays-content` applies (Chrome 108 changed the default away from resizing the layout viewport, INFERRED from Chrome release notes; this story VERIFIES it on the device) and whether adding `interactive-widget=resizes-content` to the viewport meta changes the result.
  - *Given* the probe page on the owner's Pixel-class phone in Chrome, *When* the keyboard opens and closes, *Then* `spike-findings.md` has a before/after table of the five values and the named mode.
- Samsung Keyboard split mode, floating mode and one-handed mode, and floating Gboard, are each recorded: whether `visualViewport.height` changes, whether `resize` fires, and whether the terminal cursor row stays visible with the existing `ViewportProvider` / `--viewport-height` layout.
  - *Given* each mode, *When* typing at the last terminal row, *Then* the findings state visible / hidden for the cursor row and list the modes the Story 1.4.3 budget must treat as "keyboard height unknown".
- A decision is recorded: which source drives available terminal height on Android (the `--viewport-height` var, a `visualViewport` listener, or the meta key), and whether the correction to the iOS-origin assumption needs a follow-up edit to `project_plans/mobile-ux-improvements/decisions/ADR-003-sticky-flex-layout-for-toolbar-keyboard-avoidance.md` (that ADR's context is iOS Safari's fixed-position bug only; it makes no Android claim). The in-folder `decisions/ADR-003` carries the amended assumption.
  - *Given* the findings, *When* Story 1.4.3 starts, *Then* its viewport source is the one named here.
- Cursor-row-visible (WCAG 2.4.11) gets an automated emulation check in addition to the manual UX-1 step: Playwright shrinks the visual viewport on the Pixel 7 profile and asserts the cursor row's bounding box lies inside the visible viewport and above the toolbar (the real keyboard check stays an owner-device row).
  - *Given* the viewport shrunk to 360x340, *When* the terminal is focused and the cursor is on the last row, *Then* the cursor row `boundingBox()` is inside the viewport and above the toolbar top.
**Files**: `project_plans/mobile-input-layer/implementation/spike-findings.md`, `tests/e2e/mobile-input-layer.spec.ts` (emulation check, lands with Task 5.1.1b)

##### Task 0.1.5a: Owner-device spike: Chrome Android and Samsung/floating keyboard resize modes (XS, owner wall-clock blocker)
- Run in the same phone session as Task 0.1.3b against the same probe page; record the table above for Chrome, Samsung Keyboard (split, floating, one-handed) and floating Gboard.
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`

##### Task 0.1.5b: Read the current viewport meta, `ViewportProvider` and `TerminalOutput` keyboard-avoidance code; record the decision (S)
- Inspect the viewport meta and `--viewport-height` plumbing, add the emulation-based cursor-row check to the Task 5.1.1b spec list, and write the viewport-source decision and the ADR-003 follow-up note into `spike-findings.md`.
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`

---

## Phase 1: Core layer (client)

### Epic 1.1: Pure core (no DOM)
**Goal**: Correct, replayable bytes for every event sequence, with zero DOM dependency.

#### Story 1.1.1: Grapheme-bounded rewrite diff
**As a** user, **I want** autocorrect rewrites to delete only what I just typed, **so that** earlier text in the prompt is never eaten.
**Acceptance Criteria**:
- `computeRewrite(sent: SentText, region: CompositionRegion): RewriteEdit` returns common-prefix-preserving edit with `backspaces <= graphemes(sent)`.
  - *Given* `sent="teh"` and `region="the"`, *When* `computeRewrite` runs, *Then* `{backspaces:2, insert:"he"}`.
- Empty `sent` never yields backspaces.
  - *Given* `sent=""` and `region="hello"`, *When* `computeRewrite` runs, *Then* `{backspaces:0, insert:"hello"}`.
- Emoji and ZWJ sequences count as one grapheme.
  - *Given* `sent="a👨‍👩‍👧"` and `region="ab"`, *When* `computeRewrite` runs, *Then* `{backspaces:1, insert:"b"}`.
- Regions over `MAX_REGION` or containing `\n` are reported as `{kind:"deferred"}`.
  - *Given* a 65-grapheme region, *When* `computeRewrite` runs, *Then* the result is deferred with anomaly `rewrite_bound_exceeded`.
**Files**: `web-app/src/lib/terminal/mobileInput/types.ts`, `rewrite.ts`, `rewrite.test.ts`, `policy.ts`

##### Task 1.1.1a: Define types and constants (S)
- `types.ts` (`MobileInputEvent` as a discriminated union per `kind` including `tick`, `MobileInputState`, `InputCommand`, `ImeAnomaly`, `SendResult`, `OnData = (bytes: TerminalBytes, seq: UnitSeq) => SendResult | void` defined once (with `UnitSeq` in `types.ts`; the ledger needs `seq`), branded `SentText`/`CompositionRegion`/`TerminalBytes`), constants `MAX_REGION`, `ATOMIC_EDIT_MAX_BYTES` in `policy.ts`. The reducer's exhaustive `switch` on `kind` must fail to compile if a variant (including `tick`) is unhandled.
- Files: `types.ts`, `policy.ts`

##### Task 1.1.1b: Implement `computeRewrite` with `Intl.Segmenter` (S)
- Files: `rewrite.ts`

##### Task 1.1.1c: Table tests and fast-check properties for the diff (S)
- Properties: `backspaces <= graphemes(sent)`; applying the edit to `sent` in a string model yields `region`.
- Files: `rewrite.test.ts`

#### Story 1.1.2: Composition reducer
**As a** user, **I want** every IME event shape to map to exactly the bytes a hardware keyboard would send, **so that** typing, autocomplete and Backspace work.
**Acceptance Criteria**:
- `reduce(state, event)` implements the Event-Command-Policy table in `research/architecture.md` section 6, every row covered by at least one table case.
  - *Given* `Composing{region:"hel",sent:"hel"}`, *When* `beforeinput insertCompositionText` arrives with region `"hello"`, *Then* commands are `Emit("lo")` and state `Composing{region:"hello",sent:"hello"}`.
- `keydown` with `keyCode 229` or `isComposing` emits nothing and is not marked preventDefault.
  - *Given* `Idle`, *When* `keydown{keyCode:229,key:"Unidentified"}` arrives, *Then* commands are `[]` and `preventDefault` is `false`.
- Enter flushes a pending (unsent) region first, then emits exactly one `\r`, and never also the finalized composition text. Exact bytes for each order, where `"ls"` was already emitted live:
  - *Given* `Composing{region:"ls",sent:"ls"}`, *When* `insertLineBreak` then `compositionend{data:"ls"}`, *Then* the bytes emitted from the Enter event onward are exactly `"\r"` (stream total `"ls\r"`).
  - *Given* the same state, *When* `compositionend{data:"ls"}` then `insertLineBreak`, *Then* `compositionend` emits `""` and Enter emits `"\r"` (stream total `"ls\r"`).
  - *Given* deferred-mode `Composing{region:"ls",sent:""}`, *When* `insertLineBreak` then `compositionend{data:"ls"}`, *Then* Enter emits `"ls"` then `"\r"` and `compositionend` emits nothing (stream total `"ls\r"`).
- Backspace maps to one `\x7f`; `deleteWordBackward` to `\x17` (or to repeated `\x7f` bounded by the word length if the Story 0.1.3 Ctrl-W check fails).
  - *Given* `Idle`, *When* `beforeinput deleteContentBackward` arrives, *Then* `Emit("\x7f")`.
- Backspace right after a commit, when the textarea has been reset to `""`, still emits exactly one `\x7f` (pre-mortem P1-3). The design follows the Task 0.1.2b / 0.1.3b result: either a sentinel character kept in the textarea so the browser has something to delete, or a `keydown` keyCode-8 (and 229-with-Backspace-shape) fallback path in the reducer that emits `\x7f` when no `beforeinput deleteContentBackward` arrives for that keydown; the fallback must not double-send when both arrive.
  - *Given* `Idle` after a `compositionend` commit and a textarea reset to `""`, *When* only `keydown{keyCode:8}` (or the sentinel-based `beforeinput`) arrives, *Then* `Emit("\x7f")` exactly once; *When* both `keydown{keyCode:8}` and `beforeinput deleteContentBackward` arrive, *Then* exactly one `\x7f`. Corpus fixture `backspace-on-empty-after-commit` (Task 1.1.4e) replays this.
- Orphaned `input` with no preceding `beforeinput` goes to `Degraded`, diffs value against the shadow value, emits the delta once and an anomaly `orphaned_input`.
  - *Given* `Idle` with shadow `""`, *When* `input{value:"/"}` arrives without `beforeinput`, *Then* `Emit("/")`, `Anomaly(orphaned_input)`, `ResetTextarea`.
- Selection change inside a live region finalizes it as deferred and emits `selection_in_region`; a pure caret move (space-bar swipe) emits no bytes.
  - *Given* `Composing{region:"abc",sent:"abc"}`, *When* `selectionchange{selStart:1}`, *Then* state `Idle`, no DEL, anomaly `selection_in_region`.
- Stuck composition watchdog: `blur` or a 2 s idle tick (injected clock) force-ends the composition.
  - *Given* `Composing` with last event at t=0, *When* a `tick{t:2000}` arrives, *Then* state `Idle` and anomaly `stuck_composition`.
- A `dropped` send result or `invalidate('transport')` demotes a live region to deferred-after-boundary (single rule, Story 1.2.4): nothing emitted until `compositionend` or the Enter flush, then the insert-only tail; session-class invalidation abandons the region with zero bytes.
  - *Given* `Composing{region:"ls",sent:"l"}` after `"s"` returned `dropped` and the link is back, *When* `insertLineBreak` arrives, *Then* the bytes are `"s"` then `"\r"`.
**Files**: `web-app/src/lib/terminal/mobileInput/composition.ts`, `composition.test.ts`

##### Task 1.1.2a: Reducer states and transitions (M)
- Pure `reduce`, exhaustive switch on `kind` and `MobileInputState`; no `Date.now`, clock arrives in events. Blocked by Tasks 0.1.2b and 0.1.3b (Backspace-on-empty design: sentinel character or keydown-8 fallback) and takes the Ctrl-W result from Task 0.1.3a.
- Files: `composition.ts`

##### Task 1.1.2b: Table-driven tests, one case per ECP row (M)
- Files: `composition.test.ts`

##### Task 1.1.2c: Property tests: event-order permutations and no-over-delete (S)
- Permute `compositionend`/`beforeinput` order (ghostty PR #110 case); assert identical bytes and `DEL count <= sent`.
- Files: `composition.property.test.ts`

#### Story 1.1.3: Modifier and paste semantics
**As a** user, **I want** armed Ctrl/Alt to apply to the next soft-keyboard character, **so that** Ctrl-C from the soft keyboard works.
**Acceptance Criteria**:
- A single printable character under armed Ctrl maps to `char & 0x1f`; Alt prefixes ESC; modifiers are consumed once.
  - *Given* `getModifiers()` returns `{ctrl:true}`, *When* `beforeinput insertText "c"` arrives, *Then* `Emit("\x03")` and `consumeModifiers` called once.
- A multi-character commit under armed Ctrl ignores the modifier, emits the text, clears the modifier.
  - *Given* `{ctrl:true}`, *When* `insertText "hello "` arrives, *Then* `Emit("hello ")` and modifiers consumed.
- Armed Ctrl/Alt during a live composition: the composition is flushed first (finalized as typed, no modifier applied to it, no rewrite), the modifier stays armed, and it applies only to the NEXT committed single-grapheme non-composing character, which then consumes it. Rewrite DELs and text are never modified.
  - *Given* `Composing{region:"he",sent:"he"}` with `{ctrl:true}`, *When* `insertCompositionText` with region `"hel"` arrives, *Then* the layer emits `"l"` unmodified, state `Idle`, modifier still armed; *When* `beforeinput insertText "c"` then arrives, *Then* `Emit("\x03")` and `consumeModifiers` called once.
- `insertFromPaste` and multi-line `insertText` are classified `paste` and carry a `paste: true` flag on the command for the sink. The executor wraps in bracketed paste only when the spike (0.1.1) shows DECSET 2004 is readable and on for that renderer.
  - *Given* `insertText "a\nb"` and 2004 on, *When* reduced and executed, *Then* the sink receives `"\x1b[200~a\nb\x1b[201~"`.
- When 2004 is unavailable or off, a multi-line paste is never emitted as raw `\r` (which would submit each line): newlines are joined with a single space and anomaly-free hint "multi-line paste flattened" is surfaced by the chrome hook.
  - *Given* 2004 unreadable, *When* `insertText "a\nb"` arrives, *Then* the sink receives `"a b"`.
**Files**: `web-app/src/lib/terminal/mobileInput/keymap.ts`, `keymap.test.ts`

##### Task 1.1.3a: `keymap.ts` modifier + control mapping, composition-aware modifier rule, paste policy (blocked by Story 0.1.1) (S)
- Reuse `web-app/src/lib/terminal/modifierKeys.ts` maps; add letter `& 0x1f` mapping local to the layer.
- Files: `keymap.ts`, `composition.ts`

##### Task 1.1.3b: Keymap tests (XS)
- Files: `keymap.test.ts`

#### Story 1.1.4: Replay corpus and harness
**As the** owner, **I want** a replayable event corpus, **so that** renderer swaps cannot silently break phone typing.
**Acceptance Criteria**:
- One table-driven test replays every `__fixtures__/*.json` trace and compares emitted bytes with `expectedBytes`, and the `ModelTerminal` line with `finalText`.
  - *Given* fixture `gboard-autocorrect-teh-the.json` (`source:"synthetic"`), *When* replayed, *Then* bytes equal `"teh\x7f\x7fhe"` and the model line equals `"the"`.
- Corpus contains >= 14 synthetic scenarios: plain typing, autocomplete pick, autocorrect after space, swipe word, Backspace wrapped in composition, Enter wrapped in composition, symbol-keyboard orphaned `/`, emoji, spacebar-swipe caret move, long-press accent replace, dictation burst, Samsung composition-for-everything, Enter-strips-trailing-space, stuck composition.
  - *Given* the fixtures directory, *When* the validator test counts files, *Then* count >= 14 and each has `source`.
- Report prints synthetic and device pass counts separately.
  - *Given* 21 synthetic and 0 device fixtures (the 14 scenarios above, the 5 link/session fixtures added by Story 1.2.4 and the 2 fixtures of Task 1.1.4e), *When* the suite runs, *Then* the summary line reads `synthetic 21/21, device 0/0` (acceptable for PR-A; device fixtures arrive via Story 4.1.3).
- Fixture lint: any fixture with `source:"device"` must carry `scrubbed: true` and a non-empty `signoff`, and every fixture is scanned for secret-shaped strings; otherwise the suite fails. A PR adding a device fixture must carry the owner sign-off in its description (checklist in Story 4.1.3).
  - *Given* a device fixture without `scrubbed:true`, *When* the suite runs, *Then* it fails naming the file; a synthetic fixture containing `ghp_` fails the secret scan.
**Files**: `web-app/src/lib/terminal/mobileInput/replay.test.ts`, `replayHarness.ts`, `__fixtures__/*.json`, `fixtureSchema.ts`

##### Task 1.1.4a: `replayHarness.ts` (ModelTerminal + trace runner) and `fixtureSchema.ts` validator; step kinds include `linkDown`, `linkUp` and `sessionSwitch` (S)
- Files: `replayHarness.ts`, `fixtureSchema.ts`

##### Task 1.1.4b: Author synthetic fixtures, batch 1 of 3: plain typing, autocomplete pick, autocorrect after space, swipe word, Backspace wrapped in composition (S)
- Sources: xterm.js #3600, Codeman PR 65, CodeMirror Android notes, W3C 2017 threads; each fixture states its source URL in a `notes` field. Batches 1-3 (Tasks 1.1.4b, 1.1.4f, 1.1.4g) are independent file sets and may run in parallel workers; the 14 scenarios are split 5/5/4 so no task lists more than 5 files.
- Files: `__fixtures__/gboard-typing-hello.json`, `gboard-autocomplete-pick.json`, `gboard-autocorrect-teh-the.json`, `gboard-swipe-word.json`, `gboard-backspace-wrapped-in-composition.json`

##### Task 1.1.4f: Author synthetic fixtures, batch 2 of 3: Enter wrapped in composition, symbol-keyboard orphaned `/`, emoji, spacebar-swipe caret move, long-press accent replace (S)
- Same source and `notes` rules as Task 1.1.4b.
- Files: `__fixtures__/gboard-enter-wrapped-in-composition.json`, `gboard-symbol-orphaned-slash.json`, `gboard-emoji.json`, `gboard-spacebar-swipe-caret.json`, `gboard-longpress-accent-replace.json`

##### Task 1.1.4g: Author synthetic fixtures, batch 3 of 3: dictation burst, Samsung composition-for-everything, Enter-strips-trailing-space, stuck composition (S)
- Same source and `notes` rules as Task 1.1.4b.
- Files: `__fixtures__/gboard-dictation-burst.json`, `samsung-composition-for-everything.json`, `gboard-enter-strips-trailing-space.json`, `gboard-stuck-composition.json`

##### Task 1.1.4c: `replay.test.ts` table test and per-source summary (S)
- Files: `replay.test.ts`

##### Task 1.1.4e: Backspace-on-empty and hardware-keyboard fixtures (S)
- `backspace-on-empty-after-commit.json` (pre-mortem P1-3): word committed, textarea reset, Backspace arrives in the shape recorded by Task 0.1.2b/0.1.3b; expected one `\x7f`, model line loses one character.
- `hardware-keyboard-keydown.json` (pre-mortem P3-11): real keyCodes, `isComposing:false`, letters plus Backspace plus Enter from a Bluetooth keyboard on the phone; expected bytes equal to the hardware path, with no double-send from the `beforeinput` that follows each keydown.
- Both carry `source:"synthetic"` and a `notes` URL or spike reference.
- Files: `__fixtures__/backspace-on-empty-after-commit.json`, `__fixtures__/hardware-keyboard-keydown.json`, `replay.test.ts`

##### Task 1.1.4d: Fixture lint test: `device` requires `scrubbed:true` + `signoff`, secret-pattern scan (S)
- The lint runs over EVERY committed file under the corpus dir (glob, not an allow-list) as part of the jest run in CI, so it is a CI check: it fails the build if any committed fixture has `source:"device"` without `scrubbed:true` AND a non-empty `signoff`. This is a soft guard (self-attested fields), stated as such in the fixture schema header.
- Files: `fixtureSchema.ts`, `fixtureLint.test.ts`

### Epic 1.2: DOM adapter
**Goal**: Normalize real DOM events into `MobileInputEvent` and execute commands.

#### Story 1.2.1: Hidden textarea, attribute policy, styling, platform gate
**As a** user, **I want** the soft keyboard to keep autocorrect and suggestions, **so that** phone typing feels native.
**Acceptance Criteria**:
- The textarea carries `autocapitalize="none"`, `autocomplete="off"`, `enterkeyhint="send"`, `role="textbox"`, `aria-multiline="true"`, `aria-label="Terminal input"`, `data-testid="mobile-input-textarea"`, and does NOT set `autocorrect="off"` or `spellcheck="false"`.
  - *Given* a container, *When* `createInputElement()` runs, *Then* `getAttribute("spellcheck")` is `null` and `getAttribute("autocapitalize")` is `"none"`.
- Secret-entry limitation is explicit (ADR-008): the layer cannot detect echo-off prompts, so Gboard's suggestion strip and personal dictionary can see text typed at `sudo`/`ssh`/token prompts. Mitigations in v1 are `autocomplete="off"` and `autocapitalize="none"` only; `autocorrect="off"` is applied only if a reliable echo-off signal is found by Task 1.2.1c, otherwise it stays unset and the limitation is documented in the reference page and shown as an owner-visible risk in the PR description.
  - *Given* the attribute policy table in `policy.ts`, *When* `attributesFor({echoOff:false})` and `attributesFor({echoOff:true})` run, *Then* the first omits `autocorrect`, the second (only reachable if a signal exists) sets `autocorrect="off"`, `spellcheck="false"`; both include `autocomplete="off"`.
- Style is a vanilla-extract class (1px, opacity 0, `pointer-events:none`, not `display:none`, not `aria-hidden`); only the cursor-row offset is set via a CSS variable.
  - *Given* the element, *When* inspected, *Then* `className` contains the vanilla-extract hash class (jest `.css.ts` handling confirmed against the repo jest config in this task) and `style.display` is empty.
- `platform.ts` exports TWO predicates with distinct jobs. `shouldAttachMobileInput()` (layer attach and `resolveRenderer` ONLY): Android Chrome-class, touch-primary: `matchMedia("(pointer: coarse)").matches && !matchMedia("(hover: hover)").matches && isAndroid()`, where `isAndroid()` is `navigator.userAgentData?.platform === "Android"` with UA-string `/Android/` fallback (Chrome, Firefox and Samsung Internet on Android all match; Chrome "Request desktop site" drops Android from the UA and opts out; both documented). iOS/iPadOS Safari and touch laptops with a hardware keyboard are explicitly NOT attached in v1. `isTouchPrimary()` is the generic coarse-pointer predicate (`matchMedia("(pointer: coarse)").matches`, today's semantics) used by `XtermTerminal.tsx` for the link-tap menu, selection handles and gestures on every coarse-pointer device.
  - *Given* a mocked environment table, *When* `shouldAttachMobileInput()` runs, *Then* `true` for {coarse, no-hover, Android}; `false` for {coarse, no-hover, iPhone UA}, {coarse, no-hover, iPadOS-desktop-UA with touch points}, {coarse + hover:hover Windows touch laptop}, {fine pointer Android with mouse}, {desktop}.
  - *Given* an Android phone over a plain-HTTP LAN origin (no `navigator.userAgentData`, which exists only in secure contexts), *When* `shouldAttachMobileInput()` runs, *Then* it falls back to the `/Android/` UA test and is `true`; *Given* an installed-PWA / standalone display mode on Android (`display-mode: standalone`), *Then* it is `true`; both rows are in the platform test table.
  - *Given* the same table, *When* `isTouchPrimary()` runs, *Then* `true` for every coarse-pointer row (Android, iPhone, iPadOS, Windows touch laptop) and `false` for fine-pointer rows.
**Files**: `web-app/src/lib/terminal/mobileInput/policy.ts`, `platform.ts`, `platform.test.ts`, `mobileInput.css.ts`, `inputElement.ts`, `inputElement.test.ts`

##### Task 1.2.1a: `policy.ts` attribute set and `platform.ts` `shouldAttachMobileInput` plus generic `isTouchPrimary` (S)
- Files: `policy.ts`, `platform.ts`, `platform.test.ts`

##### Task 1.2.1b: `mobileInput.css.ts` + `inputElement.ts` + test (S)
- Files: `mobileInput.css.ts`, `inputElement.ts`, `inputElement.test.ts`

##### Task 1.2.1d: Import-graph architecture test (XS)
- `mobileInput/architecture.test.ts` (CON-1): the reducer files `composition.ts`, `rewrite.ts`, `keymap.ts` and `deliveryLedger.ts` import no DOM, xterm or ghostty module (scan import statements; negative fixture proves the scan catches a violation). Keeps the layer renderer-agnostic.
- Files: `architecture.test.ts`

##### Task 1.2.1c: Echo-off signal feasibility note and attribute-policy variants (XS)
- Check whether either renderer exposes a reliable echo-off / password-prompt signal (DECSET, cursor-hidden heuristics are NOT reliable); record the result in `spike-findings.md`; implement `attributesFor({echoOff})` as data only. Absent a reliable signal, nothing toggles at runtime.
- Files: `policy.ts`, `spike-findings.md`

#### Story 1.2.2: `attachMobileInput` event normalization and execution
**As a** renderer, **I want** one function that turns textarea events into bytes on my sink, **so that** I need no IME code.
**Acceptance Criteria**:
- Returns `null` when `beforeinput` is unsupported or `shouldAttachMobileInput()` is false, adding zero listeners and zero DOM nodes.
  - *Given* a desktop jsdom (fine pointer), an iPhone UA, and a touch laptop, *When* `attachMobileInput` is called in each, *Then* result is `null` and `addEventListener` was not called on the container.
- `opts.onData` is typed `(bytes: TerminalBytes, seq: UnitSeq) => SendResult | void` from the start (Story 3.1 only adds queue behavior, no signature rework); the executor assigns a monotonic `seq` to every atomic unit and records it in the `DeliveryLedger` (Story 1.2.4).
  - *Given* an `onData` returning `"sent"`, *When* the executor runs, *Then* it type-checks and the result is observed by the executor.
- Events are normalized and reduced; `Emit` commands call `opts.onData` once per step with DEL and text in one string.
  - *Given* a jsdom textarea fed the `gboard-autocorrect-teh-the` event stream, *When* events dispatch, *Then* `onData` receives `"t"`,`"e"`,`"h"`, then once `"\x7f\x7fhe"`.
- `isReadOnly()` true suppresses emission and sets `inputmode="none"`.
  - *Given* `isReadOnly` returns true, *When* `beforeinput insertText "x"` dispatches, *Then* `onData` is not called.
- The layer never `preventDefault`s `keydown` 229 or `insertCompositionText`; it `preventDefault`s mapped physical keys (Enter, Backspace, Tab, Esc, arrows, Ctrl-x with `keyCode != 229`).
  - *Given* a physical `keydown{key:"Enter",keyCode:13}`, *When* dispatched, *Then* `defaultPrevented` is true and `onData("\r")` once, with no second `\r` from `beforeinput`.
- No double-send with the renderer's own input: while the layer is attached, neither renderer's own `onData` fires for the same keystroke, including a hardware keyboard on the phone and focus bouncing back to a renderer textarea.
  - *Given* a layer attached over a fake renderer handle exposing its own `onData` spy, *When* a keystroke dispatches on the layer textarea and then `focus` bounces to the renderer textarea and another keystroke dispatches, *Then* the sink receives each keystroke's bytes exactly once.
- Handler exceptions never escape; `onAnomaly` is called and `console.error` fires once per kind; `console.error`, `onAnomaly` and `cm.*` payloads never contain typed `data`.
  - *Given* a throwing `onData` after typing `"secret"`, *When* an event dispatches, *Then* no uncaught error, `onAnomaly("unrecognized_sequence")`, and every `console.error` argument string excludes `secret`.
- Latency budget (NFR, Task 1.2.2d): the path from a `beforeinput` event to `opts.onData` is synchronous, with no timer and no `requestAnimationFrame` per keystroke outside a live composition, so the added latency is below one animation frame.
  - *Given* fake timers and a rAF spy, *When* `beforeinput insertText "a"` dispatches, *Then* `onData` has been called before any timer advances and no timer or rAF was scheduled.
- Textarea value is reset to `""` only at commit boundaries, never during a live composition.
  - *Given* a live composition, *When* a `compositionupdate` dispatches, *Then* `textarea.value` is unchanged.
**Files**: `web-app/src/lib/terminal/mobileInput/attachMobileInput.ts`, `normalize.ts`, `inputSource.ts`, `attachMobileInput.test.ts`

##### Task 1.2.2a: `normalize.ts` and `inputSource.ts` (`TextareaSource`) (S)
- Files: `normalize.ts`, `inputSource.ts`

##### Task 1.2.2b: `attachMobileInput.ts` executor, watchdog, error guard, controller (M)
- Injectable clock and `requestAnimationFrame`-free design; tick events from an injected scheduler; `onData` returns `SendResult | void` from day one.
- Files: `attachMobileInput.ts`

##### Task 1.2.2c: jsdom adapter tests replaying 3 fixtures through real DOM events (S)
- Reuses the fixture loader from 1.1.4; not a second corpus.
- Files: `attachMobileInput.test.ts`

##### Task 1.2.2d: Latency-budget and desktop-zero-work tests (S)
- Requirement NFR coverage: synchronous `onData` per event (no timer, no rAF when not composing; watchdog tick only while composing), and zero listeners and zero DOM nodes on desktop. Reducer-to-`onData` microbenchmark is NOT a wall-clock threshold (flaky in this repo); assert call order under fake timers instead.
- Files: `attachMobileInput.test.ts`

#### Story 1.2.3: Focus ownership and gesture interplay
**As a** user, **I want** a tap on the terminal, in EITHER renderer, to open the soft keyboard on my layer's input, **so that** I can type after scrolling, selecting or a tab switch.
**Acceptance Criteria**:
- `controller.focus()` focuses the textarea synchronously (no timer, rAF or promise).
  - *Given* an attached controller, *When* `focus()` is called inside a click handler, *Then* `document.activeElement` is the textarea in the same tick.
- A layer-owned tap-to-focus entry (`controller.bindTapTarget(el)`) is installed on the VISIBLE terminal surface (the renderer container, never the 1px `pointer-events:none` textarea) and calls `controller.focus()` synchronously inside the user gesture. It does not rely solely on `pointerup`: it binds `pointerup` (touch), `touchend` and `click`, deduplicated so one tap focuses once, because `useTerminalGestures` may `preventDefault` touches and fire `pointercancel` on scroll. Used by BOTH renderers, so ghostty (which has no gesture hook) needs no per-renderer logic. The final event choice follows the Task 0.1.3b physical-phone result.
  - *Given* gesture handlers that `preventDefault` the touch and a scroll gesture that fires `pointercancel`, *When* a following tap lands on the surface, *Then* `document.activeElement` is `mobile-input-textarea` in that same tick, and a tap that ends in `pointercancel` followed by `click` still focuses once.
  - *Given* ghostty (fake handle) with the layer active, *When* a touch `pointerup` lands on the canvas container, *Then* `document.activeElement` is `mobile-input-textarea`; a mouse `pointerup` does nothing.
- Every other focus path goes through the single layer-aware entry: `TerminalOutput.tsx:1518` (`terminal?.focus()` on visibility change) and the pool dock focus call `handle.focus()`, and `useTerminalGestures` receives a `focusTerminal` callback instead of calling `t.focus()` (`:640`, `:660`).
  - *Given* the layer active and a visibility change, *When* the handler at `TerminalOutput.tsx:1518` runs, *Then* `handle.focus()` is called and `terminal.focus` is not.
- Blur-after-selection behavior (`useTerminalGestures.ts:683-689` blurs `terminal.textarea`) also blurs the layer textarea, and the xterm workaround is skipped when the layer is active.
  - *Given* the layer active and `endSelecting` fires, *When* the handler runs, *Then* the layer textarea is blurred and `terminal.textarea.blur` is not required.
- Toolbar buttons keep focus on the textarea (pointerdown `preventDefault`) and an assertion test pins that.
  - *Given* focus on the textarea, *When* the Ctrl toolbar button receives `pointerdown`, *Then* `activeElement` is still the textarea.
- On-device and e2e proof: a user tap leaves `mobile-input-textarea` as `document.activeElement` and the soft keyboard opens (Playwright activeElement assertion in Story 5.1.1; physical keyboard-open check in Story 5.1.4).
**Files**: `attachMobileInput.ts`, `web-app/src/lib/hooks/useTerminalGestures.ts`, `web-app/src/components/sessions/TerminalOutput.tsx`, `attachMobileInput.focus.test.ts`

##### Task 1.2.3a: Controller `focus/blur` and user-gesture contract (S)
- Files: `attachMobileInput.ts`

##### Task 1.2.3b: Route gesture, visibility (`TerminalOutput.tsx:1518`) and pool-dock focus through the handle; skip Android workaround when active (S)
- Files: `useTerminalGestures.ts`, `TerminalOutput.tsx`, `attachMobileInput.focus.test.ts`

##### Task 1.2.3c: Layer-owned `bindTapTarget` tap-to-focus entry on the visible surface, `pointerup`+`touchend`+`click` deduplicated, synchronous focus; used by both renderers (S, blocked by Task 0.1.3b)
- Files: `attachMobileInput.ts`, `attachMobileInput.focus.test.ts`

#### Story 1.2.4: Delivery-aware diff base and boundary invalidation (PR-A minimum, safe on a link flap)
**As a** user on a flaky link, **I want** the layer to only ever delete text it sent in the current connection and session, **so that** a reconnect, a lost frame or a session switch never eats my prompt or another session's text, even before the PR-B queue and chip exist.
**Acceptance Criteria**:
- **Honest `sent`.** `SendResult` `sent` means handed to the transport, not delivered (`pushMessage` returning gives no ack; a half-open socket can lose the frame). Delivery is trusted only within one `DeliveryEpoch`. The epoch is bumped by every transport boundary: an `isConnected` true->false transition, any reconnect, any `dropped`, a thrown send (caught by `handleError`), and a chunked paste (> 512 bytes) aborted midway. A thrown send returns `dropped`. A > 512-byte input returns synchronously and its remaining chunks run later in the `CHUNK_DELAY_MS` loop (`useTerminalFlowControl.ts` ~262-300), so a mid-paste abort cannot change the already-returned result: it is covered solely by the `isConnected`/`sessionId` invalidate triggers, and the paste unit is simply absent from `deliveredThroughSeq` (no async abort callback). Rewrite units (DEL + replacement, the insert-only tail) must be sent atomically regardless of chunking: `MAX_REGION` (64 graphemes) can exceed 512 bytes with ZWJ emoji, so the executor emits each rewrite unit as one `onData` call and `sendInput` must not chunk it; a region whose rewrite unit would exceed 512 bytes goes deferred.
  - *Given* `pushMessage` throws for a unit, *When* `sendInput` runs, *Then* it returns `dropped` (never `sent`); *Given* a 2 KiB paste where the link closes after the first chunk, *Then* `isConnected` true->false fires `invalidate('transport')` and the report's `deliveredThroughSeq` excludes the paste unit; *Given* a 64-ZWJ-emoji rewrite unit over 512 bytes, *Then* it is emitted as a single unit or the region goes deferred (never split across chunks).
- **One drop rule (replaces the earlier "ends as deferred" / "emit `lp`" ambiguity).** A `dropped` result or a transport boundary demotes the in-flight region to DEFERRED-AFTER-BOUNDARY: nothing further is emitted live; at the next commit (`compositionend` or the Enter flush of Story 1.1.2) the layer emits the INSERT-ONLY tail `final` minus `B`, where `B` is the text of the last unit known delivered (ledger `baseAfter`), and only when `final` starts with `B`; otherwise it emits zero bytes and reports anomaly `unrecognized_sequence`. It never emits a DEL for text from an earlier epoch. The same rule is stated in the reducer story (1.1.2), ADR-002 and ADR-004.
  - *Given* `"he"` sent (seq 1), `"l"` returned `dropped` (seq 2), link returns, region becomes `"help"`, *When* `compositionend{data:"help"}` arrives, *Then* the layer emits `"lp"` once (zero DELs) and the `ModelTerminal` reads `"help"`; nothing was emitted when the region changed to `"help"`.
  - *Given* the same start but the region autocorrects to `"world"`, *When* `compositionend` arrives, *Then* zero bytes are emitted, anomaly `unrecognized_sequence`, and the model line is never shorter than before the region.
- **Queue discard abandons, never re-sends.** After a queue discard (`onQueueDiscard(DeliveryReport)`, PR-B) the layer calls `invalidate('queue_discard')`: the region is abandoned (zero bytes for the rest of that composition, including its tail), the ledger is cleared and the next composition starts from an empty base. A `DeliveryReport` whose `deliveredThroughSeq` is not in the ledger takes the same path with anomaly `unrecognized_sequence`.
  - *Given* `"he"` flushed, `"l"` queued, queue discarded, region becomes `"help"`, *When* `compositionend` arrives, *Then* nothing is emitted for `"help"` and the next composition `"x"` emits `"x"` with zero DELs.
- **`controller.invalidate(reason)`** abandons the in-flight composition region (state to `Idle`, `AbandonedRegion` until `compositionend`, textarea value reset deferred to the next safe boundary, never inside a live composition, see the stale-region AC below), clears the ledger and the diff base, and bumps the epoch. Reasons `session`, `pool_reuse`, `visibility` (hidden longer than `HIDDEN_INVALIDATE_MS = 5000`) and `queue_discard` abandon fully (zero bytes, no tail, because the target prompt may differ or the units are gone). Reason `transport` applies the drop rule above (insert-only tail, the pre-boundary text kept as reference `B`). `useMobileInput` (Task 1.4.1a) calls it on a `sessionId` change, on every `isConnected` change in either direction (this covers a reconnect, including the connect-generation ref near `useTerminalStream.ts:232`; no `session_generation` field exists in the client or proto, so there is no separate trigger), on pool reuse of the terminal for another session, and on visibility-hidden longer than the bound. Residual: a session paused/restarted behind the same id with the socket staying up is not detected in PR-A.
  - *Given* `Composing{region:"hel",sent:"hel"}` for S1 and the `sessionId` prop changes to S2, *When* the next `compositionupdate` `"hel"` -> `"yo"` arrives, *Then* the sink receives zero bytes (no `\x7f\x7f\x7f` into S2) and the textarea value is empty.
  - *Given* `"hel"` sent, the socket closes (half-open: the frame was lost, screen shows `"$ he"`), *When* the region autocorrects to `"yo"` and commits, *Then* `B="hel"` is not a prefix of `"yo"`, zero bytes are emitted, and the model line never shrinks below the pre-region `"$ "` text.
- **Stale region stays silent (ADR-006).** While `AbandonedRegion` is set, a trailing `compositionend`, an orphaned `input` event (Degraded path, Story 1.1.2) and a Backspace `beforeinput` emit nothing from the stale region; the textarea reset is deferred to the next safe boundary (after `compositionend` or when no composition is live), never performed inside a live composition. A user Backspace in `Idle` still emits one `\x7f`.
  - *Given* `Composing{region:"hel"}` and `invalidate('session')`, *When* a Gboard-style trailing `compositionend{data:"hel"}`, an orphaned `input` and a Backspace `beforeinput` arrive before the reset, *Then* the sink receives zero bytes and the textarea is reset only after the composition ends (one variant of `session-switch-mid-composition.json`).
- **`no-DEL-across-boundary` invariant (the proof of the header claim).** For every replay fixture and in a property test over random link/session/IME event interleavings: the number of DELs emitted never exceeds the graphemes the layer emitted in the current epoch for the current session, and zero bytes cross a `session`-class `invalidate`. A failing interleaving fails CI.
  - *Given* random sequences of `linkDown`, `linkUp`, `sessionSwitch`, `compositionupdate` and `compositionend`, *When* replayed, *Then* the invariant holds for all of them (fast-check, fixed seed plus 200 runs).
- PR-A drop detection without a queue: `sendInput` returns `SendResult` for the disconnected case (`dropped`, reason `disconnected`), `read_only` (`dropped`), a thrown send and an aborted chunked paste (`dropped`), with no chip and no queue, and the result flows through the whole chain to the layer (Task 1.2.4b). PR-A shows no visible drop notice: the chip is PR-B, so the user sees only the existing disconnected UI (unchanged) and the counter `cm.input{event=dropped_disconnected}` is incremented.
  - *Given* PR-A only (queue sub-flag off), disconnected, `"teh"` typed (each keystroke `dropped`), link up, autocorrect to `"the"`, *When* `compositionend{data:"the"}` arrives, *Then* the prompt text `"$ "` is untouched, zero DELs, the line is `"$ the"`.
- Strict FIFO sink contract: `sendInput` must enqueue behind a non-empty queue (or a flush in progress) and never send ahead of queued units. In PR-A there is no queue, so this is asserted as a contract test on `sendInput` (a stub queue proves a live keystroke does not overtake); PR-B implements the queue behind the same contract.
  - *Given* queued units `["he"]` and the link returning, *When* the user types `"l"` before the flush completes, *Then* the sink receives `"he"` then `"l"` in that order (never `"l"` first).
- Replay cases (added to the corpus runner; five fixtures): `partial-delivery-then-drop.json`, `keystroke-after-reconnect-before-flush.json`, `session-switch-mid-composition.json` (composition live, `sessionSwitch`, next update emits zero bytes), `sent-then-close-then-autocorrect.json` (`"hel"` sent, `linkDown`, autocorrect to `"yo"`, line never shorter than `"$ "`), `enter-after-mid-region-drop.json` (`"l"` sent, `"s"` dropped, `linkUp`, Enter: stream is `"s"` then `"\r"`, line `"ls\r"`, because the pending region flushes its insert-only tail before `\r` per Story 1.1.2).
  - *Given* each fixture, *When* replayed with link and session steps, *Then* `finalText` equals the expected line and the `no-DEL-across-boundary` invariant holds.
- **Stop rule.** If two review rounds on this story each yield new blockers in the ledger or invalidate logic, simplify to "abandon on any boundary" (zero-tail) and delete the insert-only tail logic rather than patching again (Delivery Slicing).
**Files**: `attachMobileInput.ts`, `deliveryLedger.ts`, `deliveryLedger.test.ts`, `deliveryInvariant.property.test.ts`, `useMobileInput.ts`, `web-app/src/lib/hooks/useTerminalFlowControl.ts`, `useTerminalFlowControl.test.ts`, `web-app/src/components/sessions/TerminalOutput.tsx`, `web-app/src/lib/terminal/TerminalPool.tsx`, `replayHarness.ts`, `__fixtures__/partial-delivery-then-drop.json`, `__fixtures__/keystroke-after-reconnect-before-flush.json`, `__fixtures__/session-switch-mid-composition.json`, `__fixtures__/sent-then-close-then-autocorrect.json`, `__fixtures__/enter-after-mid-region-drop.json`

##### Task 1.2.4a: `sendInput` returns `SendResult`: disconnected/read-only, thrown send and mid-chunk-aborted paste are `dropped`; strict-FIFO contract test (S)
- Return type widened only; flag-independent in PR-A1 (no chip, no queue, `console.warn` retained). Existing callers unchanged; the existing `terminal-input-batching` cases pass unchanged (NFR: batching preserved).
- Files: `useTerminalFlowControl.ts`, `useTerminalFlowControl.test.ts`

##### Task 1.2.4b: Plumb `SendResult` through `deliverTerminalData` (`TerminalOutput.tsx:1049`), `handleTerminalData` (`:1069`), the pool `onData` callback (`:1426`) and the renderer `onDataRef`; chain test (S)
- Moved here from PR-B (formerly Task 3.1.1c). Callback type is the `OnData` type from Task 1.1.1a.
- Files: `TerminalOutput.tsx`, `TerminalPool.tsx`, `useMobileInput.ts`, `useTerminalFlowControl.test.ts`

##### Task 1.2.4c: `DeliveryLedger` in the layer: seq and epoch assignment, base-from-delivered-prefix, deferred-after-boundary insert-only tail, `AbandonedRegion` (M)
- Files: `deliveryLedger.ts`, `deliveryLedger.test.ts`, `attachMobileInput.ts`, `composition.ts`

##### Task 1.2.4d: Replay harness link-down/up and session-switch steps (no queue) and the five Story 1.2.4 fixtures (S)
- Files: `replayHarness.ts`, `replay.test.ts`, the five `__fixtures__/*.json` listed above

##### Task 1.2.4e: `controller.invalidate(reason)` with the `no-DEL-across-boundary` property test and the `sessionId`-change jsdom test (S)
- Files: `attachMobileInput.ts`, `attachMobileInput.invalidate.test.ts`, `deliveryInvariant.property.test.ts`


#### Story 1.2.5: Rewrite flicker budget and DeferredCommit feedback
**As a** phone user, **I want** an autocorrect or autocomplete rewrite to look like one change and a long dictation or swipe commit to show that something is happening, **so that** the pane never flashes a half-edited prompt and never looks dead while text is deferred.
**Acceptance Criteria**:
- A rewrite (the DELs plus the replacement text) is emitted as ONE `onData` call and sent as ONE `sendInput` (ADR-002), so the terminal can show at most one intermediate state. Measurable criterion: in the replay harness and in the e2e sampler, sampling the `ModelTerminal` / terminal buffer once per animation frame across an autocorrect (`teh` -> `the`, `helo wrld` -> `hello world`) shows at most ONE frame whose line differs from both the pre-rewrite and the post-rewrite text, and zero frames in which the prompt text before the composition region is shorter than it was before the rewrite.
  - *Given* fixture `gboard-autocorrect-teh-the` replayed with a per-frame sampler, *When* the rewrite executes, *Then* the sampled sequence is `teh` -> (at most one of `t`, `the`, `th`) -> `the`, and the count of distinct intermediate lines is <= 1.
  - *Given* the rewrite is delivered over the real transport in Playwright (Task 5.1.1e), *When* sampled each animation frame, *Then* the same bound holds for both renderers; a blank frame or a frame with shortened prompt text fails the test.
- The rewrite frame is emitted within one animation frame of the triggering `beforeinput` (the same budget as Task 1.2.2d: no timer, no rAF wait before `onData`).
  - *Given* fake timers and a `beforeinput insertReplacementText`, *When* it is dispatched, *Then* `onData` has been called with the full rewrite string before any timer or rAF callback runs.
- If the owner's device check (Task 5.1.4c) observes a visible flicker that the harness bound does not catch, the finding is recorded in `spike-findings.md` and a follow-up issue is filed (Task 6.1.2a list) for a preview/echo strategy; the harness bound above stays as the regression guard.
- DeferredCommit (a commit longer than 64 graphemes or containing a newline, such as dictation or a long swipe sequence, per `computeRewrite`) has a visible in-progress affordance, because the pane shows nothing until the commit lands: after 300 ms in the deferred state the chrome hook shows a one-line `role="status"` indicator "Typing... (N characters waiting)" above the toolbar; it is removed on commit (and when the commit is sent the pane update is the confirmation); no content is shown; reduced motion: no animation. It never shows for commits that complete in under 300 ms.
  - *Given* a 100-grapheme dictation burst held in `Deferred` and fake time advanced 300 ms, *When* rendered, *Then* the indicator text contains "100 characters"; *When* the commit is emitted, *Then* the indicator is removed on the same tick and `onData` carries the 100 graphemes once.
  - *Given* a deferred state that never commits, *When* the stuck-composition watchdog fires, *Then* the indicator is removed and the diagnostics ring records the watchdog.
**Files**: `web-app/src/lib/terminal/mobileInput/replayHarness.ts`, `replay.test.ts`, `attachMobileInput.test.ts`, `useMobileInputChrome.tsx`, `useMobileInputChrome.test.tsx`

##### Task 1.2.5a: Per-frame sampler in the replay harness and the "at most one intermediate state, no shortened prompt" assertions, plus the one-`onData`/one-frame latency test (S)
- Files: `replayHarness.ts`, `replay.test.ts`, `attachMobileInput.test.ts`

##### Task 1.2.5b: DeferredCommit in-progress indicator through the chrome hook (blocked by Task 1.4.1e) (S)
- Files: `useMobileInputChrome.tsx`, `useMobileInputChrome.test.tsx`

### Epic 1.3: Flag and renderer gate
**Goal**: Kill switch and a gate so ghostty is never used on the Android touch class without the layer.

#### Story 1.3.1: Server flag registration
**As an** operator, **I want** `mobile_input_layer` in the feature-flag panel, **so that** I can kill it live.
**Acceptance Criteria**:
- Flag appears in `knownFeatureFlags` with a description stating kill-switch semantics, that it affects only the Android class, the rollback timing (one load to persist the off value, a second to apply, or `?mobile_input=0` to apply at once). The `defaultValue` is conditional: `true` ONLY IF the pre-merge phone check on the isolated manual instance passes (Task 5.1.4c); otherwise `false`, with a per-scope override set for the owner. The commit carries `false` until Task 5.1.4c records a pass, and the final pre-merge commit flips it (and `MOBILE_INPUT_LAYER_DEFAULT`) to `true`.
  - *Given* a fresh server, *When* `ListFeatureFlags` is called, *Then* `mobile_input_layer` is present with the default recorded by Task 5.1.4c.
- The flag is read only through the flag store and per-scope overrides, never from an environment variable.
  - *Given* a lookalike env var set, *When* the flag is read, *Then* the value is unchanged (Task 1.3.1b).
- Per-scope override works like other flags.
  - *Given* a scope override `true` for workspace W, *When* the flag is read for W, *Then* `true`.
**Files**: `server/services/feature_flag_service.go`, `server/services/feature_flag_service_test.go`, `server/services/config_surface_proof_test.go`

##### Task 1.3.1a: Add flag + update tests that enumerate flags (S)
- Test files use `t.Parallel()`. Run `make registry-generate` and the config-surface test in this task (CI gate pre-check). The default starts `false` and is flipped by the Task 5.1.4c outcome.
- Files: `feature_flag_service.go`, `feature_flag_service_test.go`, `config_surface_proof_test.go`

##### Task 1.3.1b: Flag-not-read-from-env test (XS)
- `TestMobileInputFlags_ShouldNotBeReadFromEnvironment_When_EnvVarSet` with an injected env reader so it keeps `t.Parallel()` (no `t.Setenv`).
- Files: `feature_flag_service_test.go`

#### Story 1.3.2: Client flag, `resolveRenderer`, split touch predicates
**As the** owner, **I want** ghostty never chosen on the Android touch class when the layer is off, **so that** the dead keyboard cannot ship, with one attach predicate and one generic coarse-pointer predicate, each used for its own question.
**Acceptance Criteria**:
- `resolveRenderer({ghosttyFlag, mobileLayerFlag, attachable})` (`attachable = shouldAttachMobileInput()`) covers all 8 combinations.
  - *Given* `{ghosttyFlag:true,mobileLayerFlag:false,attachable:true}`, *When* resolved, *Then* `"xterm"`; `{true,true,true}` `"ghostty"`; `{true,false,false}` `"ghostty"` (iOS and desktop unchanged); `{false,*,*}` `"xterm"`.
- `TerminalPool.tsx` uses it instead of the inline ternary (no behavior change on non-Android or when the layer flag is on).
  - *Given* ghostty on, desktop, *When* the pool picks a renderer, *Then* `GhosttyTerminal` as before.
- Preload default is a single constant `MOBILE_INPUT_LAYER_DEFAULT` matching the server `defaultValue` (per the Task 5.1.4c outcome); persisted `"0"` or `"1"` overrides it.
  - *Given* localStorage `ssq:flag:mobile_input_layer="0"`, *When* preloaded, *Then* `false`; `"1"` -> `true`; nothing persisted -> the constant; a test asserts the constant equals the server default read from the Go-side flag list fixture.
- `?mobile_input=0|1` query param (Task 1.3.2e): read by the preload before first mount, applies in the SAME page load, and is stored per origin under a SEPARATE override key `ssq:flag-override:mobile_input_layer` (`"0"`/`"1"`) that `persistFlags` never touches; absent, nothing changes. The override beats both the persisted server value and the default (preload and `useMobileInputLayerActive`) until the user flips the flag from the feature-flag panel, which clears it (`recordFlagWrite`) so the server value governs again. (Correction 2026-11-08: the first implementation wrote `ssq:flag:mobile_input_layer`, which the next `GetFeatureFlags` refresh, up to ~30 s later, overwrote, so the param silently lasted one refresh.) A garbage value is ignored. Documented in the how-to with the per-origin note.
  - *Given* the flag persisted `"1"` and the page opened with `?mobile_input=0`, *When* the preload runs, *Then* it returns `false` in that load, `ssq:flag-override:` is `"0"` (`ssq:flag:` stays `"1"`), and the terminal mounts xterm with no layer; `?mobile_input=1` after a persisted `"0"` returns `true` in that load.
  - *Given* the override `"0"` and a later `GetFeatureFlags` reporting the flag enabled, *When* `persistFlags` runs and the preload is read again (even without the param), *Then* the override key is unchanged and the preload still returns `false`; *Given* the user flips `mobile_input_layer` in the feature-flag panel, *Then* the override key is removed and the server value governs.
- Flag off on the Android class is never silent: when `shouldAttachMobileInput()` is true and the layer flag resolves false, the chrome hook shell shows a one-line dismissible `role="status"` hint in plain language ("Phone keyboard support is off, so typing may not work well. You can turn it on in the feature flags panel.") and a console line (Task 1.4.1e). The copy never mentions a query parameter or a flag identifier; `?mobile_input=0|1` is documented only in `docs/how-to/debug-phone-typing.md` (Task 5.1.2b). This is the one surface that renders with the flag off.
  - *Given* Pixel 7 emulation, `mobile_input_layer=false`, *When* the terminal mounts, *Then* the hint is present, contains no `?` query text and no `mobile_input` identifier; on desktop and iOS it is absent.
- `persistFlags` writes the kill switch both ways: it stores `"1"`/`"0"` for `mobile_input_layer` (not only removes the key when off, as ghostty does), so the preload sees an off flag.
  - *Given* the server flag list reports `mobile_input_layer` disabled, *When* `persistFlags` runs, *Then* `ssq:flag:mobile_input_layer` is `"0"` and `preloadFlagValue` returns `false`; reported enabled -> `"1"`; the value read is the workspace-scope effective value (confirmed as ghostty already does).
- `useMobileInputLayerActive()` = flag && `shouldAttachMobileInput()`.
  - *Given* flag true and an iPhone UA, *When* the hook renders, *Then* `false`.
- Two predicates, two questions: `XtermTerminal.tsx` `isTouchPrimary` (line ~749, `(pointer: coarse)` only; it also drives the link-tap menu at ~757, selection handles at ~973 and handle-drag setup at ~1049) now imports the generic `isTouchPrimary()` from `platform.ts` with IDENTICAL semantics, so every coarse-pointer device (iPhone, iPad, touch laptop, Android) keeps today's behavior. `shouldAttachMobileInput()` is used ONLY for layer attach and `resolveRenderer`. A grep test fails if a JS `matchMedia('(pointer: coarse)')` call appears in the terminal scope (`web-app/src/lib/terminal/**` and `web-app/src/components/sessions/{XtermTerminal,GhosttyTerminal,TerminalOutput}.tsx` plus `lib/terminal/TerminalPool.tsx`) outside `platform.ts`; the generic helper is allowed. The scan is deliberately scoped to the terminal files because the repo has legitimate JS uses elsewhere (verified with `grep -rnE "matchMedia\(" web-app/src`: `lib/hooks/useCoarsePointer.ts:15`, `lib/hooks/useOpenBuffers.ts:26`, `lib/hooks/useFileTransport.ts:27`, `lib/files/analytics/filesAnalytics.ts:47`, `lib/window/useWindowFocusMemory.ts:21`, `components/backlog-stuck/StuckItem.tsx:129` (`"(hover: none), (pointer: coarse)"`), `components/sessions/ScrollModeChip.tsx:27` (`"(any-pointer: coarse)"`)), and `*.css.ts` `@media` rules are not JS calls. The regex is `matchMedia\(\s*['"]\(pointer:\s*coarse\)['"]\s*\)` (exact single-condition query), so `StuckItem.tsx` and `ScrollModeChip.tsx` could not match even if the scope grew.
  - *Given* the terminal-scope source files, *When* the grep test runs, *Then* the only match of the pinned regex is in `platform.ts`, and `XtermTerminal.tsx` imports `isTouchPrimary` from it; a fixture string `matchMedia("(any-pointer: coarse)")` and `matchMedia("(hover: none), (pointer: coarse)")` in the test's negative cases do NOT match.
  - *Given* an iPhone UA with `pointer: coarse`, *When* `XtermTerminal` mounts, *Then* the link-tap menu, long-press selection and selection handles behave as before this change (`isTouchPrimary()` true) while no `mobile-input-textarea` exists.
**Files**: `web-app/src/lib/terminal/rendererPolicy.ts`, `rendererPolicy.test.ts`, `web-app/src/lib/terminal/TerminalPool.tsx`, `web-app/src/lib/features/flagStore.ts`, `web-app/src/lib/terminal/useMobileInputLayerActive.ts`, `web-app/src/components/sessions/XtermTerminal.tsx`

##### Task 1.3.2a: Extract `resolveRenderer` (Refactor-first) and use in `TerminalPool.tsx` (S)
- Files: `rendererPolicy.ts`, `rendererPolicy.test.ts`, `TerminalPool.tsx`

##### Task 1.3.2b: `flagStore.ts` constant + pre-load; `useMobileInputLayerActive` hook (S)
- Files: `flagStore.ts`, `useMobileInputLayerActive.ts`, `flagStore.test.ts`

##### Task 1.3.2c: Point `XtermTerminal.tsx:749` `isTouchPrimary` at the generic `platform.ts` helper (same semantics) and add the scoped grep test (terminal files only, pinned regex) forbidding `matchMedia('(pointer: coarse)')` outside `platform.ts` (S)
- Do NOT route it through `shouldAttachMobileInput`. Files: `XtermTerminal.tsx`, `platform.test.ts`

##### Task 1.3.2d: `persistFlags` writes `"1"`/`"0"` for the kill switch + test (XS)
- Files: `flagStore.ts`, `flagStore.test.ts`

##### Task 1.3.2e: `?mobile_input=0|1` same-load override in the preload, persisted per origin, with tests (S)
- Test that the preload honours the param before the first mount, that a garbage value is ignored, that the override survives `persistFlags`, that `recordFlagWrite` clears it, and that `useMobileInputLayerActive` honours it ahead of the persisted server value.
- Files: `flagStore.ts`, `useMobileInputLayerActive.test.tsx`, `flagStore.test.ts`, `rendererPolicy.test.ts`

### Epic 1.4: Renderer integration
**Goal**: Both renderers consume the layer through one shared hook.

#### Story 1.4.1: Shared `useMobileInput` hook and renderer wiring
**As a** renderer, **I want** one hook that attaches the layer to each visible/docked terminal (one holds focus), **so that** neither renderer copies the effect.
**Acceptance Criteria**:
- Attach trigger is "terminal visible/docked AND `useMobileInputLayerActive()`", NOT focus, so `controller.bindTapTarget` exists before the first tap. Focus changes only move DOM focus and `inputmode` between the layer textareas; at most one visible terminal owns focus, and a terminal that is hidden/undocked disposes its layer.
  - *Given* two docked terminals, *When* terminal B gains focus, *Then* A's layer textarea is `inputmode="none"` and unfocused, B's is focused, and a terminal that is hidden or undocked has its controller `dispose` called.
  - *Given* an unfocused docked ghostty terminal (layer active), *When* the first touch `pointerup` lands on it, *Then* `bindTapTarget` already exists, `document.activeElement` is `mobile-input-textarea` in that same tick, and ghostty's own element did not receive the tap as focus.
- Layer bytes go directly to `onDataRef` (bypassing `isMouseReport`).
  - *Given* layer output `"\x1b[M abc"`, *When* emitted, *Then* the sink receives it unfiltered.
- On xterm, `.xterm-helper-textarea` gets `inputmode="none"` and `tabindex="-1"` while attached, restored on dispose.
  - *Given* the layer attached, *When* inspecting the helper textarea, *Then* those attributes are present; after `dispose`, absent.
- Ghostty mount point: the layer textarea is a SIBLING of, and outside, the element passed to ghostty-web `open()` (that element carries ghostty's blanket `beforeinput` `preventDefault` and keydown handlers). Ghostty also creates its own hidden `<textarea>`; on touch with the layer active it is set `inputmode="none"`, `tabindex="-1"` and blurred.
  - *Given* the layer mounted for ghostty (fake `open()` recording its element), *When* `insertText` is dispatched on the layer textarea, *Then* the open() element's listeners are not invoked, `event.defaultPrevented` is false, and the layer textarea is not a descendant of the open() element; ghostty's own textarea has `inputmode="none"`.
- Imperative handle `focus` calls `controller?.focus() ?? term.focus()`.
  - *Given* the layer active, *When* the handle's `focus()` runs, *Then* the layer textarea is `activeElement`.
- `TerminalOutput` passes `getModifiers`/`consumeModifiers` from its sticky state and `isReadOnly` from `readOnlyRef`; its read-only hint (existing, `TerminalOutput.tsx:1068-1080`) gains `role="status"` and `aria-live="polite"` (UX gap) and is announced once.
  - *Given* sticky Ctrl armed in `TerminalOutput`, *When* soft-keyboard `"c"` is typed, *Then* the sink gets `"\x03"` and the Ctrl state clears; *Given* a tap in a read-only session, *When* the hint renders, *Then* it has `role="status"`.
- Desktop/iOS path unchanged: no listeners, no extra DOM.
  - *Given* a non-Android device, *When* either renderer mounts, *Then* no `mobile-input-textarea` exists.
**Files**: `web-app/src/lib/terminal/mobileInput/useMobileInput.ts`, `web-app/src/components/sessions/XtermTerminal.tsx`, `GhosttyTerminal.tsx`, `TerminalOutput.tsx`, `useMobileInput.test.tsx`

##### Task 1.4.1a: `useMobileInput.ts` hook: attach when visible/docked (not on focus), focus-owner logic, `bindTapTarget` installed at attach; calls `controller.invalidate` on `sessionId` change, every `isConnected` change (covers reconnect; no `session_generation` exists), pool reuse and visibility-hidden > `HIDDEN_INVALIDATE_MS` (S)
- Files: `useMobileInput.ts`, `useMobileInput.test.tsx` (one test per invalidate trigger, with a jsdom composition live)

##### Task 1.4.1b: Wire `XtermTerminal.tsx` (helper-textarea suppression, handle focus) (M)
- Files: `XtermTerminal.tsx`

##### Task 1.4.1c: Wire `GhosttyTerminal.tsx` (textarea as sibling outside the `open()` element, ghostty's own textarea inert on touch) (S)
- Files: `GhosttyTerminal.tsx`

##### Task 1.4.1d: Pass modifiers and readOnly from `TerminalOutput.tsx`, read-only hint `role="status"`; hook tests (S)
- Files: `TerminalOutput.tsx`, `useMobileInput.test.tsx`

##### Task 1.4.1e: `useMobileInputChrome` hook SHELL (PR-A1): returns `{fragment, callbacks}` (initially the Story 1.1.3 "multi-line paste flattened" hint and the Story 1.3.2 flag-off hint; Story 2.3.1 adds the minimal diagnostics panel), and `TerminalOutput.tsx` gains its single hook call and fragment slot (S)
- PR-B/C/D only extend this hook; none creates it. Test mounts the hook alone.
- Files: `web-app/src/lib/terminal/mobileInput/useMobileInputChrome.tsx`, `useMobileInputChrome.test.tsx`, `TerminalOutput.tsx`

##### Task 1.4.1f: Read-only hides the extra-keys row and Compose toggle (XS)
- UX-13 (extra-keys half): `CMP/__tests__/TerminalOutput.readOnly.test.tsx` verifies the existing behavior that the extra-keys row does not render when the session is read-only, or adds it if absent (the Compose-toggle half is Task 4.2.1d).
- Files: `TerminalOutput.tsx` (only if behavior is missing), `__tests__/TerminalOutput.readOnly.test.tsx`

#### Story 1.4.2: One parametrized integration test over both renderers
**As a** maintainer, **I want** one test proving both renderers deliver identical bytes for the corpus, **so that** a swap regression is caught without tripping jscpd.
**Acceptance Criteria**:
- Test the layer against a FAKE renderer handle: the shared setup helper mocks the `ghostty-web` module once (a fake `Terminal` exposing `open/onData/focus/dispose`, `init()` resolving immediately; no WASM or canvas in jsdom) and uses the real `XtermTerminal` wrapper over a stubbed xterm. Real ghostty-web behavior (focus, IME, keydown 229 handling) is covered only by the Playwright spec (Story 5.1.1), and this test's header says so.
  - *Given* `describe.each(["xterm","ghostty"])` with the shared mock, *When* fixture `gboard-typing-hello` events dispatch on the layer textarea, *Then* sink bytes are `"hello"` for both and the fake ghostty `open()` element received no layer events.
- Test file shares one setup helper (no duplicated `jest.mock` blocks per renderer).
  - *Given* `bun run lint:duplicates` in `web-app/`, *When* run, *Then* threshold 0.14% not exceeded.
**Files**: `web-app/src/components/sessions/__tests__/mobileInputRenderers.test.tsx`, `web-app/src/components/sessions/__tests__/mobileInputTestSetup.ts`

##### Task 1.4.2a: Parametrized renderer integration test with the fake-ghostty mock (M)
- Files: `mobileInputRenderers.test.tsx`, `mobileInputTestSetup.ts`

##### Task 1.4.2b: ghostty-web version-pin test (XS)
- Pre-mortem P2-5. A jest test reads the installed `ghostty-web` version and fails with an instruction to re-run the Story 0.1.1 greps (keyCode 229 early return, blanket `beforeinput` preventDefault, own-textarea listeners) on any version other than the verified one. The ghostty-web Dependabot/renovate group stays labelled.
  - *Given* ghostty-web 0.4.0 installed, *When* the test runs, *Then* it passes; *Given* a bumped version, *Then* it fails naming Story 0.1.1 and Task 5.1.1d.
- Files: `web-app/src/lib/terminal/ghosttyVersionPin.test.ts`

#### Story 1.4.3: Chrome stack budget and minimum usable terminal rows
**As a** phone user in landscape or on a short phone, **I want** the terminal to stay usable with the toolbar, compose bar, status chip and keyboard all present, **so that** new chrome never squeezes the pane to nothing.
**Acceptance Criteria**:
- Layout rule (the guarantee): the extra-keys toolbar, compose bar and chip are `flex-shrink: 0` children of the existing sticky-flex shell (`TerminalOutput` column driven by `--viewport-height`, `mobile-ux-improvements` ADR-003), never `position: fixed`; the terminal container is `flex: 1; min-height: 0` with a floor of `MIN_TERMINAL_ROWS_FLOOR = 3` rows. A pure `chromeBudget({viewportHeightPx, cellHeightPx, toolbarPx, composeOpen, chipVisible})` returns the density: `comfortable` when the terminal keeps at least `MIN_TERMINAL_ROWS_COMFORT = 5` rows, else `compact`. In `compact`, in priority order: (1) the chip collapses to a 44x44 icon-plus-count badge inside the toolbar row that expands to an overlay (over the terminal, not in flow) on tap, (2) the compose bar is one line (44px) with the draft scrollable, (3) if the floor of 3 rows is still violated the compose bar auto-collapses (draft kept) and the S4/S6 hints queue instead of rendering. Toolbar is never hidden. The budget input uses the viewport source chosen by Story 0.1.5 and treats "keyboard height unknown" modes (floating/split) as comfortable-unless-measured.
  - *Given* `chromeBudget` inputs for 360x340 visual viewport (portrait with keyboard), 17px cells, toolbar 44, compose open and chip visible, *When* evaluated, *Then* density is `comfortable` and rows >= 5; for 740x170 (landscape with keyboard) *Then* density is `compact`, the chip is the toolbar badge, and rows >= 3.
- Minimum terminal rows matrix, asserted with Playwright on viewport emulation (a proxy for the real keyboard; real heights are an owner-device row, Task 5.1.4c): with toolbar + compose bar + chip + keyboard-sized viewport, the terminal shows at least 5 full rows at 360x340 and 320x300, and at least 3 full rows at 740x170 (landscape) and at 360x240 (short phone with a tall keyboard); measured as `floor(terminalContainerHeight / cellHeight)` read from the renderer.
  - *Given* each viewport above and all three surfaces visible, *When* measured, *Then* rows >= the stated minimum and the cursor row is inside the visible viewport.
- 360px width: the chip with message, Reconnect (44px) and Dismiss (44px) lays out without horizontal page scroll, wraps its text to at most two lines, and no two interactive boxes overlap or are closer than 8px; the chip never covers the compose bar (stacking order toolbar, compose bar, chip from the bottom; chip and compose collision height is bounded by the budget rule above).
  - *Given* 360x640, the chip in the Disconnected state and the compose bar open, *When* `boundingBox()` is read for chip, Reconnect, Dismiss, compose textarea and toolbar, *Then* no horizontal scroll exists, buttons are >= 44x44, boxes do not intersect and gaps are >= 8px.
**Files**: `web-app/src/lib/terminal/mobileInput/chromeBudget.ts`, `chromeBudget.test.ts`, `useMobileInputChrome.tsx`, `InputStatusChip.tsx`, `TerminalOutput.tsx` (layout slot only), `tests/e2e/mobile-input-layer.spec.ts`

##### Task 1.4.3a: `chromeBudget.ts` pure function, compact-density rules in the chrome hook and chip badge variant (blocked by Story 0.1.5 and Task 1.4.1e) (S)
- Files: `chromeBudget.ts`, `chromeBudget.test.ts`, `useMobileInputChrome.tsx`, `InputStatusChip.tsx`

##### Task 1.4.3b: Playwright matrix for minimum rows, 360px chip/compose layout and cursor-row-visible (S)
- Files: `tests/e2e/mobile-input-layer.spec.ts`

#### Story 1.4.4: Live-region contract and TalkBack typed-character announcement
**As a** TalkBack user, **I want** the characters I type and the layer's status messages announced exactly once and at the right urgency, **so that** a write-only terminal is not silent and notices do not talk over each other.
**Acceptance Criteria**:
- Two persistent live regions per terminal mount, rendered before any content changes: a polite region (`role="status"`, `aria-live="polite"`) and an assertive region (`role="alert"`). Classification (the single source for `design/ux.md` section 0): POLITE = chip states (Disconnected, Reconnecting, Reconnected/sent N, Still disconnected, keys not sent), read-only hint, flag-off hint, paste-flattened hint, IME-fallback notice and compose offer, DeferredCommit indicator, "Report copied", committed-text echo. ASSERTIVE = discard notices (overflow, TTL expiry, session change: typed content was lost) and the "Sending is stuck" final state (Task 3.1.5b). Rule: assertive only when typed content was lost or may be incomplete.
  - *Given* each message in the table, *When* it renders, *Then* its text is written into exactly the region named for it and the other region's text is unchanged.
- No double-fire: a message announces once per state change. The chip, hint and notice components do not carry their own `role`; they write through the chrome hook's `announce(kind, text, dedupeKey)`, which skips a write whose `dedupeKey` equals the region's current content and clears-then-sets on replacement. A chip that re-renders with the same state does not re-announce; a newer message replaces the older one in the same region; an assertive message does not also appear in the polite region.
  - *Given* a Disconnected chip re-rendered 5 times with an unchanged state, *When* the region's text mutations are counted, *Then* there is 1 write; *Given* a discard notice, *Then* exactly 1 write to the assertive region and 0 to the polite one.
- TalkBack typed-character announcement (research/ux.md TalkBack note, INFERRED until the device check): the textarea value equals only the in-flight composition and is reset to `""` at commit boundaries; at each commit the committed text (words, not each character, and only when the session is not read-only) is written to the polite region and cleared after 1 s, never logged and never put in the diagnostics ring (class and length only). Terminal output is NOT announced by this layer.
  - *Given* a composition `hel` -> `hello` committed with a space, *When* the commit is processed, *Then* the polite region received `hello` once, the textarea value is `""`, and the diagnostics ring contains `text(5)` only.
  - *Given* a Backspace run, *When* processed, *Then* no committed-text announcement fires.
- The spoken echo means a password typed at a prompt is read aloud by TalkBack; this is the same limitation as ADR-008 (the layer cannot detect echo-off prompts) and is added to the reference docs' secret-entry section (Task 5.1.2a).
  - *Given* the reference page, *When* grepping "TalkBack", *Then* the secret-entry section mentions the spoken echo.
- Owner-device proof lives in the pre-merge gate: Task 5.1.4d (TalkBack on the phone: typed characters and the committed word are announced once, "Terminal input, edit box" is the element name).
**Files**: `web-app/src/lib/terminal/mobileInput/announce.ts`, `announce.test.ts`, `useMobileInputChrome.tsx`, `inputElement.ts`, `docs/reference/mobile-input-layer.md`

##### Task 1.4.4a: `announce.ts` two-region contract, dedupe and classification table test, wired through the chrome hook (S)
- Files: `announce.ts`, `announce.test.ts`, `useMobileInputChrome.tsx`

##### Task 1.4.4b: Committed-text polite announcement from the adapter, no-log and ring-class-only tests (S)
- Files: `attachMobileInput.ts`, `inputElement.ts`, `attachMobileInput.test.ts`

---

## Phase 2: Observability (PR-A2, parallel to and independent of PR-A1; Story 2.3.1 is the minimal PR-A1 diagnostics panel)

### Epic 2.1: Server input metrics

#### Story 2.1.1: `recordTerminalInput` helper
**As an** operator, **I want** per-class input counters, **so that** "phone typed but nothing arrived" is diagnosable.
**Acceptance Criteria**:
- Classifier: ESC-first = `escape`; printable UTF-8 plus `\r \t \n \x7f` = `text`; else `other`; scans at most 64 bytes.
  - *Given* `[]byte("ls\r")`, *When* classified, *Then* `InputClass.Text`; `[]byte("\x1b[A")` -> `Escape`; `[]byte{0x03}` -> `Other`.
- Counters labeled only `kind`, `listener`, `path`, `stage` (`received|forwarded`); bytes counter adds `len(data)`. `received` is recorded at the entry and `forwarded` only where the writer actually forwards, so a permission-denied frame yields `received` without `forwarded`.
  - *Given* a 5-byte text frame on `PathCM`, remote, forwarded, *When* recorded, *Then* `received` +1 and `forwarded` +1 and bytes +5 with exactly those four labels.
- Tally is an LRU of 256 sessions, atomic fields, never stores data.
  - *Given* 300 distinct session ids, *When* recorded, *Then* tally size is 256 and oldest evicted.
**Files**: `server/services/terminal_input_metrics.go`, `terminal_input_metrics_test.go`

##### Task 2.1.1a: Types (`InputClass`, `InputPath`), classifier, instruments (S)
- Files: `terminal_input_metrics.go`

##### Task 2.1.1b: Tally LRU + tests with `t.Parallel()` and injected logger (S)
- Files: `terminal_input_metrics.go`, `terminal_input_metrics_test.go`

#### Story 2.1.2: Instrument all input entry points
**As an** operator, **I want** no blind transport, **so that** counts cover every path.
**Acceptance Criteria**:
- `received` is recorded before permission/read-only checks at `connectrpc_websocket.go:3131` (shared read loop), the capture-pane call near `:3849`, `connectrpc_websocket_shell.go:265`, `session_service_stream_terminal.go:439`; `forwarded` is recorded only after the checks pass, with the result threaded back from the `:1653`/`:2489` `onInput` closures (those closures are touched for that and nothing else).
  - *Given* a read-only subscriber sending `"x"`, *When* the frame is dispatched, *Then* `received{kind="text"}` +1 and `forwarded` unchanged.
  - *Given* a writer attached but `CanSendCommand` false, *When* a frame is dispatched, *Then* `received` +1 and `forwarded` unchanged (permission-denied case).
- `forwarded` is specified for EVERY site (no site silently omits it): shared read loop (`:3131`, covering the `:1653`/`:2489` closures) records `forwarded` from the closure's returned result after the read-only and `CanSendCommand` checks; capture-pane (`~:3849`), shell (`connectrpc_websocket_shell.go:265`) and `session_service_stream_terminal.go:439` each record `forwarded` immediately after their own gate passes and the write to the session is issued, and where a site has no gate Task 2.1.2b records `forwarded` right after the write and writes that fact in the site table in `docs/reference/mobile-input-layer.md`, so `received - forwarded` always means "denied or failed before write" and never "this path never records forwarded". One test per site asserts both `received` and `forwarded`, plus a denied case where the site has a gate.
  - *Given* each of the five tested sites with a permitted writer, *When* a text frame is dispatched, *Then* `received` +1 and `forwarded` +1 for that site's `path` label; *Given* a gated site with a denied writer, *Then* `received` +1 and `forwarded` unchanged.
- `/api/ws/terminal-mux` input is confirmed to reach a counted site.
  - *Given* a mux client sending input, *When* delivered, *Then* the tally for the session increments.
- An enumeration test fails if a new input site is added without the helper, keyed on the `inputReadLoopParams.onInput` wiring and an explicit allow-list of instrumented functions (not a free-text grep for `SendInput(`).
  - *Given* the set of `inputReadLoopParams` constructions and `TerminalData_Input` cases in `server/services`, *When* the test runs, *Then* each is in the allow-list.
**Files**: `server/services/connectrpc_websocket.go`, `connectrpc_websocket_shell.go`, `session_service_stream_terminal.go`, `terminal_input_sites_test.go`

##### Task 2.1.2a: Shared read loop call (:3131) + read-only/denied cases (S)
- Put the logic in a new file and thread only a returned `bool` through the `:1653`/`:2489` closures (gocognit/funlen/dupl apply to those modified closures; Task 0.1.4b).
- Files: `connectrpc_websocket.go`, `terminal_input_sites_test.go`

##### Task 2.1.2b: Shell, capture, stream sites: record `received` and `forwarded` per the site table, one test per site (S)
- Files: `connectrpc_websocket_shell.go`, `connectrpc_websocket.go`, `session_service_stream_terminal.go`

##### Task 2.1.2c: Mux bridge confirmation test and site-enumeration test (S)
- Add a server test that the mux and remote-listener paths record `listener="remote"` (so the diagnostic query on `listener="remote"` cannot read zero for a healthy remote path; pre-mortem P2-6).
- Files: `terminal_input_sites_test.go`

#### Story 2.1.3: Actuator endpoint and throttled logs
**As an** operator, **I want** a per-session view, **so that** I can see which stream is silent.
**Acceptance Criteria**:
- `GET /actuator/terminal-input` returns JSON; `?format=text` returns text.
  - *Given* two sessions with recorded input, *When* GET with default format, *Then* JSON array with `frames`, `bytes`, `byKind`, `lastAt`, no content field.
- Info log on first input per stream; summary at most once per minute per stream (injected clock).
  - *Given* 100 frames in 10 s on one stream, *When* logged, *Then* exactly 1 first-input line and 0 summary lines; at +61 s, 1 summary.
**Files**: `server/actuator.go`, `server/services/terminal_input_metrics.go`, `server/actuator_terminal_input_test.go`

##### Task 2.1.3a: Handler + registration next to `/actuator/ws-unary` (S)
- Files: `server/actuator.go`, `server/actuator_terminal_input_test.go`

##### Task 2.1.3b: Throttled logs with injected logger/clock (S)
- Files: `terminal_input_metrics.go`, `terminal_input_metrics_test.go`

### Epic 2.2: Client telemetry

#### Story 2.2.1: Allow-list `cm.input` and `cm.ime_anomaly`
**As an** operator, **I want** these events accepted server-side, **so that** client counters reach metrics.
**Acceptance Criteria**:
- Both events allow-listed with fixed-enum fields (`event`, `renderer`, `kind`); unknown values rejected.
  - *Given* a beacon `cm.input{event:"emitted",renderer:"xterm"}`, *When* posted, *Then* accepted; `renderer:"foo"` is rejected; `kind` is exactly the `ImeAnomaly` enum (so `unrecognized_sequence` is the series behind `ime_event_sequence_unrecognized_total`).
**Files**: `server/analytics/client_metrics.go`, `client_metrics_test.go`

##### Task 2.2.1a: Extend allow-list and instruments + tests (S)
- Files: `client_metrics.go`, `client_metrics_test.go`

#### Story 2.2.2: Client sender with rate limiting
**As a** developer, **I want** a bounded sender, **so that** a buggy keyboard cannot flood the beacon.
**Acceptance Criteria**:
- `cm.ime_anomaly` limited to 1 per kind per 10 s with burst 5 (injectable clock).
  - *Given* 50 `orphaned_input` anomalies within 1 s, *When* reported, *Then* at most 5 beacons sent.
- `cm.input` carries class/enum only, never data.
  - *Given* emitted text `"secret"`, *When* the beacon is built, *Then* serialized payload contains no `secret` substring.
**Files**: `web-app/src/lib/telemetry/inputTelemetry.ts`, `inputTelemetry.test.ts`, `web-app/src/lib/terminal/mobileInput/telemetry.ts`

##### Task 2.2.2a: `inputTelemetry.ts` + rate limiter + tests (S)
- Files: `inputTelemetry.ts`, `inputTelemetry.test.ts`

##### Task 2.2.2b: Connect layer `onAnomaly` and emit counters in `mobileInput/telemetry.ts` (XS)
- Files: `mobileInput/telemetry.ts`, `useMobileInput.ts`

#### Story 2.2.3: Docs table and dashboard row
**As an** operator, **I want** the metrics documented and graphed.
**Acceptance Criteria**:
- `docs/reference/client-network-telemetry.md` lists both events; dashboard JSON has an input row.
  - *Given* the doc, *When* grepping `cm.input`, *Then* a table row with its labels exists, together with the requirements-name mapping table; dashboard JSON parses and has a panel titled "Terminal input".
**Files**: `docs/reference/client-network-telemetry.md`, `docs/observability/grafana/dashboards/client-network.json`

##### Task 2.2.3a: Update doc, dashboard and the dashboard-JSON test (S)
- Add a Go or Jest test that loads `client-network.json`, asserts it parses and has a panel titled "Terminal input" with the documented diagnostic query. Add a client anomaly rate (`cm.ime_anomaly` rate) to the documented diagnostic query. Append the diagnostic-query section to `docs/how-to/debug-phone-typing.md`.
- Files: `docs/reference/client-network-telemetry.md`, `docs/observability/grafana/dashboards/client-network.json`, `docs/how-to/debug-phone-typing.md`, dashboard test file

##### Task 2.2.3b: Alert rule for "client typed, server received nothing" (S)
- Requirement OBS-4 "Alert condition": a non-paging alert rule for client `cm.input emitted` rate > 0 while `terminal_input_frames_total{kind="text",listener="remote",stage="received"}` rate == 0 for 5 minutes. Follow the repo's existing alert-provisioning convention (the worker locates it under `docs/observability/`). If no convention exists, take the explicit OWNER-DECISION FALLBACK: stop, do not invent a provisioning path, and record in `requirements.md` Observability (and in Unresolved Questions here) the line "OBS-4 alert rule: no alert-provisioning convention found at <path searched>; the dashboard panel plus the documented query satisfy the requirement, pending owner acceptance (OPEN)". The task is complete in either branch (rule file delivered, or the decision recorded); the open owner decision does not block PR-A2 and is listed in validation.md Gap 2.
  - *Given* the rule file, *When* validated, *Then* the expression parses and references only registered series names.
- Files: alert rule file beside `docs/observability/`, `requirements.md` (only on the stop path)

### Epic 2.3: Minimal diagnostics in the first PR

#### Story 2.3.1: Minimal read-only `?inputdebug=1` panel with Copy report (PR-A1)
**As the** owner, **I want** to record what my keyboard sends the same day the layer ships, **so that** a failing keyboard is diagnosable without waiting for PR-C (pre-mortem P1-1).
**Acceptance Criteria**:
- Opening the app with `?inputdebug=1` shows a read-only panel (`role="dialog"`, titled "Input diagnostics", closable) mounted through `useMobileInputChrome`, not behind `mobile_input_diagnostics`. It lists the last 50 events from the Story 4.1.1 ring buffer, a header (build id, renderer, flag state and source, stream state, keyboard hint) and a "Copy report" button producing the content-free report; clipboard blocked shows the report selectable. Without the param nothing renders and the ring buffer stays empty (push is a no-op). Reveal, capture-for-fixture, Pause and Clear are PR-C.
  - *Given* `?inputdebug=1` on Pixel 7 emulation, *When* the page loads, *Then* the dialog titled "Input diagnostics" is visible with the build id; *Given* 5 typed characters, *Then* Copy report yields JSON containing `text(1)` entries and no typed character; *Given* no param, *Then* no panel and buffer length 0.
**Files**: `web-app/src/components/sessions/MinimalInputDiagnostics.tsx`, `MinimalInputDiagnostics.test.tsx`, `web-app/src/lib/terminal/mobileInput/useMobileInputChrome.tsx`

##### Task 2.3.1a: Minimal panel, build-id header, Copy report, `?inputdebug=1` entry via the chrome shell (S)
- Reuses `diagnostics.ts` from Task 4.1.1a/b; extends `web-app/src/lib/terminal/mobileDebug.ts` rather than duplicating it; no logic in `TerminalOutput.tsx`.
- Files: `MinimalInputDiagnostics.tsx`, `MinimalInputDiagnostics.test.tsx`, `useMobileInputChrome.tsx`, `mobileDebug.ts`

---

## Phase 3: Disconnected input (PR-B; behind sub-flag `mobile_input_queue`, requires `mobile_input_layer`)

Not on the critical path to the first merge. Sub-flag `mobile_input_queue` (default off until owner approves ADR-004 in the PR) is registered in Task 3.1.1a. Flag off restores the old `console.warn` drop exactly.

### Epic 3.1: Visible drop and bounded queue

#### Story 3.1.1: `SendResult` and `onInputDropped`
**As a** user, **I want** to know when my typing was not sent, **so that** I can trust the pane.
**Acceptance Criteria**:
- `sendInput` already returns `SendResult` from PR-A (Task 1.2.4a, with the `SendResult` chain from Task 1.2.4b); this story adds `queued` and the reason reporting. The `readOnlyRef` early return (`:242`) returns `dropped` with reason `read_only`. With the sub-flag on, `onInputDropped(info)` replaces the `console.warn` for the `sendInput` path only; `ensureConnected`'s warn stays for the other dispatchers (resize etc.); flag off keeps the exact old warn.
  - *Given* a disconnected stream and sub-flag on, *When* `sendInput("hi")` runs, *Then* returns `"dropped"` (or `"queued"` after 3.1.3) and `onInputDropped` called with `{reason:"disconnected",chars:2}`; with the flag off the old warn text is logged and the existing `useTerminalFlowControl.test.ts` cases pass unchanged (widened return type does not alter existing callers).
- Atomic edits bypass chunking below `ATOMIC_EDIT_MAX_BYTES` and never split inside a UTF-8 code point.
  - *Given* an atomic edit of 600 bytes `"\x7f"*10+text`, *When* sent, *Then* exactly one WebSocket send.
**Files**: `web-app/src/lib/hooks/useTerminalFlowControl.ts`, `useTerminalFlowControl.test.ts`, `server/services/feature_flag_service.go` (sub-flag)

##### Task 3.1.1a: Add `onInputDropped` and the `queued` result, register `mobile_input_queue`, flag-gated (S)
- Files: `useTerminalFlowControl.ts`, `feature_flag_service.go`

##### Task 3.1.1b: Atomic-send option and chunk-boundary code point safety + tests (S)
- Files: `useTerminalFlowControl.ts`, `useTerminalFlowControl.test.ts`

#### Story 3.1.2: Visible status chip and the single chrome hook
**As a** phone user, **I want** a chip within 250 ms of the first dropped input, **so that** I see it immediately.
**Acceptance Criteria**:
- `InputStatusChip` has `role="status"`, `aria-live="polite"`, `data-testid="terminal-input-dropped"`, min 44x44 target for its Reconnect action, text plus icon (not color alone).
  - *Given* `onInputDropped` fires at t=0, *When* fake timers advance 250 ms, *Then* the chip is in the DOM with text containing "not being sent". (The 250 ms bound is asserted only here, with fake timers.)
- Chip clears when the stream reconnects and the queue empties.
  - *Given* a visible chip and a reconnect event, *When* queue flush completes, *Then* the chip is removed.
- The one hook `useMobileInputChrome(sessionId, flowControl)` (shell created in PR-A, Task 1.4.1e) is EXTENDED here with chip and queue-flush state; later phases extend it too. `TerminalOutput.tsx` already has its one hook call and fragment slot and gains no inline logic.
  - *Given* the diff of `TerminalOutput.tsx` for Phases 3-4, *When* reviewed, *Then* it adds one hook call, one fragment slot and prop pass-throughs only; a test mounts the hook alone and drives chip states without `TerminalOutput`.
**Files**: `web-app/src/components/sessions/InputStatusChip.tsx`, `InputStatusChip.css.ts`, `InputStatusChip.test.tsx`, `web-app/src/lib/terminal/mobileInput/useMobileInputChrome.tsx`, `useMobileInputChrome.test.tsx`, `TerminalOutput.tsx`

##### Task 3.1.2a: Component + vanilla-extract style + test (S)
- Files: `InputStatusChip.tsx`, `InputStatusChip.css.ts`, `InputStatusChip.test.tsx`

##### Task 3.1.2b: Extend `useMobileInputChrome` with chip + flush state (hook shell exists from Task 1.4.1e) (S)
- Files: `useMobileInputChrome.tsx`, `useMobileInputChrome.test.tsx`

##### Task 3.1.2c: Render the chip in the existing hook fragment slot above the extra-keys row (XS)
- Files: `useMobileInputChrome.tsx`, `TerminalOutput.tsx`

#### Story 3.1.3: Bounded plain-text queue of atomic units
**As a** user on a flaky link, **I want** typed text kept briefly and replayed, **so that** I do not retype a prompt.
**Acceptance Criteria**:
- The queue stores the ORIGINAL emitted chunks as atomic units `{bytes, enqueuedAt, sessionId}`. A DEL+text rewrite is ONE unit: never split, never merged. Flush replays units in order, each as one atomic `sendInput`.
  - *Given* units `"teh"` and `"\x7f\x7fhe"`, *When* flushed, *Then* the sink receives exactly those two strings and a `ModelTerminal` ends with `"the"`.
- Plain text only: a unit is accepted iff it contains no `\r`, `\n`, `\t`, `\x1b` or any other control byte (C0, `\x7f`, C1), EXCEPT a leading run of `\x7f` inside an atomic rewrite unit that ALSO carries non-empty plain text and is at most `ATOMIC_EDIT_MAX_BYTES`. A lone `\x7f`, Backspace-only, Esc, arrows and Ctrl bytes are rejected: `dropped`, reason `control`, counted, chip says keys not sent.
  - *Given* disconnected, *When* `"\x1b[A"`, `"\x7f"`, `"\x03"` and `"\x7f\x7fhe"` are sent, *Then* results `dropped, dropped, dropped, queued` and the queue holds only the last.
- Enter typed while disconnected is NOT queued: `dropped`, reason `enter`, chip shows "Enter not sent". `\r` and `\n` never enter the queue even inside multi-line text (the whole unit is rejected).
  - *Given* disconnected, *When* `"\r"` or `"ls\r"` is sent, *Then* `"dropped"` and the queue is unchanged.
- Capacity 4096 bytes, per-unit TTL 30 s (injected clock). Overflow or expiry of any unit discards the WHOLE queue (a later unit's DELs depend on earlier units, so dropping a prefix would replay orphan DELs) and emits `onQueueDiscard(DeliveryReport)` (`deliveredThroughSeq` = the last unit actually flushed to the pty, `discardedFromSeq`, `chars`) with the visible notice for the cause (overflow: `Discarded N chars typed while offline.`; TTL expiry: `Discarded N chars - offline for more than 30 s.`; session change: `Discarded N chars - the session changed.`, per `design/ux.md` S3); the layer then calls `invalidate('queue_discard')`: it abandons the in-flight region (no tail re-sent) and starts the next composition from an empty base (Stories 1.2.4 and 3.1.4).
- Strict FIFO: `sendInput` while the queue is non-empty or a flush is in progress enqueues behind it, and the flush drains through the same path; a live keystroke typed after reconnect but before the flush completes never overtakes queued units.
  - *Given* queued `["he"]`, link back, flush in progress, *When* `"l"` is typed, *Then* the sink order is `"he"`, `"l"`, and `"l"` returned `"queued"` until the queue is empty.
  - *Given* 4096 queued bytes, *When* 10 more bytes arrive, *Then* the queue is emptied, notice `Discarded 4106 chars typed while offline` shows, and `onQueueDiscard` fired once; *Given* a unit queued at t=0 and flush at t=31 s, *Then* it is discarded with the notice, not sent.
- A queued unit is discarded after ANY session switch: reconnect to a different session id, terminal pool reuse for another session, unmount, tab refresh (the queue is in memory only, never persisted to localStorage), or the session being paused/restarted behind the same id (no client or proto generation field exists today; detected only if a real source is added, otherwise covered by the `isConnected` trigger on reconnect).
  - *Given* queued `"hello"` for S1 and reconnect to S1 within 30 s, *When* flushed, *Then* sink receives `"hello"` once; reconnect to S2, a pool reuse, or a restart of S1 yields the discard notice and nothing is sent.
**Files**: `web-app/src/lib/terminal/InputQueue.ts`, `InputQueue.test.ts`, `useTerminalFlowControl.ts`

##### Task 3.1.3a: `InputQueue.ts` pure class of atomic units with fake-clock tests (S)
- Files: `InputQueue.ts`, `InputQueue.test.ts`

##### Task 3.1.3b: Integrate queue and flush-on-reconnect into flow control (M)
- Files: `useTerminalFlowControl.ts`, `useTerminalFlowControl.test.ts`

##### Task 3.1.3c: Session-switch, pool-reuse, restart, refresh and TTL-during-live-composition tests (S)
- Files: `useTerminalFlowControl.test.ts`, `InputQueue.test.ts`

#### Story 3.1.4: Queue reports delivery; layer base follows the delivered prefix
**As a** user, **I want** a later autocorrect not to delete text that never left the phone nor to duplicate queued text.
**Acceptance Criteria**:
- Base rule (Story 1.2.4 ledger, PR-A): the diff base is the prefix actually delivered to the pty in the current epoch, never "empty"; units queued after a boundary stay DEL-eligible because the queue replays them in order or discards the whole queue (which abandons the region). It is kept on `sent` AND on `queued` (the queue replays the same units, so the base stays true) and, on `dropped`, handled by the single deferred-after-boundary rule of Story 1.2.4; on `onQueueDiscard` the region is abandoned. This story supplies the queue side: `InputQueue` tracks the last flushed `UnitSeq` and always reports `deliveredThroughSeq`.
  - *Given* `Composing{sent:"teh"}`, the sink returned `"queued"` for `"teh"`, *When* region becomes `"the"`, *Then* the layer emits `{backspaces:2, insert:"he"}` as one unit, the queue holds `["teh","\x7f\x7fhe"]` and flush on reconnect yields `"the"` in the `ModelTerminal` (no `helhelp`, no orphan DELs).
  - *Given* `Composing{sent:"hel"}` and the sink returned `"dropped"` for `"hel"` as the FIRST unit of the region (nothing delivered), *When* the region becomes `"help"` and commits, *Then* the layer emits `"help"` at commit (`B` empty) and no anomaly.
  - *Given* `"he"` sent, `"l"` dropped (partial delivery), *When* the region becomes `"help"` and `compositionend` arrives, *Then* the layer emits `"lp"` at commit (base `"he"`, zero DELs), never `"help"` over `"he"`.
- When the queue discards ANY unit (`onQueueDiscard(DeliveryReport)`) the layer abandons the in-flight region (`invalidate('queue_discard')`, no tail re-sent, ledger cleared, next composition from an empty base) and the 'discarded' notice is surfaced; a `dropped` unit follows the deferred-after-boundary rule (Story 1.2.4).
  - *Given* a queue overflow mid-composition after `"he"` was flushed and `"l"` was still queued, *When* the next `compositionupdate` and then `compositionend` arrive, *Then* the layer emits zero bytes for that region (no DEL, no `"lp"`), the notice is visible, and the following composition starts clean.
- Replay case: disconnect -> autocorrect -> reconnect fixture `disconnect-autocorrect-reconnect.json` replays through a real `InputQueue` with a fake clock between the reducer and the `ModelTerminal`.
  - *Given* prompt text `"$ "` already on the model line, link down after `"teh"`, autocorrect to `"the"`, link up within 30 s, *When* replayed, *Then* the line is `"$ the"`: pre-disconnect prompt text is untouched, no `helhelp`-style duplication and no orphan DEL.
**Files**: `InputQueue.ts`, `useMobileInput.ts`, `attachMobileInput.test.ts`, `replayHarness.ts`, `__fixtures__/disconnect-autocorrect-reconnect.json`

##### Task 3.1.4a: `InputQueue` delivered-seq tracking, `DeliveryReport` on discard, wiring to the layer's `onQueueDiscard` (S)
- The ledger and rebase logic already exist from Task 1.2.4c (PR-A); this task adds the queue side only.
- Files: `InputQueue.ts`, `InputQueue.test.ts`, `useMobileInput.ts`

##### Task 3.1.4b: Replay harness: real queue with the existing link-down/up steps; disconnect-autocorrect-reconnect and overflow-with-DELs cases (S)
- Files: `replayHarness.ts`, `replay.test.ts`, `__fixtures__/disconnect-autocorrect-reconnect.json`

#### Story 3.1.5: Chip extra states and discard notices
**As a** phone user, **I want** every outcome of offline typing to be explicit, **so that** nothing is silent.
**Acceptance Criteria**:
- Failed reconnect shows "Still disconnected. Check your connection." and the Reconnect button stays usable.
  - *Given* the chip visible, *When* Reconnect is tapped and fails, *Then* the text changes and the button is enabled again.
- Control-only input while disconnected (Esc, arrows, Ctrl, Backspace, Enter) shows a "keys not sent" state without implying text was queued.
  - *Given* disconnected, *When* only `"\x1b[A"` is sent, *Then* the chip text contains "keys not sent" and no queue count.
- After a successful flush the chip shows "Sent N chars" and disappears within 4 s (reduced motion: no animation).
  - *Given* a flush of 5 chars, *When* 4 s fake time elapses, *Then* the chip is removed.
- Discard notices are dismissible (44px button) and announced once to TalkBack, with the exact per-cause texts from `design/ux.md` S3: overflow `Discarded N chars typed while offline.`, TTL expiry `Discarded N chars - offline for more than 30 s.`, session change `Discarded N chars - the session changed.`; `N` counts the characters discarded as reported by `DeliveryReport.chars` (defined in code points of the queued text, not bytes).
  - *Given* a discard notice for each of overflow, TTL expiry and session change, *When* rendered, *Then* its text equals the matching string above; *When* Dismiss is tapped, *Then* it is removed and focus returns to the textarea.
- Stuck and timeout states are explicit, with fake-timer thresholds (constants `RECONNECT_SLOW_MS = 10_000`, `FLUSH_SLOW_MS = 5_000`, `FLUSH_STUCK_MS = 15_000`): while Reconnecting for more than 10 s the chip reads "Still trying to reconnect. This is taking longer than usual." with Retry (44px) and Dismiss; while a queue replay is in flight it reads "Sending N typed characters..." (polite), after 5 s "Still sending N typed characters. Keep this page open.", and after 15 s without completion "Sending is stuck. Some typed text may be missing; check the terminal." with Dismiss and Reconnect, announced assertively (Story 1.4.4 table). A completed flush always replaces these with the "Sent N chars" confirmation.
  - *Given* Reconnecting and fake time advanced 10 s, *When* rendered, *Then* the slow-reconnect text and a Retry button are present; *Given* a flush of 12 chars with no completion and 5 s, then 15 s elapsed, *Then* the three texts above appear in order and the last is announced via the assertive region once; *Given* the flush completes at 7 s, *Then* the chip shows "Sent 12 chars" and removes the slow text.
**Files**: `InputStatusChip.tsx`, `InputStatusChip.test.tsx`, `useMobileInputChrome.tsx`

##### Task 3.1.5a: Chip states, discard notices and dismiss behavior (S)
- Files: `InputStatusChip.tsx`, `InputStatusChip.test.tsx`, `useMobileInputChrome.tsx`

##### Task 3.1.5b: Reconnecting-slow, flush-in-progress, flush-slow and flush-stuck copy with fake-timer tests (S)
- Files: `InputStatusChip.tsx`, `InputStatusChip.test.tsx`, `useMobileInputChrome.tsx`

---

## Phase 4: Diagnostics overlay and compose bar (PR-C and PR-D; own sub-flags; off the critical path)

Story 4.1.1 (content-free ring buffer and report) ships in PR-A1 and feeds the minimal `?inputdebug=1` panel (Story 2.3.1); it has no sub-flag. The rest of Phase 4.1 (full sheet, capture-for-fixture) ships behind sub-flag `mobile_input_diagnostics` and 4.2 behind `mobile_input_compose`, each registered in its first task, default off until the owner opts in. Neither blocks PR-A1.

### Epic 4.1: Diagnostics

#### Story 4.1.1: Ring buffer and redacted report (PR-A1; pulled forward from PR-C)
**As the** owner, **I want** a copyable, content-free event report, **so that** I can show the shape of what my phone sends without exposing what I typed.
**Acceptance Criteria**:
- Ring buffer holds the last 200 events in memory (display 50), push is a no-op when diagnostics are closed; cleared on close; never included in `cm.*`, `console` or `onAnomaly` payloads.
  - *Given* diagnostics closed, *When* 1000 events arrive, *Then* buffer length stays 0; open, then 1000 events -> length 200; close -> length 0.
- Artifact 1, the redacted diagnostics report, is content-free and shareable: header (renderer, flag, stream state, keyboard hint) and events with data class and length only. It cannot be converted into a replayable trace; that is stated in its docs. `reportToSkeleton(report)` only yields a shape summary for triage.
  - *Given* typed `"hello"`, *When* the report is built, *Then* events contain `text(5)` and the string `hello` is absent from the serialized JSON.
- "Reveal text" is session-only, off at open, never leaves the device, and does not alter the report. The revealed content is held ONLY in an in-memory `revealBuffer` owned by the sheet component (separate from the content-free `DiagnosticsEvent` ring buffer, populated only while reveal is on), never serialized, never read by the report builder, and cleared on reveal-off and on close.
  - *Given* reveal on and `"hello"` typed, *When* the sheet closes, *Then* `revealBuffer` is empty and the report builder has no reference to it.
  - *Given* reveal on, *When* Copy report runs, *Then* the copied JSON still has no content.
**Files**: `web-app/src/lib/terminal/mobileInput/diagnostics.ts`, `diagnostics.test.ts`

##### Task 4.1.1a: `diagnostics.ts` ring buffer and `onDiagnostic` wiring in the adapter (S, PR-A1)
- "Open" means the Story 2.3.1 panel (or later the full sheet) is open; `mobile_input_diagnostics` registration moves to Task 4.1.2b.
- Files: `diagnostics.ts`, `attachMobileInput.ts`

##### Task 4.1.1b: Redacted report builder, `reportToSkeleton`, no-content tests (S)
- Files: `diagnostics.ts`, `diagnostics.test.ts`

#### Story 4.1.2: Diagnostics bottom sheet
**As the** owner, **I want** a sheet I can open in <= 2 taps, **so that** I can diagnose without dev tools.
**Acceptance Criteria**:
- Sheet is `role="dialog"` titled "Input diagnostics", max 40vh, 44px rows, opened by (1) an "Input diagnostics" entry in the app settings surface (the primary, discoverable path; Task 4.1.2c), (2) the toolbar overflow item "Input debug", (3) `?inputdebug=1`, and (4) as a secondary shortcut only, long-press (800 ms) on the connection chip; state persisted in localStorage inside try/catch.
  - *Given* `?inputdebug=1`, *When* the page loads, *Then* the dialog with title "Input diagnostics" is visible.
  - *Given* the app settings surface on a phone, *When* the user taps "Input diagnostics", *Then* the dialog opens in at most 2 taps from the terminal, without needing any long-press.
- Long-press collision on Android: a long press on the chip can trigger Chrome's context menu / text-selection callout. The chip sets `user-select: none`, `-webkit-touch-callout: none` and calls `preventDefault` on `contextmenu` for that element only; the 800 ms long-press is never the only way in, and the how-to documents the settings and overflow paths first.
  - *Given* a Pixel 7 profile, *When* a 900 ms touch is held on the chip, *Then* the dialog opens and no `contextmenu` default action ran on the chip; the same dialog is reachable with the long-press handler removed.
- Phone is modal, desktop is non-modal: at `max-width: 600px` the sheet has `aria-modal="true"`, a scrim, a focus trap, and closes via Close, Back, scrim tap and Escape with focus returned to the opener; on wider viewports it is a non-modal docked panel (no scrim, no trap, `aria-modal` absent) that still closes with Escape.
  - *Given* a 390px viewport, *When* opened, *Then* `aria-modal="true"` and Tab stays inside; *Given* 1280px, *Then* no scrim, and Tab leaves the sheet.
- Header shows build id, renderer and fallback reason (the diagnostics sheet is its own sub-flagged surface and still opens with the layer flag off, header `layer: off`).
  - *Given* ghostty on, layer off, Android class, *When* opened, *Then* header reads `renderer: xterm (fallback: mobile_input_layer off)`.
- Actions: Copy report, Clear, Pause; clipboard blocked shows the report selectable with manual-copy guidance.
  - *Given* 3 events, *When* Clear is pressed, *Then* the table is empty and counters reset.
**Files**: `web-app/src/components/sessions/InputDiagnosticsSheet.tsx`, `InputDiagnosticsSheet.css.ts`, `InputDiagnosticsSheet.test.tsx`, `useMobileInputChrome.tsx`

##### Task 4.1.2a: Sheet component (modal on phone, non-modal on desktop), style, test (M)
- Files: `InputDiagnosticsSheet.tsx`, `InputDiagnosticsSheet.css.ts`, `InputDiagnosticsSheet.test.tsx`

##### Task 4.1.2b: Register `mobile_input_diagnostics`; full-sheet entry points and mount through `useMobileInputChrome` (long-press, overflow, query param upgrades the A1 minimal panel) (S)
- Extend `web-app/src/lib/terminal/mobileDebug.ts` rather than duplicating it. No logic added to `TerminalOutput.tsx`.
- Files: `useMobileInputChrome.tsx`, `mobileDebug.ts`, `server/services/feature_flag_service.go`

##### Task 4.1.2c: Settings-surface entry "Input diagnostics" and long-press collision guard on the chip (S)
- The worker locates the existing app settings surface (the page or panel that hosts per-device preferences); if none exists, record that in `spike-findings.md` and keep overflow as the discoverable path. Gated by `mobile_input_diagnostics`; shown on every device class (desktop included) because the sheet is non-modal there. No new page, so no `route-meta.ts` row unless a page is added.
- Files: the settings surface component, `useMobileInputChrome.tsx`, `InputStatusChip.tsx`, `InputDiagnosticsSheet.test.tsx`

#### Story 4.1.3: Opt-in "capture for fixture" mode (local-only)
**As the** owner, **I want** to record one short real Gboard session as replayable events, **so that** device fixtures exist without leaking what I type.
**Acceptance Criteria**:
- Artifact 2 is a separate, explicit mode, distinct from the redacted report. It records event data (`data`, `value`, selection) for ONE short user-initiated recording (max 20 s or 200 events, auto-stops), typed into a scratch field inside the sheet (never the live terminal), with the content visible on screen for review. It is never auto-sent: no network call is made, and it is cleared on stop, close or discard.
  - *Given* capture started and `"the quick"` typed in the scratch field, *When* stopped, *Then* the recording shows content for review, and a spy on `fetch`, `XMLHttpRequest` and `navigator.sendBeacon` recorded zero calls; closing the sheet empties the buffer.
- Output only by explicit user tap (Copy or Download `.json`) and is converted by `captureToTrace(capture)` into a `ReplayTrace` with `source:"device"`, `scrubbed:false`, `signoff:""` and `expectedBytes` to be completed.
  - *Given* a capture, *When* `captureToTrace` runs, *Then* the output validates against `fixtureSchema.ts` except `expectedBytes`, `scrubbed` and `signoff`.
- A secrets scrub checklist (in `docs/how-to/debug-phone-typing.md` and mirrored in the `fixtureSchema.ts` header): (1) capture only in the scratch field, never at a sudo/ssh/token prompt or with a password manager autofill; (2) type only the fixed protocol sentence; (3) inspect every `data`/`value` field; (4) grep the file for `sk-`, `ghp_`, `AKIA`, `@`, `://`, `/home/`, hostnames, long digit runs; (5) replace any non-protocol content with neutral text of identical grapheme structure; (6) set `scrubbed:true` and `signoff:"<owner login>"`; (7) PR description lists the owner sign-off.
  - *Given* the how-to, *When* grepped, *Then* all seven items are present.
- Copy and Download write captured content to the system clipboard (Gboard clipboard history may retain it) and to Downloads; the review UI says so next to the buttons, the how-to lists "clear clipboard history and delete the download" as scrub step 8, and the device-fixture PR template (a checklist block in `.github/pull_request_template.md` or the how-to) repeats it. CI guard: the Task 1.1.4d fixture lint fails the build if any committed fixture has `source:"device"` without `scrubbed:true` and a non-empty `signoff`.
  - *Given* the review UI, *When* rendered, *Then* a visible line states Copy/Download write to the clipboard/Downloads.
**Files**: `web-app/src/lib/terminal/mobileInput/captureForFixture.ts`, `captureForFixture.test.ts`, `InputDiagnosticsSheet.tsx`, `fixtureSchema.ts`

##### Task 4.1.3a: Capture recorder, scratch field and no-network guard test (S)
- Files: `captureForFixture.ts`, `captureForFixture.test.ts`, `InputDiagnosticsSheet.tsx`

##### Task 4.1.3b: `captureToTrace`, review UI, scrub checklist text (S)
- Files: `captureForFixture.ts`, `fixtureSchema.ts`, `docs/how-to/debug-phone-typing.md`

### Epic 4.2: Compose bar (opt-in, PR-D, sub-flag `mobile_input_compose`)

#### Story 4.2.1: Opt-in compose bar
**As a** phone user writing a long prompt, **I want** a native text field with a Send button, **so that** autocorrect, swipe and dictation work with review before sending.
**Acceptance Criteria**:
- Toggle in the toolbar (44px) shows a visible `<textarea>` with Send and Send+Enter; off by default; hidden when `readOnly`, and an open bar closes when the session becomes read-only.
  - *Given* compose toggle pressed, *When* rendered, *Then* `data-testid="compose-bar-input"`, Send (`aria-label="Send"`) and Send+Enter are present; *Given* the session turns read-only, *Then* the bar closes and the toggle is hidden.
- Send writes the whole string as one unit and does not press Enter; Send+Enter sends the unit then one `\r`. Bracketed paste when the renderer reports DECSET 2004 on (per spike 0.1.1); embedded `\x1b[201~` is stripped.
  - *Given* mode 2004 on and compose text `"line1\nline2"`, *When* Send, *Then* sink receives `"\x1b[200~line1\nline2\x1b[201~"` and no `\r`.
- Fallback when DECSET 2004 is unavailable or off: the string is sent as typed text with embedded newlines converted to a single space (so no line is accidentally submitted) and a one-time hint "Multi-line text was joined" shows; Send+Enter still appends one `\r`. Which branch ships per renderer is fixed by the 0.1.1 spike result.
  - *Given* mode 2004 unreadable and text `"a\nb"`, *When* Send, *Then* sink receives `"a b"`.
- Draft retention: closing keeps the draft; reopening restores it (in memory per session id, discarded on session change).
  - *Given* draft `"abc"` and Close, *When* reopened, *Then* the textarea value is `"abc"`.
- Too long to hold offline: a Send of more than 4096 chars while disconnected is not cleared and shows "Too long to hold while offline (4096 chars max)."; offline Send otherwise goes through the same `SendResult` path (compose text is queued as one unit only if it is plain text with no newline; multi-line compose offline is refused with "not sent").
  - *Given* disconnected and 5000 chars, *When* Send, *Then* the draft remains and the error shows.
**Files**: `web-app/src/components/sessions/ComposeBar.tsx`, `ComposeBar.css.ts`, `ComposeBar.test.tsx`, `useMobileInputChrome.tsx`

##### Task 4.2.1a: `ComposeBar` component + style + test, register `mobile_input_compose` (M)
- Files: `ComposeBar.tsx`, `ComposeBar.css.ts`, `ComposeBar.test.tsx`, `server/services/feature_flag_service.go`

##### Task 4.2.1b: Send semantics (bracketed paste per spike result, no-2004 fallback, sanitize `201~`) (S)
- Blocked by Story 0.1.1.
- Files: `ComposeBar.tsx`, `ComposeBar.test.tsx`

##### Task 4.2.1c: Toolbar toggle wiring via `useMobileInputChrome` (XS)
- Files: `useMobileInputChrome.tsx`

##### Task 4.2.1d: Compose extras: Send+Enter, draft retention, too-long-offline error, close on read-only (S)
- Files: `ComposeBar.tsx`, `ComposeBar.test.tsx`

#### Story 4.2.2: IME-fallback detector and notice
**As a** phone user whose keyboard sends nothing usable, **I want** a one-time offer of the compose bar, **so that** I am not stuck.
**Acceptance Criteria**:
- Offered once with a notice when no `beforeinput` arrives after 5 `keydown 229` events in a focused session (the "no text at all" trigger).
  - *Given* 5 keydown-229 events and zero `beforeinput`, *When* the 5th arrives, *Then* the notice "Keyboard input fell back to compose mode" shows and anomaly `unrecognized_sequence` fires (maps to `ime_event_sequence_unrecognized_total`).
- Notice controls: "Got it" dismisses for the session; "Turn off" persists an opt-out in localStorage (try/catch) so the notice and auto-offer never reappear; both are 44px and announced via `role="status"`.
  - *Given* "Turn off" tapped, *When* the 5-event pattern recurs after reload, *Then* no notice shows; "Got it" suppresses it only until reload.
- Partial IME breakage (wrong or garbled text, not no text) also triggers the offer: the detector counts layer anomalies of kinds `unrecognized_sequence`, `stuck_composition` and `orphaned_input_fallback` (the same anomalies that feed `ime_event_sequence_unrecognized_total`) and offers compose mode with the variant copy "Typing looks unreliable with this keyboard. Try compose mode?" after 3 such anomalies within 60 s (`WRONG_TEXT_ANOMALY_THRESHOLD = 3`, `WRONG_TEXT_WINDOW_MS = 60_000`). Same controls ("Got it" / "Turn off"), same once-per-session rule and same opt-out key. A persistent manual path always exists: the diagnostics sheet (Story 4.1.2) has a "Try compose mode" action, and the Compose toggle stays in the toolbar overflow.
  - *Given* 3 `unrecognized_sequence` anomalies within 60 s and normal `beforeinput` traffic, *When* the 3rd arrives, *Then* the variant notice shows; *Given* the same 3 anomalies spread over 90 s, *Then* no notice shows; *Given* "Turn off" persisted, *Then* neither trigger fires.
**Files**: `web-app/src/lib/terminal/mobileInput/fallbackDetector.ts`, `fallbackDetector.test.ts`, `useMobileInputChrome.tsx`

##### Task 4.2.2a: `fallbackDetector.ts` pure counter + tests (S)
- Files: `fallbackDetector.ts`, `fallbackDetector.test.ts`

##### Task 4.2.2b: Notice with "Got it" / "Turn off" and localStorage opt-out via the chrome hook (S)
- Files: `useMobileInputChrome.tsx`

##### Task 4.2.2c: Anomaly-window trigger (3 anomalies in 60 s), variant notice copy and the diagnostics "Try compose mode" action (S)
- Files: `fallbackDetector.ts`, `fallbackDetector.test.ts`, `useMobileInputChrome.tsx`, `InputDiagnosticsSheet.tsx`

---

## Phase 5: End-to-end, docs, gates, acceptance

### Epic 5.1: Prove it

#### Story 5.1.1: Playwright mobile-emulated wiring spec
**As a** maintainer, **I want** an e2e proving text typed through the layer reaches a live pane under both renderers, **so that** wiring regressions are caught in CI.
**Acceptance Criteria**:
- Spec first line `// @feature terminal:input, terminal:mobile-input`; header comment states it proves wiring only, not Gboard fidelity.
  - *Given* the spec file, *When* the e2e convention check runs, *Then* it passes.
- CDP helper `imeCompose(page, text)` / `imeCommit(page)` in `tests/e2e/helpers/ime.ts`; if spike 0.1.2 shows `Input.imeSetComposition` does not produce `insertCompositionText`, the helper falls back to `Input.insertText` plus synthetic `beforeinput` dispatch and the spec header says which events are real.
  - *Given* a focused `mobile-input-textarea`, *When* `imeCompose(page,"ls")` then `imeCommit`, *Then* the pane shows `ls` at the prompt.
- Runs for xterm and ghostty (ghostty enabled through `tests/e2e/helpers/feature-flag.ts`), `devices['Pixel 7']`, via `describe` parametrization; asserts exactly one set of bytes per keystroke (no double-send) and that a user tap leaves `mobile-input-textarea` as `document.activeElement`.
  - *Given* ghostty on and the layer on, *When* tapping the terminal then typing `"abc"`, *Then* `activeElement` is `mobile-input-textarea` and the pane shows `abc` exactly once.
  - *Given* ghostty on, the layer on and a docked terminal that has never been focused, *When* the FIRST tap lands on it, *Then* `activeElement` is `mobile-input-textarea` immediately (layer attached on visible/dock, not on focus).
  - *Given* the layer on, *When* a scroll gesture (touch drag) runs over the terminal and is followed by a tap, *Then* `activeElement` is `mobile-input-textarea` (a tap after a scroll still focuses; step in Task 5.1.1b).
- Hardware keyboard on the Android profile (pre-mortem P3-11): Playwright keyboard typing on the Pixel 7 profile into the focused layer produces each character exactly once (real keyCodes, no double-send from the following `beforeinput`).
  - *Given* the Pixel 7 profile and the layer on, *When* `page.keyboard.type("ls")` runs, *Then* the pane shows `ls` once.
- `?mobile_input=0` kill switch applies in the SAME load: opening with `?mobile_input=0` renders xterm and no `mobile-input-textarea`, and the flag-off hint is visible.
- Platform gate: an iPhone-emulated context (`devices['iPhone 14']`) does not mount `mobile-input-textarea`.
  - *Given* the iPhone device and the layer flag on, *When* the terminal mounts, *Then* no `mobile-input-textarea` exists.
- (PR-B) Disconnect case: with the stream blocked, typing shows the `terminal-input-dropped` chip using the default expect timeout (the 250 ms bound is a fake-timer unit assertion only); locators use `data-testid`/ARIA only, no `waitForTimeout`.
  - *Given* the WebSocket closed, *When* typing `"x"`, *Then* the chip becomes visible within the default expect timeout.
- Flag off: ghostty on + layer off + Android-emulated renders the xterm renderer.
  - *Given* `mobile_input_layer=false`, `ghostty_renderer=true`, Pixel 7, *When* the terminal mounts, *Then* the xterm container (`data-testid` for renderer kind) is present.
**Files**: `tests/e2e/mobile-input-layer.spec.ts`, `tests/e2e/helpers/ime.ts`, `tests/e2e/pages/TerminalPage.ts`

##### Task 5.1.1a: `helpers/ime.ts` CDP helpers per spike findings (S)
- Files: `tests/e2e/helpers/ime.ts`

##### Task 5.1.1b: Spec: typing, tap-focus activeElement (incl. tap after a scroll gesture), platform gate, both renderers, double-send check (M)
- Files: `tests/e2e/mobile-input-layer.spec.ts`, `tests/e2e/pages/TerminalPage.ts`

##### Task 5.1.1c: Spec: disconnect chip (PR-B), flag-off renderer fallback and `?mobile_input=0` same-load case (S)
- Files: `tests/e2e/mobile-input-layer.spec.ts`

##### Task 5.1.1d: Required CI check for the ghostty double-send e2e on dependency changes (XS)
- Pre-mortem P2-5. The ghostty variant of the double-send e2e (Task 5.1.1b) becomes a required check on any change to `web-app/package.json` or the lockfile, via a path-filtered job. Follow `docs/reference/ci-job-budget.md` (add it without growing the per-PR check count: reuse an existing e2e job with a path filter, or document the budget change).
- Files: `.github/workflows/*` (the existing e2e workflow), `docs/reference/ci-job-budget.md`

##### Task 5.1.1e: Spec: autocorrect rewrite frame sampler, cursor-row-visible emulation check and the minimum-rows matrix hookup, both renderers (S)
- Samples the terminal buffer each animation frame (via the page's `requestAnimationFrame` in `page.evaluate`, no `waitForTimeout`) across a CDP autocorrect and asserts the Story 1.2.5 bound: at most one intermediate state, no shortened prompt. Also hosts the Story 0.1.5 cursor-row emulation assertion. Frame counts are structural (number of distinct intermediate lines), not wall-clock thresholds.
- Files: `tests/e2e/mobile-input-layer.spec.ts`

#### Story 5.1.2: Documentation
**As a** future maintainer, **I want** how-to and reference pages, **so that** I can operate and extend the layer.
**Acceptance Criteria**:
- `docs/reference/mobile-input-layer.md` documents the seam contract, ECP table, flags and sub-flags, platform gate, metrics (with the requirements-name mapping), actuator, fixture schema (`scrubbed`, `signoff`), and the secret-entry limitation (ADR-008). ghostty-web is cited by condition text, not line numbers.
  - *Given* the page, *When* grepping `attachMobileInput`, *Then* the options table matches `MobileInputOptions` in code.
- `docs/how-to/debug-phone-typing.md` covers: flip `mobile_input_layer` (rollback only, not a user setting; flip off, reload once to persist, reload again to apply, or open with `?mobile_input=0|1` to apply at once; `localStorage` is per origin so repeat on `:8543`, `:8444`, the Tailscale host and the LAN IP), the known-good fallback state, verify the build id in the `?inputdebug=1` header and hard reload before testing (the phone may run a cached bundle), a restart (`make install-service`) is a transport boundary so do not redeploy mid-composition, hardware/Bluetooth keyboard and DeX behavior, open `?inputdebug=1`, open diagnostics, copy the redacted report, capture-for-fixture with the scrub checklist, read `/actuator/terminal-input`, interpret the client-vs-server diagnostic.
  - *Given* the how-to, *When* followed with a manual instance, *Then* each step names a real URL or command.
- CLAUDE.md "Reference Documents Index" gains rows for both; `make registry-generate` run and changes committed if markers were added.
  - *Given* `make registry-diff`, *When* run, *Then* no pending changes.
- The how-to documents, in this order, the plain-language flag-off hint's meaning, the `?mobile_input=0|1` query parameter (this is the ONLY user-facing document that names it), and the diagnostics entry paths (settings, toolbar overflow, `?inputdebug=1`, then the 800 ms chip long-press as the secondary shortcut, with the Android context-menu note).
- "Diagnosable in under a minute" is proven by a timed drill (Task 5.1.2d), not asserted: three scripted fault injections, each diagnosed from the how-to alone, stopwatch under 60 s each, results table in `spike-findings.md`.
  - *Given* the how-to open on a second device and the manual instance running, *When* the facilitator injects fault A (layer off), B (client sending, server read-only denies) or C (WebSocket blocked) without telling the operator which, *Then* the operator names the failing hop within 60 s each, and each time and verdict are recorded.
- The Unresolved Question 1 result (ghostty off alone) is stated in the how-to.
**Files**: `docs/reference/mobile-input-layer.md`, `docs/how-to/debug-phone-typing.md`, `CLAUDE.md`, `docs/registry/features/*`

##### Task 5.1.2a: Reference page (S)
- Includes the secret-entry section's TalkBack spoken-echo note (Story 1.4.4, ADR-008 amendment), the two-live-region classification table, the chrome stack budget constants and the Story 0.1.5 viewport findings.
- Files: `docs/reference/mobile-input-layer.md`

##### Task 5.1.2b: How-to page and CLAUDE.md index rows (S)
- Files: `docs/how-to/debug-phone-typing.md`, `CLAUDE.md`

##### Task 5.1.2c: Registry markers and `make registry-generate` (XS)
- Files: `docs/registry/features/*`

##### Task 5.1.2d: Timed "diagnosable in under a minute" drill: script, three fault injections, recorded times (S, needs PR-A1 and PR-A2 merged)
- Faults: (A) `?mobile_input=0` on the phone: expected diagnosis "layer off: client emits nothing" from the `?inputdebug=1` header (`layer: off`) and a zero `cm.input` rate; (B) a read-only session with the layer on: expected diagnosis "client sent, server received but denied" from `/actuator/terminal-input` (`received` rising, `forwarded` flat); (C) WebSocket blocked: expected diagnosis "client dropped while disconnected" from `cm.input{event=dropped_disconnected}` and the diagnostics `dropped` counter. A second person (or the lead agent) injects; the operator uses only `docs/how-to/debug-phone-typing.md`. Pass: each diagnosis correct and under 60 s on a stopwatch; a slower or wrong run means the how-to is revised and the drill repeated.
- Files: `project_plans/mobile-input-layer/implementation/spike-findings.md`, `docs/how-to/debug-phone-typing.md` (revisions only)

#### Story 5.1.3: Repo gates
**As a** maintainer, **I want** all gates green before ship.
**Acceptance Criteria**:
- `make ready` passes including dupl, jscpd 0.14%, zero-warning web lint; no Go test filename contains "gate"; all new Go tests call `t.Parallel()`; the fixture lint (Task 1.1.4d) is part of the jest run.
  - *Given* the branch, *When* `unset npm_config_user_agent && make ready` runs, *Then* exit 0 and the output is quoted in the PR.
- `.jscpd.json` excludes the JSON fixtures directory (Task 0.1.4a) and `bun run lint:duplicates` stays within 0.14% with the 21 fixtures present.
- `cd web-app && bunx jest --no-coverage` green.
  - *Given* the branch, *When* run, *Then* 0 failures.
**Files**: none new

##### Task 5.1.3a: Run gates, fix debt found, record output (S)
- Files: as needed

#### Story 5.1.4: Owner phone gate and acceptance (wall-clock blockers, owner)
**As the** owner, **I want** to verify on my phone BEFORE merge on the isolated manual instance and again after the local redeploy, **so that** the real keyboard is the judge of the default; rollback is `?mobile_input=0` or flipping the flag off.
**Acceptance Criteria**:
- Scripted phrase list (the single source for Tasks 0.1.3c, 5.1.4b, 5.1.4c and 5.1.4e; kept in `spike-findings.md`, deliberately not under `__fixtures__/`, so it is never mistaken for a replay fixture) with at least 22 phrases, each typed once per renderer, in `stelekit` at a Claude Code prompt that already holds the text `ssq-prompt: ` (the "eaten prompt character" canary):

| Group | Count | Phrases (examples; the worker may substitute equivalents) |
|---|---|---|
| Plain letters, digits, punctuation | 5 | `the quick brown fox`, `ls -la /tmp`, `git status`, `error: 404 (not found)`, `a1b2 c3d4` |
| Autocorrect picks | 4 | `teh` -> `the`, `recieve` -> `receive`, `helo wrld` -> `hello world`, `dont` -> `don't` |
| Autocomplete / suggestion-strip picks | 4 | type `comm`, tap `commit`; type `ref`, tap `refactor`; pick after a space; pick after punctuation |
| Swipe words | 3 | `hello`, `terminal`, `production` |
| Backspace runs | 4 | 1 char; 5 chars; Backspace immediately after a commit; Backspace after a suggestion pick; plus one long-press Backspace word delete |
| Enter and symbols | 2 | word then Enter (must not reorder); symbol-keyboard `/` and `~` |
| Total | 22 | |

- Numeric pass thresholds (apply to Tasks 5.1.4b and 5.1.4c, per renderer): exact-match 22 of 22 phrases, i.e. the pane text after each phrase equals the text a hardware keyboard produces for the same final text; ZERO eaten characters of the `ssq-prompt: ` canary in any phrase; zero duplicated characters; and keystroke loss measured on a 200-character typed script `chars in pane / chars typed` = 100% on the phone and on desktop. A single miss fails the gate: the Copy report is attached, the phrase becomes a fixture-capture task, and for Task 5.1.4c the default stays OFF. Before the fix the baseline recorded in Task 0.1.3c is expected to be `0/22` with ghostty on.
- Latency and loss are measured, not felt (Task 5.1.4e): desktop keydown-to-echo p50/p95 recorded again on the branch and compared with the Task 0.1.3c baseline; the added p95 must be <= one frame (16.7 ms) on the same machine; on the phone the diagnostics report's `beforeinput`-to-`onData` delta p95 must be <= 16.7 ms. These are manual/benchmark records in `spike-findings.md`, never CI thresholds.
- TalkBack row (Task 5.1.4d, same phone session as Task 5.1.4c): with TalkBack on, typing `hello ` announces the committed word `hello` once (Story 1.4.4), the element is announced as "Terminal input, edit box", and in PR-B/C/D sessions chip, discard notice and hint announce once at the polite/assertive level named in Story 1.4.4. Pass: all announcements once and correct; a failure reopens Story 1.4.4 before default ON (TalkBack double-speak or silence is a defect, not a note).
- Pre-merge gate (Task 5.1.4c, on the isolated manual instance, PR-A1 built to `~/.stapler-squad/manual-builds/manual-1/`, reachable over Tailscale/LAN): soft keyboard opens on the first tap, plain letters, autocorrect, swipe, Backspace right after a commit and after a suggestion pick, Enter, under both renderers, with the `?inputdebug=1` Copy report available to record a failure. Outcome decides the server default: pass means `mobile_input_layer` merges default ON; fail means default OFF plus a per-scope owner override, and the failing report becomes a fixture-capture task.
  - *Given* the manual instance at `:62871` (and `:62872` remote), *When* the owner taps the terminal and types `"the quick brown fox"` with one autocorrect, one swipe word and a Backspace after a commit, *Then* the pane shows the same final text a hardware keyboard produces and the keyboard opened on the first tap; the full 22-phrase list then passes 22/22 per renderer with zero eaten canary characters, a keyboard-resize observation (cursor row visible above the toolbar, Story 0.1.5 row) and an autocorrect-flicker observation (at most one visible intermediate state, Story 1.2.5) are recorded; result and the chosen default are recorded in `spike-findings.md`.
- Post-redeploy acceptance (Task 5.1.4b): typing, tap-to-open keyboard, autocomplete pick, autocorrect rewrite, swipe typing, Backspace and Enter work in `stelekit` on the deployed build (Gboard; Samsung if available), under BOTH renderers.
  - *Given* the deployed build, *When* the owner first confirms the build id in the `?inputdebug=1` header matches the deployed commit (hard reload if not, the phone may run a cached bundle) and the flag shows its documented default in the panel, *Then* the typing check above passes.
- Acceptance rows beyond the happy path (each recorded pass/fail in `spike-findings.md`):
  - Remote listener `:8444` over the private-CA HTTPS origin, the LAN or Tailscale URL, an installed PWA window, and a plain-HTTP LAN origin (no `navigator.userAgentData`, UA fallback): the layer attaches and types on each.
  - Hardware or Bluetooth keyboard attached to the phone or tablet (and Samsung DeX): characters arrive once, Backspace and Enter work, no doubled characters.
  - Ctrl-W / `deleteWordBackward` (Gboard long-press Backspace) deletes a word in Claude Code's input box, or the Task 1.1.2a bounded-`\x7f` fallback is in effect.
  - Redeploy (`make install-service`) drops WebSockets: typing resumes after reconnect and the first words typed right after the restart are not duplicated or lost beyond the documented insert-only residual.
- Kill switch verified: `?mobile_input=0` applies in the same load; flag off in the panel restores the xterm path after the second load (first load persists `"0"`, second applies it).
  - *Given* the flag set off live, *When* the page is reloaded twice (or opened once with `?mobile_input=0`), *Then* the diagnostics header or the rendered renderer kind shows the xterm fallback and the flag-off hint is visible.
- Device fixtures (after PR-C only): at least one `source:"device"` fixture from capture-for-fixture passes replay with `scrubbed:true` and owner sign-off; the summary reads `synthetic N/N, device k/k` with k counting scrubbed fixtures only. Before PR-C the summary is `device 0/0` and that is acceptable for PR-A1.
  - *Given* a captured, scrubbed fixture committed, *When* the suite runs, *Then* `synthetic N/N, device 1/1`; an unscrubbed device fixture fails the lint.
**Files**: `web-app/src/lib/terminal/mobileInput/__fixtures__/*.json`, `project_plans/mobile-input-layer/implementation/spike-findings.md`

##### Task 5.1.4a: Convert scrubbed capture(s) to fixtures and update findings (S, after PR-C)
- Files: `__fixtures__/*.json`, `spike-findings.md`

##### Task 5.1.4b: Owner device acceptance check after the local redeploy, recorded in `spike-findings.md` (XS)
- Not a default flip. Re-runs the 22-phrase list on the deployed build under both renderers with the same thresholds (22/22, zero eaten canary characters). Covers the post-redeploy and acceptance rows above (build-id check first). If the check fails, use `?mobile_input=0` or flip `mobile_input_layer` off (two loads) and file the failure as a fixture-capture task.
- Files: `spike-findings.md`

##### Task 5.1.4c: Pre-merge phone gate on the isolated manual instance, sets the merge default (XS, owner wall-clock blocker)
- Build PR-A1 to `~/.stapler-squad/manual-builds/manual-1/stapler-squad`, run it per the CLAUDE.md recipe (ports 62871/62872, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`), owner runs the pre-merge criterion above on the phone, including the 22-phrase list at the numeric thresholds. Record which origins were tested. Pass: commit `defaultValue: true` and `MOBILE_INPUT_LAYER_DEFAULT = true`. Fail: keep `false`, set the per-scope override for the owner's workspace and record the Copy report.
- Files: `spike-findings.md`, `server/services/feature_flag_service.go`, `web-app/src/lib/features/flagStore.ts`

##### Task 5.1.4d: TalkBack check in the pre-merge phone session: typed-character announcement, element name, announcement levels (XS, owner wall-clock blocker)
- Uses the committed-text announcement (Task 1.4.4b) and the live-region contract (Task 1.4.4a); records pass/fail per row in `spike-findings.md`.
- Files: `spike-findings.md`

##### Task 5.1.4e: Re-measure keystroke loss and latency against the Task 0.1.3c baseline (XS)
- Phone: 200-character script, `chars in pane / chars typed`, and the diagnostics report's `beforeinput`-to-`onData` delta p95. Desktop: the same Playwright script as Task 0.1.3c, p50/p95 keydown-to-echo, compared with the baseline on the same machine. Thresholds in the Story 5.1.4 criteria above.
- Files: `spike-findings.md`

---

## Phase 6: Conditional and follow-ups

### Epic 6.1: Later enhancements
(PR-E; own sub-flag `mobile_input_edit_context`; not on the critical path.)

#### Story 6.1.1: EditContext source behind sub-flag (conditional)
**As the** owner, **I want** EditContext tried on Chrome Android only if the device corpus shows `beforeinput` gaps, **so that** we adopt it on evidence.
**Acceptance Criteria**:
- Precondition: Story 5.1.4 corpus shows at least one unrecoverable `beforeinput` gap; otherwise this story closes as "not needed".
  - *Given* the device corpus, *When* reviewed, *Then* the decision is recorded in `spike-findings.md`.
- `EditContextSource implements InputSource`, active only if flag `mobile_input_edit_context` is on and `'EditContext' in window`; reducer unchanged.
  - *Given* the sub-flag on in a browser with `EditContext`, *When* `attachMobileInput` runs, *Then* `source.kind === "editcontext"`; with `EditContext` absent, `"textarea"`.
**Files**: `inputSource.ts`, `editContextSource.ts`, `server/services/feature_flag_service.go`, `editContextSource.test.ts`

##### Task 6.1.1a: Decision record and sub-flag registration (XS)
- Files: `spike-findings.md`, `feature_flag_service.go`

##### Task 6.1.1b: `EditContextSource` + tests (M)
- Files: `editContextSource.ts`, `editContextSource.test.ts`, `inputSource.ts`

#### Story 6.1.2: File follow-up issues
**As the** owner, **I want** non-scope items tracked.
**Acceptance Criteria**:
- Issues filed in the repo (not upstream without owner approval) for: Mosh-style predictive echo (out of scope), ghostty-web upstream IME issue and PR #110 release tracking, xterm.js #3600 comment with our corpus findings, iOS Safari parity check, per-keyboard quirk profiles.
  - *Given* the list, *When* `gh issue list --repo tstapler/stapler-squad --search "mobile-input-layer follow-up"` runs, *Then* 5 issues are returned.
**Files**: none (GitHub issues)

##### Task 6.1.2a: File the five follow-up issues (XS)
- Files: none

---

## Effort estimate

Method per `ESTIMATION.md`: task classes XS 20k, S 55k, M 130k raw tokens (class midpoints); output share assumed 12% for implementation runs (CU = raw x 1.48); story verification multiplier x1.75 (spec and quality review, `6-verify` layers, CI reruns). No human-time figures. Story rows are generated from the task headings by script (`#### Story` / `##### Task ... (size)`), so counts below are derived, not estimated. Delivery slicing (PR-A..PR-E) is in the Delivery Slicing section.

| Story | Tasks | Raw tokens (k) | CU (k, x1.48) | Verified CU (k, x1.75) |
|---|---|---|---|---|
| 0.1.1 Re-verify ghostty-web citations and DECSET 2004 access | 2 | 40 | 59 | 104 |
| 0.1.2 CDP IME helper feasibility | 2 | 75 | 111 | 194 |
| 0.1.3 Owner device protocol and Ctrl-W check | 3 | 60 | 89 | 155 |
| 0.1.4 Wave-0 smoke, baseline gates and parallel-worker rules | 2 | 75 | 111 | 194 |
| 0.1.5 Android soft-keyboard resize and viewport behavior | 2 | 75 | 111 | 194 |
| 1.1.1 Grapheme-bounded rewrite diff | 3 | 165 | 244 | 427 |
| 1.1.2 Composition reducer | 3 | 315 | 466 | 816 |
| 1.1.3 Modifier and paste semantics | 2 | 75 | 111 | 194 |
| 1.1.4 Replay corpus and harness | 7 | 385 | 570 | 997 |
| 1.2.1 Hidden textarea, attribute policy, styling, platform gate | 4 | 150 | 222 | 388 |
| 1.2.2 `attachMobileInput` event normalization and execution | 4 | 295 | 437 | 764 |
| 1.2.3 Focus ownership and gesture interplay | 3 | 165 | 244 | 427 |
| 1.2.4 Delivery-aware diff base and boundary invalidation (PR-A minimum, safe on a link flap) | 5 | 350 | 518 | 906 |
| 1.2.5 Rewrite flicker budget and DeferredCommit feedback | 2 | 110 | 163 | 285 |
| 1.3.1 Server flag registration | 2 | 75 | 111 | 194 |
| 1.3.2 Client flag, `resolveRenderer`, split touch predicates | 5 | 240 | 355 | 622 |
| 1.4.1 Shared `useMobileInput` hook and renderer wiring | 6 | 370 | 548 | 958 |
| 1.4.2 One parametrized integration test over both renderers | 2 | 150 | 222 | 388 |
| 1.4.3 Chrome stack budget and minimum usable terminal rows | 2 | 110 | 163 | 285 |
| 1.4.4 Live-region contract and TalkBack typed-character announcement | 2 | 110 | 163 | 285 |
| 2.1.1 `recordTerminalInput` helper | 2 | 110 | 163 | 285 |
| 2.1.2 Instrument all input entry points | 3 | 165 | 244 | 427 |
| 2.1.3 Actuator endpoint and throttled logs | 2 | 110 | 163 | 285 |
| 2.2.1 Allow-list `cm.input` and `cm.ime_anomaly` | 1 | 55 | 81 | 142 |
| 2.2.2 Client sender with rate limiting | 2 | 75 | 111 | 194 |
| 2.2.3 Docs table and dashboard row | 2 | 110 | 163 | 285 |
| 2.3.1 Minimal read-only `?inputdebug=1` panel with Copy report (PR-A1) | 1 | 55 | 81 | 142 |
| 3.1.1 `SendResult` and `onInputDropped` | 2 | 110 | 163 | 285 |
| 3.1.2 Visible status chip and the single chrome hook | 3 | 130 | 192 | 337 |
| 3.1.3 Bounded plain-text queue of atomic units | 3 | 240 | 355 | 622 |
| 3.1.4 Queue reports delivery; layer base follows the delivered prefix | 2 | 110 | 163 | 285 |
| 3.1.5 Chip extra states and discard notices | 2 | 110 | 163 | 285 |
| 4.1.1 Ring buffer and redacted report (PR-A1; pulled forward from PR-C) | 2 | 110 | 163 | 285 |
| 4.1.2 Diagnostics bottom sheet | 3 | 240 | 355 | 622 |
| 4.1.3 Opt-in "capture for fixture" mode (local-only) | 2 | 110 | 163 | 285 |
| 4.2.1 Opt-in compose bar | 4 | 260 | 385 | 673 |
| 4.2.2 IME-fallback detector and notice | 3 | 165 | 244 | 427 |
| 5.1.1 Playwright mobile-emulated wiring spec | 5 | 315 | 466 | 816 |
| 5.1.2 Documentation | 4 | 185 | 274 | 479 |
| 5.1.3 Repo gates | 1 | 55 | 81 | 142 |
| 5.1.4 Owner phone gate and acceptance (wall-clock blockers, owner) | 5 | 135 | 200 | 350 |
| 6.1.1 EditContext source behind sub-flag (conditional) | 2 | 150 | 222 | 388 |
| 6.1.2 File follow-up issues | 1 | 20 | 30 | 52 |
| **Total** | 120 | 6515 | 9642 | 16874 |

Total verified cost is about 16.9M CU (120 tasks: 26 XS, 83 S, 11 M; 43 stories), recounted by script from the headings after the triad-review repair of 2026-11-08 (previous figure: 102 tasks, 39 stories, 14.9M CU; the 18 added tasks and 4 added stories add the Android keyboard-resize spike, rewrite-flicker and DeferredCommit feedback, chrome stack budget, live-region and TalkBack contract, stuck-state copy, settings entry, anomaly-based compose offer, baseline and re-measure tasks, the timed diagnosis drill, and the 1.1.4b fixture split, which moves one M to three S). This is above the 15M top edge of the Large band (3-15M); bands are INFERRED and uncalibrated, the owner's standing direction is full scope, and the figure is a tracking number, not a scope gate. SDD review and repair loops for Phases 4-6 are not included.

**Critical path (agent waves, PR-A1 only)**: W0 smoke + `make ready` baseline (0.1.4) -> W1 spikes (0.1.1, 0.1.2/0.1.2b, 0.1.5b parallel, each in its own worktree; owner device spikes 0.1.3b, 0.1.3c and 0.1.5a in one phone session as a wall-clock item that must finish before 1.1.2a, 1.2.3c and 1.4.3a are merged) -> W2 types + rewrite (1.1.1) -> W3 reducer (1.1.2) -> W4 keymap + corpus + fixture lint (1.1.3, 1.1.4) -> W5 policy/platform/adapter (1.2.1, 1.2.2) -> W6 focus ownership + flag/gate + rewrite-frame sampler (1.2.3, 1.3.x, 1.2.5a) -> W7 hook + renderers (1.4.1), then chrome budget and live-region contract (1.4.3, 1.4.4) and DeferredCommit feedback (1.2.5b) on the chrome shell -> W8 integration test + version pin (1.4.2) and minimal diagnostics (4.1.1, 2.3.1) -> W9 e2e (5.1.1a, b, d, e, 1.4.3b) -> W10 docs + gates (5.1.2, 5.1.3) -> owner pre-merge phone gate (5.1.4c with 5.1.4d TalkBack and 5.1.4e re-measure in the same session) -> merge -> redeploy -> owner acceptance (5.1.4b); Story 1.2.4 (delivery-aware diff base, with its stop rule) runs in W6 alongside focus ownership, and the chrome hook shell (1.4.1e) in W7. PR-A2 (Phase 2 server/telemetry/alert, 3 waves) runs in parallel from W2 in its own worktree. PR-B (Phase 3), PR-C and PR-D start after W7 and are off the PR-A1 critical path. About 11 serial waves on the PR-A1 critical path.

**Overrun checkpoint**: if cumulative spend exceeds the verified CU of the completed stories by more than 25%, pause and report the delta and cause (wrong estimate or real surprise); informational, no scope cut.

**Wall-clock blockers**:
- Owner device spike (Task 0.1.3b: Backspace-on-empty, tap-opens-keyboard, origins; Task 0.1.3c baseline; Task 0.1.5a keyboard-resize modes) before Tasks 1.1.2a, 1.2.3c and 1.4.3a merge (owner: Tyler).
- Owner pre-merge phone gate on the isolated manual instance (Task 5.1.4c, with the TalkBack check 5.1.4d and the loss/latency re-measure 5.1.4e); it decides the merge default (owner: Tyler).
- Timed diagnosis drill (Task 5.1.2d) after PR-A1 and PR-A2 are merged: needs a facilitator and a second device (owner: Tyler).
- Owner phone acceptance after `make install-service` redeploy, including the build-id check (Task 5.1.4b) (owner: Tyler).
- CI runtime for `make ready` and e2e (CI).
- Owner decision to approve the ADR-004 queue behavior in the PR-B description (owner: Tyler).
- Owner scrub sign-off for each committed device fixture (owner: Tyler).
