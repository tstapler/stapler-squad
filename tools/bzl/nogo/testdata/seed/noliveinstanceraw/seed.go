// Package services stands in for server/services (the analyzer gates on that import path); never import it.
package services

type Instance struct{}

type SessionService struct{ poller map[string]*Instance }

func (s *SessionService) FindLiveInstance(id string) *Instance { return s.poller[id] }

// IsSessionLive is a monitored method that compares FindLiveInstance to nil itself.
func (s *SessionService) IsSessionLive(id string) bool {
	inst := s.FindLiveInstance(id)
	return inst != nil
}
