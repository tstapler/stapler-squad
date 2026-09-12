// Package telemetry provides OpenTelemetry instrumentation for stapler-squad.
package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
)

// Semantic attribute keys for stapler-squad operations
const (
	// Session attributes
	AttrSessionID       = "session.id"
	AttrSessionTitle    = "session.title"
	AttrSessionStatus   = "session.status"
	AttrSessionProgram  = "session.program"
	AttrSessionCategory = "session.category"

	// History attributes
	AttrHistoryProject      = "history.project"
	AttrHistorySessionID    = "history.session_id"
	AttrHistoryEntryCount   = "history.entry_count"
	AttrHistoryMessageCount = "history.message_count"

	// Search attributes
	AttrSearchQuery       = "search.query"
	AttrSearchResultCount = "search.result_count"
	AttrSearchDurationMs  = "search.duration_ms"
	AttrSearchIndexSize   = "search.index_size"

	// Storage attributes
	AttrStorageOperation  = "storage.operation"
	AttrStorageCount      = "storage.count"
	AttrStorageDurationMs = "storage.duration_ms"

	// Database attributes (SQLite)
	AttrDBOperation = "db.operation"
	AttrDBTable     = "db.table"
	AttrDBRowCount  = "db.row_count"

	// Review queue attributes
	AttrReviewQueueSize     = "review_queue.size"
	AttrReviewQueuePriority = "review_queue.priority"
	AttrReviewQueueReason   = "review_queue.reason"

	// Subprocess attributes (executor/safeexec instrumentation)
	AttrSubprocessCommand  = "subprocess.command"
	AttrSubprocessArgCount = "subprocess.arg_count"
)

// SubprocessCommandAttr creates an attribute for the subprocess command name
func SubprocessCommandAttr(cmd string) attribute.KeyValue {
	return attribute.String(AttrSubprocessCommand, cmd)
}

// SubprocessArgCountAttr creates an attribute for the subprocess argument count
func SubprocessArgCountAttr(n int) attribute.KeyValue {
	return attribute.Int(AttrSubprocessArgCount, n)
}
