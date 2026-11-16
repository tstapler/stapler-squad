package domain

// JulesDispatchFailedEndReason marks a jules_work ItemSession that was created
// as a reservation but never reached a real Jules session — the CreateSession
// call itself failed. Excluded from CountJulesItemSessionsSince alongside
// domain.JulesPendingUUIDPrefix rows so a bad key or Jules outage doesn't burn the
// daily dispatch cap on attempts nothing was ever billed for.
const JulesDispatchFailedEndReason = "dispatch_failed"
