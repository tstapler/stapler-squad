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
