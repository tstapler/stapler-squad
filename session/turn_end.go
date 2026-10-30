package session

import (
	"reflect"
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

// RecordTurnEnd records a turn end for text, classified by the provisional heuristic.
func (i *Instance) RecordTurnEnd(text string, source TurnEndSource) {
	kind, conf := provisionalClassify(text)
	i.RecordClassifiedTurnEnd(text, source, kind, conf)
}
