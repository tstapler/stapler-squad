// Package a contains nolegacylog test fixtures. This file's basename
// ("backlog_lifecycle.go") matches an entry in protectedFiles, so legacy
// Printf calls here must be flagged.
package a

import "github.com/tstapler/stapler-squad/log"

// BAD1: a legacy WarningLog().Printf call in a protected file.
func bad1(err error) {
	log.WarningLog().Printf("[BacklogLifecycle] DequeueNextQueuedItems error: %v", err) // want `legacy log\.<Level>Log\(\)\.Printf`
}

// BAD2: same violation via InfoLog.
func bad2(itemID string) {
	log.InfoLog().Printf("[BacklogLifecycle] item %s transitioned", itemID) // want `legacy log\.<Level>Log\(\)\.Printf`
}

// BAD3: same violation via ErrorLog.
func bad3(itemID string, err error) {
	log.ErrorLog().Printf("[BacklogLifecycle] GetItemSessionBySessionUUID(%s) error: %v", itemID, err) // want `legacy log\.<Level>Log\(\)\.Printf`
}

// GOOD1: the modern structured API is never flagged, even in a protected file.
func good1(itemID string, err error) {
	log.Warn("[BacklogLifecycle] DequeueNextQueuedItems error", "item", itemID, "error", err)
}

// GOOD2: a //nolint comment on the same line suppresses the finding.
func good2(err error) {
	log.WarningLog().Printf("[BacklogLifecycle] DequeueNextQueuedItems error: %v", err) //nolint:nolegacylog test fixture, not a real call
}
