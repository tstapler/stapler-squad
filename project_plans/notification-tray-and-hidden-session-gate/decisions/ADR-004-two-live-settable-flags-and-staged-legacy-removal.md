# ADR-004: Two Live-Settable Flags (the Gate Flag Has Per-Kind Overrides); Legacy Checks Stay Until Default-On (Staged Removal)

**Status**: Proposed (revised in plan repair iteration 1 for staged removal; revised in iteration 3: per-scope override built, operator decision O1 DECIDED; Phase 4: explicit-false handling extended to kind overrides, third flag `hidden_session_readonly_guards`, file renamed to match the title; **review-repair iteration 1 (2026-10-09) changed decisions 1, 2, 4, 6, 8, 10, 11 and added 12-13 for focused re-review ARCH-C7..C10, ARCH-C12/C14, ADV B3, C11..C16: a `FlagMutation` enum replaces `clear_override`, one mutex-guarded `FlagCache.Reload`, rollback by delete, kind precedence, registration of the gate flag only in PR 2a-2, fail-closed reads, and soak evidence that records gate-on time. The focused re-review has not been re-run on this text**; **review-repair iteration 2 (2026-10-09) changed decisions 4, 8, 12 and 13 for focused re-review iteration 2: ARCH-C22/C26 and ADV-N5 (enum values spelled with the type prefix, the set values carry the enabled state, a set with an empty scope is `InvalidArgument`), ADV-N4 (composite `status_detail` provider), ADV-N3/ARCH-C25 (`soak_streak_hours`), minor m3 (three Stage 1b reminders). The re-review has not been re-run on this text either**; **review-repair iteration 3 (2026-10-09) changed decisions 4, 6, 8 and 12 for focused re-review iteration 3: ARCH-C33a (for a non-zero `mutation` the server ignores the plain `enabled`), ADV-N17 (no sink I/O under `updateMu`), ARCH-C33c and minor m8 (Stage 3 reads a dedicated `explicit_off_scopes` field; the setter keeps replacing its own slot), ADV-N19 (the soak figure needs a failure-path observation, counts only events the gate decided to suppress and excludes the flip bucket). **review-repair iteration 4 (2026-10-09): decision 12 splits the failure and needs-human soak counters and adds the labelled probe counter (ADV-N25), and decision 14 adds the fifth flag `terminal_write_lease` (ARCH-C36c)**. The re-review has not been re-run on this text**; **review-repair iteration 5 (2026-10-09, simplification) changed only the wording of decision 14 (the scan no longer exists as a default, ADR-005 decision 3); status stays Proposed**)
**Date**: 2026-10-07
**Project**: notification-tray-and-hidden-session-gate

## Context

Requirements ask for live-settable flags "global + per-scope override".
`UpdateFeatureFlagRequest` has only `name` and `enabled`
(`proto/session/v1/session.proto:3098-3103`), `FeatureFlag` has `name`,
`enabled`, `description`, `status_detail` (`:3014-3024`) and `config.FeatureFlags`
is a flat `map[string]bool` (`config/config.go:359`); there is no scope concept
today. Flags are registered in `knownFeatureFlags`
(`server/services/feature_flag_service.go:169`); the Settings > Features panel is
`web-app/src/app/settings/features/page.tsx` over `FeatureFlagsContext.tsx`.
`GetFeatureFlagWithDefault` honors an explicit persisted `false`
(`config/config.go:1793`) and `GetFeatureFlagOverride` reports whether a value was
explicitly persisted (`:1826`). The precedent read,
`config.LoadConfig().GetFeatureFlagWithDefault` (`ProgramCLIFlagProbeEnabled`),
reads and parses the config file on every call (`config/config.go:1373`), which is
acceptable per RPC but not per `Publish` (~108 call sites), so the gate reads a
cache (Decision 6).

## Decision

1. Add `hidden_session_gate` (server) and `notification_tray_v2` (web) to
   `knownFeatureFlags`. **The operator decided to build the per-scope
   override** (O1); requirements.md "Risk Control" is therefore met, with no
   deviation. **`hidden_session_gate` is registered in PR 2a-2, as the last
   commit after the stats RPC is wired, not in PR 2a-1** (re-review ADV C16:
   "turn the flag on only after 2a-2" was enforced by prose). Until it is
   registered `UpdateFeatureFlag` rejects the name (`InvalidArgument`, unknown
   flag: `feature_flag_service.go:380-390`), so the flag cannot be set live before
   the soak evidence exists; once registered, enabling it returns
   `FailedPrecondition` while the stats writer is not running (plan Task 2.8g). A
   hand edit of `config.json` can still set it; the gate then logs WARN
   `delivery_gate_enabled_without_stats` and shows a `status_detail`.
