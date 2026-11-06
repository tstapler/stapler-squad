package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/flow"
)

// flipRun appends n alternating ready/in_progress flips (the dequeue-spawn-failed shape) one minute apart from start.
func flipRun(id flow.ItemID, start time.Time, n int, seq *int64) []flow.Event {
	out := make([]flow.Event, 0, n)
	from, to := domain.BacklogStatusReady, domain.BacklogStatusInProgress
	for i := 0; i < n; i++ {
		*seq++
		out = append(out, flow.Event{ItemID: id, From: from, To: to, At: start.Add(time.Duration(i) * time.Minute), Seq: *seq})
		from, to = to, from
	}
	return out
}

func TestGetItemFlowTimeline_should_ReturnTwoThrashSpansNoRawEvents_When_ThrashAFixture(t *testing.T) {
	t.Parallel()
	const id = flow.ItemID("thrash-a")
	var seq int64
	// 279 raw flips in two runs three hours apart, well past the one-hour merge gap.
	events := flipRun(id, flowToolNow.Add(-48*time.Hour), 140, &seq)
	events = append(events, flipRun(id, flowToolNow.Add(-40*time.Hour), 139, &seq)...)
	h := newRealFlowHandlers(&memFlowSources{events: events, titles: map[flow.ItemID]string{id: "Thrash A"}})

	_, m := callFlowTool(t, h.getItemFlowTimeline, map[string]any{"item_id": string(id)})

	entries := m["entries"].([]any)
	require.Len(t, entries, 2, "279 flips collapse to two thrash spans, no raw events")
	for _, e := range entries {
		assert.Equal(t, "thrash_episode", e.(map[string]any)["kind"])
	}
	assert.EqualValues(t, 2, m["total_entries"])
	assert.Equal(t, false, m["truncated"])
	assert.EqualValues(t, 2, m["thrash_episodes"])
}

func TestGetItemFlowTimeline_should_CapAtFiftyAndMarkTruncated_When_SixtyTransitions(t *testing.T) {
	t.Parallel()
	const id = flow.ItemID("long-a")
	cycle := []domain.BacklogStatus{domain.BacklogStatusInProgress, domain.BacklogStatusReview, domain.BacklogStatusQueued}
	var events []flow.Event
	prev := domain.BacklogStatusIdea
	for i := 0; i < 60; i++ {
		next := cycle[i%len(cycle)]
		events = append(events, flow.Event{ItemID: id, From: prev, To: next, At: flowToolNow.Add(-time.Duration(60-i) * 2 * time.Hour), Seq: int64(i)})
		prev = next
	}
	h := newRealFlowHandlers(&memFlowSources{events: events, titles: map[flow.ItemID]string{id: "Long"}})

	_, m := callFlowTool(t, h.getItemFlowTimeline, map[string]any{"item_id": string(id), "window_days": float64(30)})

	assert.Len(t, m["entries"], 50)
	assert.EqualValues(t, 60, m["total_entries"])
	assert.Equal(t, true, m["truncated"])
}

func TestGetItemFlowTimeline_should_RequireItemID_When_Missing(t *testing.T) {
	t.Parallel()
	h := newRealFlowHandlers(&memFlowSources{})

	_, m := callFlowTool(t, h.getItemFlowTimeline, map[string]any{})

	assert.Contains(t, errorMessage(t, m), "item_id")
}

func TestGetItemFlowTimeline_should_TruncateNoteTo300AndSanitize_When_800CharNote(t *testing.T) {
	t.Parallel()
	tl := &flow.ItemTimeline{
		ItemID: "x", Title: "T", Status: "review", TotalEntries: 1,
		Entries: []flow.TimelineEntry{{Kind: "transition", From: "review", To: "in_progress", At: flowToolNow, Count: 1, Note: injection + strings.Repeat("n", 800)}},
	}
	h := newFakeFlowHandlers(&fakeFlowAgg{timeline: tl})

	_, m := callFlowTool(t, h.getItemFlowTimeline, map[string]any{"item_id": "x"})

	note := m["entries"].([]any)[0].(map[string]any)["note"].(string)
	assert.NotContains(t, note, "\n")
	assert.LessOrEqual(t, len(note), flowLeafMaxLen+len(" [truncated]"), "an 800-char note is cut to at most 300")
	assert.Less(t, len(note), 400)
}

