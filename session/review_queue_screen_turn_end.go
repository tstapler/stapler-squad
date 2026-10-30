package session

import "strings"

// SetTurnEndTiebreaker wires the optional LLM tiebreak used by the screen stage; nil disables it.
func (rqp *ReviewQueuePoller) SetTurnEndTiebreaker(tb TurnEndTiebreaker) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	if isNilInterface(tb) {
		tb = nil
	}
	rqp.turnEndTiebreaker = tb
}

func (rqp *ReviewQueuePoller) tiebreaker() TurnEndTiebreaker {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return rqp.turnEndTiebreaker
}

// recordScreenTurnEnd is the screen fallback stage. It runs beside Determine, not inside it,
// and only records the fact: the next cycle's Determine turns it into an item. It never
// starts or revives a session, and does nothing for hidden or archived ones.
func (rqp *ReviewQueuePoller) recordScreenTurnEnd(inst *Instance, snap *InstanceSnapshot, content string, statusInfo InstanceStatusInfo, opts DeterminerOptions) {
	if !opts.ProseQuestion || content == "" || snap.Hidden || snap.ArchivedAt != nil {
		return
	}
	row := ProgramSignalFor(snap.Program)
	if !row.ScreenCapable() {
		return
	}
	// A Stop hook already reported this session's turn ends; the screen only backs up sessions without one.
	if row.Kind == SignalHook && snap.TurnEnded.Source == TurnEndSourceHook {
		return
	}
	// Same screen, already recorded: nothing new has been printed since the last turn end.
	if !snap.TurnEnded.IsZero() && !snap.LastMeaningfulOutput.After(snap.TurnEnded.At) {
		return
	}
	status, statusContext := statusInfo.ClaudeStatus, statusInfo.StatusContext
	if !statusInfo.IsControllerActive {
		status, statusContext = rqp.statusDetector.DetectWithContextFromLines(strings.Split(content, "\n"))
	}
	found, ok := DetectScreenTurnEnd(snap.Program, status, statusContext, content)
	if !ok {
		return
	}
	text := found.Text
	if found.Kind == TurnEndKindQuestion {
		text = ExtractQuestionSentence(found.Text)
	}
	inst.RecordClassifiedTurnEnd(text, TurnEndSourceScreen, found.Kind, found.Confidence)
	if found.NeedsTiebreak && opts.ProseQuestionLLMTiebreak {
		rqp.startScreenTiebreak(inst, found.Text)
	}
}

// startScreenTiebreak upgrades the just-recorded Finished to a Question when the model says
// so and no newer turn end has replaced it. The Finished is already visible, so any failure
// leaves the session as a low-priority item (fail open).
func (rqp *ReviewQueuePoller) startScreenTiebreak(inst *Instance, text string) {
	tb := rqp.tiebreaker()
	if tb == nil {
		return
	}
	snap := inst.Snapshot()
	episode := snap.TurnEnded.Episode
	RunTurnEndTiebreakAsync(tb, snap.Title, episode, text, func(class TurnEndClass) {
		now := inst.Snapshot()
		if class != TurnEndClassQuestion || now.TurnEnded.Episode != episode || now.ArchivedAt != nil || now.Hidden {
			return
		}
		inst.RecordClassifiedTurnEnd(ExtractQuestionSentence(text), TurnEndSourceScreen, TurnEndKindQuestion, TurnEndConfidenceHigh)
	})
}
