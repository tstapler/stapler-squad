// Package services contains test fixtures for the noliveinstanceraw analyzer,
// resolved via analysistest's GOPATH-style testdata overlay at exactly the
// real import path (github.com/tstapler/stapler-squad/server/services) so the
// analyzer's package-path gate and its monitoredFuncNames watchlist (gated on
// that exact package) activate during the test, the same way norawghrequest's
// testdata resolves its package at its real import path.
package services

// Instance stands in for session.Instance — only its identity as a pointer
// type matters here, not its fields.
type Instance struct{}

// SessionService stands in for the real SessionService.
type SessionService struct {
	poller map[string]*Instance
}

// FindLiveInstance stands in for the real SessionService.FindLiveInstance.
func (s *SessionService) FindLiveInstance(id string) *Instance {
	if inst, ok := s.poller[id]; ok {
		return inst
	}
	return nil
}

// findConfirmedLiveInstance stands in for the real approved wrapper.
func (s *SessionService) findConfirmedLiveInstance(id string) *Instance {
	if inst := s.FindLiveInstance(id); inst != nil {
		return inst
	}
	return nil // fall back to a direct truth check in the real implementation
}

// IsSessionLive is a MONITORED method (in monitoredFuncNames): the fixed,
// correct version delegates to findConfirmedLiveInstance rather than
// comparing FindLiveInstance's result to nil itself.
func (s *SessionService) IsSessionLive(id string) bool {
	return s.findConfirmedLiveInstance(id) != nil
}

// bad1/bad2: KillTmuxPaneOnly and StopSessionByUUID are monitored methods
// (in monitoredFuncNames) regressed back to a raw FindLiveInstance
// nil-comparison — the exact pre-fix shape of the real bug.
func (s *SessionService) KillTmuxPaneOnly(id string) error {
	inst := s.FindLiveInstance(id)
	if inst == nil { // want `raw FindLiveInstance\(\.\.\.\) nil-comparison`
		return nil
	}
	return nil
}

func (s *SessionService) StopSessionByUUID(id string) error {
	if s.FindLiveInstance(id) != nil { // want `raw FindLiveInstance\(\.\.\.\) nil-comparison`
		return nil
	}
	return nil
}

// good1: a //nolint comment on the same line suppresses the finding even
// inside a monitored method.
func (s *SessionService) goodNolint(id string) bool {
	return s.FindLiveInstance(id) != nil //nolint:noliveinstanceraw test fixture, not a real liveness decision
}

// good2: the identical raw idiom, but inside a method NOT on the watchlist —
// deliberately not flagged, proving the analyzer's narrow, named scope (see
// package doc comment) rather than a blanket ban.
func (s *SessionService) SteerActiveSession(id string) bool {
	return s.FindLiveInstance(id) != nil
}

// good3: comparing an unrelated value to nil is never flagged, even inside a
// monitored method.
func (s *SessionService) IsRetryPendingUnrelated(inst *Instance) bool {
	return inst != nil
}
