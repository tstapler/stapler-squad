package session

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// Regenerate goldens with: go test ./session -run TestDetermineGolden -update
var updateDetermineGoldens = flag.Bool("update", false, "rewrite review_queue Determine golden files")

const reviewQueueCorpusDir = "testdata/review_queue"

// corpusPrograms lists every program dir and whether the terminal detector has patterns
// for it. pi is driven by JSON events, so its terminal tails are expected to go undetected.
var corpusPrograms = map[string]bool{
	"claude":   true,
	"aider":    true,
	"agy":      true,
	"gemini":   true,
	"opencode": true,
	"pi":       false,
}

const minCasesPerDetectedProgram = 5

// determineOutcome is the stable, human-readable projection of a DetectionResult.
type determineOutcome struct {
	Action        string `json:"action"`
	Reason        string `json:"reason"`
	Priority      string `json:"priority"`
	Context       string `json:"context"`
	ClaudeStatus  string `json:"claude_status"`
	CleanWorktree bool   `json:"clean_worktree"`
}

type determineGolden struct {
	Label            string           `json:"label"`
	Source           string           `json:"source"`
	DetectedStatus   string           `json:"detected_status"`
	LabelMatched     bool             `json:"label_matched"`
	NoController     determineOutcome `json:"no_controller"`
	ControllerActive determineOutcome `json:"controller_active"`
}

// goldenClock stands in for an injected clock: Determine reads the wall clock directly,
// so each instance's timestamps are taken from time.Now() at construction (parallel
// subtests start late; a shared early instant drifts past the 5s idle threshold).
type goldenClock struct{}

func (c goldenClock) instance(title string) *Instance {
	now := time.Now()
	inst := &Instance{Title: title, UUID: "uuid-" + title, Status: Running}
	inst.started.Store(true)
	inst.UpdatedAt = now
	inst.LastMeaningfulOutput = now.Add(-time.Second)
	inst.SyncAtomicTimestamps()
	return inst
}

func actionName(a DetectionAction) string {
	switch a {
	case DetectionActionAdd:
		return "add"
	case DetectionActionRemove:
		return "remove"
	default:
		return "skip"
	}
}

func outcomeOf(r DetectionResult) determineOutcome {
	return determineOutcome{
		Action:        actionName(r.Action),
		Reason:        string(r.Reason),
		Priority:      r.Priority.String(),
		Context:       r.Context,
		ClaudeStatus:  r.ClaudeStatus.String(),
		CleanWorktree: r.CleanWorktree,
	}
}

// labelOf derives the operator-labelled expectation from the fixture filename prefix.
func labelOf(name string) string {
	for _, l := range []string{"approval", "prose_question", "finished", "executing", "idle"} {
		if strings.HasPrefix(name, l) {
			return l
		}
	}
	return "unlabelled"
}

func labelMatched(label string, r DetectionResult) bool {
	switch label {
	case "approval":
		return r.Action == DetectionActionAdd && r.Reason == ReasonApprovalPending
	case "prose_question":
		return r.Action == DetectionActionAdd && r.Reason == ReasonInputRequired
	case "finished":
		return r.Action == DetectionActionAdd && r.Reason == ReasonTaskComplete
	case "executing":
		return r.Action == DetectionActionRemove
	case "idle":
		return r.Action != DetectionActionRemove && r.Reason != ReasonApprovalPending && r.Reason != ReasonInputRequired
	}
	return false
}

func determineCase(clock goldenClock, program, name, content string) determineGolden {
	detector := detection.NewStatusDetector()
	determiner := NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig())
	title := program + "-" + name

	noController := determiner.Determine(clock.instance(title), content, InstanceStatusInfo{}, detector, DeterminerOptions{})

	detected, detectedCtx := detector.DetectWithContextFromLines(strings.Split(content, "\n"))
	withController := determiner.Determine(clock.instance(title), "", InstanceStatusInfo{
		IsControllerActive: true,
		ClaudeStatus:       detected,
		StatusContext:      detectedCtx,
		IdleState:          detection.IdleStateInfo{State: detection.IdleStateWaiting},
	}, detector, DeterminerOptions{})

	label := labelOf(name)
	source := "captured"
	if strings.Contains(name, ".synth") {
		source = "synthesized"
	}
	return determineGolden{
		Label:            label,
		Source:           source,
		DetectedStatus:   detected.String(),
		LabelMatched:     labelMatched(label, noController),
		NoController:     outcomeOf(noController),
		ControllerActive: outcomeOf(withController),
	}
}

type corpusCase struct {
	program, name, fixturePath, goldenPath string
}

func corpusCases(t *testing.T) []corpusCase {
	t.Helper()
	var cases []corpusCase
	for program := range corpusPrograms {
		files, err := filepath.Glob(filepath.Join(reviewQueueCorpusDir, program, "*.txt"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			name := strings.TrimSuffix(filepath.Base(f), ".txt")
			cases = append(cases, corpusCase{program, name, f, strings.TrimSuffix(f, ".txt") + ".golden.json"})
		}
	}
	return cases
}

func TestDetermineGolden_Should_MatchCommittedGoldens_When_RunOverClaudeAiderAgyPiGeminiOpencodeCorpus(t *testing.T) {
	t.Parallel()
	clock := goldenClock{}
	perProgram := map[string]int{}

	for _, c := range corpusCases(t) {
		perProgram[c.program]++
		t.Run(c.program+"/"+c.name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(c.fixturePath)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(determineCase(clock, c.program, c.name, string(raw)), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

			if *updateDetermineGoldens {
				if err := os.WriteFile(c.goldenPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(c.goldenPath)
			if err != nil {
				t.Fatalf("missing golden %s (run with -update): %v", c.goldenPath, err)
			}
			if string(want) != string(got) {
				t.Errorf("golden mismatch for %s\n--- want\n%s\n--- got\n%s", c.goldenPath, want, got)
			}
		})
	}

	for program, hasPatterns := range corpusPrograms {
		if hasPatterns && perProgram[program] < minCasesPerDetectedProgram {
			t.Errorf("program %s has %d corpus cases, want >= %d", program, perProgram[program], minCasesPerDetectedProgram)
		}
	}
}

func TestDetermineGolden_Should_ReportMissingSignalPerProgram_When_ProgramRowIsNone(t *testing.T) {
	t.Parallel()
	clock := goldenClock{}
	for _, c := range corpusCases(t) {
		if corpusPrograms[c.program] {
			continue
		}
		raw, err := os.ReadFile(c.fixturePath)
		if err != nil {
			t.Fatal(err)
		}
		g := determineCase(clock, c.program, c.name, string(raw))
		if g.DetectedStatus != detection.StatusUnknown.String() && g.DetectedStatus != detection.StatusIdle.String() {
			t.Errorf("%s/%s: program has no detector row but detected %q; update corpusPrograms", c.program, c.name, g.DetectedStatus)
		}
		if g.Label != "idle" && g.LabelMatched {
			t.Errorf("%s/%s: label %q unexpectedly matched for a program with no detector row", c.program, c.name, g.Label)
		}
	}
}