func TestListFlowThrashEpisodes_should_PageAndLabel_When_35ThrashItems(t *testing.T) {
	t.Parallel()
	agg := &liveShapedAgg{}
	h := &flowHandlers{svc: services.NewBacklogFlowService(agg)}

	_, m := callFlowTool(t, h.listFlowThrashEpisodes, map[string]any{})

	items := m["items"].(map[string]any)
	assert.Len(t, items["items"], 10)
	assert.EqualValues(t, 35, items["total_count"])
	assert.EqualValues(t, 10, items["returned_count"])
	assert.Equal(t, true, items["has_more"], "an agent must not conclude there are only 10")
	assert.Equal(t, []flow.SectionName{flow.SectionAnomalies}, agg.gotSummary.Sections)
	assert.Equal(t, 30, agg.gotSummary.Window.Days)
}

func TestListFlowThrashEpisodes_should_TruncateNotesTo300_When_800CharNote(t *testing.T) {
	t.Parallel()
	s := liveShapedSummary(flow.FlowQuery{})
	s.Anomalies.Meta = flow.SectionMeta{Status: flow.SectionPartial, Error: injection + strings.Repeat("c", 800)}
	h := newFakeFlowHandlers(&fakeFlowAgg{summary: s})

	text, m := callFlowTool(t, h.listFlowThrashEpisodes, map[string]any{"limit": float64(5)})

	errText := fieldPath(m, "status.error").(string)
	assert.LessOrEqual(t, len(errText), flowLeafMaxLen+len(" [truncated]"), "an 800-char free-text leaf is cut to 300")
	assert.True(t, strings.HasSuffix(errText, " [truncated]"), errText)
	assert.NotContains(t, text, `\n`)
	assert.NotContains(t, m, "summary", "the list response carries no aggregate sections or free-form notes")
}

func TestListFlowDeadlockCandidates_should_PageAndLabel_When_41Candidates(t *testing.T) {
	t.Parallel()
	agg := &liveShapedAgg{}
	h := &flowHandlers{svc: services.NewBacklogFlowService(agg)}

	_, m := callFlowTool(t, h.listFlowDeadlockCandidates, map[string]any{})

	page := m["deadlocks"].(map[string]any)
	rows := page["items"].([]any)
	require.Len(t, rows, 10)
	assert.EqualValues(t, 41, page["total_count"])
	assert.EqualValues(t, 10, page["returned_count"])
	assert.Equal(t, true, page["has_more"])
	assert.NotEmpty(t, m["weak_filter"])
	assert.Contains(t, fieldPath(m, "meta.estimated"), flow.EstimatedDeadlockHeuristic)
	prev := 1 << 30
	for _, r := range rows {
		row := r.(map[string]any)
		for _, k := range []string{"sequence", "resolution", "run_length", "repeated_criterion", "diff_unchanged_pairs", "agent_claimed_done"} {
			assert.Contains(t, row, k)
		}
		rl := int(row["run_length"].(float64))
		assert.LessOrEqual(t, rl, prev, "ranked by run_length")
		prev = rl
	}
	assert.Equal(t, []flow.SectionName{flow.SectionReview}, agg.gotSummary.Sections)
}

func TestFlowListTools_should_DescribeDeadlockListAsHeuristic_When_Registered(t *testing.T) {
	t.Parallel()
	dl := listFlowDeadlockCandidatesTool().Description
	assert.Contains(t, dl, "heuristic")
	assert.Contains(t, dl, "ranked review-fail streak")
	assert.Contains(t, dl, "Example:")
	assert.Contains(t, listFlowThrashEpisodesTool().Description, "Example:")
	assert.Contains(t, getItemFlowTimelineTool().Description, "Example:")
}
