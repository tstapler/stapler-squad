// Package inbox is the client for gh-signal's loopback agent-inbox HTTP API
// (PR Watch protocol v1). Wire types here mirror docs/agent-inbox.md in gh-signal.
package inbox

import (
	"encoding/json"
	"time"
)

// Coverage reports whether GitHub delivers webhooks for a watched repository.
type Coverage string

const (
	CoverageExisting    Coverage = "existing"
	CoverageRegistered  Coverage = "registered"
	CoverageUnavailable Coverage = "unavailable"
)

// Covered reports whether events will arrive; unknown values are not covered.
func (c Coverage) Covered() bool {
	return c == CoverageExisting || c == CoverageRegistered
}

// Event is the normalized PR event envelope. Every field except Details is
// untrusted text from GitHub.
type Event struct {
	Version         int             `json:"version"`
	ID              string          `json:"id"`
	DeliveryID      string          `json:"delivery_id"`
	Source          string          `json:"source"`
	SourceEventType string          `json:"source_event_type"`
	Repository      string          `json:"repository"`
	PullRequest     int             `json:"pull_request"`
	Kind            string          `json:"kind"`
	Action          string          `json:"action"`
	Actor           string          `json:"actor"`
	OccurredAt      time.Time       `json:"occurred_at"`
	HeadSHA         *string         `json:"head_sha"`
	Summary         string          `json:"summary"`
	Fingerprint     string          `json:"fingerprint"`
	Trust           string          `json:"trust"`
	Details         json.RawMessage `json:"details"`
}

// Item is one pending inbox entry; ID is what Ack takes.
type Item struct {
	ID             string    `json:"id"`
	SubscriptionID string    `json:"subscription_id"`
	CreatedAt      time.Time `json:"created_at"`
	Event          Event     `json:"event"`
}

// LastActivity is routing metadata about the newest webhook seen for a subscription.
type LastActivity struct {
	EventType  *string   `json:"event_type"`
	DeliveryID *string   `json:"delivery_id"`
	Outcome    string    `json:"outcome"`
	Reason     *string   `json:"reason"`
	ObservedAt time.Time `json:"observed_at"`
}

// Subscription ties a consumer to one (repository, pull request).
type Subscription struct {
	ID           string        `json:"id"`
	ConsumerID   string        `json:"consumer_id"`
	Repository   string        `json:"repository"`
	PullRequest  int           `json:"pull_request"`
	CreatedAt    time.Time     `json:"created_at"`
	Active       bool          `json:"active"`
	LastActivity *LastActivity `json:"last_activity"`
}

// WatchResult is the POST /v1/watch response. Reason is set only when
// Coverage is unavailable.
type WatchResult struct {
	Coverage     Coverage     `json:"coverage"`
	Subscription Subscription `json:"subscription"`
	Reason       string       `json:"reason,omitempty"`
}

// Health is the unauthenticated GET /v1/health response.
type Health struct {
	Status          string        `json:"status"`
	ProtocolVersion int           `json:"protocol_version"`
	Database        string        `json:"database"`
	Ingress         HealthIngress `json:"ingress"`
}

// HealthIngress describes how events reach the inbox.
type HealthIngress struct {
	Mode       string `json:"mode"`
	Status     string `json:"status"`
	HTTPIngest bool   `json:"http_ingest"`
}
