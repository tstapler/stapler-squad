package headless

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrFanoutCeilingExceeded is returned when a first-call stream crossed its
// configured turn or subagent ceiling (CallOptions.MaxTurns/MaxSubagents).
// Unwrap a *FanoutCeilingError from the returned error for the counters.
var ErrFanoutCeilingExceeded = errors.New("headless pool: fan-out ceiling exceeded")

// FanoutCeilingError carries the counters at the moment the ceiling tripped —
// the only record of partial spend, since a cancelled call never emits the
// terminal result event that carries total_cost_usd.
type FanoutCeilingError struct {
	Turns, Subagents       int
	MaxTurns, MaxSubagents int
}

func (e *FanoutCeilingError) Error() string {
	return fmt.Sprintf("%v: turns=%d (max %d), subagents=%d (max %d)",
		ErrFanoutCeilingExceeded, e.Turns, e.MaxTurns, e.Subagents, e.MaxSubagents)
}

func (e *FanoutCeilingError) Unwrap() error { return ErrFanoutCeilingExceeded }

// FanoutLimits are per-call ceilings; a zero field disables that limit.
type FanoutLimits struct {
	MaxTurns     int
	MaxSubagents int
}

func (l FanoutLimits) enabled() bool { return l.MaxTurns > 0 || l.MaxSubagents > 0 }

// fanoutCounter tallies assistant turns and subagent launches from a
// stream-json call. Unparseable or unrecognized lines are ignored (fail open):
// a stream-format change must degrade to "no ceiling", never to a false abort.
type fanoutCounter struct {
	limits    FanoutLimits
	seenIDs   map[string]struct{} // one entry per assistant message; bounded by the turn ceiling
	turns     int
	subagents int
}

func newFanoutCounter(limits FanoutLimits) *fanoutCounter {
	return &fanoutCounter{limits: limits, seenIDs: map[string]struct{}{}}
}

// streamAssistantLine is the subset of an assistant stream-json event we count.
type streamAssistantLine struct {
	Type    string `json:"type"`
	Message struct {
		ID      string          `json:"id"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type streamContentBlock struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// subagentToolNames are the tool names that launch a subagent: "Agent" in
// current transcripts, "Task" in older ones.
var subagentToolNames = map[string]bool{"Agent": true, "Task": true}

// observe records one stream line and returns a non-nil *FanoutCeilingError once a
// limit is exceeded (concrete type on purpose: avoids a typed-nil error). The cheap substring check keeps non-assistant lines (the bulk of
// tool_result traffic) off the JSON decoder.
func (c *fanoutCounter) observe(line string) *FanoutCeilingError {
	if !strings.Contains(line, `"assistant"`) {
		return nil
	}
	var ev streamAssistantLine
	if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Type != "assistant" {
		return nil
	}
	// One assistant message may be split across several lines sharing an id.
	if ev.Message.ID == "" {
		c.turns++
	} else if _, dup := c.seenIDs[ev.Message.ID]; !dup {
		c.seenIDs[ev.Message.ID] = struct{}{}
		c.turns++
	}
	var blocks []streamContentBlock
	if json.Unmarshal(ev.Message.Content, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "tool_use" && subagentToolNames[b.Name] {
				c.subagents++
			}
		}
	}
	return c.exceeded()
}

func (c *fanoutCounter) exceeded() *FanoutCeilingError {
	if (c.limits.MaxTurns > 0 && c.turns > c.limits.MaxTurns) ||
		(c.limits.MaxSubagents > 0 && c.subagents > c.limits.MaxSubagents) {
		return &FanoutCeilingError{Turns: c.turns, Subagents: c.subagents,
			MaxTurns: c.limits.MaxTurns, MaxSubagents: c.limits.MaxSubagents}
	}
	return nil
}
