package tokens

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
)

// parseState is everything the parser accumulates across a transcript's lines,
// so parsing can stop at a line boundary and resume later from the same state.
type parseState struct {
	result         *ParseResult
	modelCounts    map[string]int
	ring           healthTurnRing
	humanTurnIndex int
}

func newParseState() *parseState {
	return &parseState{
		result:      &ParseResult{ToolUsage: make(map[string]ToolTokenStats)},
		modelCounts: make(map[string]int),
	}
}

// finalize derives the end-of-transcript fields in place and returns the
// result. It is idempotent, so a finalized state can keep accumulating lines.
func (st *parseState) finalize() *ParseResult {
	resolveCompactTokensAfter(st.result)
	st.result.ContextHealth = extractContextHealthSignals(&st.ring)
	st.result.PrimaryModel = primaryModel(st.modelCounts)
	st.result.Models = sortedKeys(st.modelCounts)
	return st.result
}

// clone deep-copies the mutable parts so the original can stay published to
// concurrent readers while the copy keeps parsing. Element values (including
// the ToolNames slices inside TurnStats) are never mutated after creation, so
// they are shared.
func (st *parseState) clone() *parseState {
	r := *st.result
	r.TurnTimeline = slices.Clone(st.result.TurnTimeline)
	r.SkillActivations = slices.Clone(st.result.SkillActivations)
	r.CompactEvents = slices.Clone(st.result.CompactEvents)
	r.ToolUsage = maps.Clone(st.result.ToolUsage)
	return &parseState{
		result:         &r,
		modelCounts:    maps.Clone(st.modelCounts),
		ring:           st.ring, // arrays copy by value; records are immutable once pushed
		humanTurnIndex: st.humanTurnIndex,
	}
}

// checkpointVersion must be bumped whenever parser behavior or ParseResult's
// shape changes: persisted checkpoints of an older version are discarded, so
// unchanged transcripts are reparsed instead of serving stale aggregates.
const checkpointVersion = 1

// stateJSON is parseState's serialized form.
type stateJSON struct {
	Version        int
	Result         *ParseResult
	ModelCounts    map[string]int
	Ring           []healthTurnRecord
	HumanTurnIndex int
}

// encode serializes st as gzipped JSON; transcripts' timelines are highly repetitive.
func (st *parseState) encode() ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	err := json.NewEncoder(zw).Encode(stateJSON{
		Version: checkpointVersion, Result: st.result, ModelCounts: st.modelCounts,
		Ring: st.ring.inOrder(), HumanTurnIndex: st.humanTurnIndex,
	})
	if err != nil {
		return nil, fmt.Errorf("encode parse state: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("compress parse state: %w", err)
	}
	return buf.Bytes(), nil
}

func decodeParseState(blob []byte) (*parseState, error) {
	zr, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return nil, fmt.Errorf("open parse state: %w", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("decompress parse state: %w", err)
	}
	var j stateJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, fmt.Errorf("decode parse state: %w", err)
	}
	if j.Version != checkpointVersion || j.Result == nil {
		return nil, fmt.Errorf("decode parse state: unsupported version %d", j.Version)
	}
	st := newParseState()
	st.result = j.Result
	if st.result.ToolUsage == nil {
		st.result.ToolUsage = make(map[string]ToolTokenStats)
	}
	if j.ModelCounts != nil {
		st.modelCounts = j.ModelCounts
	}
	for _, rec := range j.Ring {
		st.ring.push(rec)
	}
	st.humanTurnIndex = j.HumanTurnIndex
	return st, nil
}
