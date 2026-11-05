package server

import (
	"fmt"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// wireLiveWorkProbe builds the one session.LiveWorkProbe and hands the same instance
// to the lifecycle listener and the backlog service (which gives it to its cleanup
// gate), then asserts the three consumers share one non-nil probe and records
// ssq_live_work_probe_wired. It is the narrow seam the wiring test drives, because
// BuildDependencies itself makes network calls.
func wireLiveWorkProbe(listener *session.BacklogLifecycleListener, backlog *services.BacklogService, live session.LiveSnapshotSource, store session.InstanceDataReader) error {
	probe := session.NewSnapshotLiveWorkProbe(live, store)
	listener.SetLiveWorkProbe(probe)
	backlog.SetLiveWorkProbe(probe)
	if err := assertLiveWorkProbeShared(listener, backlog); err != nil {
		session.SetLiveWorkProbeWired(false)
		return err
	}
	session.SetLiveWorkProbeWired(true)
	return nil
}

func assertLiveWorkProbeShared(listener *session.BacklogLifecycleListener, backlog *services.BacklogService) error {
	fromListener, fromService := listener.LiveWorkProbe(), backlog.LiveWorkProbe()
	var fromGate session.LiveWorkProbe
	if g := backlog.CleanupGate(); g != nil {
		fromGate = g.Probe()
	}
	if fromListener == nil || fromService == nil || fromGate == nil {
		return fmt.Errorf("live-work probe not wired everywhere (listener=%t backlog service=%t gate=%t)", fromListener != nil, fromService != nil, fromGate != nil)
	}
	if fromListener != fromService || fromService != fromGate {
		return fmt.Errorf("live-work probe instances differ between the lifecycle listener, backlog service and cleanup gate")
	}
	return nil
}
