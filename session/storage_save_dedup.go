package session

import (
	"encoding/json"
	"hash/fnv"
	"sync"
	"time"
)

// savedFingerprints remembers a hash of the InstanceData last persisted per
// title, so a save of an unchanged instance can skip EntRepository.Update. That
// call is one transaction of ~19 statements (session SELECT, tag delete and
// re-insert, diff_stats upsert, claude_sessions update, ...) queued on the
// single writer connection; callers such as SaveInstances(allInstances()) and
// per-event saves re-persisted idle sessions byte for byte.
type savedFingerprints struct {
	mu sync.Mutex
	m  map[string]uint64
}

// fingerprintInstanceData hashes everything persisted except UpdatedAt, which
// ToInstanceData stamps with time.Now() on every call. ok is false when the
// data cannot be marshalled; callers then always write.
func fingerprintInstanceData(data InstanceData) (sum uint64, ok bool) {
	data.UpdatedAt = time.Time{}
	raw, err := json.Marshal(data)
	if err != nil {
		return 0, false
	}
	h := fnv.New64a()
	_, _ = h.Write(raw)
	return h.Sum64(), true
}

// unchanged reports whether data matches what was last recorded for its title.
func (f *savedFingerprints) unchanged(title string, sum uint64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	prev, seen := f.m[title]
	return seen && prev == sum
}

func (f *savedFingerprints) record(title string, sum uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = make(map[string]uint64)
	}
	f.m[title] = sum
}

// forget must be called whenever a row is deleted or renamed away so a
// re-created session with the same title is written, not skipped.
func (f *savedFingerprints) forget(title string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, title)
}
