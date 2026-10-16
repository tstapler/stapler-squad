package tokens

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spaolacci/murmur3"

	"github.com/tstapler/stapler-squad/config"
)

const (
	// healthWindowTurns is how many trailing assistant turns the signals cover.
	healthWindowTurns = 20
	// maxFingerprintBytes bounds how much of a tool input is hashed.
	maxFingerprintBytes = 256
)

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

// normalizeToolInput canonicalises a tool input for fingerprinting. Matching is
// exact after normalisation, and inputs sharing the first maxFingerprintBytes
// are treated as equal.
func normalizeToolInput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	s := strings.ToLower(strings.TrimSpace(string(raw)))
	if len(s) > maxFingerprintBytes {
		s = s[:maxFingerprintBytes]
	}
	return s
}

// toolCallFingerprint hashes a tool call so the raw input is never retained.
func toolCallFingerprint(name string, raw json.RawMessage) uint64 {
	return murmur3.Sum64([]byte(name + "\x00" + normalizeToolInput(raw)))
}

type confusionPattern struct {
	Name string
	Re   *regexp.Regexp
}

// confusionPatterns are tried in priority order.
var confusionPatterns = []confusionPattern{
	{"apology", regexp.MustCompile(`(?i)\bi (?:apologize|apologise)\b|\bmy apologies\b|\bi'?m sorry\b`)},
	{"self-correction", regexp.MustCompile(`(?i)\bi (?:made|was) (?:a mistake|wrong|mistaken)\b|\bthat (?:was|is) (?:my|a) (?:mistake|error)\b`)},
	{"retry", regexp.MustCompile(`(?i)\bthat didn'?t work\b|\blet me try (?:a |an )?(?:different|another) approach\b|\blet me try again\b`)},
}

// confusionFastPathMarkers gate the regexps: text with none of them cannot match.
var confusionFastPathMarkers = []string{"sorry", "apolog", "mistake", "didn't work", "didnt work", "try again", "another approach", "different approach", "wrong", "mistaken"}

// matchConfusionPatterns returns the name of the first matching pattern, or "".
// The matched text is never returned.
func matchConfusionPatterns(text string) string {
	lower := strings.ToLower(text)
	found := false
	for _, m := range confusionFastPathMarkers {
		if strings.Contains(lower, m) {
			found = true
			break
		}
	}
	if !found {
		return ""
	}
	for _, p := range confusionPatterns {
		if p.Re.MatchString(text) {
			return p.Name
		}
	}
	return ""
}

// healthTurnRecord is the per-assistant-turn data the window needs.
type healthTurnRecord struct {
	Fingerprints  []uint64
	ToolNames     []string
	ConfusionHits []string
	At            time.Time
}

// healthTurnRing keeps the last healthWindowTurns records. Parser-local, so it
// needs no lock.
type healthTurnRing struct {
	entries [healthWindowTurns]healthTurnRecord
	head    int // next write slot
	count   int
}

func (r *healthTurnRing) push(rec healthTurnRecord) {
	r.entries[r.head] = rec
	r.head = (r.head + 1) % healthWindowTurns
	if r.count < healthWindowTurns {
		r.count++
	}
}

// inOrder returns records oldest first.
func (r *healthTurnRing) inOrder() []healthTurnRecord {
	out := make([]healthTurnRecord, 0, r.count)
	start := (r.head - r.count + healthWindowTurns) % healthWindowTurns
	for i := 0; i < r.count; i++ {
		out = append(out, r.entries[(start+i)%healthWindowTurns])
	}
	return out
}

// extractContextHealthSignals walks the window, tracking the identical-call
// streak across turn boundaries.
func extractContextHealthSignals(ring *healthTurnRing) ContextHealthSignals {
	var sig ContextHealthSignals
	var prev uint64
	streak := 0
	for _, rec := range ring.inOrder() {
		for i, fp := range rec.Fingerprints {
			sig.ToolCallsInWindow++
			if streak > 0 && fp == prev {
				streak++
			} else {
				streak = 1
			}
			prev = fp
			if streak > sig.MaxConsecutiveRepeats {
				sig.MaxConsecutiveRepeats = streak
				sig.RepeatedToolName = rec.ToolNames[i]
			}
		}
		for _, hit := range rec.ConfusionHits {
			sig.ConfusionPhraseCount++
			sig.LastConfusionPhrase = hit
		}
		sig.WindowEndsAt = rec.At
	}
	if sig.MaxConsecutiveRepeats < 2 {
		sig.RepeatedToolName = ""
	}
	return sig
}

// EvaluateContextHealth applies thresholds to raw signals. Pure; a zero-value
// cfg behaves as the defaults.
func EvaluateContextHealth(sig ContextHealthSignals, cfg config.ContextHealthConfig) ContextHealthVerdict {
	cfg = cfg.ContextHealthConfigOrDefault()
	if sig.ToolCallsInWindow < cfg.MinToolCallSamples {
		return ContextHealthVerdict{Level: HealthUnknown, Signals: sig}
	}

	var reasons []string
	loop := sig.MaxConsecutiveRepeats >= cfg.LoopRepeatThreshold
	confused := sig.ConfusionPhraseCount >= cfg.ConfusionPhraseThreshold
	if loop {
		reasons = append(reasons, fmt.Sprintf("Repeated the same %s call %d times in a row", sig.RepeatedToolName, sig.MaxConsecutiveRepeats))
	}
	if confused {
		reasons = append(reasons, fmt.Sprintf("%d self-correction messages in the last %d turns", sig.ConfusionPhraseCount, healthWindowTurns))
	}

	level := HealthGreen
	switch {
	case len(reasons) == 0:
	case (loop && confused) ||
		sig.MaxConsecutiveRepeats >= 2*cfg.LoopRepeatThreshold ||
		sig.ConfusionPhraseCount >= 2*cfg.ConfusionPhraseThreshold:
		level = HealthRed
	default:
		level = HealthAmber
	}
	return ContextHealthVerdict{Level: level, Reason: strings.Join(reasons, "; "), Signals: sig}
}
