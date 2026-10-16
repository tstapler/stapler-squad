# Plan: fix-review-gate-duplicate-spawn

1. Repro test (session pkg): fake sessionCreator with blocking SpawnReviewSession; fire onSessionExited-path spawn + TriggerReviewForSession + reconcile re-spawn concurrently for one item; assert spawn count (expect 2+ today). Run with -race.
2. Add `reviewReservations` (mutex + map) to `BacklogLifecycleListener`; `tryReserveReview(ctx,itemID) (release func(), ok bool)`: under lock, return !ok if reserved OR storage shows open review ItemSession; else reserve.
3. Wrap `spawnReviewGate` body: reserve -> defer release -> `runner.Run`. Release only after `CreateItemSession` has returned (Run is synchronous, so deferring in spawnReviewGate suffices).
4. `TriggerReviewForSession`: add status==review check (and keep SkipReviewGate); the reservation handles the rest. Do the reservation before taking `reviewSem` so skipped duplicates do not consume slots.
5. Expose `ReviewInFlight(itemID) bool` / reserve API on the listener; have `AutoRespawnReview` and `TriggerReReview` (server/services) consult it via an interface injected like `ReviewRespawner`. Reserve for the full headless run.
6. Panic/error safety: release via defer; test release on SpawnReviewSession error and on early returns (empty diff, security block).
7. Tests: concurrent-trigger dedup, release-after-failure allows retry, post-ended-review allows re-review, headless-vs-interactive exclusion, reconcile-tick-during-window.
8. Docs: short note in docs/reference/backlog-completion-gate-and-cleanup.md; run `make ready`.

## Adversarial review notes
- Risk: reservation leak blocks reviews forever -> mitigate with defer + test; optionally TTL (e.g. 30m) on map entries.
- Risk: lock held across slow storage call -> do DB read inside lock only briefly; keep spawn outside lock (map entry is the reservation).
- Risk: legit re-review after FAIL+rework: DB check must test EndedAt==nil, not any review row (FindReviewItemsWithoutGate's "any row" semantics unchanged).
- Risk: dupl/gocognit gates on modified functions: keep helper small.
- Out of scope: cross-process dedup, DB unique index (follow-up).
