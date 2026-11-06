package wakes

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ErrNoReconcilePredicate is returned for a (trigger_type, kind) that cannot be
// re-checked from persisted state: the EventBus drops events and is in-process,
// so a kind with nothing to re-evaluate could be missed forever (ADR-006 rule 4).
var ErrNoReconcilePredicate = errors.New("no_reconcile_predicate")

const kindArgSep = ":"

// backlogKindRule is one supported backlog_event spec kind: the bus event that
// fires it and the persisted-state predicate reconcile evaluates. An arg-taking
// kind is written "<name>:<arg>" (status_reached:review).
type backlogKindRule struct {
	name     string
	wire     events.BacklogChangeKind
	takesArg bool
	// eventArg is the arg an event satisfies, "" when it satisfies none.
	eventArg func(p *events.BacklogItemEventPayload) string
	holds    func(arg string, st ItemState) bool
}

var backlogKindRules = []backlogKindRule{
	{
		name: "status_reached", wire: events.BacklogChangeStatusTransition, takesArg: true,
		eventArg: func(p *events.BacklogItemEventPayload) string { return p.NewStatus },
		holds:    func(arg string, st ItemState) bool { return st.Exists && st.Status == arg },
	},
	{
		name: "verdict_recorded", wire: events.BacklogChangeVerdictRecorded,
		holds: func(_ string, st ItemState) bool { return st.Exists && st.HasVerdict },
	},
	{
		name: "item_archived", wire: events.BacklogChangeItemArchived,
		holds: func(_ string, st ItemState) bool { return st.Exists && st.Archived },
	},
	{
		name: "item_removed", wire: events.BacklogChangeItemRemoved,
		holds: func(_ string, st ItemState) bool { return !st.Exists },
	},
	{
		name: "session_attached", wire: events.BacklogChangeSessionAttached, takesArg: true,
		eventArg: attachedSessionRole,
		holds:    func(arg string, st ItemState) bool { return slices.Contains(st.SessionRoles, arg) },
	},
}

// rejectedBacklogWireKinds are bus kinds that are transient edges with no
// persisted state to re-evaluate. Together with the wire kinds of
// backlogKindRules they must cover every events.BacklogChangeKind
// (TestEveryKindHasReconcilePredicateOrIsRejected).
var rejectedBacklogWireKinds = []events.BacklogChangeKind{
	events.BacklogChangeItemUpdated,
	events.BacklogChangeTriageProgressUpdated,
	events.BacklogChangeActivityNoteAdded,
}

func attachedSessionRole(p *events.BacklogItemEventPayload) string {
	if p.Item == nil {
		return ""
	}
	for _, s := range p.Item.ItemSessions {
		if s.SessionUUID == p.SessionID {
			return s.Role
		}
	}
	return ""
}

func splitKind(kind string) (name, arg string) {
	name, arg, _ = strings.Cut(kind, kindArgSep)
	return name, arg
}

func backlogRuleFor(kind string) (backlogKindRule, string, bool) {
	name, arg := splitKind(kind)
	for _, r := range backlogKindRules {
		if r.name != name {
			continue
		}
		if r.takesArg != (arg != "") || strings.Contains(arg, kindArgSep) {
			return backlogKindRule{}, "", false
		}
		return r, arg, true
	}
	return backlogKindRule{}, "", false
}

// backlogKindHolds is the persisted-state predicate of a backlog_event kind;
// a kind with no predicate never holds.
func backlogKindHolds(kind string, st ItemState) bool {
	r, arg, ok := backlogRuleFor(kind)
	return ok && r.holds(arg, st)
}

// backlogEventKinds lists the spec kinds a bus event satisfies.
func backlogEventKinds(p *events.BacklogItemEventPayload) []string {
	var out []string
	for _, r := range backlogKindRules {
		if r.wire != p.Kind {
			continue
		}
		if !r.takesArg {
			out = append(out, r.name)
			continue
		}
		if arg := r.eventArg(p); arg != "" {
			out = append(out, r.name+kindArgSep+arg)
		}
	}
	return out
}

// PR and CI kinds (Spike S4: derived from the PR poller, not the bus).
const (
	PRKindMerged = "merged"
	PRKindClosed = "closed"

	CIKindSuccess   = "success"
	CIKindFailure   = "failure"
	CIKindCompleted = "completed"
)

// Session states a session_state wake can wait for. "idle" is a live session
// ready for input; the rest are lifecycle statuses.
var sessionStates = []string{"idle", "active", "paused", "stopped", "hibernated", "crashed", "failed", "permanently_failed"}

// IsSessionState reports whether s is a state a session_state wake accepts.
func IsSessionState(s string) bool { return slices.Contains(sessionStates, s) }

func noPredicate(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrNoReconcilePredicate, fmt.Sprintf(format, args...))
}

// ValidateSpec checks that spec has a persisted-state predicate reconcile can
// evaluate. Time-based and guidance specs always do.
func ValidateSpec(spec domain.WakeSpec) error {
	switch s := spec.(type) {
	case domain.BacklogEventSpec:
		if _, _, ok := backlogRuleFor(s.Kind); !ok {
			return noPredicate("backlog_event kind %q is not supported; use %s", s.Kind, supportedBacklogKinds())
		}
	case domain.SessionStateSpec:
		if !IsSessionState(s.State) {
			return noPredicate("session_state %q is not a known state; use one of %s", s.State, strings.Join(sessionStates, ", "))
		}
	case domain.PREventSpec:
		if s.Kind != PRKindMerged && s.Kind != PRKindClosed {
			return noPredicate("pr_event kind %q is not supported; use %s or %s", s.Kind, PRKindMerged, PRKindClosed)
		}
	case domain.CIEventSpec:
		if !slices.Contains([]string{CIKindSuccess, CIKindFailure, CIKindCompleted}, s.Kind) {
			return noPredicate("ci_event kind %q is not supported; use %s, %s or %s", s.Kind, CIKindSuccess, CIKindFailure, CIKindCompleted)
		}
	}
	return nil
}

func supportedBacklogKinds() string {
	names := make([]string, len(backlogKindRules))
	for i, r := range backlogKindRules {
		names[i] = r.name
		if r.takesArg {
			names[i] += kindArgSep + "<arg>"
		}
	}
	return strings.Join(names, ", ")
}
