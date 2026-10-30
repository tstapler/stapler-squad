package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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
	m  map[string][sha256.Size]byte
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
func (f *savedFingerprints) persist(data InstanceData, write func() bool) {
	sum, hashed := fingerprintInstanceData(data)
	f.mu.Lock()
	defer f.mu.Unlock()
	if hashed {
		if prev, seen := f.m[data.Title]; seen && prev == sum {
			return
		}
	}
	if !write() || !hashed {
		return
	}
	if f.m == nil {
		f.m = make(map[string][sha256.Size]byte)
	}
	f.m[data.Title] = sum
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
