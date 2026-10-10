package deliverygate

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

const (
	// tombstoneTTL is how long a deleted hidden session (and a renamed hidden
	// session's old title) keeps resolving as hidden, so a late routine event
	// from a still-running hook does not leak.
	tombstoneTTL = 24 * time.Hour
	// maxTombstones bounds the in-memory tombstone set (oldest evicted first).
	maxTombstones = 10_000

	tagBacklogReview   = "backlog:review"
	tagBacklogDiagnose = "backlog:diagnose"
	tagBacklogTriage   = "backlog:triage"
)

// Entry is one session as the gate sees it.
type Entry struct {
	UUID     string
	Title    string
	TmuxName string
	Hidden   bool
	Kind     HiddenKind // meaningful when Hidden
}

// KindFromTags derives the hidden kind with a total order
// review > diagnose > triage > other (tags are mutable and may co-occur).
func KindFromTags(tags []string) HiddenKind {
	switch {
	case slices.Contains(tags, tagBacklogReview):
		return KindReview
	case slices.Contains(tags, tagBacklogDiagnose):
		return KindDiagnose
	case slices.Contains(tags, tagBacklogTriage):
		return KindTriage
	default:
		return KindOther
	}
}

// EntryFromSnapshot builds an Entry from a lock-free instance snapshot.
func EntryFromSnapshot(s *session.InstanceSnapshot) Entry {
	e := Entry{
		UUID:   s.UUID,
		Title:  s.Title,
		Hidden: s.Hidden,
	}
	if s.Title != "" {
		e.TmuxName = tmux.NewSessionName(s.Title, s.TmuxPrefix).String()
	}
	if s.Hidden {
		e.Kind = KindFromTags(s.Tags)
	}
	return e
}

type aliasEntry struct {
	entry   *Entry
	expires time.Time
}

type tombstone struct {
	kind    HiddenKind
	expires time.Time
	seq     uint64
}

// indexState is immutable once published.
type indexState struct {
	keys    map[string]*Entry // uuid, title and tmux name -> entry
	aliases map[string]aliasEntry
	tombs   map[string]tombstone
	tombSeq uint64
}

func (s *indexState) clone() *indexState {
	n := &indexState{
		keys:    make(map[string]*Entry, len(s.keys)+4),
		aliases: make(map[string]aliasEntry, len(s.aliases)+1),
		tombs:   make(map[string]tombstone, len(s.tombs)+1),
		tombSeq: s.tombSeq,
	}
	for k, v := range s.keys {
		n.keys[k] = v
	}
	for k, v := range s.aliases {
		n.aliases[k] = v
	}
	for k, v := range s.tombs {
		n.tombs[k] = v
	}
	return n
}

// VisibilityIndex is a copy-on-write map of session identity keys to Entry.
// Reads are one atomic load. Writers take a private mutex that guards the
// copy-and-swap and calls nothing, so Upsert under Instance.mu cannot invert.
// Only positive results are stored: a miss is never cached.
type VisibilityIndex struct {
	state  atomic.Pointer[indexState]
	seeded atomic.Bool
	now    Clock
	// maxTombs is maxTombstones; a field so tests need not delete 10,000 sessions.
	maxTombs int

	writeMu sync.Mutex // guards copy-and-swap only; calls nothing while held
}

// NewVisibilityIndex returns an empty, unseeded index.
func NewVisibilityIndex(now Clock) *VisibilityIndex {
	i := &VisibilityIndex{now: now, maxTombs: maxTombstones}
	i.state.Store(&indexState{
		keys:    map[string]*Entry{},
		aliases: map[string]aliasEntry{},
		tombs:   map[string]tombstone{},
	})
	return i
}

// Seeded reports whether Replace (the synchronous startup seed) has run.
func (i *VisibilityIndex) Seeded() bool { return i.seeded.Load() }

// Len is the number of identity keys (diagnostics, benchmarks).
func (i *VisibilityIndex) Len() int { return len(i.state.Load().keys) }

// Lookup resolves one identity string: live entry, then rename alias, then tombstone.
func (i *VisibilityIndex) Lookup(key string) (Entry, bool) {
	if key == "" {
		return Entry{}, false
	}
	s := i.state.Load()
	if e, ok := s.keys[key]; ok {
		return *e, true
	}
	now := i.now()
	if a, ok := s.aliases[key]; ok && now.Before(a.expires) {
		return *a.entry, true
	}
	if t, ok := s.tombs[key]; ok && now.Before(t.expires) {
		return Entry{Hidden: true, Kind: t.kind}, true
	}
	return Entry{}, false
}