2. **Scope model: hidden-session kind.** A scope is one of `review`, `triage`,
   `diagnose`, `other`, derived when the session is indexed from its tags
   (`backlog:review`, `backlog:triage`, `backlog:diagnose`; any other hidden
   session is `other`). **Tags are mutable and may co-occur, so the order is
   total: `review` > `diagnose` > `triage` > `other`**, and the index re-derives
   the kind on every tag change (feed point tested: an `UpdateSession` tag edit
   changes the index kind). **No producer creates a `backlog:triage` Instance**
   (the literal appears only in a read at `session/session_driver.go:1112`;
   re-review ARCH-C9, ADV C13), so `kind:triage` is an inert control until Spike
   1.3h proves it reachable: `knownFeatureFlags` lists `kind:triage` as a valid
   scope, and the Settings panel shows the control, only if the spike says so;
   otherwise it is trimmed from both. The panel also shows, per kind, the count
   of events resolved to that kind in the last 24h (from the stats RPC) so a dead
   scope is visible. Sessions resolved `Unresolved` or `NotASession` use the
   global value (stated in the panel help text). It is the smallest scope that
   serves "enable the gate for review sessions only". Rejected: per-session UUID (ephemeral, created at
   tens per day, an override is stale before it matters and the panel could not
   list them) and per-workspace/instance (one server instance is one operator and
   `STAPLER_SQUAD_INSTANCE` already isolates state directories, so it is already
   "global" for that state). Only flags marked scopable accept a scope:
   `hidden_session_gate`. `notification_tray_v2` is a per-viewer UI flag and
   rejects a scope.
3. **Precedence, most specific first**: kind override (explicit on or off) >
   explicit global value > registry default (`GetFeatureFlagWithDefault`). A
   session with no resolvable kind (`Unresolved`, `NotASession`) uses the global
   value. Storage: `Config.FeatureFlagScopes map[string]map[string]bool` (JSON
   `feature_flag_scopes`, omitted when empty) beside `FeatureFlags`.
4. **API** (re-review ARCH-C8, ADV C11: this cannot change after Contract PR 2
   merges, so it is settled here): `UpdateFeatureFlagRequest` gains
   `string scope = 3` (`"kind:review"`, or the literal `"global"` for a global
   set) and `FlagMutation mutation = 4`, an enum spelled with the type prefix that
   `ENUM_VALUE_PREFIX` (part of the `STANDARD` lint in `buf.yaml`) requires
   (review-repair iteration 2, ARCH-C22): `FLAG_MUTATION_UNSPECIFIED = 0` (the
   legacy shape an old client sends: **`scope` empty**, `enabled` read, global
   set), `FLAG_MUTATION_SET_ENABLED = 1`, `FLAG_MUTATION_SET_DISABLED = 2`,
   `FLAG_MUTATION_CLEAR_SCOPE = 3`, `FLAG_MUTATION_RESET_GLOBAL = 4` (abbreviated
   `SET_ENABLED` and so on elsewhere). This replaces the earlier
   `bool clear_override`, whose meaning flipped on whether `scope` was empty (a
   client that dropped `scope` on an "Inherit" click would have deleted the global
   value), and the first patch's single `SET`, whose value came from the plain
   `bool enabled`: omitted means `false`, so `SET` without `enabled` wrote `false`
   (ARCH-C26). The set values **carry the enabled state in the enum**, so a set is
   always explicit, and `enabled` is ignored for every non-UNSPECIFIED mutation (a `SET_ENABLED` with `enabled=false` is neither a conflict nor an error; only `FLAG_MUTATION_UNSPECIFIED` reads `enabled`; review-repair iteration 3, ARCH-C33a).
   Validation (ADV-N5, ARCH-C22): **a set with an empty `scope` is
   `InvalidArgument`** (a global set must carry the literal `scope = "global"`, so
   a client that drops `scope` on a per-kind click is refused instead of silently
   writing the global flag); `FLAG_MUTATION_UNSPECIFIED` with a non-empty `scope`
   and any unknown enum value (proto3 enums are open: a newer client or a client
   bug) are `InvalidArgument`, so a scoped change can never be applied as a global
   write; `CLEAR_SCOPE` needs a `kind:` scope and `RESET_GLOBAL` forbids one
   (`InvalidArgument`); a request with no new fields never deletes a persisted
   value (tested). Disabling, `CLEAR_SCOPE` and `RESET_GLOBAL` are never refused by
   the Task 2.8g stats-writer precondition (only enabling is).
   `FeatureFlag` gains `repeated FeatureFlagScopeOverride scopes = 5`. The
   response lists the overrides **read back from the persisted config**, and a
   client that sent a scoped change and receives a response without that scope's
   entry must treat it as an error, not a success (re-review ARCH-C14d: a new
   client talking to an older server drops the unknown `scope` and the server
   would apply the change as a global write). The earlier "Residual, stated" (a
   new client that drops `scope` on a `SET` writes the global value) is **closed
   on the server** by the empty-scope rule above, and the read-back error stays as
   the check against an older server; the `flag_change` record (previous and new
   value, ADR-010 decision 6) now exists from PR 2a-2 (plan Task 2.8h), the first
   PR in which the gate can flip, not from Epic 5. The Settings panel shows an "Overrides by session kind"
   disclosure (Inherit / On / Off) for scopable flags.
