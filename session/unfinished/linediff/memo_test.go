package linediff

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
)

func TestFileDiffMemo_ShouldMiss_WhenSizeMtimeCtimeOrInodeChanges(t *testing.T) {
	base := fileStamp{Size: 4096, MtimeNs: 1.7e18, CtimeNs: 1.7e18, Inode: 7}
	head := plumbing.NewHash("1111111111111111111111111111111111111111")
	m := NewFileDiffMemo(0, func() time.Time { return time.Unix(0, 1.7e18).Add(time.Hour) })
	stamp := base
	m.stat = func(string) (fileStamp, bool) { return stamp, true }
	key := func(h plumbing.Hash) FileDiffMemoKey {
		k, ok := m.KeyFor("/w/a.go", h)
		if !ok {
			t.Fatal("keyFor refused a regular file")
		}
		return k
	}
	m.Store(key(head), EditCounts{Insertions: 1})
	if _, ok := m.Lookup(key(head)); !ok {
		t.Fatal("unchanged stamp missed")
	}
	for name, mutate := range map[string]func(*fileStamp){
		"size":  func(s *fileStamp) { s.Size++ },
		"mtime": func(s *fileStamp) { s.MtimeNs++ },
		"ctime": func(s *fileStamp) { s.CtimeNs++ },
		"inode": func(s *fileStamp) { s.Inode++ },
	} {
		stamp = base
		mutate(&stamp)
		if _, ok := m.Lookup(key(head)); ok {
			t.Errorf("memo hit after %s changed", name)
		}
	}
	stamp = base
	if _, ok := m.Lookup(key(plumbing.NewHash("2222222222222222222222222222222222222222"))); ok {
		t.Error("memo hit for a different HEAD blob")
	}
}

func TestFileDiffMemo_ShouldStayUnderBudgetAndCountEvictions_When10000Files(t *testing.T) {
	const files = 10_000
	budget := int64(256 << 10)
	m := NewFileDiffMemo(budget, func() time.Time { return time.Unix(0, 1.7e18).Add(time.Hour) })
	for i := range files {
		m.Store(FileDiffMemoKey{Path: fmt.Sprintf("/w/dir/file%05d.go", i), Size: 1, MtimeNs: 1.7e18, CtimeNs: 1.7e18, Inode: uint64(i)}, EditCounts{})
	}
	if got := m.bytes.Load(); got > budget {
		t.Errorf("bytes = %d, want <= budget %d", got, budget)
	}
	c := m.Counters()
	if c.Evictions == 0 {
		t.Error("no evictions counted although 10,000 entries exceed the budget")
	}
	if got := int64(m.m.Size()) + c.Evictions; got != files {
		t.Errorf("entries %d + evictions %d = %d, want %d", m.m.Size(), c.Evictions, got, files)
	}
	var sum int64
	m.m.Range(func(k FileDiffMemoKey, _ EditCounts) bool { sum += entryBytes(k); return true })
	if sum != m.bytes.Load() {
		t.Errorf("byte accounting drifted: tracked %d, actual %d", m.bytes.Load(), sum)
	}
}

func TestFileDiffMemo_ShouldRefuseKey_WhenPathDoesNotExist(t *testing.T) {
	if _, ok := NewFileDiffMemo(0, nil).KeyFor(filepath.Join(t.TempDir(), "missing"), plumbing.ZeroHash); ok {
		t.Error("keyFor accepted a path that does not exist")
	}
}
