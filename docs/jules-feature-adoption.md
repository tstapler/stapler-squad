# Jules Feature Adoption — Candidate List

Analysis of Google Jules' design against stapler-squad's backlog/session pipeline
(`BacklogItem`, `ItemSession`, `BacklogStatusEvent`, `WorktreePRPoller`). Jules and
stapler-squad solve the same problem — supervising an AI coding agent through a
plan → execute → review loop — so most of what makes Jules good already has a
home in an existing primitive here. The list below is what's worth building next,
ranked by leverage, extending existing services rather than inventing parallel ones.

## Already shipped

- **Plan-before-execute gate.** `BacklogItem.plan_approved` / `plan_artifacts_path`,
  `TransitionGuard` (`session/backlog.go`), `BacklogService.ApprovePlan`, and the
  `GateVerdictBox`/`Approve Plan` UI in `BacklogItemDetail.tsx` already implement
  this end-to-end — a `ready` item can't spawn a session until its plan is approved
  or `skip_planning` is set. Verified via existing Go tests
  (`TestApprovePlan_*`, `TestTransitionGuard_ReadyToInProgress_*`) and a new e2e
  test (`tests/e2e/plan-gate.spec.ts`) that exercises the gate/skip path in the UI.
  The backend/frontend registry entries were out of date (`tested: false` despite
  passing tests, no e2e coverage) and have been corrected as part of this item.

## Candidates for future backlog items

| # | Feature | Extends | Notes |
|---|---|---|---|
| 1 | PR-summary card | `WorktreePRPoller`, `github/client.go` | One JSON column on `ItemSession` (`pr_summary`), following the `ac_snapshot`/`triage_result` pattern. Confirm poller already exposes diff-stat/check-status data before scoping the field shape. Must handle staleness (PR closed/rebased after snapshot). |
| 2 | Activity/progress feed | `backlog_service.go` | One new read RPC joining `BacklogItem` + `ItemSession` + `BacklogStatusEvent`, ordered by timestamp. Needs pagination — items can accumulate hundreds of status events. |
| 3 | Auto-fix-on-CI-failure loop | `WorktreePRPoller`/`PRStatusPoller` | Re-invoke the session's agent via `steer_session`/`run_command` on a failed check. Must only fire when `plan_approved=true` (no bypassing an unapproved plan), and needs a max-retry/backoff so a flaky check doesn't loop forever burning API quota. |
| 4 | `GitHubIssueDetector` (omnibar) | existing `Detector` registry | Same shape as `GitHubPRDetector`, priority ~15 (between PR at 10 and Branch at 20). Must be benchmarked against all existing detectors, not assumed safe — detection ≠ validation, so malformed/private-repo issue URLs should still match the shape and fail gracefully downstream. |
| 5 | Max-concurrent-sessions throttle | session spawn path | Prerequisite guardrail before any "queue N tasks" UX ships — local parallel execution has no VM ceiling and will contend for CPU/RAM/disk/API rate limits the way Jules' per-task cloud VM doesn't. A `plan_pending` session must not count against the running-slot limit until approved. |

## Explicitly deferred / rejected

- **Container-per-session (Docker/Podman) sandboxing, à la Jules' per-task cloud VM.**
  Not scoped as an ADR. No concrete local-resource-contention problem is documented
  yet — item 5 below (concurrency throttle) is the cheaper fix for the contention
  that *is* observed today. Only write a container-per-session ADR if the throttle
  proves insufficient and a specific, measured contention incident is on record;
  until then this stays a rejected idea, not a queued one.
- **Audio changelog** — no TTS infra, low ROI. Deferred entirely pending explicit
  product sign-off; not scheduled as a backlog item.

## Sequencing notes

- PR-summary card and the activity feed are independent and can start any time.
- The auto-fix loop depends on the plan gate's semantics (now shipped) so re-invocation never bypasses an unapproved plan.
- The concurrency throttle should land before any bulk/queue dispatch entry point is added to the activity feed.
