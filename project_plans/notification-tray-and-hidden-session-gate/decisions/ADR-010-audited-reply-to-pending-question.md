# ADR-010: One Audited, Rate-Limited Reply Path Into a Hidden Session (One of Two Exceptions to Read-Only)

**Status**: Proposed; **repaired 2026-10-10 against the measured Spike 1.3g facts and the second Reply review (decisions 2 to 5 rewritten: one digit, one write, identity by structural match of the live capture; free text, supersede, promotion, token and boilerplate rules deleted; a focused re-review of this text is pending, see the end of this file). The iteration notes that follow are history; where they name `ContentThenEnter`, supersede, a status set, a token-in-capture rule or a hook snapshot, decisions 2 to 5 below win** (operator decision O2, DECIDED 2026-10-07: add an audited Reply control now; triad iteration 1: dedicated kill switch `hidden_session_reply`, proto moved to Contract PR 3 after Spike 1.3g; **review-repair iteration 1 (2026-10-09) rewrote decisions 2, 3, 5, 7 and 8 for the focused re-review's blockers: ARCH-B1/ADV-B1 release-on-failed-write double-typing, ADV-B2 claim bound to a question rather than the dialog on screen, ARCH-B2 false "authenticated" claim on :8543, ARCH-B4 wrong-session attribution. The re-review has NOT been re-run on this text; status stays Proposed until it passes**, see plan "Pending focused re-review" item 1; **review-repair iteration 2 (2026-10-09) rewrote decisions 1 to 9 again for focused re-review iteration 2: ARCH-NB2 (the claim was bound to `StatusNeedsApproval`, the permission-dialog state, so a permission dialog after an answered question could be promoted and approved by a later Reply), ARCH-C15 to C21 and C17/C18 (abort race, write overlap, limiter order, `reply_id` binding, `IN_PROGRESS`, in-handler verdict), ADV-N1 (the session driver is a concurrent writer), N2, N6, N8, N9, N10, N11. The re-review has not been re-run on this text either; status stays Proposed**; **review-repair iteration 3 (2026-10-09) changed decisions 2, 3, 6, 7 and 9 and added the "Residual risk and sequencing" section for focused re-review iteration 3: ARCH-NB3 (the write lease is a passed `*HeldLease`, one acquirer per chain, its own Story 5.0), ARCH-NB4 and ADV-N14 (the rebinding Host allowlist is the forward-DNS-verified set; the `:8444` and IP-literal rules), ARCH-C31 (supersede path matching and the promotion compare-and-set), ARCH-C32 (spike unknowns), ADV-N15 (hook sender proof, boilerplate denylist), ADV-N16 (keystroke plan per question shape, pre-Enter recheck), ADV-N17 and N18 (audit paths), and the operator decision to keep Reply, sequenced last. **review-repair iteration 4 (2026-10-09) rewrote decision 2d4 (the hook proof is keyed to the session UUID, kept out of argv, replaces stale entries, has a sender table, dedupes proofless duplicates, is cause-labelled, and its honest protection is stated: not the agent itself) and decisions 3, 6 and 7 for ADV-N23, ARCH-C37, ADV-N21, ADV-N24, ARCH-NB5, ARCH-C38, ADV-m11 and ADV-m12, and recorded the accepted limit that Reply works only for sessions started after hook proof ships.** The re-review has not been re-run on this text; status stays Proposed**) **Review-repair iteration 5 (2026-10-09, simplification) edited decisions 3 (the lease paragraph) and 9 only, to point at the simplified guard set of ADR-005 decision 3; the Reply design is unchanged and the status stays Proposed.**
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
   outstanding question in one **hidden** session with one option digit (`reply_text` is a single ASCII digit, decision 5).
   A visible session is rejected with outcome `NOT_HIDDEN` ("answer in the
   terminal").
   **Signaling channel (re-review C14c, decided): expected business outcomes are
   a typed `ReplyOutcome` enum in the response, never Connect errors.** Connect
   errors are reserved for `InvalidArgument` (reply digit or ids malformed),
   `Internal` (audit append failed or an unexpected fault; 0 terminal writes, so
   a client Retry is safe), `PermissionDenied` (decision 7: refused because the
   peer or Host is not acceptable) and `Unauthenticated` (auth middleware). The
   enum has `REPLY_OUTCOME_UNSPECIFIED = 0` (buf lint) and, with the type prefix
   that `ENUM_VALUE_PREFIX` in the repo's `STANDARD` lint requires,
   `REPLY_OUTCOME_SENT`, `_SEND_INDETERMINATE`, `_NOT_SENT`, `_NO_PENDING`,
   `_STALE_PROMPT`, `_NOT_HIDDEN`, `_NOT_WAITING`, `_RATE_LIMITED`,
   `_DISABLED` (plan Story 1.7; this ADR abbreviates them without the prefix).
   **There is no `IN_PROGRESS`** (review-repair iteration 2, ARCH-C21): the
   in-flight rule makes a second caller with the same `reply_id` wait and return
   the first result, a different `reply_id` against a claimed question is
   `NO_PENDING`, and a busy write lease is `NOT_SENT`, so no path could emit it
   and ux.md has no state for it. The metric label set is the lower-cased enum
   names plus `invalid`, `audit_failed`, `refused`; plan, ux.md and validation
   use these names only.
2. **Pending question registry; the live screen decides identity, the hook stream does not (repaired 2026-10-10 against the Spike 1.3g facts).**
   a. *Attribution.* A `PendingQuestion` is registered **only when the question's
      session was resolved from the `X-CS-Session-ID` header (or a verified hook
      proof, d below) to a live instance**. `resolveSessionID`
      (`approval_handler.go:841-870`) falls back to the longest cwd-prefix match,
      and a hidden review session and a visible session can share a path
      (`CreateDirectorySession(item.RepoPath, ...)`,
      `session_service_diagnose.go:57-60,103-108`). A sibling
      `resolveSessionAttribution` (Task 5.6b; `resolveSessionID` keeps its
      signature) returns `header | cwd | unknown`; a `cwd` or `unknown` question
      registers nothing and is never replyable (the toast keeps "View output").
   b. *Key and identity.* The store is keyed by the instance **UUID**; the request
      `session_id` (which `MatchesID` also accepts as a title or tmux name,
      `session/instance_terminal.go:72-80`) is resolved through `findInstanceByID`
      and `Claim` requires `inst.UUID == pending.SessionUUID`. `question_id` is 128
      random bits.
   c. *Payload and shape (replaces the earlier `tool_input["prompt"]` rule).* The
      real `tool_input` is `{"questions":[{question, header, options:[{label,
      description}], multiSelect}]}` (Spike 1.3g, VERIFIED, Claude Code v2.1.296;
      there is no `prompt` key). `question_shape = single` **iff**
      `len(questions) == 1 && !questions[0].multiSelect && 1 <= len(options) <= 7
      && every label is non-empty and at most 80 runes and `multiSelect` is present`; any other payload is `multi` (more than one
      question, or `multiSelect`) or `unknown` and is **not replyable, hard**: the
      card says "Answer in the terminal" and nothing is registered. The entry keeps
      `questions[0].question`, `header`, the ordered option labels and a derived
      `QuestionToken` = the whitespace-normalized first 40 runes of the question
      text. **The token labels the card, the audit line and the log; it is not an
      identity** (a forged hook can supply any token). `broadcastQuestionNotification`
      reads `questions[0].question` and `header` (it read `tool_input["prompt"]`,
      so every real question fell back to "Check the terminal to respond.") and
      stamps `question_id`, `question_shape` and `question_options` (the JSON label
      list) in the notification metadata, so the card needs no extra RPC. There is
      no "return to the operator" branch: a `single` question is the replyable case
      the spike measured.
   d. *Dialog identity is one positive structural match of a live capture,
      `DialogMatch(capture, entry)`, used as both the fingerprint and the
      pre-write guard.* It replaces the status set, the token-in-capture test, the
      boilerplate denylist, the title-region parse and the stored region hash.
      Input: a live `tmux capture-pane -p` of the pane (not the cached status).
      Normalization per line: strip trailing whitespace, strip a leading `│ `
      gutter (2 cells, measured at 80 columns), replace the selection glyph `❯` in
      the first two cells with spaces, collapse whitespace runs. Region: the lines
      after the **last** header row (`☐ <header>`, its text equal to
      `questions[0].header`) through the **last** footer line (a line that starts
      with `Enter to select`); a capture with no header row or no footer does not
      match. **Bottom anchor (re-review blocker 1):** after that last footer line
      the capture may hold only blank lines and unnumbered lines; **any numbered
      row (`<n>. `, with or without the `❯` glyph) or a line containing `Do you
      want` after it is no match**. Agent-controlled text inside a permission
      dialog (a command, a diff) can reproduce the header, question, rows and
      footer, but the dialog's own option rows (`❯ 1. Yes ...`) are drawn after
      that text and are numbered, so the forged block is never the last one (the
      layout order command-then-options is INFERRED from the one captured Bash
      dialog; a deterministic fake-pane test fails without the anchor). It matches **iff**: (1) the lines before the first numbered row, joined
      by one space, equal the normalized `questions[0].question`; (2) the numbered
      rows (`<n>. <text>`) carry exactly the indices `1..N+2`, once each, in order
      (a command box that repeats `1.`/`2.` lines therefore adds a duplicate index
      and fails); (3) row `i` for `1 <= i <= N` has text equal to `options[i-1].label`;
      (4) row `N+1` is `Type something.` and row `N+2` is `Chat about this`.
      Unnumbered description lines are ignored. The match is pinned to the measured
      Claude Code rendering; any other layout fails closed (not replyable, the
      `stale_prompt` counter shows it) and a fixture of the captured pane keeps the
      rule honest. **Status is not an input**: a question dialog and a Bash
      permission dialog are both `INPUT_REQUIRED` with one context string and
      neither produced `NEEDS_APPROVAL` (Spike 1.3g (b), (f)), and the cached status
      is blank for 6 to 9 s after any keypress. A permission dialog has no header
      row, no `Type something.` row and no `Chat about this` row, so it cannot match.
   e. *No supersede, no promotion.* Hooks do not decide which question is on screen:
      two parallel tool calls fire two hooks before either answer and no hook fires
      when the second appears (Spike 1.3g (e)), so "newer hook supersedes older"
      would invalidate the question that is on screen. Each entry is claimable only
      while the live capture shows **its own** question (d); with questions A and B
      registered, a Reply to A while the pane shows B is `STALE_PROMPT`. There is
      no `candidate` state and no confirmation poll: the render delay is zero
      within the 100 ms tick (measured), and the check happens at claim time. A new
      registration with the same question text and labels as a live entry of the
      session replaces it (one card, not two). Consequence, stated: a stale entry
      whose question is asked again identically will answer that new dialog; the
      question text and labels are the same, so the digit means the same option; descriptions and header are not compared, so a re-asked question with identical text and different descriptions is answered by the stale card (accepted residual: a wrong answer, never a permission approval, because the bottom anchor still requires this question's dialog on screen). Unmeasured and fail-closed: a long label that wraps at 80 columns leaves an unnumbered continuation line, the row text is a prefix of the label, and the question is not replyable; an absent `multiSelect` is treated as `unknown`; an empty `header` leaves the `☐` row unmatched; the gutter was measured at 80 columns only; the footer is matched by prefix (INFERRED; fixture-tested).
   f. *Bounds.* 256 entries, at most 8 per session, TTL 30 minutes (INFERRED);
      removed on reply, TTL, replacement and session deletion. The per-session
      limiter map is bounded (LRU 1024, evicted on session delete). A reply to an
      entry whose dialog is gone is `STALE_PROMPT` ("no longer on screen"), which is
      also what the operator sees after answering in the terminal.
   g. *Hook sender proof: best-effort provenance, not the control.*
      `/api/hooks/permission-request` is a plain mux route with no sender
      authentication (`server/server.go:824`), so any local process can post a
      forged question. The proof (`HMAC-SHA256(secret, "ssq-hook-v1" || session
      UUID)`, delivered through a per-session 0600 file outside every worktree that
      the shared hook command reads through the **already-injected
      `STAPLER_SESSION_UUID`** with `curl -H @file`, not in argv, fail-open; full
      command form in plan Story 5.6 (a)) stops a request from a process that
      cannot read that file from registering a replyable question, and a verified
      proof's UUID is the attribution (the title header is ignored, ADV-N29). **It
      does not stop the agent or any same-user process, and it is not the guard
      against typing into the wrong dialog: `DialogMatch` is.** No nonce. Senders:
      only the per-session curl hook and the single-hook curl carry a proof; the
      other four senders of `X-CS-Session-ID` are not replyable. A request without
      a valid proof raises the toast and registers nothing replyable (cause
      `no_proof` or `bad_proof`).
      **Rollout and revocation, as measured (Spike 1.3g (h8), VERIFIED):** Claude
      Code does **not** snapshot hooks at start. A rewritten
      `.claude/settings.local.json` reached the running agent on its next dialog,
      and the hook subprocess inherits `STAPLER_SESSION_UUID` (equal to the session
      UUID). So (i) a session whose entry already has the file form picks up a
      rotated secret on its next hook run, with no restart: rotating the secret and
      rewriting the proof files **does** revoke running sessions; (ii) a session
      whose entry predates the file form gains the proof the next time an injection
      pass rewrites its entry (resume, restart or the service-boot helper), again
      with no agent restart; (iii) a session that no pass has touched keeps
      `no_proof` and shows "Reply unavailable for this session" until one does. The
      earlier "Reply works only for sessions started after hook proof ships" limit
      rested on a snapshot premise that is false and is withdrawn.
3. **Capability, not a flag; claim lifecycle; one digit, one write.**
   - `PendingQuestionStore.Claim(session, questionID, replyID)` is **one critical
     section** under the store mutex: lookup by UUID, state check (`registered`, not
     expired), in-flight check for `reply_id`, then `registered -> claimed`. The
     rate limiters are checked **before** `Claim` (so `RATE_LIMITED` never has to
     un-claim) and **after the idempotency lookup**. The result is a single-use
     `PendingQuestionClaim` (unexported fields, one constructor) or an outcome:
     `NO_PENDING`, `NOT_HIDDEN`. A session delete that arrives while the entry is
     `claimed` marks it `claimed-deleted`; the before-first-byte checks stop the
     write.
   - **Order of the request path: flag, validate, idempotency lookup (cached result,
     then in-flight), limiters, claim.** A repeated `reply_id` while the first is
     **in flight** waits for it (singleflight keyed by `reply_id`, bounded by the
     send deadline) and returns the first result; after a terminal result it returns
     the cached result, so a double tap is not `RATE_LIMITED`. The limiters use
     `rate.Limiter.Reserve()` and `Delay() == 0` so the reservation can be cancelled
     on `NOT_SENT`. `reply_id` is bound to its payload: the cache stores the
     SHA-256 of `(session UUID, question_id, digit)` and the same `reply_id` with a
     different hash is `InvalidArgument`. The result cache and the in-flight map are
     bounded (1024 entries, LRU, 10-minute TTL, INFERRED); an evicted id retried
     later finds the question consumed and gets `NO_PENDING` with 0 writes.
   - **One writer per pane (VERIFIED, ADV-N1).** The session driver is a concurrent
     writer to the same pane (`sendAnswerKey` sends `"1\n"` into a
     `StatusNeedsApproval` dialog that matches `shouldApprovePrompt`,
     `session/session_driver.go:365,824-836,1338-1354`; initial prompt `:706`; idle
     nudges `:884`; `performNudge`, `server/mcp/tools_diagnose.go:266,287`). Neither
     measured dialog was `NEEDS_APPROVAL`, so the driver did not answer either
     (static read), but a version that reports it would. Every writer therefore goes
     through the per-instance **`TerminalWriteLease`** (plan Story 5.0, its own PR
     5a): a `*HeldLease` that exactly one function per call chain acquires and every
     primitive below it only receives. Reply takes `Try` with no wait (busy, or the
     live flag `terminal_write_lease` off, is `NOT_SENT`: claim released,
     reservation cancelled); the lease is **released by the writing goroutine**, not
     when the caller returns; a wedged write keeps it (gauge, WARN, one tray-level
     warning per episode that is never pushed; remedy Delete or a service restart;
     no forced release); it is never taken while holding `i.mu` or the store mutex.
   - `access.QuestionReplyWriter(claim)` is the only way to get a writer for a
     `ReadOnly` target. Its single method `SubmitReply` does **not** call
     `SubmitContentWithEnter` (`SubmitDriverContent` writes a blind second Enter,
     `session/pane_submit.go:126`). It is `SubmitReplyOnce` (Task 5.6k, new
     `session/pane_reply.go`): `ctx` check, `VerifyPaneOwner`, the lease, the
     **before-first-byte checks** (decision 4), **one write of exactly one ASCII
     digit** through the count-returning `Instance.SendKeysN` under
     `ReplySendTimeout`, and then a bounded **post-write closed check**. There is
     no content write, no Enter, no settle wait, no pre-Enter compare and no retry:
     a digit selects immediately and a stray Enter would pick the default of the
     next dialog (Spike 1.3g (c)). The write is guarded by one compare-and-swap
     (`idle -> writing` in the goroutine, `idle -> aborted` on the caller's
     timeout; whichever loses backs off), so after the caller returns at most the
     one write already inside the syscall can still land (stated limit).
   - **Post-write closed check.** After the write, `SubmitReplyOnce` re-captures
     every 100 ms for at most 2 s (the pane updated within 0.5 s in the spike,
     INFERRED margin). `DialogMatch` no longer matching is `SENT` (audit outcome
     `dialog_closed`). Still matching at 2 s is `SEND_INDETERMINATE` (audit
     outcome `dialog_still_open`; the digit may have been dropped, the session may
     be busy): the claim is consumed and the card says to check the terminal.
   - **Failure classes.** `NOT_SENT` = failure strictly **before** the first byte
     (ctx done, `VerifyPaneOwner` failed, the lease busy, a before-first-byte check
     failed with a more specific outcome, or `SendKeysN` returns `0, err`:
     `TmuxSession.SendKeys` returns `0, err` when `GetPTY` fails,
     `session/tmux/tmux.go:1349-1354`, and `Instance.SendKeys` drops the count,
     `session/instance_tmux.go:1539-1545`, hence the mandatory `SendKeysN`): the
     claim is **released** and the reservation cancelled; Retry with the same
     `reply_id` is safe. A timeout or ctx expiry after the byte was handed over, or
     `n > 0` with an error, is **`SEND_INDETERMINATE`**: claim consumed, result
     cached under `reply_id`, no Retry. `SENT` means the digit was written and the
     dialog closed; it does not mean Claude accepted the answer.
   - Tests use a fault-injecting `paneSubmitter` per stage (decision 9 list).
4. **Before-first-byte checks, in this order, all after the audit append and
   immediately before the write, under the lease:** (1) `AccessFor(inst)` still
   `ReadOnly`, `inst.UUID` equals the claim's, the claim is not `claimed-deleted`;
   (2) a status controller is active for the instance (`IsControllerActive`; the
   status **value** is not consulted: it is blank for 6 to 9 s after a keypress and
   cannot tell a question from a permission dialog); (3) `DialogMatch` on a **fresh
   live capture** is true (which includes row `<digit>` carrying exactly the label
   stored for it, the one the card showed), else `STALE_PROMPT` with 0 writes.
   Check (3) is authoritative and mandatory. The audit append does an
   `fsync`, so the window between decision and write is not zero; that is why the
   checks run after it. **Residual, stated:** the interval between the final
   capture and the byte reaching the pane (one `tmux capture-pane` subprocess plus
   the write: tens of milliseconds, plus one write already in the syscall) cannot be closed from userland, and it is the
   **whole** residual risk: a digit that lands in a permission dialog that drew in
   that interval approves option 1 or 2. The kill switch stops further replies; it
   cannot undo a keystroke.
5. **Narrow input: one digit, validated against the entry.** `reply_text` is
   **exactly one ASCII digit** `1..N`, where `N = len(options)` of the entry
   (`N <= 7`, so `N+2 <= 9` is still one digit). Anything else is
   `InvalidArgument` with 0 writes: more than one rune, a non-digit, `0`, a
   full-width digit, whitespace, and in particular `N+1` (`Type something.`, which
   only focuses a text field and would report `SENT` while answering nothing) and
   `N+2` (`Chat about this`, which abandons the question). **Digits `N+1` and `N+2`
   are never written.** Free text (`DigitThenTextEnter`/`ContentThenEnter`),
   `multiSelect` and multi-question calls are **out of v1**: free text was never
   measured end to end, a first-character digit would select an option and the rest
   plus Enter would land in the next dialog (routinely a Bash permission dialog
   whose default is "1. Yes"), and the typed text is echoed inside the dialog
   region. They show "Answer in the terminal" until a separate spike records a
   safe sequence. `ContentThenEnter`, `ReplyKeystrokes`, the byte limit, the
   `Cf`/C1 validator and its client port are deleted from the Reply design.
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
   `session_title`, `question_id`, `reply_id`, `option_index`,
   `option_label` (first 80 runes), `question_token`,
   `listener` (`local`/`remote`), `peer_addr` (connect `req.Peer()`), `host`,
   `origin`, `user_agent`, `auth_mode`; the outcome line adds `outcome` and the
   failure stage (Reply: `dialog_closed`, `dialog_still_open`, `stale_prompt`).
   **There is no hash of the reply** (a digit is brute-forceable, so a hash adds
   nothing; the audit line records which option was chosen instead: `option_index`,
   `option_label`, `question_token`); the INFO log `hidden_session_reply` carries
   `option_index` and `question_token` only, never the label or any free text. The O7
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
   ADR: a fault-injecting submitter per stage (before-first-byte, write error,
   timeout, dialog still open) asserting exactly one one-byte write across a
   Retry; `DialogMatch` against the captured pane fixture, a Bash permission dialog,
   a forged hook whose question text sits inside a permission dialog's command
   box, and a parallel pair (A on screen, B registered); digits `N+1`, `N+2`, `0`,
   multi-rune and non-digit rejected; wrong-session path sharing; Host rebinding;
   the write lease against a fake driver, including one `NEEDS_APPROVAL` pane that
   matches `shouldApprovePrompt`; the compare-and-swap abort race; double tap
   inside 5s (validation sections J and K).
10. **Client**: a Reply card in the read-only view (design/ux.md Surface 12b) and
    a "Reply" action beside "View output" on the hidden `INPUT_REQUIRED` toast,
    tray row and Background row, shown only while a replyable (header-attributed,
    single-question, single-select) question is pending. The card offers one
    numbered button per option label and no text field; every other question shows
    "Answer in the terminal".

## Residual risk and sequencing (operator decision to keep Reply, review-repair iteration 3)

The adversarial review recommended shipping Reply later as its own project (it is hard-gated on an unrun spike, has produced a new blocker or cross-cutting concern in every review pass, and nothing else depends on it). **The operator chose to keep Reply in scope.** Reply is therefore sequenced **last in Epic 5**; nothing else in Epics 1-5 depends on it (the write lease is Story 5.0, the audit sink core is Task 2.8h, the guard set is Story 1.8, and the scan's Reply entry is added by Task 5.6d); it is hard-gated on (a) Spike 1.3g's recorded results and (b) a passing re-review of the Reply design as it then stands; and it ships behind the `hidden_session_reply` kill switch.

**Residual risk, in plain words**: a digit can land in the wrong dialog. The worst case is that it approves a permission dialog (option 1 is "Yes"), which runs a tool in a hidden agent. The kill switch stops further replies; it **cannot undo a keystroke that was already sent**. The design lowers the chance: the digit is written only after a positive structural match of a fresh live capture (decision 2d) that no permission dialog can satisfy, one byte is written, and nothing follows it. Not closed: (a) the interval between the final capture and the byte reaching the pane, which cannot be closed from userland and is the **whole** residual risk of the write itself, (b) operator keystrokes in the terminal in that same interval, and (c) a hostile same-user process, **including the hidden agent itself, which can read its own hook file and the secret**: the proof does not stop it, and the structural match is the control. Hooks and the proof are best-effort provenance: supersede does not exist and a forged hook can only create an entry that the live screen then refuses. Until Story 5.6 ships, the hidden `INPUT_REQUIRED` toast, tray row and Background row show "View output" only, which is the reduced version with no typing.


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
- The keystrokes are measured (Spike 1.3g, v2.1.296): a digit `1..N` selects a
  single-select option immediately, with no Enter. Only that is v1. The layout
  literals (`Type something.`, `Chat about this`, the `│ ` gutter, the header row
  and footer) are pinned to that version; a different rendering makes `DialogMatch`
  fail and Reply fail closed.
- A late reply to a dialog that changed is rejected by `DialogMatch` alone
  (`STALE_PROMPT`): the question text, the option labels in order and the two
  trailing rows must be on the live screen. Residual: the capture-to-write
  interval, stated in "Residual risk".
- `SEND_INDETERMINATE` is a real, user-visible state: the operator may have to
  look at the terminal. That is chosen over the alternative of ever typing twice.
- The audit file is the only record of who replied; it is local, size-capped and
  not exposed through any RPC (INFERRED sufficient for a single operator). The
  `session_title` of review sessions is `uniqueDispatchTitle("review", item.ID)`
  and so embeds the backlog item id (harmless; note it if the file is exported).
- Story 5.6 is hard-gated on Spike 1.3g (run 2026-10-10) and on a passing focused
  re-review of this repaired design (sequenced last in Epic 5, see "Residual risk
  and sequencing"). Still unmeasured and not needed by v1: free text end to end,
  the forged-hook negative test against a real permission dialog, the supersede
  frequency with a visible peer (moot, supersede is deleted), the per-sender
  header table.
- Cost: one server story and one client story, sized as cost classes in plan
  "Effort" (Story 5.6); the Reply proto lands in a separate Contract PR 3
  (Story 1.7) after Spike 1.3g, and the whole path has its own kill switch,
  `hidden_session_reply` (ADR-004 decision 11).

## Addendum 2026-10-10: Spike 1.3g measured results (VERIFIED live; full record under plan.md Task 1.3g)

Measured on an isolated manual instance with real Claude Code v2.1.296 and a hidden `backlog:review` session. These supersede the INFERRED statements above wherever they conflict:

- **Payload shape.** `tool_input` is `{"questions":[{question, header, options:[{label, description}], multiSelect}]}`; there is no `prompt` key. Any rule that reads `tool_input["prompt"]` yields no token. The `QuestionToken` and `question_shape` derive from `questions[]` (single = one question and `multiSelect:false`).
- **Status set cannot discriminate.** The question dialog and a Bash permission dialog raised right after it are both `DETECTED_STATUS_INPUT_REQUIRED` with the same context ("Selection prompt with numbered options"); neither produced `NEEDS_APPROVAL`. The status set is a coarse pre-filter only, and it is blank for about 6 to 9 s after any operator keypress. Hook event plus live fingerprint decide.
- **Keystrokes.** Single-select: a digit selects immediately, no Enter (`DigitOnly`). "Type something." is the row after the last option: its digit only focuses a text field, then text and Enter submit. multiSelect and multi-question calls need navigation (tabs, a review page); they are not single-keystroke replies and are "Open the terminal to answer".
- **Fingerprint.** A live `capture-pane` of the dialog region was byte-identical across 8 captures over about 32 s of idle; the selection glyph `❯` changes it and must be normalised; at 80 columns the question wraps with a `│ ` gutter.
- **Parallel questions.** Two tool calls in one turn fire two hooks before either is answered, the pane shows A then B, and no hook fires when B appears. A "newer hook supersedes older" rule would mark A superseded while A is the dialog on screen; the fingerprint, not hook order, must decide which question the pane shows.
- **Render timing.** Dialog visible, hook delivered and `INPUT_REQUIRED` reported within one 100 ms tick; no render-delay window is needed beyond hook delivery.
- **Hook environment.** The hook subprocess inherits `STAPLER_SESSION_UUID` (equal to the session UUID); the `X-CS-Session-ID` header carries the title. A rewritten `.claude/settings.local.json` reached the RUNNING agent, so the "hooks are snapshotted at start" premise is false for this version: the hook-proof design must not rely on it.
- **Not measured**: forged-hook negative test and boilerplate denylist (h6), supersede frequency with a visible peer (h7), per-sender header table, behaviour with no controller.

Consequence: the Spike 1.3g gate is **run**; the status-set, payload-shape, supersede and hook-snapshot assumptions it falsified are repaired in decisions 2 to 5 above, and Story 5.6 and Contract PR 3 wait only on the focused re-review recorded in plan.md.

### Second Reply review, 2026-10-10: FAIL (3 blockers), repaired the same day

The independent review found 3 blockers and 9 non-blocking items (full text: plan.md "Second Reply review, 2026-10-10"). Repair, in this file's decisions 2 to 5: (1) v1 is single-question, single-select, answered by exactly one digit `1..N` that the live capture shows against the card's label; free text, `multiSelect` and multi-question calls are "Answer in the terminal"; digits `N+1` and `N+2` are never written; `ContentThenEnter` is deleted. (2) Identity is the positive structural match `DialogMatch`, used as both fingerprint and pre-write guard; status and tokens are not inputs. (3) The payload rule reads `questions[]` and derives `QuestionToken` from `questions[0].question`. Non-blocking: hooks no longer supersede (parallel pair); status dropped from the rule; every "hooks are snapshotted at start" statement replaced by the measured behavior (decision 2g); the audit records `option_index`, `option_label` and `question_token`; the capture-to-write interval is stated as the whole residual risk; `SENT` adds the post-write closed check; one test keeps a `NEEDS_APPROVAL` pane for the session driver; `multi` is hard not-replyable and the 2-cell gutter is recorded; the title header is still ignored when a proof verifies (ADV-N29). Decisions 6 to 8 (authz, audit, flag, kill switch, lease, claim lifecycle) held and are unchanged except the audit fields. Status: repaired, **focused re-review pending** (plan.md "Reply re-review, 2026-10-10").
