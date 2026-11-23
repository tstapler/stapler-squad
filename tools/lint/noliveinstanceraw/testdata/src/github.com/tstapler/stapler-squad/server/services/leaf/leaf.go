// Package leaf is a fixture for a monitored method that moved into a leaf
// package under server/services: the analyzer must still check it.
package leaf

// Instance stands in for session.Instance.
type Instance struct{}

// SessionService stands in for a service type that moved into a leaf.
type SessionService struct{}

// FindLiveInstance stands in for the real method, declared in the leaf.
func (s *SessionService) FindLiveInstance(id string) *Instance { return nil }

// IsSessionLive is monitored; the raw comparison must be reported in a leaf too.
func (s *SessionService) IsSessionLive(id string) bool {
	return s.FindLiveInstance(id) != nil // want `raw FindLiveInstance\(\.\.\.\) nil-comparison`
}

// otherDecision is not monitored, so a raw comparison in a leaf must not be flagged.
func (s *SessionService) otherDecision(id string) bool {
	return s.FindLiveInstance(id) != nil
}

// confirmed stands in for the approved wrapper; delegating to it in a monitored method is clean.
func (s *SessionService) confirmed(id string) *Instance { return s.FindLiveInstance(id) }

// KillTmuxPaneOnly is monitored and clean: it goes through the wrapper.
func (s *SessionService) KillTmuxPaneOnly(id string) bool {
	return s.confirmed(id) != nil
}
