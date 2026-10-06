# Pitfalls
- Skipping must apply only to *automatic* triggers; manual TriggerTriage / feedback refine must always run (operators rely on it).
- A skipped item stays `idea`: nothing advances it to ready. Skip path must leave it in a state the stuck-detector/reconciler does not re-fire forever (reconcileOrphanedTriageItems only acts on items with a prior triage ItemSession, so a never-triaged skipped item is not re-fired; AutoRespawnTriage skip must not loop).
  Mitigation: skip only advances nothing; note explains "set status/approve manually". Items with AC but no plan: skip triage → operator can still trigger manually or spawn work (check SkipPlanning semantics in plan phase).
- Activity note spam: dedupe by writing the note once per skip decision (MaybeTriggerTriage runs once per create; AutoRespawn checks hash first).
- Semaphore resize: channel capacity fixed at construction; read config at NewBacklogService; clamp to [1,64].
- Pinned pipeline-mode model must still win over the config default (ComputeExecutorHash uses RAW executor model; do not hash the fallback).
- Hash must be stable: sha256 over title\x00description\x00AC text only (not status/notes, which triage itself changes).
