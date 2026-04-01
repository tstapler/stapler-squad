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
)

// SessionIDAttr creates an attribute for session ID
func SessionIDAttr(id string) attribute.KeyValue {
	return attribute.String(AttrSessionID, id)
}

// SessionTitleAttr creates an attribute for session title
func SessionTitleAttr(title string) attribute.KeyValue {
	return attribute.String(AttrSessionTitle, title)
}

// SessionStatusAttr creates an attribute for session status
func SessionStatusAttr(status string) attribute.KeyValue {
	return attribute.String(AttrSessionStatus, status)
}

// HistoryEntryCountAttr creates an attribute for history entry count
func HistoryEntryCountAttr(count int) attribute.KeyValue {
	return attribute.Int(AttrHistoryEntryCount, count)
}

// SearchQueryAttr creates an attribute for search query
func SearchQueryAttr(query string) attribute.KeyValue {
	return attribute.String(AttrSearchQuery, query)
}

// SearchResultCountAttr creates an attribute for search result count
func SearchResultCountAttr(count int) attribute.KeyValue {
	return attribute.Int(AttrSearchResultCount, count)
}

// SearchDurationMsAttr creates an attribute for search duration in milliseconds
func SearchDurationMsAttr(durationMs int64) attribute.KeyValue {
	return attribute.Int64(AttrSearchDurationMs, durationMs)
}

// StorageOperationAttr creates an attribute for storage operation type
func StorageOperationAttr(op string) attribute.KeyValue {
	return attribute.String(AttrStorageOperation, op)
}

// StorageCountAttr creates an attribute for storage item count
func StorageCountAttr(count int) attribute.KeyValue {
	return attribute.Int(AttrStorageCount, count)
}

// DBOperationAttr creates an attribute for database operation
func DBOperationAttr(op string) attribute.KeyValue {
	return attribute.String(AttrDBOperation, op)
}

// DBTableAttr creates an attribute for database table
func DBTableAttr(table string) attribute.KeyValue {
	return attribute.String(AttrDBTable, table)
}

// DBRowCountAttr creates an attribute for database row count
func DBRowCountAttr(count int) attribute.KeyValue {
	return attribute.Int(AttrDBRowCount, count)
}

// ReviewQueueSizeAttr creates an attribute for review queue size
func ReviewQueueSizeAttr(size int) attribute.KeyValue {
	return attribute.Int(AttrReviewQueueSize, size)
}
