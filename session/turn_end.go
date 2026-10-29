package session

import (
	"reflect"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// TurnEndSink persists a turn-end fact; it never writes the acked episode.
type TurnEndSink interface {
	PersistTurnEnded(title string, turn TurnEnded) error
}

// SetTurnEndSink sets the turn-end persister at registration; nil (including a typed-nil
// pointer) keeps the no-op default.
func (i *Instance) SetTurnEndSink(sink TurnEndSink) {
	if isNilInterface(sink) {
		sink = nil
	}
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

func isNilInterface(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

func (i *Instance) collaborators() (TurnEndSink, SessionEventRecorder) {
	i.collabMu.RLock()
	defer i.collabMu.RUnlock()
	return i.turnEndSink, orNoopRecorder(i.eventRecorder)
}

// RecordTurnEnd stores the turn-end fact and touches activity under i.mu, then, after
// releasing it, persists via the sink and records the event once each.
func (i *Instance) RecordTurnEnd(text string, source TurnEndSource) {
	turn := TurnEnded{Excerpt: text, Source: source, At: time.Now()}
	var title string
	err := i.sendSyncErr(func(s *instanceState) error {
		s.inst.mu.Lock()
		s.inst.TurnEnded = turn
		s.inst.touchLocked(SessionEventTurnEnded, turn.At)
		snap := buildSnapshot(s.inst)
		s.inst.mu.Unlock()
		s.inst.snapshot.Store(snap)
		title = snap.Title
		return nil
	})
	if err != nil {
		// Actor stopped: the in-memory fact was not stored, so don't persist or record under an empty title.
		log.Warn("turn end dropped; actor unavailable", "error", err)
		return
	}
	sink, rec := i.collaborators()
	if sink != nil {
		if err := sink.PersistTurnEnded(title, turn); err != nil {
			log.ForSession(title).Warn("persist turn end failed; keeping in-memory fact", "error", err)
		}
	}
	rec.RecordTurnEnded(title, turn)
}
