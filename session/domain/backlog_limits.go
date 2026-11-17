package domain

import "time"

// RemediationBackoffSchedule holds one gap per automated stuck-item remediation
// attempt; session.remediationBackoffSchedule documents the semantics. Lowered so
// the backlogstore leaf can read MaxRemediationAttempts without importing session.
var RemediationBackoffSchedule = []time.Duration{
	30 * time.Minute,
	2 * time.Hour,
	8 * time.Hour,
	24 * time.Hour,
	72 * time.Hour,
}

// MaxRemediationAttempts is len(RemediationBackoffSchedule); a var because len of a
// slice literal is not a compile-time constant.
var MaxRemediationAttempts = int32(len(RemediationBackoffSchedule))

// ItemSessionBatchChunk keeps IN (...) lists well under SQLite's bound-variable limit.
const ItemSessionBatchChunk = 500
