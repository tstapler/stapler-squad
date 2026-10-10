package services

import "github.com/tstapler/stapler-squad/session"

// Feed points for the delivery gate's VisibilityIndex (Task 2.2c). Both are
// nil-safe (services built without a gate) and lock-free on the gate side, so
// they are safe to call while holding instance or service locks.

// indexSessionForDelivery upserts inst's visibility. Call it right after the
// Instance exists and before it is started, so the session's first hook cannot
// race an un-indexed (fail-open) lookup.
func (s *SessionService) indexSessionForDelivery(inst *session.Instance) {
	if s.deliveryGate != nil {
		s.deliveryGate.UpsertInstance(inst)
	}
}

// countLegacyHiddenSuppressed records that a legacy hidden-session check just
// swallowed a notification (the soak compares these with the gate's decisions
// before the legacy checks are removed). Observability only.
func (s *SessionService) countLegacyHiddenSuppressed(site string, notificationType int32) {
	if s.deliveryGate != nil {
		s.deliveryGate.CountLegacySuppressedType(site, notificationType)
	}
}

// unindexSessionForDelivery tombstones a deleted session by any identity key.
func (s *SessionService) unindexSessionForDelivery(key string) {
	if s.deliveryGate != nil {
		s.deliveryGate.RemoveSession(key)
	}
}
