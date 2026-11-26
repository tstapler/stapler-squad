// Package goalrunner runs one backlog item through Claude Code's /goal as a
// direct subprocess (no tmux), with a cost ceiling and a heartbeat around it.
package goalrunner

import "time"

// EndReason is why a goal run stopped.
type EndReason string

const (
	EndGoalMet      EndReason = "goal_met"
	EndTurnLimit    EndReason = "turn_limit"
	EndImpossible   EndReason = "impossible"
	EndError        EndReason = "error"
	EndCostExceeded EndReason = "cost_exceeded"
)

// Outcome is the structured result of one run.
type Outcome struct {
	EndReason EndReason
	// CostUSD is the final result's cost, or the running estimate when the
	// ceiling killed the process before a result line arrived.
	CostUSD  float64
	Turns    int
	WallTime time.Duration
	// Err carries the launch or wait failure when EndReason is EndError.
	Err error
}
