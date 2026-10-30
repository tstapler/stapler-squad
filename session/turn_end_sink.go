package session

// turnEndPersister is the slice of Storage that turnEndSink needs.
type turnEndPersister interface {
	UpdateInstanceTurnEnded(title string, turn TurnEnded) error
}

type turnEndSink struct{ store turnEndPersister }

// NewTurnEndSink adapts Storage's narrow turn-end writer to TurnEndSink. It never writes the
// acked episode: only the AcknowledgeSession path does.
func NewTurnEndSink(store turnEndPersister) TurnEndSink {
	return turnEndSink{store: store}
}

func (s turnEndSink) PersistTurnEnded(title string, turn TurnEnded) error {
	return s.store.UpdateInstanceTurnEnded(title, turn)
}