5. **Staged removal (chosen over default-on).** PR 2a ships the gate with
   `hidden_session_gate` default **off** and leaves every legacy hidden-session
   check in place (LOW-priority check in `notification_service.go`, four
   branches in `session_service_events.go`, `autonomous_orchestration_service.go`
   L619, `suppressForHidden` in `review_queue_manager.go`). With the flag off,
   hidden-session suppression is exactly today's, plus shadow counters, so
   merging cannot re-flood. The operator flips the flag on live (optionally for
   `review` only first) and soaks; PR 2b is **one commit** that flips the
   default on and deletes the legacy checks.
6. The gate never calls `config.LoadConfig()` on the publish path. It reads a
   `FlagCache`: one immutable `{global, per-kind overrides}` snapshot behind an
   `atomic.Pointer`. **There is one writer path** (re-review ARCH-C7, ADV C12):
   `FlagCache.Reload()` takes a `reloadMu`, loads the config **inside** the lock
   and swaps; the 5s ticker (to pick up external config edits) and
   `UpdateFeatureFlag` (called after its save, still under `updateMu`) both call
   it, so a ticker that read the file before a save can no longer publish its
   stale snapshot over the operator's rollback (a stale window is at most the
   duration of one load, not 5s). `FeatureFlagService` reaches the cache through
   a one-method observer interface (`OnFlagChanged(name)`) registered like
   `SetStatusDetailProvider`, so `services` depends on a one-method interface and
   `deliverygate` does not import `services`. A reload error keeps the last good
   snapshot (no flip on a read error). The ticker stops with the server context
   and is joined. **Rollback by delete**: the existing rollback
   (`previousEnabled := cfg.GetFeatureFlag(name)` then `SetFeatureFlag`,
   `feature_flag_service.go:407,428`) turns an absent key into an explicit
   `false` after a controller failure, which is exactly the trap decision 8
   describes; the scoped path, and the global path (collateral fix in the same
   story), record whether the key existed (`GetFeatureFlagOverride`,
   `config/config.go:1826`) and roll back by `DeleteFeatureFlag` or
   `DeleteFeatureFlagScope` when it did not. `UpdateFeatureFlag` does a
   whole-config load-modify-save, so a concurrent writer of another config field
   can lose an update (pre-existing, `:433,437`); the scopes map is written under
   the same `updateMu` and the limit is documented, not fixed here. **No sink I/O happens while `updateMu` is held** (review-repair iteration 3, ADV-N17): a loosening flip's audit append runs before it is taken, under a 2s bound, and a tightening flip only queues its line, so a stalled `fsync` cannot make the kill switch wait.
7. After PR 2b, flag **off** means "deliver everything for hidden sessions" (the
   legacy checks no longer exist). This is the documented escape hatch; its
   failure direction is over-delivery, which cannot lose a needed notification.
   Rolling back fully after 2b is a `git revert` of the 2b commit.
