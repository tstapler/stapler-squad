# ADR-005: Hidden Sessions Open Read-Only; Enforcement Is Server-Side, Derived From `Snapshot().Hidden`

**Status**: Accepted; implemented on this branch except Reply (Story 5.6, PR 5r, not built yet). The revision notes below were written while Proposed; the focused re-reviews they mention were superseded by the phase 6 verify review (verdict REFACTOR, follow-ups fixed in the commits after `b5cefeb8e`). (Revision history: revised in plan repair iteration 3: capability-based write helpers, typed capabilities plus four pinned checks (the broader derived scan of that revision was replaced in iteration 5 and is a deferred upgrade, decision 3), exactly one audited exception for Reply per operator decision O2; Phase 4: a second, narrow exception for the shipped backlog steer per operator decision O7, **confirmed by the operator on 2026-10-09**, plus a live-settable flag for the unary guards; triad iteration 1: Reply has its own kill switch, ADR-004 decision 11, and the capabilities, the O7 exemption and the Reply path are **pending a focused architecture + adversarial re-review**, see plan "Pending focused re-review"; **review-repair iteration 1 (2026-10-09) rewrote decisions 3, 5, 8 and the consequences for re-review ARCH-B3 ("exactly two" described UI entries only), ARCH-C4..C6 and ADV C6..C10; the re-review has not been re-run on this text**; **review-repair iteration 2 (2026-10-09) rewrote decisions 3, 5 and 8 and the consequences again for focused re-review iteration 2: ARCH-NB1 (the scan rule forbade `UpdateSession` from calling `steerInstance`, which the design requires, and the token that was meant to distinguish the UI caller was constructible anywhere in package `services`; the writer is split into the token-gated `steerAuthorized` and the internal `steerInternal` and token construction becomes a scanned property), ADV-N1 (every listed writer must take the per-instance write lease), ARCH-C17/C18 and ADV-N7 (the in-handler verdict has no `LoopbackBound` condition), ARCH-C24 (the `tools_diagnose.go` citation names its function). The re-review has not been re-run on this text either**; **review-repair iteration 3 (2026-10-09) rewrote decision 3 again and touched decisions 2, 5, 6 and the consequences for focused re-review iteration 3: ARCH-NB3 (scan rule S4 required every listed writer to reference a non-reentrant lease although the listed writers call each other, so the O7 steer and `SteerActiveSession` would have seen "busy" from their own inner call; the lease becomes a passed `*HeldLease` capability with one acquirer per call chain, released by the writing goroutine, and S4 is replaced by the typed rules L1-L3), ARCH-C27 (token construction by `var`, `new` and conversion, the interface-dispatched `TriggerRemediationNow` path as a reviewed list, an injectable handler set), ARCH-C28 and ADV-N13 (the `./session` write surface, scroll forwarding, `SwitchWorkspace`), ADV-N18 (`guard_bypass` refuses with the sink down), ARCH-NB4 and ADV-N14 (the rebinding Host allowlist is the verified set, and the `:8444` rule). **review-repair iteration 4 (2026-10-09) rewrote decision 3 (the five-class write surface, rules L1-L3 scoped to `leaseWrite`, no `ErrNestedLease`) and decision 6 (`UpdateSession`'s `program` and `auto_approve` fields, the `SwitchWorkspace` ordering) for ARCH-NB6, ADV-N22, ARCH-C35, ARCH-C39 and ADV-N28; the re-review has not been re-run on this text**; **review-repair iteration 5 (2026-10-09, the last repair round, a simplification and not a finding-driven round) rewrote decision 3: the whole-tree five-class scan (rules L1-L3, P1-P3, S1-S3, the internal-writer lists) was removed from the default design in favor of typed capabilities plus four cheap checks (capability-literal lint, RPC-descriptor classification, a short pinned-caller table, R1/R2), with the removed scan recorded as a deferred upgrade that Task 5.1d-0 may add and with an explicit list of what the simpler design does not catch; decisions 5 and 8 and the consequences were edited to match; every behavioral protection is unchanged; the re-review has not been re-run on this text and nothing is marked verified-resolved**)
**Date**: 2026-10-07
**Project**: notification-tray-and-hidden-session-gate

## Context

`GetSession` already returns hidden sessions
(`server/services/session_service_crud.go`, no Hidden filter) and
`web-app/src/app/page.tsx:222-250` `fetchHiddenSessionFallback` resolves
`?session=<id>`, but the effect returns early while `sessions.length === 0`
(page.tsx:243). The session then loads in the normal interactive terminal:
`TerminalData.Input`/`Resize` frames are accepted at
`server/services/session_service_stream_terminal.go:405`,
`connectrpc_websocket.go:2701` and `:3337`, `connectrpc_websocket_shell.go:257`,
and `WriteToSession` (`terminal_service.go:102`, delegated at
`session_service_delegates.go:931`) has no guard. A UI-only flag is bypassable
(`instinct_mcp_dispatched_agent_tool_surface`).

## Decision

1. The server decides read-only per stream from `inst.Snapshot().Hidden` at
   attach time. A client-supplied `read_only` field, if ever added, may only
   tighten.
2. Input and Resize frames are dropped (debug-logged) through one shared
   helper (`acceptsInput()` on the stream params) at all four sites. Resize is
   dropped too, so a read-only viewer cannot vote on tmux pane size
   (#728/#731). A read-only attach also skips scroll forwarding (`Instance.ForwardScroll`, `session/instance_scroll_forward.go:105`, a PageUp pane write reached from `connectrpc_websocket.go:2805`; review-repair iteration 3).
3. **Provable by types, plus four checks** (simplified in review-repair
   iteration 5; the earlier whole-tree scan is recorded below as a deferred
   upgrade). **Compile-time half.** A typed `TerminalAccess`
   (`ReadOnly`/`ReadWrite`) is produced only by `AccessFor(inst)` from
   `inst.Snapshot().Hidden`; `access.Writer()` yields a `TerminalWriter`
   capability (an error for `ReadOnly`). Every non-MCP terminal-write helper and
   every params struct on a UI write path holds a `TerminalWriter`, never a
   `readOnly bool`, so an ignored check cannot compile into a write
   (architecture C4). The per-instance write lease is a passed
   `*session.HeldLease` (plan Story 5.0): the primitives that type under it
   (`SubmitDriverContent`, `SubmitContentWithEnter`, `SendKeysWithTimeout`,
   `SubmitReplyOnce`, `steerAuthorized`) take one, so "forgot to take the lease" is
   a compile error for every primitive that takes it, and the steer writer takes a
   `steerAuthorization` token (kind plus instance UUID; refuses a zero value or a
   token for another instance at runtime). The two UI exceptions are capabilities
   (`PendingQuestionClaim` for Reply, `BacklogReviewLink` for the O7 steer), each
   with unexported fields and one constructor, and each exception's entry point
   (`replyToPendingQuestion`, `steerHiddenReviewViaBacklogLink`) takes its
   capability as a parameter, so neither can be reused for another write.
   **Checks for what the compiler cannot see** (run in `make test-delivery-guards`
   behind the `sinkguard` tag, type-based via `go/packages`; plan Story 5.1d):
   **(a) literal lints**: **(a2)** no string constant `"send-keys"`, `"paste-buffer"`, `"load-buffer"`, `"resize-window"` or `"resize-pane"` in a non-test file outside `session/tmux`, `session/tymux` and the definitions of `sendInputToTmux` and `resizeExternalCapturePaneSession` (the subprocess-`tmux` class is not a method call; added in iteration 6 after `handleCapturePaneInput` was found to type through `sendInputToTmuxWithRetry` with no primitive-set member), and the **capability-literal lint**: no composite literal, `new`, conversion or
   initializer-less `var` of `session.HeldLease`, `steerAuthorization`,
   `BacklogReviewLink`, `PendingQuestionClaim` or the concrete `TerminalWriter`
   outside that type's one defining file (inside a package an unexported type is
   constructible by any function, so this closes the gap the compiler leaves);
   **(b) RPC-descriptor classification** (T-RO-17): every RPC of every service in the generated package (14) is
   classified as a terminal write or not, `UpdateSession` by field
   (`steer_message`, `program`, `auto_approve`), `SwitchWorkspace` as a write, and
   an unclassified new RPC fails; this is the part that protects hidden sessions
   from a **new write RPC**; **(c) a short pinned-caller table**: the set of
   non-test callers of the pane-typing primitives (`Instance.SendKeys`,
   `SendKeysN`, `WriteToPTY`, `TapEnter`, `SubmitDriverContent`,
   `SubmitContentWithEnter`, `SendKeysWithTimeout`, `SubmitReplyOnce`,
   `ClaudeController.SendCommandImmediate`, `changeDirectory`, the restart-marker
   path), of the steer entry points (`steerUnderLease`, `steerInternal`,
   `SteerActiveSession`), of `AccessForUnary` and of the lease acquire API must
   equal a table with a one-line reason per row (kinds `ui`, `acquirer`,
   `receiver`, `lifecycle`, `none`), in both directions, so a reviewer has to read
   every new or removed caller; the PTY layer (`session/tmux`, `session/tymux`,
   `session/mux`, the four process-manager files, `instance_tmux.go`) is exempt by
   name. The acquirer rows are the **closed list** of lease acquirers (no
   lifecycle unit is on it, which makes "Pause and Delete do not queue behind a
   wedged write" a checked property); **(d) `uiWrite` R1/R2**: every `./server`
   function that calls a UI-stream primitive (`WriteToPTY`,
   `SendInputViaControlMode`, `ResizePTY*`, `SetWindowSize*`, `RequestResize`,
   `ForwardScroll`, and the capture-pane helpers `sendInputToTmux`,
   `sendInputToTmuxWithRetry` and `resizeExternalCapturePaneSession`, which are
   real `./server` callers behind an injectable `tmuxInputSender` seam, **together with the seam's own `SendInput` and `Resize` methods**, so a handler that reaches the sender without a `TerminalWriter` still fails R1/R2; the helper definitions and, by name, the real sender's two methods are the only exemptions; review-repair iteration 6b, ADV-N32), and every `ui` row, must hold a `TerminalWriter` or
   `QuestionReplyWriter` (R1) and reference it in its body (R2), which protects
   the read-only guarantee. Negative controls (an unlisted caller, a stale row, a
   writer never referenced or absent, a capability constructed five ways outside
   its file, a Pause-like acquirer) and one positive control (the real tree passes
   all four with the table as written, T-RO-44) run in the same target. The
   steer chains are additionally run at runtime (T-RO-43: one write on a free
   lease, the retryable busy result on a held lease, no link reporting busy to
   itself). Task 5.1d-0 derives the table from the real tree and records its row
   count before any rule is encoded.
   **What this does NOT catch** (stated honestly; none is a regression against
   `main`, where nothing was checked): (1) a **new pane-typing helper called from
   a new handler without a capability** is caught only by the pinned-caller test
   and review: by the test when the helper or the handler calls a primitive-set
   member from outside the PTY layer (a new row a reviewer must read and accept),
   and by review alone when it reaches the pane through the PTY layer without
   calling one (for example a new `Instance` method in `instance_tmux.go` that
   types through the tmux session); a new RPC is additionally stopped by check
   (b) until classified, and a new stream site is not stopped by (b) but by (d)
   only if it calls a UI-stream primitive; (2) a new `ProcessManager` or `TmuxManager`
   implementation that writes sits in the by-name exemption until a reviewer
   extends it; (3) a **nested lease acquire** in code no chain test runs
   (including direct nesting inside a new function) is not caught statically, it
   shows up as a spurious busy result and is caught only by T-RO-43, T-WL-02 and
   the per-acquirer `AssertLeaseFree` tests (there is no `ErrNestedLease`, no
   context marker and no production panic); (4) a handler that passes
   `steerAuthorized` a token not obtained from `decideSteerAccess` is no longer a
   scan rule: the token's instance UUID and kind are verified at runtime and the
   callers are pinned; (5) a guard that is present but not control-flow correct is
   covered by per-site behavioral tests, not by these checks.
   **Deferred heavier scan (the earlier design; kept so the upgrade is cheap).**
   Review-repair iterations 2 to 4 built a whole-tree scan: a five-class partition
   of every unit that calls a primitive (`leaseWrite`, `uiWrite`,
   `primitiveLayer`, `lifecycleExempt`, `noProductionCaller`), rules **L1-L3**
   (a `leaseWrite` unit takes a `*HeldLease` or acquires; the lease is used; a
   lease-holding unit calls no acquirer, and `HeldLease` literals only in
   `instance_write_lease.go`), boundary rules **P1-P3** for the primitive layer
   (including every `ProcessManager`/`TmuxManager` implementation derived by
   `types.Implements`), steer rules **S1-S3** (a handler passes a token that came
   from `decideSteerAccess`; token construction only in `decideSteerAccess` and
   `steerInternal`; no handler calls `steerInternal` or a named internal-writer
   function), a named per-function internal-writer list and a reviewed list of
   UI-reachable internal writers. It was removed from the default design because
   the type-level capability already makes its lease half largely redundant (a
   primitive that takes `*HeldLease` cannot be called without one), because the
   partition, the boundary rules and the long pinned lists cost maintenance to
   guard unreviewed future code, and because each review pass found fresh
   defects in exactly that machinery (NB1: the rule forbade the plan's own
   `UpdateSession -> steer` call; NB3: S4 contradicted the non-reentrant lease;
   NB6 and ADV-N22: L1 failed on the table's own no-lease rows and on about
   twelve inner session functions; ARCH-C35: the nesting marker was not
   buildable). **Task 5.1d-0 is the only mechanism that may bring it back, in
   whole or as the single rule that closes a gap**: it runs the old rules against
   the real tree only far enough to decide whether the four checks leave a real
   path from an RPC handler or stream site to a pane write with neither a
   capability nor a table row, and whether the pinned table stays short. The
   direction is upgrade only; the spike never removes a check. If it upgrades,
   the cost of Story 5.1d returns toward its previous M (4xM + 1xS).
4. Resize is covered beyond the frame dispatch: `applyOneControlModeResize`
   (`connectrpc_websocket.go:1504`), `applyOneShellResize`
   (`connectrpc_websocket_shell.go:148`), `shellPanePTY.ResizePTY`
   (`connectrpc_websocket.go:2358`), hub `RequestResize`
   (`connectrpc_websocket.go:1986,2161`) and attach-time size negotiation
   (`session/streamhub/hub.go` `AttachSubscriber`, `applyNegotiatedSize`). A
   read-only subscriber capability in `StreamHub` ignores its size.
5. Unary operator writes (`TerminalService.WriteToSession`, the steer branch of
   `UpdateSession`) return `FailedPrecondition` for a hidden target, **with one
   exemption: the backlog steer (operator decision O7, confirmed by the
   operator on 2026-10-09; the exemption itself awaits the focused re-review)**. The shipped backlog "Steer" composer calls
   `updateSession(sessionId, { steerMessage })`
   (`web-app/src/components/backlog/BacklogItemDetail.tsx:628`) on `review`
   sessions that `isSteerable` allows (`web-app/src/lib/backlog/sessionKind.ts:52-55`)
   and `SpawnReviewSession` creates them `Hidden: true`
   (`session_service_diagnose.go:54-60`), so a blanket guard would break it
   (pre-mortem #1). The steer branch therefore allows a hidden target only
   through `access.BacklogSteerWriter(link)`, where `link` is a
   `BacklogReviewLink` returned by `BacklogLinkResolver.ResolveLiveReviewLink`.
   **The predicate keys on durable state, never a tag** (re-review ARCH-C5,
   ADV C7): `row := GetItemSessionBySessionUUID(uuid)` (it returns the **newest**
   row of **any** role, `session/storage_backlog.go:380`, so a newer non-review
   row means no link) with `row.Role == SessionRoleReview` and
   `row.EndedAt == nil` (as `IsDiagnoseCaller`, `session/storage.go:1358`, keys on
   `Role`), **and** the instance is live (status not Stopped, Crashed or Paused;
   `VerifyPaneOwner` passes), re-checked after the audit append because
   `EndedAt` is written asynchronously. The `backlog:review` tag is mutable
   (`applyTagsUpdate`, `session_service_update.go:60-75`, runs earlier in the same
   request) and is not part of the predicate; a tag edit never changes steer
   eligibility. A triage, diagnose, `other`, unlinked or ended session has no
   link, so its steer is unrepresentable.
   **Placement and ordering** (re-review ARCH-B3, C4): the access decision is made
   **first** in `UpdateSession`, right after the instance is found
   (`session_service_update.go:156-161`) and before any title, tag, category,
   note, working-directory or auto-approve mutation (several publish in memory
   immediately, `:166-169`, so a rejection after them leaves partial state). For
   a hidden target the request must carry **only** `steer_message` (otherwise
   `InvalidArgument`, `steer_must_be_alone`; the composer sends only that field),
   input validation and the pre-write audit append also happen there, and the
   write happens in the existing branch position. `steerInstance` is **split**
   (review-repair iteration 2, ARCH-NB1): the token-gated
   `steerAuthorized(ctx, auth steerAuthorization, inst, msg)` is what
   `UpdateSession` calls at the existing position, with a token that the named
   function `decideSteerAccess` built (from a visible-target decision or a
   `BacklogSteerWriter`; the only constructor of the `visible` and `backlog_link`
   kinds), and the thin `steerInternal(ctx, inst, msg)` is what
   `SteerActiveSession` calls (the only other constructor, kind `internal`); the
   capability-literal lint confines token construction to one file and the
   pinned-caller test pins the callers of `steerInternal`, `steerUnderLease` and
   `SteerActiveSession` (decision 3, checks (a) and (c); review-repair iteration 5
   replaced the earlier rules S1 to S3), and `steerAuthorized` refuses a token
   whose instance UUID is not `inst.UUID` and a zero-value token. Honest limit:
   inside package `services` an unexported token is constructible by any
   function, so the lint, not the compiler, enforces this, and no rule checks
   that a handler passes a token that came from `decideSteerAccess` (the runtime
   UUID check does). The writer still never decides access (its second
   caller has no capability); it only verifies the token. **Lease (review-repair iteration 3, ARCH-NB3):** the visible path calls the acquirer `steerUnderLease`, which takes the write lease once and passes the `*HeldLease` into `steerAuthorized`; `steerInternal` and `steerHiddenReviewViaBacklogLink` are acquirers too; `steerAuthorized` never acquires. The in-handler
   Host/Origin verdict is the Story 1.8 **rebinding gate**, which has no
   `LoopbackBound` condition, so a legitimate LAN-hostname browser on the
   supported bind keeps steering exactly as before; the steer does not apply the
   Reply-only local-caller gate (ADR-010 decision 7), and the audit line records
   `peer_loopback` and `proxied`.
   **Audit continues in both flag states** (re-review ARCH-C6, ADV C8): the path
   is audited in the same sink as Reply (no audit, no write) whether or not
   `hidden_session_readonly_guards` is on, with the explicit degraded modes of
   ADR-010 decision 6 when the sink is down (guards on: refuse with an explicit
   message; guards off: the qualifying steer proceeds with a fallback log record). While the flag is off, a steer to a
   non-qualifying hidden target is allowed (the documented escape hatch) but
   audited as `kind=guard_bypass` with a WARN at most once a minute and a counted
   Settings status line; **with the audit sink down a `guard_bypass` write is refused**, like Reply (review-repair iteration 3, ADV-N18: the hatch exists to restore the review composer, not to open triage, diagnose and `other` sessions while the audit is blind; the hatch flip itself and the qualifying review steer keep the fallback record). The flag is read through an injected `GuardsFlag`
   interface (`ReadOnlyGuardsEnabled() bool`, backed by an atomic that the flag
   service updates, **not** a `guardsEnabled bool` parameter any caller can pass
   as `true`, and not `config.LoadConfig()` per RPC since `WriteToSession` is not
   a rare RPC) and **fails closed**: an unreadable config keeps the guards ON.
   `AccessForUnary(inst, flag GuardsFlag)` is the constructor. The O7 steer text
   keeps the 10000-byte `MaxSteerMessageLength` limit and is rejected if it
   contains ESC or any other control character except LF and TAB, C1, U+2028/9 or
   the bidi and invisible format characters of ADR-010 decision 5 (the unchanged
   `SteerActiveSession` and visible-target steer keep today's behavior).
   **Tasks 5.2b and 5.2e land together** (one PR): between them every backlog
   steer of a review session would fail. A characterization test of the shipped
   behavior is written and green on `main` BEFORE the guard lands.
   Read RPCs (`GetSession`, `GetTerminalSnapshot`, `GetSessionDiff`) stay open.
   The unary guards (`WriteToSession`, the steer rejection, Restart) are behind
   the live-settable flag `hidden_session_readonly_guards` (default on, global
   only, ADR-004 decision 10) so an incident has a non-deploy escape hatch; the
   stream drops and the Reply path are not behind it. **Safety rail, not a
   security boundary**: a holder of the auth cookie or any process on `:8543`
   can already create a shell session, steer visible sessions and flip this flag
   through `UpdateFeatureFlag`; the guards protect against confused-deputy and UI
   bugs. Every change of this flag is recorded (ADR-010 decision 6: a flip to off is
   persisted even when the audit sink is down, as the escape hatch for a sink
   fault) and the O7 steer is checked against the Host and Origin rebinding gate
   (ADR-010 decision 7), called from inside the handler; unlike Reply it is **not**
   refused for a remote caller on an unauthenticated listener, because that would
   regress the shipped composer for LAN-hostname browsers.
6. `RestartSession` (`session_service_lifecycle.go:616`) and `RestartShell`
   (`session_service_shells.go:159`) reject hidden targets from the UI handlers
   (they mutate the pane); internal callers are unaffected. **`SwitchWorkspace`** (`server/services/workspace_service.go:341`) is guarded the same way (review-repair iteration 3): its directory switch types `cd "<dir>"` and a newline into the pane (`session/instance_workspace.go:362`); revision and worktree switches restart the agent (`:167-229`), so it is guarded for **every switch type** and **before** the in-flight store and the pre-switch checkpoint (review-repair iteration 4, ARCH-C39). **`UpdateSession`'s `program` and `auto_approve` fields are guarded the same way (review-repair iteration 4, ADV-N28)**: both call `Restart(true)` for an Active instance (`session_service_update.go:241,300`, `instance_program.go:102`, `instance_actor_setters.go:618`), which kills the agent and types an `echo` marker and an Enter into the new pane (`instance.go:2608-2618`); the earlier reason for exempting the marker, "the UI `RestartSession` is refused for a hidden target", covered one of three UI routes, and a pinned caller list now keeps a fourth from appearing unnoticed. Pause/resume/delete
   are lifecycle, not terminal writes, and stay unchanged. Approval resolution
   and `RunCommand`-style RPCs are listed in the Story 5.2 PR with a
   disposition each. (Confirm at review.)
7. MCP write tools keep working for dispatchers. VERIFIED: no `server/mcp`
   code calls `TerminalService.WriteToSession`; MCP uses
   `session.SendKeysWithTimeout` (`tools_terminal.go:325`) and `inst.SendKeys`
   (`:402`) directly, so the unary guard does not affect them. A
   characterization test pins this; `withDiagnoseGate` is not wired.
8. No UI "unlock". Nudging a stuck hidden session stays on
   `diagnose_nudge_session`. **One narrow exception (operator decision O2,
   ADR-010)**: `ReplyToPendingQuestion` answers an outstanding "Claude has a
   question" (`INPUT_REQUIRED`) in a hidden session. It is reachable only through
   a single-use `PendingQuestionClaim` capability that exists while a question
   is pending, needs an audit line before the write, is rate-limited, is single
   line and 500 bytes, and rejects visible sessions. It is one of two
   **UI** exceptions (the other is the O7 backlog steer of
   decision 5; the token-gated `steerAuthorized` and the `acquirer` rows of the
   pinned-caller table of decision 3 are separate), and each
   exception's entry point must take its own capability parameter
   (`PendingQuestionClaim`, `BacklogReviewLink`), so neither exception can be
   reused for another write. The Reply write path is its own primitive
   (`SubmitReplyOnce`), not the steer primitive (ADR-010 decision 3). `WriteToSession`, the steer branch for a
   non-qualifying hidden target, stream input and resize all stay blocked.
9. Client: `readOnly` mode on `XtermTerminal.tsx`/`TerminalOutput.tsx`, a
   "Background session - read-only" banner that renders even when the session
   was deleted (falls back to the notification's captured message). Until the unary guards (plan PR 5u) merge, its secondary text claims only what the stream guards deliver ("Terminal input is disabled in this view."), and PR 5u switches it to "You can read output but not type." (review-repair iteration 6b, ADV-N36). Never set
   `IncludeHidden` on the main list call; Background activity uses a new
   `hidden_only` flag on `ListSessionsRequest` with polling (`WatchSessions`
   keeps dropping hidden).

## Alternatives Considered

- Whole-tree five-class scan with rules L1-L3, P1-P3 and S1-S3 as the default
  (built in review-repair iterations 2 to 4): removed in iteration 5 and kept as a
  deferred upgrade (decision 3); a spike, not a default, decides whether it is
  worth its maintenance cost.
- Un-hide in the main list: rejected by requirements.
- Client-only `readOnly`: rejected (bypassable).
- New hidden-session list RPC: rejected; a request flag reuses the existing
  filter in `session_service_crud.go:61,101`.

## Consequences

- One proto field (`hidden_only`, next free number in `ListSessionsRequest`,
  currently up to 8) and `make proto-gen`; generated output stays gitignored.
- Hidden-session pause/resume/delete RPCs are left alone (decision 6).
- A new UI write handler cannot reach a hidden session by calling a pane-typing
  primitive without either holding a `TerminalWriter` (impossible for a hidden
  target; the stream writers are checked by R1/R2) or adding a row to the
  pinned-caller table, which a reviewer must read; a new write RPC fails the
  descriptor classification until classified. The guarantee therefore rests on
  typed capabilities (`TerminalWriter`, `*HeldLease`, the steer token, the two
  UI-exception capabilities), the RPC-descriptor test and the pinned-caller test;
  decision 3 states what that does not catch (a new helper that reaches the pane
  through the PTY layer without a primitive-set call, a new `ProcessManager` that
  writes, a nested lease acquire in untested code), which only review catches.
  The UI exceptions are two (Reply and the O7 backlog steer), the token-gated
  writer is `steerAuthorized` (its token constructed only in one file) and the
  internal writers are the `acquirer` rows of the table.
- The read-only promise now reads: "a hidden session accepts no terminal input
  and no resize from the UI, except one audited, rate-limited reply to a question
  that is actually outstanding, and the backlog steer of a live, backlog-linked
  review session (audited, typed, shipped behavior preserved)." O7 is
  confirmed (option (a) in plan.md); option (b), which would have removed the
  second entry and `BacklogReviewLink` and the backlog UI's steer for review
  sessions, was rejected and is recorded only so the rejected path stays visible.
- The guard checks and the sink guard run through `make test-delivery-guards` (part of
  `make ci`), not the inner `make test` loop.
