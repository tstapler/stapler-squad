package server

import "github.com/tstapler/stapler-squad/session"

// wireAttentionCollaborators registers the turn-end sink and the event recorder on every
// instance the poller tracks (all creation paths end in its AddInstance/SetInstances). The
// recorder is gated on session_event_log, read live per call, so with the flag off it
// stays a no-op whatever inner is.
func wireAttentionCollaborators(poller *session.ReviewQueuePoller, storage *session.Storage, inner session.SessionEventRecorder, eventLogEnabled func() bool) {
	rec := session.NewGatedSessionEventRecorder(inner, eventLogEnabled)
	sink := session.NewTurnEndSink(storage)
	poller.SetEventRecorder(rec)
	poller.SetInstanceWirer(func(inst *session.Instance) {
		inst.SetTurnEndSink(sink)
		inst.SetEventRecorder(rec)
	})
}

func sessionEventLogEnabled() bool { return liveAttentionFlags.get().SessionEventLog }
