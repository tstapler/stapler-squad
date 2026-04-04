package crew

import (
	"time"

	"github.com/tstapler/stapler-squad/session/queue"
)

// SweepStatus represents the outcome of a Sweep run.
type SweepStatus int

const (
	SweepStatusPass         SweepStatus = iota // All checks passed
	SweepStatusFail                            // One or more checks failed
	SweepStatusNoTestsFound                    // No test runner detected
	SweepStatusTimeout                         // Test command exceeded timeout
	SweepStatusError                           // Internal sweep error
)

// SweepResult holds the structured output of a Sweep run.
type SweepResult struct {
	Status        SweepStatus
	TestOutput    string   // ANSI-stripped, capped at 4000 chars
	FailingTests  []string
	FailureHash   string // SHA256 of sorted failing test names (for oscillation detection)
	Duration      time.Duration
	ExitCode      int
	RunnerName    string
	RunnerCommand string
	DiffSummary   *queue.DiffSummary
}

// TestRunner describes a detected test runner for the project.
type TestRunner struct {
	Name    string
	Command string
	Timeout time.Duration
}
