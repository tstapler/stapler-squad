// Package telemetry provides OpenTelemetry instrumentation for stapler-squad.
package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
)

// Semantic attribute keys for stapler-squad operations
const (
	// Session attributes
	AttrSessionProgram  = "session.program"
	AttrSessionCategory = "session.category"

	// History attributes
	AttrHistoryProject      = "history.project"
	AttrHistorySessionID    = "history.session_id"
	AttrHistoryMessageCount = "history.message_count"

	// Search attributes
	AttrSearchQuery     = "search.query"
	AttrSearchIndexSize = "search.index_size"

	// Storage attributes
	AttrStorageDurationMs = "storage.duration_ms"

	// Review queue attributes
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
