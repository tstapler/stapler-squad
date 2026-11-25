// Package servicesx shares a name prefix with server/services but is not beneath it: the
// analyzer must stay silent here (guards inScope's "/" boundary).
package servicesx

// Instance stands in for session.Instance.
type Instance struct{}

// SessionService stands in for a look-alike service type.
type SessionService struct{}

// FindLiveInstance is declared here, outside the analyzer's scope.
func (s *SessionService) FindLiveInstance(id string) *Instance { return nil }

// IsSessionLive has the monitored name and the raw shape, but is out of scope: no diagnostic.
func (s *SessionService) IsSessionLive(id string) bool {
	return s.FindLiveInstance(id) != nil
}
