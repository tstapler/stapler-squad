package flow

import "time"

// Config holds every tunable threshold; defaults live only in DefaultConfig.
type Config struct {
	// ThrashGap is the largest gap between flip events inside one thrash run.
	ThrashGap time.Duration
	// ThrashMinFlips is the minimum event count of a thrash episode.
	ThrashMinFlips int
	// ThrashMergeGap merges adjacent same-shape episodes closer than this.
	ThrashMergeGap time.Duration
	// DeadlockMinRun is the minimum run of consecutive judgment non-PASS verdicts.
	DeadlockMinRun int
	// DeadlockRepeatCriterion is how many consecutive verdicts must fail the same criterion.
	DeadlockRepeatCriterion int
	// LowSampleN is the sample size below which a statistic is flagged LowSample.
	LowSampleN int
	// StalledAfter is how long a reviewing/working item may sit before it counts as stalled.
	StalledAfter time.Duration
	// MinCompareItems is the smallest per-item sample (in each window) a compared metric needs.
	MinCompareItems int
	// MinCompareVerdicts is the smallest verdict sample (in each window) a compared rate needs.
	MinCompareVerdicts int
}

// DefaultConfig returns the plan defaults.
func DefaultConfig() Config {
	return Config{
		ThrashGap:               15 * time.Minute,
		ThrashMinFlips:          6,
		ThrashMergeGap:          time.Hour,
		DeadlockMinRun:          3,
		DeadlockRepeatCriterion: 3,
		LowSampleN:              5,
		StalledAfter:            time.Hour,
		MinCompareItems:         20,
		MinCompareVerdicts:      30,
	}
}

// CollapsePad is how far before the window start the reader must fetch events
// so a thrash episode straddling the boundary is classified exactly. It is
// derived so changing a threshold cannot silently under-pad.
func (c Config) CollapsePad() time.Duration {
	return c.ThrashGap*time.Duration(c.ThrashMinFlips) + c.ThrashMergeGap
}
