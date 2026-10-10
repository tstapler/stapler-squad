# ADR-010: One Audited, Rate-Limited Reply Path Into a Hidden Session (One of Two Exceptions to Read-Only)

**Status**: Proposed (operator decision O2, DECIDED 2026-10-07: add an audited Reply control now; triad iteration 1: dedicated kill switch `hidden_session_reply`, proto moved to Contract PR 3 after Spike 1.3g; **review-repair iteration 1 (2026-10-09) rewrote decisions 2, 3, 5, 7 and 8 for the focused re-review's blockers: ARCH-B1/ADV-B1 release-on-failed-write double-typing, ADV-B2 claim bound to a question rather than the dialog on screen, ARCH-B2 false "authenticated" claim on :8543, ARCH-B4 wrong-session attribution. The re-review has NOT been re-run on this text; status stays Proposed until it passes**, see plan "Pending focused re-review" item 1; **review-repair iteration 2 (2026-10-09) rewrote decisions 1 to 9 again for focused re-review iteration 2: ARCH-NB2 (the claim was bound to `StatusNeedsApproval`, the permission-dialog state, so a permission dialog after an answered question could be promoted and approved by a later Reply), ARCH-C15 to C21 and C17/C18 (abort race, write overlap, limiter order, `reply_id` binding, `IN_PROGRESS`, in-handler verdict), ADV-N1 (the session driver is a concurrent writer), N2, N6, N8, N9, N10, N11. The re-review has not been re-run on this text either; status stays Proposed**; **review-repair iteration 3 (2026-10-09) changed decisions 2, 3, 6, 7 and 9 and added the "Residual risk and sequencing" section for focused re-review iteration 3: ARCH-NB3 (the write lease is a passed `*HeldLease`, one acquirer per chain, its own Story 5.0), ARCH-NB4 and ADV-N14 (the rebinding Host allowlist is the forward-DNS-verified set; the `:8444` and IP-literal rules), ARCH-C31 (supersede path matching and the promotion compare-and-set), ARCH-C32 (spike unknowns), ADV-N15 (hook sender proof, boilerplate denylist), ADV-N16 (keystroke plan per question shape, pre-Enter recheck), ADV-N17 and N18 (audit paths), and the operator decision to keep Reply, sequenced last. **review-repair iteration 4 (2026-10-09) rewrote decision 2d4 (the hook proof is keyed to the session UUID, kept out of argv, replaces stale entries, has a sender table, dedupes proofless duplicates, is cause-labelled, and its honest protection is stated: not the agent itself) and decisions 3, 6 and 7 for ADV-N23, ARCH-C37, ADV-N21, ADV-N24, ARCH-NB5, ARCH-C38, ADV-m11 and ADV-m12, and recorded the accepted limit that Reply works only for sessions started after hook proof ships.** The re-review has not been re-run on this text; status stays Proposed**) **Review-repair iteration 5 (2026-10-09, simplification) edited decisions 3 (the lease paragraph) and 9 only, to point at the simplified guard set of ADR-005 decision 3; the Reply design is unchanged and the status stays Proposed.**
**Date**: 2026-10-07 (repaired 2026-10-09)
**Project**: notification-tray-and-hidden-session-gate
**Amends**: ADR-005 (read-only is enforced server-side; this is one of its two UI exceptions, the other being the O7 backlog steer, ADR-005 decision 5). Resolves the dead end noted in ADR-002.

## Context

A hidden session's `INPUT_REQUIRED` notification, "Claude has a question", is
published by `broadcastQuestionNotification`
(`server/services/approval_handler.go:759-776`) when Claude calls
`AskUserQuestion`. The approval hook then defers to the native terminal dialog
(`:420-428`) and the notification carries `nil` metadata, so the question has no
durable record and the only way to answer is to type into the pane. ADR-005
makes the hidden-session view read-only, so without an answer path this
needs-human event would be a dead end.

What exists, and why it is not reused as is:
- `GuidanceRequestService` (`proto/session/v1/guidance_request.proto`,
  `server/services/guidance_request_service.go`) is a durable store for questions
  an agent creates on purpose. `AnswerGuidanceRequest` persists the answer and
  publishes a notification (`:317-350`); it never types into a terminal, and the
  `AskUserQuestion` flow does not create a guidance request. A session-scoped
  guidance request on a hidden session stays answerable through that RPC, which
  is not a terminal write and needs no exception.
- The steer branch of `UpdateSession` sends arbitrary text through
  `session.SubmitContentWithEnter` (`session/pane_submit.go:141`). Re-opening it
  for hidden sessions would unblock every write ADR-005 exists to block.
- **`SubmitContentWithEnter` cannot be reused for Reply** (re-review ARCH-B1,
  VERIFIED at `session/pane_submit.go:95-161`): `SubmitDriverContent` writes the
  content (`:105`), waits, writes Enter (`:115`), and on no pane change **blindly
  writes a second Enter** (`:126`) before returning `ErrSubmitNotConfirmed`
  (`:132`); the wrapper returns `context.DeadlineExceeded` (`:159`) at
  `3*DefaultPaneSettleMaxWait + 2s` = 8s (`DefaultPaneSettleMaxWait = 2s`,
  `:20`) while the inner goroutine may already have written. "It returned an
  error" therefore does not mean "nothing was typed", and an extra Enter in a
  selection dialog picks the default option of whatever follows.
- The product has a single operator and no per-user identity: the auth middleware
  only answers a bool (`server/middleware/auth.go:9-12`). **And on the local
  listener there is no auth at all**: `SetupAuth` (`server/server.go:1321`) has no
  non-test caller (`grep -rn 'SetupAuth(' --include='*.go' .` shows only its
  definition), so `localChain` (`:1700-1708`) runs `ProbeGuard` for exactly one
  procedure and nothing else (`server/middleware/probeguard.go:27-45`); auth
  exists only on the `:8444` remote chain (`remoteChain`, `:1712-1720`).

## Decision

1. A typed RPC `ReplyToPendingQuestion{session_id, question_id, reply_text,
   reply_id}` (contract-PR field numbers in plan Story 1.7) answers one
   outstanding question in one **hidden** session with one single-line reply.
   A visible session is rejected with outcome `NOT_HIDDEN` ("answer in the
   terminal").
   **Signaling channel (re-review C14c, decided): expected business outcomes are
   a typed `ReplyOutcome` enum in the response, never Connect errors.** Connect
   errors are reserved for `InvalidArgument` (reply text or ids malformed),
   `Internal` (audit append failed or an unexpected fault; 0 terminal writes, so
   a client Retry is safe), `PermissionDenied` (decision 7: refused because the
   peer or Host is not acceptable) and `Unauthenticated` (auth middleware). The
   enum has `REPLY_OUTCOME_UNSPECIFIED = 0` (buf lint) and, with the type prefix
   that `ENUM_VALUE_PREFIX` in the repo's `STANDARD` lint requires,
   `REPLY_OUTCOME_SENT`, `_SEND_INDETERMINATE`, `_NOT_SENT`, `_NO_PENDING`,
   `_SUPERSEDED`, `_STALE_PROMPT`, `_NOT_HIDDEN`, `_NOT_WAITING`, `_RATE_LIMITED`,
   `_DISABLED` (plan Story 1.7; this ADR abbreviates them without the prefix).
   **There is no `IN_PROGRESS`** (review-repair iteration 2, ARCH-C21): the
   in-flight rule makes a second caller with the same `reply_id` wait and return
   the first result, a different `reply_id` against a claimed question is
   `NO_PENDING`, and a busy write lease is `NOT_SENT`, so no path could emit it
   and ux.md has no state for it. The metric label set is the lower-cased enum
   names plus `invalid`, `audit_failed`, `refused`; plan, ux.md and validation
   use these names only.
2. **Pending question registry, bound to the dialog on screen (re-review
   ADV-B2, ARCH-B4, C1).**
   a. *Attribution.* A `PendingQuestion` is registered **only when the question's
      session was resolved from the `X-CS-Session-ID` header to a live
      instance**. `resolveSessionID` (`approval_handler.go:841-870`) falls back to
      the longest cwd-prefix match over `Path` and `WorkingDir`, and a hidden
      review/diagnose session and a visible session can share a path
      (`CreateDirectorySession(item.RepoPath, ...)`,
      `session_service_diagnose.go:57-60,103-108`). A sibling
      `resolveSessionAttribution` (Task 5.6b; `resolveSessionID` keeps its
      signature) returns `header | cwd | unknown`; a `cwd` or `unknown` question
      is never replyable (the toast keeps "View output" only).
   b. *Key and identity.* The store is keyed by the instance **UUID**; the request
      `session_id` (which `MatchesID` also accepts as a title or tmux name,
      `session/instance_terminal.go:72-80`) is resolved through
      `findInstanceByID` and `Claim` requires `inst.UUID == pending.SessionUUID`.
      `question_id` is 128 random bits (unguessable, not sequential).
   c. *Supersede.* Registering a question for a session **supersedes every older
      pending id for that session**; claiming a superseded id returns
      `SUPERSEDED`. This is acceptance, not a spike outcome: it is what stops a
      late answer to question A being typed into question B.
   d. *Candidate until seen, and bound to this question's dialog.* The hook-time
      entry is a `candidate`. It becomes `confirmed` only when (i) the detector
      reports a status in **the set that Spike 1.3g records for the
      AskUserQuestion dialog** with an active controller (this ADR does not
      hard-code the status: the detector has `StatusNeedsApproval` for permission
      dialogs and `StatusInputRequired` for question and "enter X:" prompts,
      `session/detection/detector.go:93-94`, matched separately at
      `pattern_set.go:134,138`, VERIFIED; hard-coding `StatusNeedsApproval` would
      either never match a real question dialog or, worse, match a **permission
      dialog** raised after the question was answered), (ii) a **live pane
      capture** contains the `QuestionToken` (the normalized first 40 runes of the
      question text from `tool_input`, else the first option label), and (iii) the
      **pane fingerprint** (a SHA-256 of the dialog region, whose boundaries Spike
      1.3g records; the region **must contain the question text and the option
      labels** and exclude animated lines) is captured from that live capture, all
      **within a promotion window of seconds from the hook** (value recorded by the
      spike; not the 30-minute TTL, which applies to confirmed entries only). A
      candidate that is not confirmed in the window expires and is never claimable
      (the dialog may not have rendered, or may not be a question at all). A claim
      on a candidate returns `NOT_WAITING` ("the question is not showing yet"). A
      payload that yields no `QuestionToken` (no question text and no option
      label) is registered **not replyable**: nothing identifies the dialog.
   d2. *Supersede on every dialog-producing hook (ARCH-NB2).* Besides a newer
      question, **every `PermissionRequest` hook for the session supersedes every
      pending question for it**, whatever the classifier then decides, and so does
      every `AskUserQuestion` whose attribution is `cwd` or `unknown` and whose cwd
      matches a live instance (it registers nothing, ARCH-B4, but it is evidence
      that another dialog is coming); the same applies to each further hook event
      that Spike 1.3g finds coinciding with a dialog. The supersede runs in
      `approval_handler.go` right after the session is resolved (`:316-321`) and
      before every branch, including the defer-to-terminal branch (`:420-428`), and
      covers the resolved instance and every live instance whose path matches the
      cwd; it runs before the new question is registered, so a registration never
      supersedes itself. Over-superseding is the safe direction: its cost is a
      refused Reply. Failure scenario this closes: the operator answers q1 in the
      terminal, a Bash permission dialog appears during the confirmation polls, q1
      is promoted with the permission dialog's fingerprint, and a later `Reply "1"`
      approves a tool run in a hidden agent.
   d3. *Path matching and promotion (review-repair iteration 3, ARCH-C31).* The supersede of d2 uses a helper `matchingInstanceIDs(cwd)` that returns every live instance whose `Path`, `WorkingDir` or `Worktree.WorktreePath` is a path-boundary prefix of the cwd (or the reverse), because `resolveSessionID` (`approval_handler.go:841-870`) keeps one longest `Path` or `WorkingDir` prefix, uses a bare `strings.HasPrefix` and ignores the worktree path (`session/storage.go:186-188`), so reusing it unchanged misses a hidden session whose `Path` is the repository root while the hook's cwd is its worktree. Hidden review sessions share `item.RepoPath` with visible work sessions (`session_service_diagnose.go:57-60`), so the broad supersede will make `SUPERSEDED` frequent in a busy repository: the counter carries a `cause` label (`question`, `permission_request`, `path_share`), Spike 1.3g measures it before Contract PR 3 freezes the UX, and the recorded fallback narrows the path-share supersede to questions whose live fingerprint no longer matches (an extra risk that the re-review of the Reply design judges). Promotion `candidate -> confirmed` is a compare-and-set under the store mutex with a per-session supersede epoch read before the pane capture, so a supersede that arrives between the capture and the promotion is not lost.
   d4. *Hook sender proof: what it protects and what it does not (review-repair iteration 3, ADV-N15; redesigned in iteration 4, ADV-N23, ARCH-C37).* `/api/hooks/permission-request` is a plain mux route with no sender authentication (`server/server.go:824`; the injected hook sends only `X-CS-Session-ID`, which is the session title), so any local process, or any LAN host on a wildcard bind, can post a forged `AskUserQuestion` for a victim session: it would supersede the real question, put attacker text on the toast and, if its token is boilerplate that every permission dialog contains ("Do you want to proceed?"), be promoted onto a permission dialog and make a later `Reply "1"` approve a tool run. **The proof stops a forged request from another local process, or from a LAN host that cannot read the session's files, from registering a replyable question. It does not stop the agent itself** (the hidden review agent is the untrusted principal: it reads PR content, the driver auto-answers its approvals, and it can read its own `.claude/` files, the same-user secret file and any session's hook file) **nor any same-user process**; a same-user reader can forge a proof for any session. The control that actually stops attacker-influenced text from approving a permission dialog is the **live pane check** (the dialog title region, the boilerplate denylist and the question token in the fingerprint region; plan T-RP-61); the proof only narrows who can create a candidate. Design: `proof = HMAC-SHA256(secret, "ssq-hook-v1" || session UUID)` under a random 32-byte secret kept 0600 in the config directory, **keyed to the stable session UUID** so a rename does not end replyability, header `X-CS-Hook-Proof: <uuid>.<proof>`, delivered through a **per-session 0600 file outside every worktree** (`<config dir>/hook-proofs/<uuid>`, written atomically) that the shared hook command reads through the **existing** `$STAPLER_SESSION_UUID` (injected into every local tmux session by `Instance.buildExtraEnv`, `session/instance_tmux.go:729-733`; no second variable, review-repair iteration 6b, ADV-N33) with `curl -H @file`, so it is **not in argv** (`ps` shows a path; the earlier design put it in the argv of every hook curl) and a session sharing its root with others carries its own proof; the command adds the header **only when the file is readable** (`curl -H @missing` exits 26 without sending, VERIFIED), so a missing file degrades to `no_proof` and the approval hook still POSTs (the local-token `-H @file` that `hookAuthArg()` appends has the same exit-26 property and is guarded the same way in the same command; `buildHookCommand` is the single producer of the whole command and the merge compares against it, so `upgradeHookAuth` is removed and there is no second in-place rewriter; review-repair iteration 6b, ADV-N35); a verified proof's UUID **is** the attribution and the title header is ignored (review-repair iteration 6, NB8, N29); **no nonce** (Claude Code runs the same hook command every time, so a nonce can only be per injection, revoking one needs server-side per-session state that survives a restart, and the realistic attacker reads the file or is the agent; rotating the secret is the revocation tool and shows as `bad_proof`). Re-injection **replaces** a stale entry (`hookAlreadyPresent` kept an entry by URL, so the earlier "session start re-injects" was false). The proof file is deleted only when `HookPermissionApproval` is in the removed set (the triage drift-hook removal must not delete a live session's proof, ADV-N34a) and is rewritten by one idempotent helper at creation, resume, restart and service boot (ADV-N34c). **Live panes keep their original environment and hook snapshot, so rotating the secret revokes only sessions that already have the new command; it is not a revocation tool for every live session** (ARCH-C57). Six senders of `X-CS-Session-ID` exist (`hook_injector.go:380`, `approval_handler.go:1001`, the remote socat form `remoteApprovalHookCommand`, `session/mux/hooks.go:113`, `session/sshremote/approval_relay.go:627`, `scripts/ssq-hook-handler:494`); only the first two carry a proof, so the others are not replyable. A request without a valid proof still supersedes and raises the toast but registers nothing replyable, except that a proofless **duplicate** of a live proofed question (same dialog digest) neither supersedes nor registers, while a proofless request with a different digest still supersedes (the safe direction). Every unreplyable registration is counted by `cause` (`no_proof`, `bad_proof`, `no_token`, `boilerplate`, `shape`, `path_only`) and stamped `reply_unavailable=<cause>`, and the card says "Reply unavailable for this session" for `no_proof` and `bad_proof` (UX RP-17). **Accepted limit (default applied; the operator may override): Reply works only for sessions started after hook proof ships**, because Claude Code most likely snapshots its hooks at start (INFERRED; Spike 1.3g). The `QuestionToken` rejects a dialog-boilerplate denylist (seeded with "Do you want to proceed?" and "Chat about this", extended from the spike's captures), promotion parses the dialog title region of the live capture, and the card shows the pane-derived question text when it differs from the hook text.
   e. *Bounds.* 256 entries, TTL default 30 minutes for confirmed entries (INFERRED, adjusted by Spike
      1.3g; an unconfirmed candidate lives only for the promotion window of 2d), at most 8 live entries per session; entries are removed on reply,
      TTL, supersede, session deletion and when the session leaves the waiting
      state. The per-session limiter map is bounded (LRU 1024, evicted on session
      delete).
   f. *Payload shape.* The code reads `tool_input["prompt"]`
      (`approval_handler.go:761`), which comes from a Draft task doc
      (`docs/tasks/askuserquestion-ui.md`); Claude Code's real `AskUserQuestion`
      input is reported to be a `questions[]` array with options and
      multi-select (UNVERIFIED here). Spike 1.3g records the real payload. The
      card degrades gracefully: with no question text but an option label it says
      "Open the terminal output to read the question" above a composer that still
      works; with neither (no `QuestionToken`) it says the same with View output
      only and no composer; with a multi-question or multi-select
      payload (`question_shape != single`, stamped in metadata) it offers **no
      composer** ("Open the terminal to answer") because one typed line cannot
      answer a form. The spike checks the payload shape **first** (review-repair iteration 3, ARCH-C32): if the real payload yields no `QuestionToken`, every question is not replyable and the plan returns to the operator; it also records whether each hidden kind has an active status controller while its question is open (otherwise `NOT_WAITING` is permanent for that kind) and the longest token that survives terminal wrapping at 80 columns (the comparison is whitespace-normalized).
3. **Capability, not a flag; claim lifecycle; its own send sequence.**
   - `PendingQuestionStore.Claim(session, questionID, replyID)` is **one critical
     section** under the store mutex: lookup by UUID, state check (`confirmed`,
     not expired, not superseded, id is the newest for the session), in-flight
     check for `reply_id`, then `confirmed -> claimed`. The rate limiters are
     checked **before** `Claim` so `RATE_LIMITED` never has to un-claim, but
     **after the idempotency lookup** (next bullet). The result is a single-use
     `PendingQuestionClaim` (unexported fields, one constructor) or an outcome:
     `NO_PENDING`, `SUPERSEDED`, `NOT_WAITING`, `NOT_HIDDEN`. **Claim edges
     (ARCH-C20):** a supersede or a session delete that arrives while the entry is
     `claimed` marks it `claimed-superseded`, and the before-first-byte checks stop
     the write; a later `NOT_SENT` release returns a `claimed-superseded` entry to
     `superseded`, never to `confirmed`, and any released claim is re-validated
     (expiry, newest id for the session, still hidden) before it can be claimed
     again.
   - **Order of the request path (ARCH-C19): flag, validate, idempotency lookup
     (cached result, then in-flight), limiters, claim.** A repeated `reply_id`
     while the first is **in flight** waits for it (a singleflight keyed by
     `reply_id`, bounded by the send deadline) and returns the first result; a
     repeated `reply_id` after a terminal result returns the cached result. The
     lookup precedes the limiters, otherwise the first request consumes the
     per-session burst and the double tap, the very case the rule exists for,
     would get `RATE_LIMITED`. The limiters use `rate.Limiter.Reserve()` and a
     `Delay() == 0` check (not `Allow()`), so the reservation can be cancelled on
     `NOT_SENT`. **`reply_id` is bound to its payload** (ARCH-C20, ADV-N10): the
     cache stores the SHA-256 of `(session UUID, question_id, text)` and the same
     `reply_id` with a different hash is `InvalidArgument`, never a cached `SENT`
     for a reply that was never sent. The result cache and the in-flight map are
     bounded (1024 entries, LRU, 10-minute TTL, INFERRED); an evicted id retried
     later finds the question consumed and gets `NO_PENDING` with 0 writes.
   - **One writer per pane (ADV-N1, ARCH-C16; VERIFIED).** The session driver is a
     concurrent writer to the same pane: backlog sessions, which include hidden
     review and diagnose sessions, run a driver whose `sendAnswerKey` sends
     `"1\n"` into any `StatusNeedsApproval` dialog whose text contains "allow
     reading", "allow writing", "allow executing" or "do you want to proceed"
     (`session/session_driver.go:365,824-836`, `shouldApprovePrompt` at
     `:1338-1354`, latched once per dialog hash), and it also sends the initial
     prompt (`:706`) and idle nudges (`:884`); `performNudge` writes through
     `SubmitContentWithEnter` (`server/mcp/tools_diagnose.go:266,287`). So (a) an
     `AskUserQuestion` whose text matches those phrases can be auto-answered with
     option 1 before the operator sees the notification (existing behavior,
     which Spike 1.3g records; the Reply then fails `STALE_PROMPT`), and (b) a
     driver tick can land **between a reply's content and its Enter** and the
     mangled answer would still be reported `SENT`. Every writer
     therefore goes through a per-instance **`TerminalWriteLease`** (plan Story 5.0, its own PR 5a, independent of Reply; review-repair iteration 3, ARCH-NB3): a `*HeldLease` capability that exactly one function per call chain acquires (the outermost writer: `replyToPendingQuestion`, `steerUnderLease`, `steerHiddenReviewViaBacklogLink`, `steerInternal`, a driver tick, `performNudge`, an MCP tool, the compactor) and that every primitive below it (`SubmitReplyOnce`, `SubmitContentWithEnter`, `SubmitDriverContent`, `SendKeysWithTimeout`, `steerAuthorized`) only receives. Reply takes `Try` with no wait (busy is `NOT_SENT`: claim released, reservation cancelled; with the live flag `terminal_write_lease` off it is `NOT_SENT` too, because its safety needs real serialization); the driver's answer key and nudge use `Try` (busy: skip the tick without latching the dialog hash; the initial prompt consumes no send attempt); the other writers return a retryable busy result; and the lease is **released by the writing goroutine** (it is handed into the goroutine), not when the caller returns, so an abandoned blocked write cannot overlap the next writer. A wedged write keeps the lease (a gauge, a WARN and one tray-level warning per episode that is never pushed; the remedy is Delete or a service restart (T-WL-07: closing the PTY does not wake the write, so Pause then Resume does not clear it); lifecycle actions do not take the lease; a forced release is deliberately not offered, it would allow the interleave), and the lease is never taken while holding `i.mu` or the store mutex. The type signature enforces it (a primitive that takes a `*HeldLease` cannot be called without one) and the acquirer list is closed and pinned (ADR-005 decision 3, checks (c)); nesting is caught at runtime by the real chain tests and the nesting fixture, there is no static nesting rule since review-repair iteration 5, and there is no `ErrNestedLease` (review-repair iteration 4, ARCH-C35).
   - `access.QuestionReplyWriter(claim)` is the only way to get a writer for a
     `ReadOnly` target. Its single method `SubmitReply` does **not** call
     `SubmitContentWithEnter`. It is a Reply-specific primitive (Task 5.6k; new
     `session/pane_reply.go`, classified as a terminal write in the Story 5.1d
     table): `ctx` check, `VerifyPaneOwner`, the write lease, then the
     **before-first-byte checks** (decision 4: audit already appended, session
     still hidden, `inst.UUID` still matches, the claim is not
     `claimed-superseded`, a status in the Spike 1.3g set with an active
     controller as a cheap pre-filter, and the **live** pane fingerprint equal to
     the stored one), then **one** content write, a bounded settle wait, **one**
     Enter write **only in the `ContentThenEnter` keystroke mode**, and **no**
     blind Enter retry and no pane-change retry. The keystroke sequence is a typed
     `ReplyKeystrokes` chosen from Spike 1.3g's recorded dialog behavior (ADV-N11):
     if a digit key selects immediately the mode is `DigitOnly` and **no Enter is
     written**, because a stray Enter would pick the default option of whatever
     dialog follows. Each write goes through the count-returning
     `Instance.SendKeysN` and runs under its own timeout. **The abort state is a
     compare-and-swap word, not a flag read and then acted on** (ARCH-C15): the
     goroutine takes `idle -> writing` immediately before each write, the caller on
     timeout takes `idle -> aborted` (or `writing -> writing-aborted` when a write
     is already in the syscall), and whichever loses backs off, so after the caller
     returns **at most the one write already inside the syscall can still land**
     (stated limit). The total bound is its own constant (content write + settle +
     Enter, each bounded; `ReplySendTimeout`, value chosen in Task 5.6k), not
     the "5s" this ADR previously stated (the steer primitive's real bound is 8s).
   - **Keystroke plan per question shape and the pre-Enter check (review-repair iteration 3, ADV-N16).** `ReplyKeystrokes` is a function `(question_shape, reply_text) -> ReplyKeystrokes` recorded by Spike 1.3g, not one global mode: a single digit on a select dialog is always `DigitOnly` (a digit that selects immediately followed by an Enter would approve the next dialog, whose default is option 1 "Yes"); free text only for a recorded text-field shape; any other shape is not replyable. Immediately before the Enter, `SubmitReplyOnce` re-runs the live-fingerprint compare under the lease (`beforeEnter`); on a mismatch (the operator answered in the terminal, or a new dialog drew during the settle wait) it writes **no Enter**, the outcome is `SEND_INDETERMINATE` (text typed, not submitted; claim consumed; no Retry) and the audit outcome line records `aborted_before_enter`. The fingerprint normalizes the selection glyph, so arrowing in the terminal reads as a changed dialog (a false `STALE_PROMPT`, the safe direction). What remains open is the operator typing in the terminal between the final compare and the Enter write (milliseconds).
   - **Failure classes.** `NOT_SENT` = failure strictly **before** the first byte
     is handed to the pane (ctx already done, `VerifyPaneOwner` failed, the write
     lease is busy, any before-first-byte check failed with a more specific outcome
     such as `STALE_PROMPT`/`NOT_WAITING`, **or a write that `SendKeysN` proves
     wrote zero bytes**: `0, err` from `TmuxSession.SendKeys` when `GetPTY` fails,
     `session/tmux/tmux.go:1349-1354`, or the not-started and paused check; the
     count accessor is **mandatory** because `Instance.SendKeys` drops the count,
     `session/instance_tmux.go:1539-1545`, which would make the commonest zero-byte
     failures indeterminate for nothing, ADV-N2): the claim is **released** and the
     limiter reservation cancelled; Retry with the same `reply_id` is safe. Any
     error from a write that wrote `n > 0` bytes, a timeout, a ctx expiry or client
     disconnect between the two writes (the text stays typed, not submitted), or an
     Enter write error is **`SEND_INDETERMINATE`**: the claim is **consumed**, the
     result is cached under `reply_id`, there is no automatic retry and the client
     shows no Retry button ("Sent? It may have been typed but not submitted. Check
     the terminal output"). `SENT` is both writes returning nil (or the single
     digit write in `DigitOnly` mode); it means keystrokes were written, not that
     Claude accepted them (the receipt text stays "waiting for the session to
     continue").
   - Tests use a fault-injecting `paneSubmitter` per stage (decision 9 list).
4. **Before-first-byte checks (re-review C3), in this order, all after the audit
   append and immediately before the first write, under the write lease:** (1)
   `AccessFor(inst)` still `ReadOnly` (the session is still hidden), `inst.UUID`
   equals the claim's UUID and the claim is not `claimed-superseded`; (2) the
   waiting-state pre-filter: `GetDetectedStatus()`
   (`session/instance_state.go:326-338`) returns a status **in the set recorded by
   Spike 1.3g** for the AskUserQuestion dialog **and** a controller is active
   (`StatusUnknown`, which that function returns without a controller, is
   rejected; a hidden `other` session can be a shell and a Reply there is a shell
   command; the status set is never hard-coded to `StatusNeedsApproval`, see
   decision 2d). **This status is cached, not live** (VERIFIED: `GetDetectedStatus`
   returns `mgr.GetStatus(i)`, the controller's last result, which can be a poll
   interval stale), so it is only a cheap pre-filter; (3) the **live** pane
   capture's fingerprint of the question region re-read now equals the stored one,
   else `STALE_PROMPT` with 0 writes. **The live fingerprint is the authoritative
   guard** (ADV-N9): it must contain the question text and option labels, so when
   question B is never registered (no header, so A is never superseded) only the
   fingerprint separates A from B, and two different questions with the same
   option layout compare unequal; a region that included an animated line would
   make every Reply `STALE_PROMPT`, which is why Spike 1.3g records the idle-redraw
   stability result. The audit append does an fsync, so the window between
   decision and write is not zero; that is why the checks run after it. The
   fingerprint check is mandatory, not "evaluated".
5. **Narrow input.** `reply_text` is 1..500 **bytes** (`MaxQuestionReplyLength`;
   the unit is bytes on the server and on the client, which measures with
   `TextEncoder` and shows an "N / 500 bytes" counter), valid UTF-8, single line,
   and rejected by one shared function (`session.ValidateSingleLineText`, a TS
   port with the same vector fixture for the client sanitizer) if it contains: C0
   (including ESC, CR, LF, TAB, NUL), DEL, the C1 range U+0080-U+009F (U+0085
   included), U+2028/U+2029 (`Zl`, `Zp`), Unicode noncharacters, the whole tag block
   U+E0000-U+E007F (including its unassigned code points) and **every Unicode `Cf`
   (format) code point except U+200C and U+200D**. That covers the bidi and
   invisible controls (U+202A-U+202E, U+2066-U+2069, U+200E, U+200F, U+061C,
   U+180E, U+2060-U+2064, U+FEFF), the soft hyphen U+00AD, the interlinear
   annotation characters U+FFF9-U+FFFB and U+0600-U+0605. **This resolves the
   earlier open design call** (review-repair iteration 2, ADV-N8): the first
   patch used an explicit list because a blanket `Cf` rejection also rejects
   U+200D (the emoji joiner, used in family and skin-tone sequences) and U+200C
   (needed for Persian), but an exception list of exactly those two meets that
   concern and closes the invisible-text channel that an explicit list left open.
   The 200-emoji and ZWJ-sequence fixtures stay valid. Violations are
   `InvalidArgument` with 0 writes. In `DigitOnly` mode the reply must also be one
   of the option digits the spike recorded.
6. **Audit before write ("no audit, no write"; re-review C2).** Append-only JSONL
   `<config dir>/audit/hidden-session-replies.jsonl` (`O_APPEND|O_CREATE|O_WRONLY`,
   file 0600 in a 0700 `audit/` directory, rotated at 5 MiB, 3 files kept). The
   sink invariants: (a) one **process-wide mutex** around append and rotate (the
   Reply, O7 steer and flag-change writers share it); (b) the **pre-write line is
   `fsync`ed** before the terminal write; (c) the config directory is resolved
   **once at construction** (`config.GetConfigDirForDir` honors
   `STAPLER_SQUAD_TEST_DIR`, `STAPLER_SQUAD_INSTANCE`, test-mode pid directories
   and a workspace preference, `config/config.go:122-170`, so a workspace switch
   must not move the sink mid-run); (d) a rotation or append failure is
   `audit_failed` (`Internal`) with 0 writes. A line carries `ts`, `kind`
   (`reply`, `backlog_steer`, `guard_bypass`, `flag_change`, `prune`), `session_uuid`,
   `session_title`, `question_id`, `reply_id`, `reply_len`, `reply_sha256`,
   `listener` (`local`/`remote`), `peer_addr` (connect `req.Peer()`), `host`,
   `origin`, `user_agent`, `auth_mode`; the outcome line adds `outcome` and the
   failure stage. **There is no `reply_preview` for Reply** (the answer to "what
   is the token?" is arbitrary text and `staplersquad.log` is wider-readable and
   OTel-exportable); the INFO log `hidden_session_reply` carries `reply_len` and
   `reply_sha256` only and the reply text never appears in any log line. The O7
   steer keeps its 80-character preview (it already echoes its message in
   `notifySteerSent`). "Who" is remote address, Host, user agent and auth mode,
   because no per-user identity exists. The durable trail is the file, not the
   7-day notification history (`server/notifications/store.go:19`). Every
   `UpdateFeatureFlag` change of `hidden_session_readonly_guards`,
   `hidden_session_reply` and `hidden_session_gate` (global and per kind) records
   a `flag_change` line with the previous and new value, **from the first PR in
   which the gate can flip** (the sink core is plan Task 2.8h in PR 2a-2, not Epic
   5; ADV-N5; its request fields come from the Story 1.8 record, so PR 1f-a, the raw record stamp, merges before PR 2a-2; ADV-N20, ADV-N26), and a `PruneHiddenSessionNotifications` apply appends a
   `kind=prune` line (matched count, filter flags, no row content) before it
   deletes (plan Task 2.7c). **Two paths and explicit degraded modes (review-repair
   iteration 2, ADV-N6):**
   - *Tightening flips* (`hidden_session_reply` to off, `hidden_session_readonly_guards`
     to on, every `hidden_session_gate` flip) are **persisted first**; the line goes
     on a bounded queue (64) drained by one goroutine that takes the sink mutex, so
     a request **never waits on the sink mutex or an `fsync`** (the earlier
     "best-effort and never blocked" append shared the mutex with the fsyncing Reply
     and steer writers, so a stalled `fsync` could hold the kill switch inside
     `UpdateFeatureFlag` under `updateMu`). A full queue or a failed drain writes the
     same fields as a WARN `flag_change_audit_degraded` record in the main log.
   - *Loosening flips*: `hidden_session_reply` to on is audited and `fsync`ed
     **before** it is persisted and is refused with `Internal` if the append fails
     (nothing needs Reply while the sink is down, because Reply would refuse
     anyway). **The append runs before `updateMu` is taken** (`feature_flag_service.go:400-401`) and is bounded by a 2s timeout (a helper goroutine; a timeout returns `Internal` with nothing persisted), and no sink call is ever made while `updateMu` is held, so a stalled `fsync` cannot make the kill switch queue behind a loosening flip (review-repair iteration 3, ADV-N17). **The pair of lines (review-repair iteration 4, ADV-N24)**: the pre-lock line is a durable `phase=requested` line that carries only the requested value (an intent, never a previous value, because the previous value is read under `updateMu`, `feature_flag_service.go:407`); every flip, tightening included, then writes a `phase=result` line after the persist with `outcome` (`applied`, `aborted_persist_failed`, `aborted_controller_failed`, `aborted_rolled_back`), the **true previous value** read under `updateMu` and a `seq` taken inside `updateMu`, so a reader orders by `seq` and never by file position, a failed persist leaves an `aborted_*` result instead of a line for a change that never happened, and a `requested` line with no `result` reads as indeterminate. `hidden_session_readonly_guards` to off is **not refused when the
     sink is down**: it is the escape hatch for a broken composer, and refusing it
     in exactly the incident where the sink is the cause would leave the operator
     with no way to restore the steer; it is persisted with the fallback WARN
     record.
   - *Degraded modes of the steer and Reply when the sink is unavailable* (disk
     full, read-only, stalled): **Reply** refuses (`Internal`, 0 writes; no audit,
     no write) in every state. The **O7 steer** with the guards **on** refuses with
     the explicit message "Audit log unavailable, steer not sent"; MCP
     `steer_session` and the terminal remain the fallback. The **qualifying review
     steer with the guards off** takes the typed path and records a WARN
     `steer_audit_degraded` record (same fields) in the main log in place of the
     audit line (`hidden_session_audit_degraded_total`), because the operator
     explicitly flipped the hatch to restore it. A **`guard_bypass`** write (a non-qualifying hidden steer while the guards are off) is audited before the write exactly like Reply (blocking, `fsync`ed); **with the sink down it is refused like Reply** (review-repair iteration 3, ADV-N18: the earlier "same fallback record" let a triage, diagnose or `other` session be written on a WARN line alone, which contradicts the stated reason for the guards-off hatch; allowing the hatch flip itself and the qualifying review steer on the fallback record stays acceptable, because that is how the composer is restored). The main log is not tamper-evident
     and is wider-readable; the fallback record carries lengths and hashes only,
     never message text beyond the steer's existing 80-character preview.
7. **What actually protects the RPC (corrected; re-review ARCH-B2).** The earlier
   sentence "Authenticated like every other UI write: the global auth middleware
   (`server/server.go:1321`)" is **false on `:8543`**. The rule is now: on
   `:8444` the remote auth middleware applies; on `:8543` there is no auth, so
   the `ProbeGuard` mechanism is generalized into a **guarded-procedure set**
   (`middleware.LocalWriteGuard`, plan Story 1.8, **its own PR 1f-b (PR 1f-a stamps the request record) that merges
   before the Reply and prune PRs**; review-repair iteration 2 moved it out of Epic
   5) and `ReplyToPendingQuestion` is added to it beside `ProbeProgram` (plan Task
   5.6l), as is `PruneHiddenSessionNotifications` (Task 2.7c; whole procedure).
   **The verdict (ARCH-C17, C18, ADV-N7):** `ProbeProgram` keeps today's exact rule
   (POST, loopback-bound listener, Host and Origin loopback or allowed). For the
   steer, Prune and Reply the **rebinding gate** applies: Host must be a loopback
   name, one of `GetVerifiedHostnames()` (the forward-DNS-verified set, recomputed every detector cycle; **never** `GetHostnames()`, which also holds unverified reverse-DNS names that the network controls, `hostname_detector.go:240-249`; review-repair iteration 3, ARCH-NB4), an IP literal that is the listener's own or a local interface address, or the host of a
   non-wildcard listen address, and Origin, if present, loopback, in `GetOrigins()`
   or same-origin with that already-allowed Host; the Host check is what stops DNS
   rebinding, which CORS does not. On the authenticated chain (`auth_mode == required`: `:8444`, or `:8543` when an `authMiddleware` is set) the **Host-membership test is skipped** and only the same-origin Origin check stays, because the auth cookie is the boundary and that chain's Host is a LAN or Tailscale name (`server/server.go:1710-1711`) that the test would reject, which would take the shipped Steer away from a remote operator (ADV-N14). `auth_mode` is `required` **only** when an explicit `requiresAuth` is true, set where `middleware.Auth` is built with a non-nil validator (`main.go:1614`): `middleware.Auth(nil)` is a pass-through and an identity wrapper is not a validator, so neither may skip the Host test (review-repair iteration 4, ARCH-C38). An IP-literal Host cannot be a rebinding Host (rebinding needs an attacker-controlled name), so a local IP literal is allowed and a non-local one rejected. The verified set is **recomputed by the detector every cycle** as the names (those in `d.networks` that are not IP literals or `localhost`, plus the boot-time `verifiedHostnames`, `main.go:1556-1564`) for which `validateFn` returns true in that cycle; it is **never copied from `d.networks`**, which holds the raw boot-time PTR and avahi candidates and the IP literals that `resolveAndValidate` skips and so never validates (`main.go:1526`, `hostname_detector.go:147-151,287-289`; review-repair iteration 4, ARCH-NB5); it is not add-only (a name whose record changed drops out at the next cycle, ADV-m12), and it stays static from boot when the detector is disabled. The boot feeder is empty when remote access is not configured. Residual: `verifyHostnameOwnership` proves the name resolves to a non-loopback address of this machine, so an attacker who controls the network's DNS for both the PTR and the forward record can still get a name verified (the certificate and RPID trust model). The new guard set, like `ProbeGuard`, is not installed when an `authMiddleware` is set, and the Prune dry run needs an allowed Host too (minor m9, m10). **There is no `LoopbackBound` condition**: with
   it, a `--listen 0.0.0.0:8543` deployment (`main.go:869-870`) would make every
   backlog Steer of a hidden review session `PermissionDenied`, and non-loopback
   LAN-hostname browsers would be refused even with the guards flag off, both
   regressions of shipped behavior the operator confirmed as O7. The **Reply-only
   local-caller gate** adds: when `auth_mode == none` and the caller is remote (the
   peer is not loopback, **or any proxy header is present**, because behind a
   local reverse proxy or Tailscale Serve the peer is loopback while the caller is
   not), the handler refuses with `PermissionDenied` (outcome counted `refused`).
   The steer and Prune do not apply it (the steer is shipped behavior; Prune has
   the exposure `ClearNotificationHistory` has today) and record `peer_loopback`
   and `proxied` in their audit lines. A Connect handler cannot read `Host` from
   `connect.Request`; the verdict reuses the existing in-repo mechanisms:
   `services.WithRequestHost` stamps `r.Host` into the context for the
   `SessionService` handler (`server/server.go:430`,
   `server/services/capture_tap_service.go:33-38`), a sibling stamps Origin, the
   proxied flag, the listener and the auth mode (both chains), the peer comes from
   `req.Peer().Addr`, and the proxy rule is `requireLoopbackCaller`'s
   `isProxyHeader` (`capture_tap_service.go:124-180`), moved to `server/middleware`
   so there is one implementation. The steer path cannot be a procedure-level guard
   (`UpdateSession` serves every property edit), so the rebinding gate is called
   from inside the handler for the hidden-steer branch only (ADR-005 decision 5).
   The audit line records `listener`, `auth_mode`, `peer_loopback` and `proxied`.
   Residual, stated: a deployment whose browser uses a Host outside the verified
   hostnames is refused (auth off) with an explicit message naming the Host. No MCP tool
   exposes the RPC.
   **This is a safety rail, not a security boundary.** A holder of the auth
   cookie, or any local process on `:8543`, can already create a session running
   an arbitrary program, steer visible sessions and call `UpdateFeatureFlag`
   (including to turn the guards flag off). What the read-only rule protects is
   confused-deputy writes and UI bugs, plus the prompt-injection path into review
   agents that carry repo and MCP privileges; the audit line plus the flag-change
   audit are what make an abuse attributable afterward. Behind a proxy or
   Tailscale `Peer().Addr` may be the proxy; no `X-Forwarded-For` field is
   recorded unless a trusted-proxy list exists.
8. **Rate limits** (`golang.org/x/time/rate`, as `server/services/rate_limiter.go`):
   per session 1 reply per 5s (burst 1), global 10 per minute, both checked after
   the idempotency lookup (decision 3) and before `Claim`; exceeding returns
   `RATE_LIMITED` with `retry_after_seconds` (the client text is "Please wait N
   seconds"), and a double tap with the same `reply_id` returns the first result
   instead. The limiter is an operator rail, not
   a security control: O7 steers are not limited. Flag reads **fail closed**:
   `hidden_session_reply` reads as off and `hidden_session_readonly_guards` as on
   when the config cannot be read or parsed (ADR-004 decisions 6, 10, 11).
9. **Typed capabilities and a short pinned-caller table keep the guarantee checkable, with an honest boundary** (simplified from a whole-tree scan in review-repair iteration 5; ADR-005 decision 3 lists what is not caught). Story
   5.1d has two **UI** exceptions: `server/services/hidden_question_reply.go`
   `replyToPendingQuestion` (requires a `PendingQuestionClaim`) and the O7
   `steerHiddenReviewViaBacklogLink` in `hidden_review_steer.go` (requires a
   `BacklogReviewLink`), plus the token-gated `steerAuthorized` (which a handler may
   call only with a token built by `decideSteerAccess`) and the `acquirer` rows of the
   pinned-caller table for internal non-RPC writers (ADR-005 decision 3, simplified in
   review-repair iteration 5); the lease is a passed `*HeldLease` with one acquirer per
   chain (proved at runtime). "Two" is a statement about UI exceptions only. **Boundary
   between Reply and steer:** Reply answers one outstanding `AskUserQuestion`
   and is claim-gated; the backlog steer is the shipped composer for a live
   review session and is link-gated; neither re-opens the general steer branch.
   Tests in the same run assert `WriteToSession`, the steer branch, stream
   input/resize and Restart still reject a hidden target. Test list for this
   ADR: a fault-injecting submitter per stage (before-first-byte, content write
   error, between the writes, Enter write error, timeout) asserting exactly one
   content write and one Enter across a Retry; supersede; **a permission dialog
   after an answered question (the hook supersedes; never promoted with the
   permission dialog's fingerprint)**; fingerprint mismatch and a question token
   in the region; wrong-session path sharing; Host rebinding; the waiting
   pre-filter with shell-prompt and permission-dialog fake panes; the write lease
   against a fake driver; the compare-and-swap abort race; double tap inside 5s
   (validation sections J and K).
10. **Client**: a Reply card in the read-only view (design/ux.md Surface 12b) and
    a "Reply" action beside "View output" on the hidden `INPUT_REQUIRED` toast,
    tray row and Background row, shown only while a replyable (header-attributed,
    single-question) question is pending.

## Residual risk and sequencing (operator decision to keep Reply, review-repair iteration 3)

The adversarial review recommended shipping Reply later as its own project (it is hard-gated on an unrun spike, has produced a new blocker or cross-cutting concern in every review pass, and nothing else depends on it). **The operator chose to keep Reply in scope.** Reply is therefore sequenced **last in Epic 5**; nothing else in Epics 1-5 depends on it (the write lease is Story 5.0, the audit sink core is Task 2.8h, the guard set is Story 1.8, and the scan's Reply entry is added by Task 5.6d); it is hard-gated on (a) Spike 1.3g's recorded results and (b) a passing re-review of the Reply design as it then stands; and it ships behind the `hidden_session_reply` kill switch.

**Residual risk, in plain words**: a typed answer can land in the wrong dialog. The worst case is that it approves a permission dialog (option 1 is "Yes"), which runs a tool in a hidden agent. The kill switch stops further replies; it **cannot undo a keystroke that was already sent**. The design lowers the chance (supersede on every dialog-producing hook, the live fingerprint checked before the first byte and again before the Enter, a keystroke plan per question shape, a hook sender proof, audit before write) and does not remove it. Not closed: (a) the gap between the last live-fingerprint compare and the keystroke write, which cannot be closed from userland (microseconds to milliseconds, plus one write already in the syscall; `DigitOnly` shrinks the worst case to a digit typed into a dialog that drew in that window), (b) operator-typed partial input outside the fingerprint region (INFERRED: the input line is not in the question-and-options region), and (c) a hostile local process of the same user, **including the hidden agent itself, which can read its own hook file and the secret** (the proof does not stop it; the live pane check is the control). A change during the settle wait is no longer a residual: the pre-Enter compare catches it (review-repair iteration 4, ADV-m11). **Accepted limit (default applied; the operator may override): Reply works only for sessions started after hook proof ships**; a session alive at upgrade shows "Reply unavailable for this session" until it is restarted. Until Story 5.6 ships, the hidden `INPUT_REQUIRED` toast, tray row and Background row show "View output" only, which is the reduced version with no typing.


## Alternatives Considered

- **Banner only ("answer it elsewhere")** (the earlier recommendation, option a):
  superseded by the operator decision.
- **Make it informational** (option c: tray and Background only, no toast): would
  hide a blocking question; superseded.
- **Re-open the steer branch for hidden sessions**: rejected; unblocks arbitrary
  writes.
- **Reuse `SubmitContentWithEnter` and release the claim on any error** (the
  earlier decision 3): rejected by the re-review; it types the answer twice.
- **Reuse `AnswerGuidanceRequest`**: rejected for this flow; it does not type
  into a terminal and `AskUserQuestion` creates no guidance request.
- **Allow Reply on visible sessions too**: rejected; they have the interactive
  terminal, and a smaller surface is easier to prove.
- **Guard all of `UpdateSession` with the local write guard**: rejected; it would
  lock a LAN-hostname browser out of every session edit.

## Consequences

- One new RPC, one in-memory store, one new send primitive, one audit file, one
  pinned `ui` row (`replyToPendingQuestion`, the Reply capability's writer; the
  O7 steer's rows belong to ADR-005); the read-only claim becomes "no terminal input except one audited,
  rate-limited reply to the question that is on screen and the backlog steer of
  a live review session".
- The reply keystrokes depend on how the native `AskUserQuestion` dialog accepts
  input (option number, typed text, arrows): UNKNOWN until Spike 1.3g, which
  chooses the `ReplyKeystrokes` mode (a digit that selects immediately gets no
  Enter). If it
  cannot be answered reliably by typed text, the card degrades to "Open the
  terminal to answer" or the plan returns to the operator before Story 5.6
  starts.
- A late reply to a dialog that changed is rejected by supersede (a newer
  question **or any dialog-producing hook**, so a permission dialog after an
  answered question cannot inherit a claim), the waiting-state pre-filter and the
  live fingerprint together; the detector status set and the fingerprint are Story
  5.6 acceptance criteria, not optimizations. Residual:
  between the final fingerprint read and the first `SendKeys` the operator can
  still type in the terminal (milliseconds); the design does not claim to close
  that window.
- `SEND_INDETERMINATE` is a real, user-visible state: the operator may have to
  look at the terminal. That is chosen over the alternative of ever typing twice.
- The audit file is the only record of who replied; it is local, size-capped and
  not exposed through any RPC (INFERRED sufficient for a single operator). The
  `session_title` of review sessions is `uniqueDispatchTitle("review", item.ID)`
  and so embeds the backlog item id (harmless; note it if the file is exported).
- Story 5.6 is **hard-gated on Spike 1.3g** (Phase 4, pre-mortem #4) and, after review-repair iteration 3, on a passing re-review of the Reply design, and it is sequenced last in Epic 5 (see "Residual risk and sequencing"): it does not
  start until the waiting-state status set, the fingerprint region rule (with its
  question token), the real payload shape, the promotion window, the dialog hook
  list and the answer keystroke mode are recorded. If detection is
  unreliable the plan returns to the operator.
- Cost: one server story and one client story, sized as cost classes in plan
  "Effort" (Story 5.6); the Reply proto lands in a separate Contract PR 3
  (Story 1.7) after Spike 1.3g, and the whole path has its own kill switch,
  `hidden_session_reply` (ADR-004 decision 11).
