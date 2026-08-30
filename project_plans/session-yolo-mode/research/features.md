# Research: Features — Prior Art for `auto_approve`/Yolo Mode

## Headline finding: this feature already exists — for Claude only, creation-time only, no badge

Before tracing `autonomous_mode`/`one_off` as templates, a closer precedent turned up that changes
the shape of this work: **`auto_yes` is already a fully-wired boolean field**, end to end, from the
Omnibar checkbox down to the Claude Code launch command. It is Claude-only, has no session-card
badge, and has no post-creation toggle. This is not a build-from-scratch feature — it's an
extend-and-finish of `auto_yes`, or a deliberate rename/superset of it into `auto_approve`. Either
way, the plan phase needs to decide explicitly whether `auto_approve` **is** `auto_yes` (rename +
extend) or a **new, second field** that composes with it — leaving both as separate booleans
invites exactly the kind of duplicate-purpose confusion this doc found between `auto_yes` and
`permission_mode` (see "Existing overlapping mechanisms" below).

### `auto_yes` end-to-end trace (the closest real precedent, closer than `autonomous_mode`)

| Hop | File:Line | What it does |
|---|---|---|
| Proto request field | `proto/session/v1/session.proto:496-497` | `bool auto_yes = 8;` — "Optional: Auto-approve prompts without user interaction." |
| Go service — read from request | `server/services/session_service.go:1376` | `autoYes := req.Msg.AutoYes` |
| Go service — profile-default merge guard | `server/services/session_service.go:1392,1416` | `if !autoYes && resolved.AutoYes` — only overrides from a saved profile's default when the request itself didn't explicitly set it |
| Go service — passed into instance opts | `server/services/session_service.go:1489` | `AutoYes: autoYes,` in the `session.InstanceOptions` literal |
| Go batch path | `server/services/session_service.go:3567` | `AutoYes: batchReq.AutoYes,` (workflow/batch creation also threads it) |
| Instance field | `session/instance.go:128-129, 465-466` | `AutoYes bool` on both `Instance` and `InstanceOptions` — "true if the instance should automatically press enter when prompted" |
| Instance construction | `session/instance.go:604` | `AutoYes: opts.AutoYes,` |
| Persistence (storage.go) | `session/storage.go:30` | `AutoYes bool \`json:"auto_yes"\`` — persisted to `sessions.json` |
| Snapshot/checkpoint | `session/instance_snapshot.go:92,158`, `session/instance_checkpoint.go:201` | Round-trips through session restore/checkpoint |
| **Command injection (Claude only)** | `session/instance_tmux.go:154-156` | `if i.AutoYes { parts = append(parts, "--permission-mode", PermissionModeBypassPermissions) }` inside `buildClaudeCommand` |
| Frontend type | `web-app/src/components/sessions/Omnibar.tsx:68,92,122` | `autoYes: boolean` in `OmnibarFormState`, default `false` |
| Frontend alias support | `web-app/src/components/sessions/Omnibar.tsx:524` | `setFormField("autoYes", alias.autoYes)` — saved aliases carry an `autoYes` default |
| Frontend submit | `web-app/src/components/sessions/Omnibar.tsx:1052,1100,1118,1180` | Threaded through submit paths (batch/day-to-day + wizard) |
| Frontend UI checkbox | `web-app/src/components/sessions/OmnibarCreationPanel.tsx:802-810` | Rendered checkbox, label **"Auto-approve prompts (experimental)"** |
| Context passthrough | `web-app/src/lib/contexts/OmnibarContext.tsx:222` | `autoYes: data.autoYes,` passed to `createSession` call |
| RPC call body | `web-app/src/lib/hooks/useSessionService.ts:272` | `autoYes: request.autoYes,` in the ConnectRPC request |

**Gaps vs. the requirements doc, precisely:**
1. **Claude-only.** `buildLaunchCommand` (`session/instance_tmux.go:105-119`) classifies `i.Program`
   into a sealed `programKind` (`claudeProgram` vs `plainProgram`, lines 50-67); `AutoYes` injection
   lives *inside* `buildClaudeCommand`, called only for `claudeProgram`. Aider (and anything else)
   is `plainProgram` and gets **zero** flag injection today — `AutoYes=true` with Aider selected is
   a silent no-op. This is exactly the "unsupported agent" edge case the task asks about, and it
   already happens in production for `auto_yes`, today, for any non-Claude program.
