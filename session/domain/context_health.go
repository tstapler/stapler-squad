package domain

import "time"

// ContextHealthLevel is the green/amber/red verdict. The zero value is
// HealthUnknown so an unset struct is never a false green.
type ContextHealthLevel int

const (
	HealthUnknown ContextHealthLevel = iota
	HealthGreen
	HealthAmber
	HealthRed
)

func (l ContextHealthLevel) String() string {
	switch l {
	case HealthGreen:
		return "green"
	case HealthAmber:
		return "amber"
	case HealthRed:
		return "red"
	default:
		return "unknown"
	}
}

// ContextHealthSignals are raw, config-independent counts over the trailing
// window. It holds no message content: tool names, a confusion pattern name,
// and counts only.
type ContextHealthSignals struct {
	MaxConsecutiveRepeats int
	RepeatedToolName      string
	ConfusionPhraseCount  int
	LastConfusionPhrase   string
	ToolCallsInWindow     int
	WindowEndsAt          time.Time
}

// ContextHealthVerdict is Signals judged against config thresholds.
type ContextHealthVerdict struct {
	Level   ContextHealthLevel
	Reason  string
	Signals ContextHealthSignals
}
