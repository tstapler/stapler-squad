package deliverygate

import "github.com/tstapler/stapler-squad/pkg/events"

// RowClass is what the index can say about the session a stored row belongs to.
type RowClass int

const (
	// RowUndeterminable covers Unresolved and NotASession rows and rows whose
	// identity strings resolve to a hidden and a visible session; the prune
	// always keeps them.
	RowUndeterminable RowClass = iota
	RowVisible
	RowHidden
)

// RowResolution is ClassifyStored's answer for one stored notification row.
type RowResolution struct {
	Class RowClass
	Kind  HiddenKind
	Title string
	// Form is the identity form the row matched by (uuid, title, tmux, alias
	// or tombstone).
	Form MatchForm
	// ByAlias is true when the row matched only through a rename alias or a
	// tombstone, so the operator can see it before applying.
	ByAlias bool
}

// ClassifyStored resolves a stored row's session_id (and item_id stamp)
// against the index without side effects: unlike Resolve it records no miss
// and starts no refresh, so a prune dry run cannot perturb delivery counters.
// The store holds whichever ID form each producer used (UUID, title or tmux
// name) and the index resolves all of them. A row whose two identity strings
// resolve to a hidden and a visible session is RowUndeterminable.
func (r *Resolver) ClassifyStored(sessionID string, metadata map[string]string) RowResolution {
	byID, okID := r.lookupStored(sessionID)
	byItem, okItem := r.lookupStored(metadata[events.MetadataKeyItemID])
	switch {
	case okID && okItem:
		if (byID.Class == RowHidden) != (byItem.Class == RowHidden) {
			return RowResolution{Class: RowUndeterminable}
		}
		return byID
	case okID:
		return byID
	case okItem:
		return byItem
	default:
		return RowResolution{Class: RowUndeterminable}
	}
}

func (r *Resolver) lookupStored(key string) (RowResolution, bool) {
	e, form, ok := r.index.LookupForm(key)
	if !ok {
		return RowResolution{}, false
	}
	res := RowResolution{Class: RowVisible, Title: e.Title, Form: form, ByAlias: form == FormAlias || form == FormTombstone}
	if e.Hidden {
		res.Class, res.Kind = RowHidden, e.Kind
	}
	return res, true
}