2. **No session-card badge.** `SessionCard.tsx` has badges for autonomous mode, workflow, and
   pending-program-change (see below) but nothing renders for `session.autoYes`.
3. **No post-creation toggle.** `UpdateSessionRequest` (`proto/session/v1/session.proto:579-617`)
   has no `auto_yes`/`auto_approve` field at all — see `UpdateSession` section below.
4. **Labeled "(experimental)".** Signals it was soft-launched and not considered a finished/promoted
   feature — worth surfacing to the user during planning: is this PR meant to graduate `auto_yes`
   out of experimental, or build something adjacent?

## 1. `autonomous_mode` end-to-end trace (secondary template, for the "flag on existing type" pattern itself)

| Hop | File:Line |
|---|---|
| Session-state proto fields | `proto/session/v1/types.proto:203,215-220` — `bool autonomous_mode = 60;` plus `autonomous_turn`, `autonomous_max_turns`, `autonomous_outcome` |
| Create request field | `proto/session/v1/session.proto:557` — `bool autonomous_mode = 23;` |
| Update request field | `proto/session/v1/session.proto:612` — `optional bool autonomous_mode = 10;` (note: **`optional`**, unlike the plain `bool` on create — this is the pattern to copy for the post-creation toggle) |
| Go handler — creation gating | `server/services/session_service.go:1263,1358-1361` — path-required guard skips path validation when `AutonomousMode` and no path given |
| Go handler — instance construction | `server/services/session_service.go:1503` — `AutonomousMode: req.Msg.AutonomousMode,` |
| Go handler — driver start on create | `server/services/session_service.go:1625-1634` — starts `AutonomousDriver` if `headlessPool != nil`, else logs a warning (does **not** fail the request) |
| Go handler — `UpdateSession` toggle | `server/services/session_service.go:1802-1815` — the exact template for "toggle after creation": guarded by `req.Msg.AutonomousMode != nil && *req.Msg.AutonomousMode != instance.AutonomousMode`, precondition-checks a dependency (`headlessPool`), calls `instance.SetAutonomousMode(...)`, starts/stops a live side effect, appends to `updatedFields` |
| Frontend `sessionTypeMap`-equivalent | N/A — `autonomous_mode` is a flag, not a `sessionType`, so it isn't in `OmnibarContext.tsx`'s `sessionTypeMap`; it's passed as a plain boolean field like `autoYes` is |
| Frontend badge | `web-app/src/components/sessions/SessionCard.tsx:570-615` — three badge states (`badge-autonomous`, `badge-autonomous-done`, `badge-autonomous-stuck`), including a **clickable badge that itself toggles the flag off** via `onToggleAutonomousMode` (line 571-580) — a stronger UX than a static badge, worth considering for `auto_approve` too (click badge → open confirm → toggle off) |

## 2. `one_off` — now folded into the `SessionType` enum, not a flag (partially superseded)

