package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
)

// savedFingerprints remembers a hash of the InstanceData last persisted per
// title, so a save of an unchanged instance can skip EntRepository.Update. That
// call is one transaction of ~19 statements (session SELECT, tag delete and
// re-insert, diff_stats upsert, claude_sessions update, ...) queued on the
// single writer connection; callers such as SaveInstances(allInstances()) and
// per-event saves re-persisted idle sessions byte for byte.
type savedFingerprints struct {
	mu sync.Mutex
	m  map[string]savedEntry
}

// savedEntry is the whole-row fingerprint plus one per child-table section, so
// a save where only session columns changed (timestamps, output signature)
// skips the tag delete/re-insert, worktree, diff_stats and claude_sessions
// statements instead of rewriting them byte for byte.
type savedEntry struct {
	whole [sha256.Size]byte
	secs  sectionSums
}

type sectionSums struct {
	diff, tags, worktree, claude [sha256.Size]byte
}

// updateSkip names the child-table sections Update must leave alone because the
// row already holds exactly this data.
type updateSkip struct {
	DiffStats, Tags, Worktree, Claude bool
}

func hashJSON(parts ...any) (sum [sha256.Size]byte) {
	h := sha256.New()
	for _, p := range parts {
		raw, err := json.Marshal(p)
		if err != nil {
			return sum // zero sum never equals a recorded one (see sectionsOf)
		}
		_, _ = h.Write(raw)
	}
	copy(sum[:], h.Sum(nil))
	return sum
}

// sectionsOf hashes the child-table sections of data. DiffStats.Content is
// json:"-" but Update persists it, so it is hashed explicitly.
func sectionsOf(data InstanceData) sectionSums {
	tags := slices.Clone(data.Tags)
	slices.Sort(tags)
	return sectionSums{
		diff:     hashJSON(data.DiffStats, data.DiffStats.Content),
		tags:     hashJSON(tags),
		worktree: hashJSON(data.Worktree),
		claude:   hashJSON(data.ClaudeSession),
	}
}

func (s sectionSums) skipAgainst(prev sectionSums) updateSkip {
	var zero [sha256.Size]byte
	same := func(a, b [sha256.Size]byte) bool { return a == b && a != zero }
	return updateSkip{
		DiffStats: same(s.diff, prev.diff),
		Tags:      same(s.tags, prev.tags),
		Worktree:  same(s.worktree, prev.worktree),
		Claude:    same(s.claude, prev.claude),
	}
}

// fingerprintInstanceData hashes everything persisted except UpdatedAt, which
// ToInstanceData stamps with time.Now() on every call. ok is false when the
// data cannot be marshalled; callers then always write.
func fingerprintInstanceData(data InstanceData) (sum [sha256.Size]byte, ok bool) {
	data.UpdatedAt = time.Time{}
	raw, err := json.Marshal(data)
	if err != nil {
		return sum, false
	}
	// DiffStats.Content is json:"-" but Update persists it, so hash it separately.
	h := sha256.New()
	_, _ = h.Write(raw)
	_, _ = h.Write([]byte(data.DiffStats.Content))
	copy(sum[:], h.Sum(nil))
	return sum, true
}

// persist runs write unless data matches what was last written for its title,
// and records the fingerprint only when write reports success. The mutex spans
// check, write and record so the recorded fingerprint always describes the
// last row written; without it, two racing saves could leave a stale
// fingerprint that makes a later save of newer data look unchanged. Writes
// already serialize on the single writer connection, so this adds no waiting.
func (f *savedFingerprints) persist(data InstanceData, write func(skip updateSkip) bool) {
	sum, hashed := fingerprintInstanceData(data)
	secs := sectionsOf(data)
	f.mu.Lock()
	defer f.mu.Unlock()
	var skip updateSkip
	if prev, seen := f.m[data.Title]; seen {
		if hashed && prev.whole == sum {
			return
		}
		skip = secs.skipAgainst(prev.secs)
	}
	ok := write(skip)
	if !hashed {
		delete(f.m, data.Title) // row changed but nothing to compare next time
		return
	}
	if !ok {
		return
	}
	if f.m == nil {
		f.m = make(map[string]savedEntry)
	}
	f.m[data.Title] = savedEntry{whole: sum, secs: secs}
}

// forget must be called whenever a row is deleted or renamed away so a
// re-created session with the same title is written, not skipped.
func (f *savedFingerprints) forget(title string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, title)
}

// reset drops every fingerprint, for row writers that only know a session id.
func (f *savedFingerprints) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m = nil
}

// writeRow runs a direct row write and clears the title's fingerprint on both
// sides of it. Clearing only before leaves a window where a concurrent persist
// re-records the pre-write fingerprint, so the caller's follow-up re-save of
// the unchanged in-memory instance would be skipped.
func (s *Storage) writeRow(title string, write func() error) error {
	s.saved.forget(title)
	err := write()
	s.saved.forget(title)
	return err
}

func (s *Storage) forgetAll(titles []string) {
	for _, t := range titles {
		s.saved.forget(t)
	}
}

// titlesMatchingID resolves an id (uuid or title) to row titles so id-keyed
// writers clear only the affected fingerprints; on lookup failure it clears all.
func (s *Storage) titlesMatchingID(ctx context.Context, client *ent.Client, id string) []string {
	titles, err := client.Session.Query().
		Where(entsession.Or(entsession.UUID(id), entsession.Title(id))).
		Select(entsession.FieldTitle).
		Strings(ctx)
	if err != nil {
		s.saved.reset()
		return nil
	}
	return titles
}
