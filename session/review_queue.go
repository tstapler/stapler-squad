package session

import "github.com/tstapler/stapler-squad/session/queue"

// ReviewQueueWriter is the write-side interface for the review queue.
// It is satisfied by *queue.ReviewQueue and can be used in place of the concrete
// type wherever only Add is required, making it easy to supply a test double.
type ReviewQueueWriter interface {
	Add(item *queue.ReviewItem) bool
}

// ReviewQueueRemover is the removal-side interface for the review queue,
// matching ReviewQueueWriter's placement/convention above. Satisfied by
// *queue.ReviewQueue.
type ReviewQueueRemover interface {
	RemoveWithInfo(sessionID string, info queue.RemovalInfo) bool
}