// Upsert adds or replaces a session. A visible entry evicts any tombstone or
// alias on its keys so title reuse cannot make a visible session hidden.
func (i *VisibilityIndex) Upsert(e Entry) {
	if e.UUID == "" && e.Title == "" {
		return
	}
	i.writeMu.Lock()
	defer i.writeMu.Unlock()
	s := i.state.Load().clone()
	i.upsertLocked(s, e)
	i.state.Store(s)
}

func (i *VisibilityIndex) upsertLocked(s *indexState, e Entry) {
	entry := e
	if prev, ok := s.keys[e.UUID]; ok && e.UUID != "" {
		i.aliasRenamedKeys(s, prev, &entry)
	}
	for _, k := range []string{e.UUID, e.Title, e.TmuxName} {
		if k == "" {
			continue
		}
		s.keys[k] = &entry
		delete(s.aliases, k)
		delete(s.tombs, k)
	}
}

// aliasRenamedKeys keeps the old identity strings of a renamed hidden session
// resolving (running hooks carry the title they started with).
func (i *VisibilityIndex) aliasRenamedKeys(s *indexState, prev, next *Entry) {
	for _, old := range []string{prev.Title, prev.TmuxName} {
		if old == "" || old == next.Title || old == next.TmuxName {
			continue
		}
		delete(s.keys, old)
		if prev.Hidden {
			s.aliases[old] = aliasEntry{entry: next, expires: i.now().Add(tombstoneTTL)}
		}
	}
}

// Remove deletes a session; a hidden one leaves tombstones on all its keys.
func (i *VisibilityIndex) Remove(uuid string) {
	i.writeMu.Lock()
	defer i.writeMu.Unlock()
	cur := i.state.Load()
	prev, ok := cur.keys[uuid]
	if !ok || uuid == "" {
		return
	}
	s := cur.clone()
	for _, k := range []string{prev.UUID, prev.Title, prev.TmuxName} {
		if k == "" {
			continue
		}
		delete(s.keys, k)
		if prev.Hidden {
			s.tombSeq++
			s.tombs[k] = tombstone{kind: prev.Kind, expires: i.now().Add(tombstoneTTL), seq: s.tombSeq}
		}
	}
	evictOldestTombstones(s, i.maxTombs)
	i.state.Store(s)
}

func evictOldestTombstones(s *indexState, limit int) {
	for len(s.tombs) > limit {
		var oldestKey string
		oldest := ^uint64(0)
		for k, t := range s.tombs {
			if t.seq < oldest {
				oldest, oldestKey = t.seq, k
			}
		}
		delete(s.tombs, oldestKey)
	}
}

// Replace is the batch seed: O(n), marks the index seeded. Existing aliases and
// tombstones are kept.
func (i *VisibilityIndex) Replace(entries []Entry) {
	i.writeMu.Lock()
	defer i.writeMu.Unlock()
	cur := i.state.Load()
	s := &indexState{
		keys:    make(map[string]*Entry, 3*len(entries)),
		aliases: cur.aliases,
		tombs:   cur.tombs,
		tombSeq: cur.tombSeq,
	}
	for _, e := range entries {
		entry := e
		for _, k := range []string{e.UUID, e.Title, e.TmuxName} {
			if k != "" {
				s.keys[k] = &entry
			}
		}
	}
	i.state.Store(s)
	i.seeded.Store(true)
}

// RangeKeys is a test and diagnostics hook.
func (i *VisibilityIndex) RangeKeys(fn func(key string, e Entry)) {
	for k, e := range i.state.Load().keys {
		fn(k, *e)
	}
}

// UpsertMany upserts a batch under one copy-and-swap (refresh backstop).
func (i *VisibilityIndex) UpsertMany(entries []Entry) {
	if len(entries) == 0 {
		return
	}
	i.writeMu.Lock()
	defer i.writeMu.Unlock()
	s := i.state.Load().clone()
	for _, e := range entries {
		if e.UUID == "" && e.Title == "" {
			continue
		}
		i.upsertLocked(s, e)
	}
	i.state.Store(s)
}

func entryFromData(d session.InstanceData) Entry {
	e := Entry{UUID: d.UUID, Title: d.Title, Hidden: d.Hidden}
	if d.Title != "" {
		e.TmuxName = tmux.NewSessionName(d.Title, d.TmuxPrefix).String()
	}
	if d.Hidden {
		e.Kind = KindFromTags(d.Tags)
	}
	return e
}
