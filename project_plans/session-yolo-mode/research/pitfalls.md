# Pitfalls Research: Per-Session Auto-Approve ("Yolo Mode") Toggle

**Phase**: 2 — Research (Pitfalls dimension) | **Source**: `project_plans/session-yolo-mode/requirements.md`

## 0. Headline finding: this partially exists already for Claude Code

Before the five requested sections: the single most important pitfall is that **stapler-squad
already has a bool field that injects the functional equivalent of
`--dangerously-skip-permissions` for Claude Code sessions**, and the requirements doc's problem
statement ("stapler-squad has no first-class concept of this") is not quite accurate.

- `session/ent/schema/session.go:46-47` — `field.Bool("auto_yes").Default(false)`
- `session/instance_tmux.go:154-156`, inside `buildClaudeCommand` (Claude-only):
  ```go
  if i.AutoYes {
      parts = append(parts, "--permission-mode", PermissionModeBypassPermissions)
  }
  ```
  where `PermissionModeBypassPermissions = "bypassPermissions"` (`session/instance.go:429`).
  Per Anthropic's own CLI reference (confirmed via web search below), `--dangerously-skip-permissions`
  *is* `--permission-mode bypassPermissions` under the hood — so `AutoYes=true` already does
  exactly what this feature proposes to add, for Claude Code only.
- `AutoYes` is **dual-purpose**: besides the flag injection above, it also drives
  `TapEnter()` (`session/instance_tmux.go:346-354`), which sends a literal Enter keypress into
  the tmux pane whenever a prompt is detected — a keystroke-level fallback that predates (or
  coexists with) the flag-injection behavior.
- Non-Claude programs (including Aider) are classified as `plainProgram` and passed through
  **completely unchanged** (`session/instance_tmux.go:63-67`, `105-119`) — `AutoYes` has zero
  effect on them today.

Design implication to carry into planning: decide explicitly whether the new `auto_approve`
field **replaces/extends `auto_yes`** (rename + extend to cover Aider, keep the existing
Claude behavior and TapEnter fallback) or is a **second, separate field**. Shipping a second
field named e.g. `auto_approve` next to the existing `auto_yes` without reconciling them is a
direct violation of `.claude/rules/interface-pollution-checklist.md`'s "no-op
duplication"/"unjustified new surface" spirit — a user (or future maintainer) will not know
which of two similarly-named booleans controls what, and a session could theoretically have
`auto_yes=true, auto_approve=false` or vice versa, an inconsistent state with undefined
behavior. This should be raised as an explicit decision point in the plan phase, not silently
resolved by adding a new column.

---

## 1. Safety/security pitfalls

