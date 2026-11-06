package wakes

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// constValuesOfType lists the string values of every const of typeName in file,
// so a newly added kind cannot slip past the table tests.
func constValuesOfType(t *testing.T, file, typeName string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)
	var out []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			id, ok := vs.Type.(*ast.Ident)
			if !ok || id.Name != typeName {
				continue
			}
			for _, v := range vs.Values {
				lit, ok := v.(*ast.BasicLit)
				require.True(t, ok, "const of %s must be a string literal", typeName)
				s, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				out = append(out, s)
			}
		}
	}
	return out
}

func TestEveryKindHasReconcilePredicateOrIsRejected(t *testing.T) {
	t.Parallel()
	wire := constValuesOfType(t, "../../pkg/events/types.go", "BacklogChangeKind")
	require.NotEmpty(t, wire)

	handled := map[string]bool{}
	for _, r := range backlogKindRules {
		require.NotNil(t, r.holds, "%s is accepted but has no reconcile predicate", r.name)
		handled[string(r.wire)] = true
	}
	for _, k := range rejectedBacklogWireKinds {
		assert.False(t, handled[string(k)], "%s is both accepted and rejected", k)
		handled[string(k)] = true
	}
	for _, k := range wire {
		assert.True(t, handled[k], "events.BacklogChangeKind %q has neither a reconcile predicate nor a rejection: add it to backlogKindRules or rejectedBacklogWireKinds", k)
	}
}

func TestValidateSpec_should_AcceptSupportedAndRejectKindsWithoutPredicate(t *testing.T) {
	t.Parallel()
	backlog := func(kind string) domain.WakeSpec { return domain.BacklogEventSpec{ItemID: itemX, Kind: kind} }
	tests := []struct {
		name string
		spec domain.WakeSpec
		ok   bool
	}{
		{"status_reached with a status", backlog("status_reached:review"), true},
		{"verdict_recorded", backlog("verdict_recorded"), true},
		{"item_archived", backlog("item_archived"), true},
		{"item_removed", backlog("item_removed"), true},
		{"session_attached with a role", backlog("session_attached:review"), true},
		{"bare status_transition has no target status", backlog("status_transition"), false},
		{"status_reached without a status", backlog("status_reached"), false},
		{"verdict_recorded with a stray argument", backlog("verdict_recorded:x"), false},
		{"item_updated", backlog("item_updated"), false},
		{"triage_progress_updated", backlog("triage_progress_updated"), false},
		{"activity_note_added", backlog("activity_note_added"), false},
		{"session_attached without a role", backlog("session_attached"), false},
		{"session_state idle", domain.SessionStateSpec{SessionUUID: "p1", State: "idle"}, true},
		{"session_state nonsense", domain.SessionStateSpec{SessionUUID: "p1", State: "thinking"}, false},
		{"pr merged", domain.PREventSpec{Repo: "o/r", Number: 1, Kind: PRKindMerged}, true},
		{"pr opened", domain.PREventSpec{Repo: "o/r", Number: 1, Kind: "opened"}, false},
		{"ci success", domain.CIEventSpec{Repo: "o/r", Number: 1, Kind: CIKindSuccess}, true},
		{"ci running", domain.CIEventSpec{Repo: "o/r", Number: 1, Kind: "running"}, false},
		{"guidance answered", domain.GuidanceAnsweredSpec{GuidanceRequestID: "g1"}, true},
		{"at", domain.AtSpec{At: t0}, true},
		{"cron", domain.CronSpec{Expr: "0 * * * *"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSpec(tt.spec)
			if tt.ok {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrNoReconcilePredicate)
		})
	}
}

func TestBacklogKindHolds_should_EvaluatePersistedState_ForEverySupportedKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind   string
		true_  ItemState
		false_ ItemState
	}{
		{"status_reached:review", ItemState{Exists: true, Status: "review"}, ItemState{Exists: true, Status: "done"}},
		{"verdict_recorded", ItemState{Exists: true, HasVerdict: true}, ItemState{Exists: true}},
		{"item_archived", ItemState{Exists: true, Archived: true}, ItemState{Exists: true}},
		{"item_removed", ItemState{}, ItemState{Exists: true}},
		{"session_attached:review", ItemState{Exists: true, SessionRoles: []string{"work", "review"}}, ItemState{Exists: true, SessionRoles: []string{"work"}}},
	}
	covered := map[string]bool{}
	for _, tt := range tests {
		assert.True(t, backlogKindHolds(tt.kind, tt.true_), tt.kind)
		assert.False(t, backlogKindHolds(tt.kind, tt.false_), tt.kind)
		name, _ := splitKind(tt.kind)
		covered[name] = true
	}
	for _, r := range backlogKindRules {
		assert.True(t, covered[r.name], "no predicate case for %s", r.name)
	}
}

func TestBacklogEventKinds_should_TranslateBusEventsToSpecKinds(t *testing.T) {
	t.Parallel()
	attached := &events.BacklogItemEventPayload{
		Kind: events.BacklogChangeSessionAttached, SessionID: "s1",
		Item: &session.BacklogItemData{ID: itemX, ItemSessions: []session.ItemSessionSummary{{SessionUUID: "s1", Role: "review"}}},
	}
	assert.Equal(t, []string{"session_attached:review"}, backlogEventKinds(attached))
	assert.Equal(t, []string{"status_reached:review"}, backlogEventKinds(&events.BacklogItemEventPayload{Kind: events.BacklogChangeStatusTransition, NewStatus: "review"}))
	assert.Equal(t, []string{"verdict_recorded"}, backlogEventKinds(&events.BacklogItemEventPayload{Kind: events.BacklogChangeVerdictRecorded}))
	assert.Empty(t, backlogEventKinds(&events.BacklogItemEventPayload{Kind: events.BacklogChangeItemUpdated}))
}
