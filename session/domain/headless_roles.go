package domain

import "strings"

// Synthetic ItemSession UUID prefixes. Headless roles (no tmux pane) are in
// headlessRolePrefixes; callers must not treat such a UUID as a tmux session.
// Jules prefixes are plain constants: a jules_work row is also non-tmux but is
// not a headless role.
const (
	HeadlessTriageUUIDPrefix   = "headless-triage-"
	HeadlessReReviewUUIDPrefix = "headless-re-review-"

	// JulesSessionUUIDPrefix is prepended to a Jules session name once
	// CreateSession succeeds; JulesPendingUUIDPrefix marks the reservation
	// written before that call (ADR-004).
	JulesSessionUUIDPrefix = "jules-"
	JulesPendingUUIDPrefix = "jules-pending-"
)

// HeadlessRole names a role whose ItemSessions carry a synthetic UUID prefix.
type HeadlessRole string

const (
	HeadlessRoleTriage   HeadlessRole = "triage"
	HeadlessRoleReReview HeadlessRole = "re-review"
)

// headlessRolePrefixes is the one role <-> UUID-prefix table; a new headless
// role is one row. No prefix may be a prefix of another.
var headlessRolePrefixes = []struct {
	role   HeadlessRole
	prefix string
}{
	{HeadlessRoleTriage, HeadlessTriageUUIDPrefix},
	{HeadlessRoleReReview, HeadlessReReviewUUIDPrefix},
}

// HeadlessRoleOf returns the headless role that minted sessionUUID, if any.
func HeadlessRoleOf(sessionUUID string) (HeadlessRole, bool) {
	for _, row := range headlessRolePrefixes {
		if strings.HasPrefix(sessionUUID, row.prefix) {
			return row.role, true
		}
	}
	return "", false
}

// IsHeadlessRole reports whether sessionUUID was minted for role.
func IsHeadlessRole(sessionUUID string, role HeadlessRole) bool {
	got, ok := HeadlessRoleOf(sessionUUID)
	return ok && got == role
}