8. **An explicit persisted `false` survives the default flip** (it is the
   operator's choice and is not overridden), which after 2b would silently
   re-open the flood. So 2b logs WARN `hidden_session_gate_explicit_false` at
   startup, sets the flag's `status_detail` ("Gate is OFF: hidden sessions
   deliver everything") so the Settings panel shows it, lists the check in the
   Stage 3 prerequisites, and names it in the PR and the reference doc. **The
   same applies to a persisted per-kind `false` override** (`feature_flag_scopes`):
   one WARN per kind and a `status_detail` naming the kind, so the Stage 3
   prerequisite is mechanical (`GetDeliveryGateStats` returns an empty `soak.explicit_off_scopes`; the joined `status_detail` can also carry benign text such as "stats writer not running", so the prerequisite reads the dedicated field, review-repair iteration 3, ARCH-C33c). That holds only if the three contributions (stats writer not
   running, explicit global false, per-kind false) are **composed, not
   overwritten**: `SetStatusDetailProvider` keeps one provider per flag name
   (`server/services/feature_flag_service.go:326-333`, `statusDetailProviders[name]
   = fn`), so a later registration would hide an explicit false and the Stage 3
   prerequisite would pass falsely. A composite provider owned by plan Task 2.8g
   (`AddStatusDetailSource`, ordered contributions joined with "; ") fixes it
   (review-repair iteration 2, ADV-N4). The existing `SetStatusDetailProvider` keeps replacing its own named slot (`SetStatusDetailProvider("backlog", ...)` is re-registered by `feature_flags_test.go:150,169`; minor m8). A global "Reset to default" (`mutation = RESET_GLOBAL`) deletes the explicit key, so a soak-time rollback does not linger as
   an explicit `false`.
9. `notification_tray_v2` default off; off renders the existing unbounded toast
   list and modal panel. One flag-independent exception: `clearAll` no longer
   removes pinned/pending-decision toasts, because the old behavior can delete
   an unread approval toast and violates #738. The flag stays off after the PR 3
   merge until the operator has recorded the ADR-009 device checks.

10. **Third flag, `hidden_session_readonly_guards` (Phase 4, operator decision
    O7 repair).** A global-only, default-on flag in `knownFeatureFlags` that the
    Story 5.2 unary guards (`WriteToSession`, the steer branch's hidden rejection,
    `RestartSession`/`RestartShell`) read per RPC through `AccessForUnary`, so an
    incident has a non-deploy escape hatch (`feedback_rollout_flags_live_settable_no_env_vars`).
    The stream drops (Story 5.1) are not behind it, and **neither is Reply**
    (decision 11). It is not scopable and rejects a scope. Known weakness carried
    to the pending focused re-review: while off it also opens the steer guard for
    hidden targets that do not qualify for the O7 path. **Repaired (re-review
    ARCH-C6, ADV C8):** while off, the O7 path stays active and audited, a steer
    to a non-qualifying hidden target is allowed only with an audit line
    `kind=guard_bypass` and a WARN at most once a minute, and the flag is read
    through an injected `GuardsFlag` interface backed by an atomic (not a
    `guardsEnabled bool` parameter and not `LoadConfig` per RPC) and **fails
    closed** (an unreadable config keeps the guards ON). Flipping it is audited
    (ADR-010 decision 6) and it is a safety rail, not a security boundary
    (ADR-005 decision 5).

11. **Fourth flag, `hidden_session_reply` (triad iteration 1, engineering gap).**
    A global-only, default-**on** flag (kill-switch semantics) that
    `ReplyToPendingQuestion` reads per RPC. Off: outcome `DISABLED` (a response
    enum value, not a Connect error), 0 writes, the claim is not taken, and the client hides the
    Reply card and action. It is deliberately independent of
    `hidden_session_readonly_guards`, so rolling back Reply never re-opens the
    unary guards and rolling back the guards never re-opens Reply. Registered in
    `knownFeatureFlags`, no env var, not scopable (plan Task 5.6j). The read
    **fails closed**: an unreadable or unparsable config reads as off (Reply
    disabled). Turning it on is audited before it is persisted (ADR-010 decision
    6).

12. **Soak evidence records when the gate was on (review-repair iteration 1,
    adversarial ADV-B3).** The stats RPC and file carry per-bucket
    `gate_on_seconds` (global and per kind), `uptime_seconds`, a `kind` label on
    `suppressed`, `would_suppress` and `hidden_delivered`, `process_start`, a
    short flag-change history and a **server-computed `soak_streak_hours`**
    (formerly `soak_gate_on_hours_with_traffic`), so the Stage 3 prerequisite is
    one number rather than the operator's memory of when he flipped the flag (plan
    Story 1.6 and Story 2.8). It is settled before Contract PR 2 because that PR
    freezes the response shape. **Reshaped in review-repair iteration 2 (ADV-N3,
    ARCH-C25):** the first definition summed gate-on hours over the whole window
    when any hidden event was seen and no off flip was in the window, so one event
    in 48 hours made all 48 hours count, failure-only traffic (which exercises no
    suppression) counted, and an off flip zeroed the figure for 72 hours. Now the
    bucket carries `routine_events_while_on`, and `soak_streak_hours` is the sum of
    `min(gate_on_seconds, uptime_seconds)/3600` over the buckets **since the last
    off flip** (`soak.last_off_flip_at`), counting only buckets with
    `routine_events_while_on > 0`; an off flip restarts the streak at the flip. The
    Stage 3 prerequisite also requires `stats_file_status.writable`, because a
    figure held by a file that cannot be written is lost on restart. **Review-repair iteration 3 (ADV-N19):** `routine_events_while_on` counts only events the gate **decided to suppress** (an `Unresolved` session is delivered fail-open and does not count, so a broken resolver cannot accumulate streak hours), the bucket that contains `last_off_flip_at` is excluded (it carries up to an hour of pre-flip gate-on seconds), and `soak.failure_delivered_while_on` **and `soak.needs_human_delivered_while_on`** must each be at least 1 (organic, or the labelled live probe of the Stage 2 checklist: a `[synthetic-probe]` notification sent with `-p low` against an existing hidden session, which traverses the real gate, is not pushed and is counted separately in `soak.probe_delivered_while_on`; review-repair iteration 4, ADV-N25), because a streak made only of routine hours never exercises the failure path the gate promises to keep open, and the earlier merged count let one `error` delivery satisfy both classes. The probe proves the `ssq-notify` path only.

