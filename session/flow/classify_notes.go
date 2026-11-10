package flow

import (
	"slices"
	"strings"
)

// Story 0.1.1 findings (verified by grep on this tree):
//   - Stuck hook points: EntRepository.MarkStuck is at session/ent_repository_backlog.go:2332
//     and ResolveStuck at :2418 (the plan cited :2403 from origin/main before the sync).
//   - Partial unique index: ent supports entsql.IndexWhere (already used at
//     session/ent/schema/backlog_stuck_episode.go:49) and sql.ConflictWhere
//     (entgo.io/ent v0.14.5 dialect/sql/builder.go:259).
//   - Stage order: no exported accessor. DefaultWorkflowEngine keeps its graph in the
//     unexported transitions map (session/workflow_engine.go:46); the ent BacklogStage table
//     has no order column and EntStageConfigRepository.ListEnabledStages
//     (session/ent_stage_config_repository.go:26) returns rows unordered, so Story 2.1.4a
//     must add an exported PipelineStages(engine) helper.
//   - Note literals: "dequeued: WIP slot freed" and "dequeue spawn failed"
//     (server/services/backlog_service_triage.go:870, :883), "auto-reopened after failed
//     review verdict" (:2128), "auto-reopened for PR fix (CI/review)" (:2525). Correction:
//     "all work sessions ended" is not in that file; it is the in_progress>review note at
//     session/transition_gate.go:315. The agent bounce note "request_review recovering
//     stale/blocked review" is at server/mcp/tools_backlog.go:1506.

// noteRule maps a note prefix to the Cause it labels.
type noteRule struct {
	prefix string
	cause  Cause
}

// noteRules is the table-driven classifier (Strategy as data). Longer prefixes
// that share a stem must come first. Thrash membership never depends on it (ADR-002).
var noteRules = []noteRule{
	{"dequeued: WIP slot freed", CauseDequeueClaim},
	{"dequeue spawn failed", CauseDequeueSpawnFailed},
	{"auto-reopened for PR fix", CausePRFixReopen},
	{"auto-reopened after failed review verdict", CauseReviewReopen},
}

// ClassifyNote labels a raw event note; unknown notes are CauseUnclassified.
// Leading and trailing whitespace is ignored and trailing detail is tolerated.
func ClassifyNote(note string) Cause {
	note = strings.TrimSpace(note)
	for _, r := range noteRules {
		if strings.HasPrefix(note, r.prefix) {
			return r.cause
		}
	}
	return CauseUnclassified
}

// NoteTally counts classified notes; Unclassified is always reported.
type NoteTally struct {
	ByCause      map[Cause]int
	Unclassified int
}

// Classified is the number of notes mapped to a known Cause.
func (t NoteTally) Classified() int {
	n := 0
	for _, c := range t.ByCause {
		n += c
	}
	return n
}

// ClassifyNotes classifies every note and tallies the result.
func ClassifyNotes(notes []string) NoteTally {
	t := NoteTally{ByCause: make(map[Cause]int)}
	for _, n := range notes {
		c := ClassifyNote(n)
		if c == CauseUnclassified {
			t.Unclassified++
			continue
		}
		t.ByCause[c]++
	}
	return t
}

// notePrefixMismatches returns, sorted, every snapshot literal whose
// classification under rules differs from want. Used by the snapshot guard test.
func notePrefixMismatches(rules []noteRule, want map[string]Cause) []string {
	var bad []string
	for literal, cause := range want {
		got := CauseUnclassified
		for _, r := range rules {
			if strings.HasPrefix(literal, r.prefix) {
				got = r.cause
				break
			}
		}
		if got != cause {
			bad = append(bad, literal)
		}
	}
	slices.Sort(bad)
	return bad
}
