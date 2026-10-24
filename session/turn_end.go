package session

import (
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// turnEndSink persists a turn-end fact; it never writes the acked episode.
type turnEndSink interface {
	PersistTurnEnded(title string, turn TurnEnded) error
}

// SetTurnEndSink sets the turn-end persister at registration; nil keeps the no-op default.
func (i *Instance) SetTurnEndSink(sink turnEndSink) {
	i.collabMu.Lock()
	i.turnEndSink = sink
	i.collabMu.Unlock()
}

// SetEventRecorder sets the event recorder at registration; nil keeps the no-op default.
func (i *Instance) SetEventRecorder(r SessionEventRecorder) {
	i.collabMu.Lock()
	i.eventRecorder = r
	i.collabMu.Unlock()
}

func (i *Instance) collaborators() (turnEndSink, SessionEventRecorder) {
	i.collabMu.RLock()
	defer i.collabMu.RUnlock()
	return i.turnEndSink, orNoopRecorder(i.eventRecorder)
}

// RecordTurnEnd stores the turn-end fact and touches activity under i.mu, then, after
// releasing it, persists via the sink and records the event once each.
func (i *Instance) RecordTurnEnd(text string, source TurnEndSource) {
	turn := TurnEnded{Excerpt: text, Source: source, At: time.Now()}
	var title string
	_ = i.sendSyncErr(func(s *instanceState) error {
		s.inst.mu.Lock()
		s.inst.TurnEnded = turn
		s.inst.touchLocked(SessionEventTurnEnded, turn.At)
		snap := buildSnapshot(s.inst)
		s.inst.mu.Unlock()
		s.inst.snapshot.Store(snap)
		title = snap.Title
		return nil
	})
	sink, rec := i.collaborators()
	if sink != nil {
		if err := sink.PersistTurnEnded(title, turn); err != nil {
			log.ForSession(title).Warn("persist turn end failed; keeping in-memory fact", "error", err)
		}
	}
	rec.RecordTurnEnded(title, turn)
}
