package dbobs

import (
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
)

// maxCallerLabels bounds metric cardinality: the repo has a few hundred distinct
// query call sites; anything past the cap is folded into callerOverflow.
const (
	maxCallerLabels = 256
	callerOverflow  = "other"
)

var (
	callerLabelMu   sync.Mutex
	callerLabelSeen = map[string]struct{}{}
)

// callerLabel reduces a "leaf<-parent<-grandparent" caller chain to its leaf
// (the repository method that issued the query), so Grafana can rank query cost
// by call site. Bounded; unknown callers share "unknown".
func callerLabel(caller string) string {
	leaf, _, _ := strings.Cut(caller, "<-")
	if leaf == "" {
		leaf = "unknown"
	}
	callerLabelMu.Lock()
	defer callerLabelMu.Unlock()
	if _, ok := callerLabelSeen[leaf]; ok {
		return leaf
	}
	if len(callerLabelSeen) >= maxCallerLabels {
		return callerOverflow
	}
	callerLabelSeen[leaf] = struct{}{}
	return leaf
}

func callerAttr(caller string) attribute.KeyValue {
	return attribute.String("db.caller", callerLabel(caller))
}
