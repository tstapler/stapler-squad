# Validation: AC -> test mapping
- AC0/1: TestReviewGate_ConcurrentTriggers_SingleSpawn (-race)
- AC2/3: TestReviewGate_ReservationHeldUntilItemSessionPersisted; _ReleasedOnSpawnError
- AC4: TestTriggerReviewForSession_NoopWhenReviewOpenOrNotInReview
- AC5: TestAutoRespawnReview_BlockedByInFlightListenerReservation
- AC6: TestReviewGate_ReReviewAllowedAfterPriorEnded
- AC7: make ready
Pre-mortem: (a) leaked reservation -> TTL + defer tests; (b) guard in wrong layer misses headless path -> AC5 test; (c) repro may show reconcile path already safe -> still fix the TriggerReviewForSession/onSessionExited gaps, record result in PR.
