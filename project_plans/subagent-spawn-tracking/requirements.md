# Requirements: subagent-spawn-tracking

Source: backlog item `200b9070-2483-4a97-91a7-f759422357e0`, migrated from
`TylerStaplerAtFanatics/stapler-squad#183`. Re-triaged as backlog item
`9209b4b9-a561-49df-a6b1-2d8d7d22dd0c` (same GitHub issue #183, same
requirements — no content changes needed, this run continues the existing
plan straight into the plan/validate phases below).

## Problem

When Claude Code spawns subagents (via the `Task` tool) or background
shells/monitors, the parent session's card in the stapler-squad UI shows a
generic PROCESSING/WAITING_FOR_AGENT state. Users cannot see *how many*
concurrent subtasks are running, or drill into what those subtasks are
doing, so multi-agent orchestration sessions look indistinguishable from
single-threaded work.

## Existing ground truth (found during triage — read before designing)

This is NOT a greenfield feature. The detection package already has a
partial version of this:

- `session/detection/detector.go` defines `StatusWaitingForAgent` with two
  regex patterns that already contain a captureable count:
  - `waiting_for_background_agent`: matches Claude Code's own
    `✻ Waiting for N background agent(s) to finish` / `N dynamic
    workflow(s) to finish` lines.
  - `shells_still_running` / `monitors_still_running`: match `N shell(s)
    still running` / `N monitor(s) still running` in the turn-completion
    status bar.
  - Today these patterns only classify the *status enum* — the `N` is
    matched by `\d+` but never captured or surfaced anywhere.
- Detection in this codebase is 100% terminal-scrollback regex matching
  (`session/detection/pattern_set.go`, `detector.go`). There is **no**
  existing JSONL-parsing pipeline that reads Claude Code's
  `~/.claude/projects/.../*.jsonl` transcript files. The original issue's
  "parse Claude Code JSONL events for `tool_use` blocks" proposal is a new
  architectural capability, not an extension of an existing one — a second,
  independent detection source living alongside the terminal-scraping path.
- Status flows: `detector.go` → `DetectedStatus` (Go iota) →
  `proto_mapping.go` (`DetectedStatusToProto`, `DetectedStatusToSubStatus`)
  → `sessionv1.DetectedStatus` / `sessionv1.SubStatus` proto enums →
  `session/instance_status.go` (`InstanceStatusInfo`) → web UI. Any new
  count field needs to ride this same pipeline (a plain `int`, not a new
  enum) to reach the frontend.
- `session/instance_status.go`'s `InstanceStatusManager` already uses
  `xsync.Map` for lock-free reads on the hot `GetStatus` path — a new
  per-instance mutable counter must respect that concurrency pattern, not
  reintroduce a mutex.

## Scope decision (ponytail / YAGNI applied)

Given the above, the lazy-correct scope for a first version is: **capture
the count that the existing regex already matches** and thread it through
the existing pipeline, rather than building a new JSONL ingestion system.
JSONL-based per-subagent descriptions (the "tooltip: list of active
subagent descriptions" stretch goal) is real new scope and is called out
separately below so it can be explicitly deferred or accepted.

## In scope (V1)

1. Extend the `waiting_for_background_agent`, `shells_still_running`, and
   `monitors_still_running` regexes (or add a parallel capturing variant)
   to capture the numeric count, not just match `\d+`.
2. Add an integer count field (e.g. `AgentCount` / `SubagentCount`) to the
   detection result path (`DetectedStatus` result struct /
   `DetectionEvent` — whichever already exists — must be checked in
   research, not assumed) so the parsed count survives from regex match to
   `InstanceStatusInfo`.
3. Add `subagent_count` (int32) to `SessionStatus`/the relevant proto
   message in `proto/session/v1/types.proto`, regenerate via
   `make proto-gen`.
4. Reset the count to 0 when status transitions away from
   `StatusWaitingForAgent` (turn completes / goes idle) — mirror however
   the codebase already resets other transient per-turn state, don't
   invent a new mechanism.
5. Session card in the web UI: a small badge (e.g. `⊕ 3 tasks`) rendered
   only when `subagent_count > 0`.

## Out of scope / explicitly deferred (flag as open questions, don't build)

- Parsing Claude Code JSONL transcripts for `tool_use`/`Task` blocks to get
  per-subagent names/descriptions. This is a materially larger effort (new
  file-watching + JSONL schema parsing subsystem) and should be its own
  backlog item if wanted.
- Tooltip/expandable list of active subagent descriptions (depends on the
  JSONL work above).
- Aggregate WORKING state across parent + all subagents (depends on
  per-subagent tracking, which doesn't exist without JSONL parsing).
- The maki `AgentEvent`/`Envelope` architecture (Rust project, different
  codebase) is a pattern reference only, not something to port 1:1 — this
  repo's detection model is regex-over-terminal-text, not a structured
  event bus.

## Acceptance criteria (draft — refined further in plan/validate)

1. When Claude Code's terminal output shows "Waiting for N background
   agent(s)" or "N shell(s)/monitor(s) still running", the parsed count N
   is captured and available on the session's detection/status result
   (not just the boolean/enum classification).
2. The count reaches the frontend via proto (`subagent_count` field),
   regenerated with `make proto-gen`.
3. The session card shows a `⊕ N tasks`-style badge only when count > 0;
   no badge when count is 0 or status is not `WAITING_FOR_AGENT`.
4. Count resets to 0 once the session leaves `WAITING_FOR_AGENT` (turn
   completes, goes idle, errors, etc.) — no stale counts persist into the
   next turn.
5. Existing detection tests (`detector_test.go`, `pattern_set_test.go`,
   `proto_mapping_test.go`, `snapshot_test.go`) continue to pass, and new
   tests cover count extraction (including 0/1/N and pattern-not-matched
   cases).
6. No new external dependency added; no JSONL file-watching subsystem
   introduced in this version.

## Risk

Low — this is additive, display-only, and reuses an existing regex/pipeline
rather than introducing a new detection source. Miscounting is cosmetic,
not functionally harmful (matches the original issue's own risk rating).
