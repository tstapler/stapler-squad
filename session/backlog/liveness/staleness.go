package liveness

import "time"

// MaxHeadlessTriageSessionStaleness bounds how long an open headless-triage session is
// trusted before reconcileOrphanedTriageItems flags it as orphaned. MUST stay strictly
// greater than server/services.triageCallBudget (the real per-call LLM budget, currently
// 3h — raised 2026-09-08 alongside headless.idleTimeout, which is now the primary defense
// against a hung call; see triageCallBudget's own doc comment) with real margin —
// confirmed live 2026-08-01 (BUG-055) that headless triage calls routinely run right up
// to their full call budget (27m41s, 27m53s, 30m38s observed against the old 30m budget
// in one incident), not the 7-15 minutes this constant was originally tuned against. At a
// value equal to or below triageCallBudget, this sweep's periodic tick races the call's
// own natural completion/timeout on every slow call. IsTriageLive (checked by the shape-1
// branch below) is the structural fix for that race — this margin is a defense-in-depth
// belt-and-suspenders measure on top of it, not a substitute: even with a real liveness
// check, there's no reason to court the race in the first place when a full call is still
// plausibly finishing.
const MaxHeadlessTriageSessionStaleness = 3*time.Hour + 15*time.Minute

// MaxWorkSessionStaleness is the longest an in_progress work session can go without
// reporting progress before ReconcileStuck flags it as stale. Mirrors the order of
// magnitude of maxTriageSessionAge (server/services/backlog_service_triage.go).
const MaxWorkSessionStaleness = 2 * time.Hour
