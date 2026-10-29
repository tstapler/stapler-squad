package prwatch

import (
	"encoding/json"
	"strings"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

// supportedProtocolVersion is the PR Watch protocol version classify accepts.
const supportedProtocolVersion = 1

// ciFailureConclusions are the completed-suite conclusions that steer (D7).
var ciFailureConclusions = map[string]bool{
	"failure":         true,
	"timed_out":       true,
	"startup_failure": true,
	"action_required": true,
}

// classify applies the D7 event policy. It reads only the kind, action and
// the details fields ci_kind, conclusion and state; it never returns event
// text. When ok is false, reason says why the event is acked without a steer.
func classify(ev inbox.Event) (kinds []Kind, reason DropReason, ok bool) {
	if ev.Version != supportedProtocolVersion {
		return nil, DropUnknownKind, false
	}
	switch Kind(ev.Kind) {
	case KindComment, KindReviewComment:
		if ev.Action != "created" {
			return nil, DropEditedOrDeleted, false
		}
		return []Kind{Kind(ev.Kind)}, "", true
	case KindReview:
		if ev.Action != "submitted" {
			return nil, DropEditedOrDeleted, false
		}
		if strings.EqualFold(detailString(ev.Details, "state"), "approved") {
			return nil, DropApprovedReview, false
		}
		return []Kind{KindReview}, "", true
	case KindCI:
		return classifyCI(ev.Details)
	}
	if ev.Kind == "pull_request" {
		return nil, DropPullRequestEvent, false
	}
	return nil, DropUnknownKind, false
}

// isPullRequestClosed reports a pull_request event with action closed (merged
// or not); D7 retires the PR from the watch set on it.
func isPullRequestClosed(ev inbox.Event) bool {
	return ev.Kind == "pull_request" && ev.Action == "closed"
}

func classifyCI(details json.RawMessage) ([]Kind, DropReason, bool) {
	switch detailString(details, "ci_kind") {
	case "check_suite", "workflow_run":
		if ciFailureConclusions[detailString(details, "conclusion")] {
			return []Kind{KindCI}, "", true
		}
		return nil, DropCINotFailure, false
	case "check_run":
		return nil, DropCICheckRun, false
	}
	return nil, DropCINotFailure, false
}

// detailString returns the named top-level string field of details,
// lowercased and trimmed; missing, null or non-string values yield "".
func detailString(details json.RawMessage, key string) string {
	if len(details) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(details, &m) != nil {
		return ""
	}
	var s string
	if json.Unmarshal(m[key], &s) != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(s))
}