13. **Stage 1b needs PR 2e (review-repair iteration 1, ADV C15).** The
    review-only flip depends on the per-kind override, so PR 2e is inside R1 and
    the Stage 1b reminders are created only once both PR 2a-2 and PR 2e have
    merged; the reminder never degrades to a global flip. There are **three**
    reminders (review-repair iteration 2, minor m3): flip for kind `review`, flip
    globally once the `review` phase is clean (Stage 3 prerequisite (2) needs the
    global figure), and open PR 2b.

14. **Fifth flag, `terminal_write_lease` (review-repair iteration 4, ARCH-C36c).** The per-instance write lease (plan Story 5.0, PR 5a) changes behavior for every session, visible ones included (`WriteToSession`, the visible steer, MCP writes and every driver write return a retryable busy result while another writer holds the lease), so it ships behind a global-only, default-**on** flag in `knownFeatureFlags` (no env var, not scopable, plan Task 5.0f), read through an injected atomic that fails closed (an unreadable config keeps the lease on). Off: `TryTerminalWriteLease` and `AcquireTerminalWriteLease` return a valid **non-exclusive** `*HeldLease` at once, so the capability types keep working and only the serialization is skipped, which is today's behavior; `ReplyToPendingQuestion` returns `NOT_SENT` while it is off, because its safety needs real serialization. Independent of `hidden_session_readonly_guards` and `hidden_session_reply`. Every flip is a `flag_change` line (plan Task 2.8h).

## Alternatives Considered

- **Ship `hidden_session_gate` default ON with off as the escape hatch**:
  rejected. Default-on would apply the policy to the live instance with no
  shadow period; shadow counters are the verification instrument. Staged
  removal costs one extra PR and a soak window (at least 24h with non-zero hidden traffic, a named wall-clock blocker in the plan).
- **Delete legacy checks in PR 2a with flag-off = deliver everything** (earlier
  draft): rejected; merge would re-flood hidden-session notifications until the
  operator flips the flag.
- Env var rollout flags: rejected (`feedback_rollout_flags_live_settable_no_env_vars`).
- Global-only flags with a documented deviation (the earlier recommendation):
  superseded by operator decision O1.
- A generic `scope` string with free-form keys: rejected; a closed set of four
  kinds is validated and testable.
- Separate `hidden_session_gate_shadow` flag: rejected; shadow is the flag-off
  state, one fewer knob.

## Consequences

- PR 2a changes no hidden-session suppression. Its flag-independent behavior
  changes (enum fix, metadata stamps, the crash notification) are listed in plan
  Stage 0 and the PR description.
- Seven temporary `legacy_hidden_suppressed{site,type,class}` counter sites
  (closed `site` set in plan "Observability Plan") exist between 2a and 2b.
- Two flag states to test per legacy site until 2b; Story 2.9 writes
  characterization tests on both before deleting.
- After 2b, turning the flag off (globally, or for one kind) re-opens the flood
  for that scope; accepted and documented rather than hidden.
- The flag API and panel grow a scope dimension (Story 2.11, cost class in plan
  "Effort"); `UpdateFeatureFlag` without a scope behaves exactly as before. The
  scope fields land in Contract PR 2 (Story 1.6) only after the focused
  re-review, with the `FlagMutation` enum (decision 4) in place of the
  overloaded `clear_override`.
- **Binary rollback drops per-kind overrides.** An older binary's `Config` has no
  `feature_flag_scopes`, so its next `SaveConfig` rewrites `config.json` without
  it and a persisted per-kind `false` silently returns that kind to the global
  value. Documented in the reference doc and as a Stage 3 checklist line; this
  project does not add a `json.RawMessage` catch-all.