The requirements doc's characterization of `one_off` as "flag on existing type" is **stale**:
`session.proto:516-519` shows the old `bool one_off = 14` field is now `reserved 14; reserved
"one_off";` with a comment `// Deprecated: use session_type = SESSION_TYPE_ONE_OFF instead.` It has
since become its own `SessionType` enum value: `types.proto:365` `SESSION_TYPE_ONE_OFF = 5;`, and
`server/services/session_service.go:1448,1649,1663` (`session.SessionTypeOneOff`) confirm it's
handled as a real session type today, not a boolean flag. This doesn't change the plan for
`auto_approve` (which correctly should stay a flag, not a type — it composes with every session
type, unlike one-off's distinct lifecycle), but the requirements doc's citation of `one_off` lines
~510-615 as a "flag" reference is outdated and should not be copied literally; use the `AutonomousMode` `UpdateSession` block (1802-1815) and the `auto_yes` create-path trace above instead.

## 3. `UpdateSession` — current fields and what extending it costs

`UpdateSessionRequest` (`proto/session/v1/session.proto:579-617`) currently supports:
`status`, `category`, `title`, `program`, `tags`, `working_dir`, `rate_limit_enabled`,
`pause_reason`, `autonomous_mode`, `steer_message` — all `optional` scalar fields, each handled by
its own `if req.Msg.X != nil { ... }` block in the `UpdateSession` handler
(`server/services/session_service.go`, ~1780-1830+).

Extending it for `auto_approve` is low-cost and directly mechanical:
1. Add `optional bool auto_approve = 12;` (next free field number) to `UpdateSessionRequest`.
2. Add a handler block following the `autonomous_mode` block's exact shape (nil-check, no-op if
   unchanged, `instance.SetAutoApprove(...)`, append to `updatedFields`) — no live side effect is
   needed (unlike autonomous mode's driver start/stop) since this only affects the *next* launch
   command, per the requirements doc's explicit scope ("takes effect on next launch/restart, not a
   live in-place flag swap").
3. `make proto-gen` + regenerate TS bindings.

This is genuinely cheap — the registry/pattern is already proven three times over (`status`,
`autonomous_mode`, `rate_limit_enabled` all show the same shape). No new plumbing paradigm needed.

## 4. Edge cases

### Unsupported agent (flag has no mapping for the detected program)

Already happens today, silently, for `auto_yes` + Aider (see gap #1 above) — `classifyProgram`
(`session/instance_tmux.go:63-67`) only distinguishes Claude vs. "everything else"; Aider gets no
flag treatment of any kind. Requirements explicitly scope Claude + Aider only (not "extensible to
others" yet), but the mapping mechanism should not silently no-op for a third, unmapped agent in
the future — `.claude/rules/interface-pollution-checklist.md`'s guidance ("concrete type over
interface until 2+ implementations exist") suggests a small `switch` on `classifyProgram`'s kind (or
a lookup table keyed by kind) rather than a new interface, but the `default` arm of that switch
should be a deliberate, visible no-op — a code comment, not silence — and ideally the UI should
gray out / hint-disable the auto-approve toggle when the detected program isn't Claude or Aider, so
the user isn't offered a checkbox that does nothing. Nothing currently surfaces "this flag has no
effect for this program" anywhere in the UI — this needs a decision in the plan phase, not
inference.

### Toggling on a running session — restart-required messaging

There **is** an existing, directly-reusable UI pattern for exactly this: `hasPendingProgramChange`
(`web-app/src/components/sessions/SessionCard.tsx:22-30`) plus its badge
(`SessionCard.tsx:627-634`, `data-testid="badge-pending-program"`). Mechanism: it compares the
session's persisted `program` field against `session.launchCommand` (the command string the session
was *actually* launched with, which always starts with the program string used at launch time —
per the comment at `SessionCard.tsx:17-21`); if `program` changed since last launch and the session
is `PAUSED`/`STOPPED`, it shows a badge reading "Program was changed since this session last
launched — takes effect on resume/restart."

This predicate as written won't automatically catch an `auto_approve` change, because toggling
`auto_approve` doesn't change `session.program` — it changes a separate field that also feeds
`buildLaunchCommand`. Two options for the plan phase:
- Extend `hasPendingProgramChange` (or add a sibling predicate) to also compare whether
  `launchCommand` contains the flag implied by the *current* `auto_approve`/`AutoYes` value (e.g.
  `--permission-mode bypassPermissions` presence) vs. what's stored — same shape, one more
  comparison.
- Reuse the exact same badge component/copy, parameterized ("Auto-approve setting changed — takes
  effect on resume/restart"), so there's a single generic "pending relaunch-only field" concept
  instead of two near-identical bespoke ones. This is the DRYer option and matches the "flag on
  existing type" spirit of not introducing new abstractions for one more boolean.

### Overlapping existing mechanisms (found during this trace, not asked for but directly relevant)

Two other fields already do adjacent things and were not mentioned in the requirements doc:
- **`permission_mode`** (`session.proto:551-553`, values `default`/`acceptEdits`/`bypassPermissions`/`auto`) —
  a free-form string, independently settable, that can already express `bypassPermissions` (the
  same value `AutoYes` injects). `session/instance_tmux.go:151-156` applies **both** `PermissionMode`
  (if set) and then unconditionally *also* appends `--permission-mode bypassPermissions` again if
  `AutoYes` is true — i.e., a session can already end up with `--permission-mode` passed twice on
  the command line if both are set. `session/backlog_review.go:419` sets `PermissionMode:
  PermissionModeBypassPermissions` directly for headless backlog-review sessions, bypassing
  `AutoYes` entirely.
- **`allowed_tools`** (`session.proto:547-549`) — pre-approves specific tools without full bypass;
  a narrower, safer alternative already exists in the product and is used by backlog automation
  (`session/backlog_review.go:418`, `headless.CodebaseReadAllowedTools`).
- `docs/gap-analysis.md:498` independently flags that `allowed_tools`/`permission_mode` are **not**
  exposed in the workflow/scheduler path (`WorkflowProto`, workflow create/update, `FireNow`) at
  all, and that the only current workaround for unattended cron workflows needing no-prompt
  behavior is embedding `--dangerously-skip-permissions` directly into the `agent_type`/`program`
  string — "undiscoverable and not validated." If `auto_approve` is meant to be the one clear
  first-class way to express "no prompts," the plan should decide whether it also needs to reach
  the workflow/scheduler creation path, not just interactive Omnibar creation — this is currently
  unstated in the requirements' scope but is a real adjacent gap the codebase already documents.

**Recommendation for the plan phase:** decide explicitly whether `auto_approve` (a) *is* a rename/
promotion of `auto_yes` out of "(experimental)," (b) is a new field that supersedes `AutoYes` and
`PermissionMode`'s bypass value into one canonical toggle, or (c) is additive and coexists — option
(c) revives the double-flag command-line bug already latent in today's code (both `PermissionMode`
and `AutoYes` independently appending `--permission-mode`). This determination changes several
"Must Have" items (e.g., whether the Omnibar checkbox is new UI or a relabel of the existing one).

## 5. Unstated needs

### Visibility/filterability in session list/search
`ListSessionsRequest` (`session.proto:428-454`) filters on `status`, `category`, `hide_paused`,
`search_query` (fuzzy title/path/branch), `project_id`, `include_hidden`, `workflow_id`,
`include_archived` — **no** filter by any permission/auto-approve field today, and no precedent
field like `autonomous_mode` has a list filter either (autonomous sessions are only distinguished by
the card badge, not a list filter). Given that precedent, `auto_approve` likely doesn't *need* a
dedicated list filter for parity, but given the feature is explicitly framed as a safety-relevant
"this session runs unguarded" signal (per the Problem Statement), a user scanning many sessions to
find which ones are unguarded may want more than a badge — worth flagging as a nice-to-have, not
inferring it into scope silently.

### Backlog automation defaults
No hits for `AutoYes` anywhere in `session/backlog*.go` — backlog/review-queue automation does
**not** set `AutoYes` today. It instead sets `PermissionMode: PermissionModeBypassPermissions`
directly for headless backlog-review sessions (`session/backlog_review.go:419`) and
`AllowedTools: headless.CodebaseReadAllowedTools` for narrower cases (`:418`). So backlog-driven
sessions are already effectively "auto-approved" via the parallel `PermissionMode` mechanism, just
not through the field this feature is adding a UI for. This matters for the "badge on session card"
requirement: if `auto_approve`/badge is keyed only off the new field (or off `AutoYes`), backlog
review sessions running with `PermissionMode: bypassPermissions` directly would **not** show the
"unguarded" badge even though they functionally are unguarded — a visibility gap worth naming
explicitly in the plan rather than discovering after ship. Whether backlog sessions should surface
the badge is a product decision (the Problem Statement's stakeholder is "User (Self)," so this may
be intentional — backlog automation is expected to run unattended by design) but the mismatch
between "badge reflects `auto_approve` field" and "actual unguarded-ness reflects `PermissionMode`
too" should be a named, deliberate scope decision, not an oversight.
