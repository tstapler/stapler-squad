package streamhub

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
)

// ResizeVote is a {SubscriberID, TerminalSize} tuple submitted by a
// capability-eligible Subscriber toward the hub's NegotiatedSize (Task
// 1.3.1b).
type ResizeVote struct {
	SubscriberID SubscriberID
	Size         TerminalSize
}

// NegotiatedSize returns the hub's current resolved TerminalSize: the
// component-wise minimum across every CanResize subscriber that has ever
// called RequestResize (ADR-002's smallest-common-size model). It is the
// zero TerminalSize until the first accepted vote.
func (h *StreamHub) NegotiatedSize() TerminalSize {
	h.resizeMu.Lock()
	defer h.resizeMu.Unlock()
	return h.negotiatedSize
}

// RequestResize records subscriber id's vote for size and re-runs
// negotiation, but only if that subscriber was attached with
// SubscriberCapability.CanResize == true (Task 1.3.1c) — a read-only sink's
// vote is rejected and logged, never applied. Requesting resize for an
// unknown SubscriberID is a no-op.
//
// ctx should be scoped to the calling connection's lifetime (canceled when
// that caller disconnects), not a fixed background timeout — it becomes the
// deadline applyNegotiatedSize bounds its SetWindowSize/CapturePaneContentRaw
// calls with, so an early disconnect stops those calls instead of always
// waiting out a fixed ceiling.
func (h *StreamHub) RequestResize(ctx context.Context, id SubscriberID, size TerminalSize) {
	h.mu.Lock()
	sub, ok := h.subscribers[id]
	h.mu.Unlock()
	if !ok {
		return
	}
	if !sub.capability.CanResize {
		log.Info("streamhub resize vote rejected: subscriber cannot resize",
			"session", h.sessionName, "subscriber_id", string(id))
		return
	}

	// resizeApplyMu is held across the entire negotiate-then-apply
	// sequence below (vote recording through applyNegotiatedSize's
	// return), not just the negotiatedSize read/write — otherwise two
	// concurrent callers can each observe changed == true and both invoke
	// applyNegotiatedSize at once. See resizeApplyMu's doc comment in
	// hub.go for the full race this closes.
	h.resizeApplyMu.Lock()
	defer h.resizeApplyMu.Unlock()

	sub.recordResizeVote(size)
	votes := h.collectResizeVotes()
	negotiated := negotiateSize(votes)

	h.resizeMu.Lock()
	previous := h.negotiatedSize
	if negotiated != (TerminalSize{}) {
		h.negotiatedSize = negotiated
	}
	current := h.negotiatedSize
	h.resizeMu.Unlock()

	changed := current != previous
	log.Info("streamhub resize negotiated",
		"session", h.sessionName, "votes", len(votes),
		"negotiated_cols", current.Cols(), "negotiated_rows", current.Rows(), "changed", changed)
	recordResizeNegotiation(changed)

	if changed {
		h.applyNegotiatedSize(ctx, current)
	}
}

// collectResizeVotes gathers one ResizeVote per attached CanResize
// subscriber that has ever voted, skipping never-voted subscribers entirely
// (Task 1.3.1e) — their absence from the vote set is exactly what lets them
// default to "no constraint" instead of a hardcoded size.
func (h *StreamHub) collectResizeVotes() []ResizeVote {
	h.mu.Lock()
	defer h.mu.Unlock()

	votes := make([]ResizeVote, 0, len(h.subscribers))
	for id, sub := range h.subscribers {
		if !sub.capability.CanResize {
			continue
		}
		if size, voted := sub.currentResizeVote(); voted {
			votes = append(votes, ResizeVote{SubscriberID: id, Size: size})
		}
	}
	return votes
}

// negotiateSize reduces votes to the component-wise minimum across Cols and
// Rows independently (Task 1.3.1d, ADR-002). It returns the zero
// TerminalSize if votes is empty — callers must treat that as "no
// negotiation input available," never as a legitimate 0x0 target.
func negotiateSize(votes []ResizeVote) TerminalSize {
	if len(votes) == 0 {
		return TerminalSize{}
	}
	result := votes[0].Size
	for _, v := range votes[1:] {
		if v.Size.cols < result.cols {
			result.cols = v.Size.cols
		}
		if v.Size.rows < result.rows {
			result.rows = v.Size.rows
		}
	}
	return result
}