`--dangerously-skip-permissions` (and Aider's yolo-equivalent) exist to remove a human
approval gate from an agent that can run arbitrary shell commands and edit arbitrary files.
Verified via web search (queries: "Claude Code CLI --dangerously-skip-permissions flag 2026
documentation", "--dangerously-skip-permissions data loss incident agent unattended risk"):

- Anthropic's own guidance: *"Letting Claude run arbitrary commands can result in data loss,
  system corruption, or data exfiltration via prompt injection. Only use
  `--dangerously-skip-permissions` in a sandbox without internet access."* Even Anthropic's own
  researchers reportedly caveat running it with "run this in a container, not your actual
  machine."
- Documented incident class: a study cited by multiple 2026 sources found **32% of developers
  using `--dangerously-skip-permissions` hit at least one unintended file modification, and 9%
  reported actual data loss or corruption**. One concrete anecdote: an agent asked to clean up
  a repo generated `rm -rf tests/ patches/ plan/ ~/` — the trailing `~/` expanded to the user's
  entire home directory, destroying desktop files, keychain data, and application state.
  (Sources: morphllm.com/claude-code-dangerously-skip-permissions,
  kiteworks.com/cybersecurity-risk-management/ai-data-governance-dangerously-skip-permissions-risk,
  ksred.com/claude-code-dangerously-skip-permissions-when-to-use-it-and-when-you-absolutely-shouldnt)
- The risk is compounded by prompt injection: an unattended agent reading web content, PR
  descriptions, or issue text with no approval gate can be steered into destructive or
  exfiltrating actions by content it reads, not just by user intent.

**Guardrails to design against, beyond "the badge shows it's on"** (raise as options for the
plan phase, since requirements.md's Constraints section only commits to "clearly labeled and
opt-in only, never a silent default" — it does not yet decide *how much further* to go):
- Confirmation on enable (creation-time checkbox is not itself an accidental-click risk since
  it's already an explicit action, but a *post-creation* toggle flipped on a live/resumed
  session is a bigger blast-radius action and could warrant a lightweight "this session will
  run without approval prompts" confirm, matching the weight of other destructive-ish toggles
  in this codebase).
- **Never a bulk/default-on path**: confirm no batch-creation or template/preset flow can set
  `auto_approve: true` as a bundled default (`server/services/session_service.go:3567`
  already threads `AutoYes` through a batch request path — `batchReq.AutoYes` — so the same
  batch path exists for whatever field ships and needs the same "must be explicit per item,
  never inherited as a default" scrutiny).
- Audit trail: session state already has `created_at`/`updated_at`; consider whether the
  toggle-after-creation action (touchpoint 4 below) should be logged/attributable (who/when
  flipped it), consistent with this repo's `feedback_document_ai_decisions_in_edge_cases`
  memory precedent of not letting consequential state changes happen silently. This is a
  "should have," not a hard requirement, and belongs in the plan doc as an explicit yes/no
  rather than an assumption.
- This feature is explicitly out of scope for changing "the actual permission-prompt/approval
  logic itself" per requirements.md — so it is strictly a pass-through toggle. That makes the
  above guardrails (confirmation, audit) the *only* safety net stapler-squad can add; it cannot
  make the underlying bypass safer, only make turning it on more deliberate and visible.

## 2. Stale/incorrect flag mapping

Backlog items can be wrong; verified current flag names via web search rather than trusting
`requirements.md`'s claim at face value:

- **Claude Code**: `--dangerously-skip-permissions` — confirmed current and correct. It's
  documented as equivalent to `--permission-mode bypassPermissions` (same mechanism this repo
  already uses for `AutoYes`, see §0). Since v2.1.126 the flag reportedly "skips more than it
  used to," and Claude Code shipped an official middle-ground "auto mode" in March 2026 as an
  alternative — worth a one-line awareness note in the plan doc that the binary flag mapping
  may need revisiting if Anthropic changes semantics again, but no action needed now.
- **Aider**: requirements.md says `--yes`/similar. Verified: the canonical, documented flag is
  **`--yes-always`** (also settable via `AIDER_YES_ALWAYS` env var or `yes-always:` YAML key).
  `--yes` was **renamed** to `--yes-always` in a past Aider release; `--yes` still works today
  only as an accepted abbreviation on the command line, not as the documented/stable flag name.
  **Recommendation: inject `--yes-always`, not `--yes`**, in the per-agent flag map — relying on
  abbreviation-resolution behavior is more fragile than using the documented flag, and a future
  Aider release could tighten abbreviation matching or introduce a colliding flag that starts
  with `--yes`. (Sources: aider.chat/docs/config/options.html, aider.chat/HISTORY.html)

**What happens when the map goes stale** (design-time question, not a bug to fix now): the
per-agent flag lookup table (per requirements.md's Architecture research dimension, "a small
lookup table keyed by detected agent binary") is intrinsically a hardcoded mapping to an
external CLI's flag surface, which the backlog item itself already got half-wrong (`--yes` vs
`--yes-always`). Two failure modes to design against:
- **Silent no-op**: if a future agent CLI renames/removes the flag, stapler-squad keeps
  injecting a now-unrecognized flag; depending on the target CLI's argument parser this either
  errors out at launch (visible, safe-ish) or is silently ignored (session *looks* like
  auto-approve is on per the badge, but isn't — this is the worse failure mode and is exactly
  the kind of "no error is not confirmation" gap called out by this environment's evidence
  standards). There is no runtime verification that the flag actually took effect — the badge
  reflects stored `auto_approve` state, not observed CLI behavior.
- **Unknown program**: `classifyProgram` (`session/instance_tmux.go:63-67`) only distinguishes
  `claudeProgram` vs `plainProgram` (a catch-all for everything else, including Aider today).
  A new agent binary (or a wrapped/aliased command string like `env -u VAR aider`) needs its own
  classification arm or the flag map silently won't match it, exactly as `isClaude`'s existing
  basename-matching-with-alias-guard logic already accounts for on the Claude side (see
  `isClaude`'s doc comment on `env -u VAR claude` / `claude-squad` false-positive avoidance —
  the new Aider matcher should follow the same basename-based, alias-aware pattern rather than
  a naive substring check).

## 3. Toggle-after-creation pitfalls

This is the sharpest pitfall class, and it has a **direct structural analog already documented
in this repo**: `.claude/rules/tmux-keep-server-on-restart.md` records a confirmed incident
where restarting the stapler-squad *service* killed the tmux server and every live session,
including the one running the active Claude Code conversation, with scrollback lost and
sessions silently rebuilt from scratch rather than resumed.

Repo research (`session/instance.go:1509` `Restart(preserveOutput bool)`,
`session/instance_tmux.go:282-289` `KillSession`) confirms the **same risk shape exists one
level down, at session granularity**:
- `Instance.Restart` always calls `KillSession()` — which tears down that session's tmux
  pane/server-side object — before creating a new pane.
- There is a `preserveOutput=true` mitigation: `Restart` captures the pane's scrollback via
  `CapturePaneContentWithOptions("-", "-")` before killing and writes it back into the new pane
  after. This is **capture-and-replay text**, not a resumed process — any in-flight agent turn
  (a command mid-execution, a model response mid-stream, unsaved state the agent process holds
  in memory) is lost even though the *visible transcript* is preserved. This distinction should
  be made explicit in the plan doc's UX copy for the toggle ("takes effect on next restart" —
  the user needs to understand this is a kill-and-recreate, not a graceful flag hot-swap).
- The concrete trigger path an `auto_approve` post-creation toggle would most likely reuse is
  `Instance.SwitchProgram` (`session/instance_program.go:75-79`), which already calls
  `Restart(true)` whenever the session is `Active` and its program string changes — this is the
  existing "flag change → restart" precedent (used for switching between e.g. `claude` and
  `claude --model sonnet`), and it's the natural integration point since the injected flag is
  baked into the same `buildLaunchCommand` string, not a separate live channel.

Specific races/pitfalls to design against:
- **Toggle-while-mid-turn race**: if a user flips `auto_approve` while the agent is actively
  executing a tool call or awaiting its own approval prompt, restarting mid-turn discards that
  in-flight turn. The plan should decide: queue the restart until the agent is idle/between
  turns, warn the user synchronously ("this will interrupt the current turn"), or accept the
  interruption as documented behavior (requirements.md already scopes this as "not a live
  in-place flag swap," so some interruption is accepted by design — the open question is
  whether it's silent or confirmed).
- **Restart is per-session, not per-service** — meaningfully narrower blast radius than the
  documented incident (one pane, not the whole tmux server/every session), but it is the *same
  failure category* (kill-and-recreate instead of in-place mutation) and should not be
  dismissed as "already solved" just because `preserveOutput` exists — `preserveOutput` was
  built for a different original purpose (general session restart) and hasn't necessarily been
  audited for this specific flag-toggle path's requirements (e.g. does a paused session's
  restart-on-resume path also need to check for a pending `auto_approve` change and honor it
  before relaunching, or could a stale flag persist past a restart that didn't go through
  `SwitchProgram`?). This needs verification in the plan/implementation phase, not assumed.
- **UpdateSession's existing update paths are a mix of live and restart-requiring** — per repo
  research, `AutonomousMode` and `RateLimitEnabled` mutate the running `Instance` in place with
  *no* restart, while `Program` requires `SwitchProgram` + conditional `Restart`. An
  `auto_approve` field is architecturally closer to `Program` (baked into the launch command)
  than to `AutonomousMode` (checked live at runtime) — the plan must not accidentally implement
  it as a bare in-memory field flip (copying the `AutonomousMode` pattern) when it actually
  needs the `Program`/restart pattern, since a bare flip would silently do nothing until the
  next *unrelated* restart, contradicting the badge's implied "this is now on."

## 4. Schema pitfalls

- **ent generate flag**: `.claude/rules/ent-schema-generation.md` documents that omitting
  `--feature sql/upsert` "silently breaks `UpsertRule` and similar upsert methods — the
  generated code compiles but the upsert operations don't exist." The correct command (also in
  `session/ent/generate.go`'s `//go:generate` directive) is:
  ```
  go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
  ```
  This is an easy miss because the wrong command *also compiles* — there's no build-time signal
  that the flag was omitted, only a runtime gap in generated upsert methods.
- **Default value**: confirmed every existing bool field in `session/ent/schema/session.go`
  sets an explicit default — `auto_yes` (`.Default(false)`, line 46-47), `autonomous_mode`
  (`.Default(false)`, line 48-50), `is_expanded` (`.Default(true)`, line 59-60), etc. A new
  `auto_approve` column **must** call `.Default(false)` explicitly — without it, ent's SQLite
  migration will not reliably backfill existing rows to a safe value, and the requirements
  doc's own safety constraint ("never a silent default") would be violated at the schema layer
  before any application code even runs if existing sessions ended up with a NULL/zero-value
  ambiguity instead of an explicit `false`.
- **`make proto-gen` after any proto change**: forgetting this after adding the
  `auto_approve`/similar field to `CreateSessionRequest`/`UpdateSessionRequest` leaves the Go
  bindings (`session/gen/session/v1/*.go`) and TS bindings
  (`web-app/src/gen/session/v1/*_pb.ts`) out of sync with the `.proto` source — a common
  failure mode is editing the `.proto` file, writing Go handler code against the *not-yet-
  regenerated* stale bindings, and the build succeeding against the stale generated code with
  no compile error pointing at the real cause.
- **Two-place proto changes**: per the `UpdateSession` precedent (`optional bool
  autonomous_mode = 10;` in `proto/session/v1/session.proto:612`), a post-creation toggle needs
  its own `optional bool` field on `UpdateSessionRequest` *and* a corresponding field on
  `CreateSessionRequest` for creation-time — missing either half only breaks one of the two
  required entry points (creation vs toggle), and only one will be exercised by manual testing
  unless both flows are tested explicitly.

## 5. Registry pitfalls

Two independent registries both gate CI (`.claude/rules/feature-registry.md`,
`.claude/rules/feature-testing-registry.md`); missing either fails CI silently until someone
runs the generator or the PR is flagged in review.

**Feature registry** (`.claude/rules/feature-registry.md`) — files that must be touched:
- New/updated backend entry for the `UpdateSession` RPC change:
  `docs/registry/features/backend/session/update.json` (confirmed to exist already, currently
  `tested: false` — this PR should flip it to `true` with real `testIds` once the toggle has
  Go/e2e test coverage, not create a duplicate file).
- The `CreateSession` RPC entry: `docs/registry/features/backend/session/create.json`
  (confirmed to exist) needs its `lastModified` bumped and `testIds` extended once creation-time
  `auto_approve` is covered by a test.
- New frontend entry for the omnibar checkbox and/or session-card badge, following the pattern
  of the existing `docs/registry/features/frontend/ui/session-create-one-off.json` (one-off
  creation toggle) and `docs/registry/features/autonomous-fix.json` (autonomous badge/mode) —
  these two are the closest direct precedents for a new boolean-session-attribute frontend
  entry.
- `make registry-generate` must be run and the resulting diff to the aggregated
  `docs/registry/backend-features.json` / `docs/registry/frontend-features.json` committed —
  per the rule, editing the aggregated files directly instead of the per-feature source files
  is explicitly wrong.

**Omnibar dual-registry** (`.claude/rules/feature-testing-registry.md`) — **this feature likely
does NOT need either registry**, and that itself is a pitfall to get right rather than wrong in
the other direction (over-registering):
- **OmnibarAction union** (`types.ts`/`dispatch.ts`/`dispatch.test.ts`): only needed if
  `auto_approve` becomes a new *user-triggerable action* distinct from session creation (e.g. a
  standalone "toggle auto-approve" omnibar command). Per requirements.md's Should-Have, the
  toggle is described as "an explicit session action" — if that's implemented as a button/menu
  item on the session card (matching the existing `autonomousBadge` click-to-toggle pattern at
  `web-app/src/components/sessions/SessionCard.tsx:570-615`) rather than an omnibar-dispatched
  action, this registry is **not** triggered. If it *is* surfaced via the omnibar as a new
  action type, the checklist in `.claude/rules/feature-testing-registry.md` applies in full
  (union variant + dispatch case + test).
- **DetectorRegistry**: not applicable — there is no new auto-detected input pattern (URL,
  shorthand, etc.) here; this is a checkbox/flag, not something a user would type as freeform
  omnibar input. Per that rule's own decision tree, a plain boolean toggle correctly falls into
  the "None of the above → may only need OmnibarCreationPanel + form state changes" branch, not
  either registry.
- The one registry decision genuinely required either way is `.claude/rules/
  session-creation-registry.md`, and requirements.md already explicitly scopes it **out**
  ("this is a new session *attribute*, not a new session *creation mode*") — correct per the
  `autonomous_mode`/`one_off` precedent of reusing the existing `SessionType` rather than adding
  an enum value. Re-confirm this decision doesn't quietly flip during planning (e.g. if
  someone decides `auto_approve` needs its own `SessionType` for some reason) — if it does,
  all 7 touchpoints in that rule would apply and the scope estimate in requirements.md would be
  wrong.

---

## Sources (web search, flag-name verification)

- [claude --dangerously-skip-permissions (2026): What It Does, 5 Safer Setups & the New Auto Mode](https://www.morphllm.com/claude-code-dangerously-skip-permissions)
- [Claude Code --dangerously-skip-permissions: What It Does and When Not to Use It](https://www.truefoundry.com/blog/claude-code-dangerously-skip-permissions)
- [There's No "--dangerously-skip-permissions" for Your Data](https://www.kiteworks.com/cybersecurity-risk-management/ai-data-governance-dangerously-skip-permissions-risk/)
- [Claude Code --dangerously-skip-permissions: Safe Usage Guide + Configs](https://www.ksred.com/claude-code-dangerously-skip-permissions-when-to-use-it-and-when-you-absolutely-shouldnt/)
- [Options reference | aider](https://aider.chat/docs/config/options.html)
- [Release history | aider](https://aider.chat/HISTORY.html)
