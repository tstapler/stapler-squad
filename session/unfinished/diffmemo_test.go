package unfinished

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// useDiffMemo installs a fresh process-wide memo whose clock reads `ahead`
// past the real now. A backdated mtime (testrepo.Age) cannot change ctime, so
// the clock must run >= 10 s ahead for both to read as old at store time.
func useDiffMemo(t *testing.T, budget int64, ahead time.Duration) *fileDiffMemo {
	t.Helper()
	m := newFileDiffMemo(budget, func() time.Time { return time.Now().Add(ahead) })
	prev := activeDiffMemo.Load()
	activeDiffMemo.Store(m)
	t.Cleanup(func() { activeDiffMemo.Store(prev) })
	return m
}

const memoFile = "a.go"

func numberedLines(n int, prefix string) string {
	var sb strings.Builder
	for i := range n {
		fmt.Fprintf(&sb, "%s %04d\n", prefix, i)
	}
	return sb.String()
}

// dirtyRepo commits memoFile (~4 KB) and rewrites it with a few changed lines,
// backdated so the racy guard does not fire under a clock 1 h ahead.
func dirtyRepo(t *testing.T) (dir, path string) {
	t.Helper()
	repo, dir := testrepo.NewRepo(t)
	base := numberedLines(300, "line")
	testrepo.Commit(t, repo, map[string]string{memoFile: base}, "init", testrepo.Epoch)
	path = filepath.Join(dir, memoFile)
	writeAged(t, path, strings.Replace(strings.Replace(base, "line 0007", "CHANGED", 1), "line 0100", "CHANGED2", 1))
	return dir, path
}

func writeAged(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	testrepo.Age(t, path, time.Hour)
}

func TestFileDiffMemo_ShouldDoZeroBlobReadsAndDiffs_WhenFileUnchanged(t *testing.T) {
	memo := useDiffMemo(t, 0, time.Hour)
	dir, _ := dirtyRepo(t)
	r := NewCountingReader(Flags{DiffCounter: true})
	r.Track(t, dir)

	first, err := r.diffShortstatUncached(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Files != 1 || first.Insertions != 2 || first.Deletions != 2 {
		t.Fatalf("first = %+v, want 1 file +2 -2", first)
	}
	afterFirst, blobsFirst := memo.counters(), r.BlobReads()
	if afterFirst.Diffs != 1 || afterFirst.Hits != 0 {
		t.Fatalf("first poll counters = %+v, want 1 diff and 0 hits", afterFirst)
	}

	second, err := r.diffShortstatUncached(dir)
	if err != nil {
		t.Fatal(err)
	}
	after := memo.counters()
	if second != first {
		t.Errorf("memoized result %+v != computed %+v", second, first)
	}
	if after.Hits <= 0 { // engagement: the racy guard must not keep the memo bypassed (GA2)
		t.Errorf("memo hits = %d, want > 0", after.Hits)
	}
	if after.Diffs != afterFirst.Diffs {
		t.Errorf("diffs went %d -> %d on an unchanged file, want no new diff", afterFirst.Diffs, after.Diffs)
	}
	if got := r.BlobReads() - blobsFirst; got != 0 {
		t.Errorf("blob reads on the memoized poll = %d, want 0", got)
	}
}

func TestFileDiffMemo_ShouldNotEngage_WhenDiffCounterOff(t *testing.T) {
	memo := useDiffMemo(t, 0, time.Hour)
	dir, _ := dirtyRepo(t)
	r := readerWithFlags(Flags{DiffCounter: false}, &GoGitVCSReader{})
	for range 2 {
		if _, err := r.diffShortstatUncached(dir); err != nil {
			t.Fatal(err)
		}
	}
	if c := memo.counters(); c != (DiffMemoCounters{}) {
		t.Errorf("memo touched with the flag off: %+v", c)
	}
}

func TestFileDiffMemo_ShouldMatchLegacyCounts_WhenDiffCounterOn(t *testing.T) {
	repo, dir := testrepo.NewRepo(t)
	files := map[string]string{}
	for i := range 8 {
		files[fmt.Sprintf("f%d.txt", i)] = numberedLines(120+i, "row")
	}
	files["gone.txt"] = "x\ny\nz\n"
	files["staged.txt"] = "one\ntwo\nthree\n"
	testrepo.Commit(t, repo, files, "init", testrepo.Epoch)
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("f0.txt", numberedLines(120, "row")+"extra\n")                 // append
	write("f1.txt", "")                                                  // emptied
	write("f2.txt", strings.ToUpper(numberedLines(122, "row")))          // fully rewritten
	write("f3.txt", strings.TrimSuffix(numberedLines(123, "row"), "\n")) // no trailing newline
	write("new1.txt", numberedLines(50, "u"))
	write("new2.txt", "no newline")
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	write("staged.txt", "one\nTWO\nthree\nfour\n")
	if _, err := wt.Add("staged.txt"); err != nil {
		t.Fatal(err)
	}

	want, err := (readerWithFlags(Flags{DiffCounter: false}, &GoGitVCSReader{})).diffShortstatUncached(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"f0.txt", "f2.txt", "f3.txt", "staged.txt"} {
		testrepo.Age(t, filepath.Join(dir, name), time.Hour) // keep the 2 s racy guard from bypassing the memo
	}
	memo := useDiffMemo(t, 0, time.Hour)
	defer func() {
		if memo.counters().Hits == 0 {
			t.Error("memo never hit: the second pass did not exercise it")
		}
	}()
	r := readerWithFlags(Flags{DiffCounter: true}, &GoGitVCSReader{})
	for pass := range 2 { // second pass exercises memo hits
		got, err := r.diffShortstatUncached(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("pass %d: diff_counter on = %+v, legacy = %+v", pass, got, want)
		}
	}
}

func TestFileDiffMemo_ShouldMiss_WhenSizeMtimeCtimeOrInodeChanges(t *testing.T) {
	base := fileStamp{Size: 4096, MtimeNs: 1.7e18, CtimeNs: 1.7e18, Inode: 7}
	head := plumbing.NewHash("1111111111111111111111111111111111111111")
	m := newFileDiffMemo(0, func() time.Time { return time.Unix(0, 1.7e18).Add(time.Hour) })
	stamp := base
	m.stat = func(string) (fileStamp, bool) { return stamp, true }
	key := func(h plumbing.Hash) FileDiffMemoKey {
		k, ok := m.keyFor("/w/a.go", h)
		if !ok {
			t.Fatal("keyFor refused a regular file")
		}
		return k
	}
	m.store(key(head), EditCounts{Insertions: 1})
	if _, ok := m.lookup(key(head)); !ok {
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
		if _, ok := m.lookup(key(head)); ok {
			t.Errorf("memo hit after %s changed", name)
		}
	}
	stamp = base
	if _, ok := m.lookup(key(plumbing.NewHash("2222222222222222222222222222222222222222"))); ok {
		t.Error("memo hit for a different HEAD blob")
	}
}

func TestFileDiffMemo_ShouldMiss_WhenEqualLengthEditRestoresMtime(t *testing.T) {
	memo := useDiffMemo(t, 0, time.Hour)
	dir, path := dirtyRepo(t)
	r := readerWithFlags(Flags{DiffCounter: true}, &GoGitVCSReader{})
	first, err := r.diffShortstatUncached(dir)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Same length, a third line now differs from HEAD; then put mtime back.
	edited := strings.Replace(string(body), "line 0200", "LINE 0200", 1)
	if len(edited) != len(body) || edited == string(body) {
		t.Fatal("fixture edit must keep the length and change the content")
	}
	newCtimeTicker(t).next() // coarse ctime: the rewrite must not share the stored entry's tick
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	diffsBefore := memo.counters().Diffs
	second, err := r.diffShortstatUncached(dir)
	if err != nil {
		t.Fatal(err)
	}
	if memo.counters().Diffs != diffsBefore+1 {
		t.Error("memo trusted a file rewritten with identical size and restored mtime")
	}
	if second.Insertions != first.Insertions+1 || second.Deletions != first.Deletions+1 {
		t.Errorf("second = %+v, want one more insertion and deletion than %+v", second, first)
	}
}

func TestFileDiffMemo_ShouldNotTrust_WhenModifiedWithin2sOfStore(t *testing.T) {
	dir, path := dirtyRepo(t)
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// The clock reads 1 s after the file's mtime, inside the 2 s window.
	storeTime := fi.ModTime().Add(time.Second)
	memo := useDiffMemo(t, 0, 0)
	memo.now = func() time.Time { return storeTime }
	r := readerWithFlags(Flags{DiffCounter: true}, &GoGitVCSReader{})
	for range 2 {
		if _, err := r.diffShortstatUncached(dir); err != nil {
			t.Fatal(err)
		}
	}
	c := memo.counters()
	if c.Hits != 0 || c.Diffs != 2 || c.RacySkips == 0 {
		t.Errorf("counters = %+v, want 0 hits, 2 diffs, racy skips > 0", c)
	}
}

func TestFileDiffMemo_ShouldBeDisabled_WhenCtimeUnavailable(t *testing.T) {
	memo := useDiffMemo(t, 0, time.Hour)
	memo.stat = func(string) (fileStamp, bool) { return fileStamp{}, false } // the !unix build's behaviour
	dir, _ := dirtyRepo(t)
	r := readerWithFlags(Flags{DiffCounter: true}, &GoGitVCSReader{})
	for range 2 {
		if _, err := r.diffShortstatUncached(dir); err != nil {
			t.Fatal(err)
		}
	}
	if c := memo.counters(); c.Hits != 0 || c.Misses != 0 || c.Diffs != 2 {
		t.Errorf("counters = %+v, want the memo never consulted and 2 diffs", c)
	}
	if _, ok := (&fileDiffMemo{stat: statFileStamp}).keyFor(filepath.Join(t.TempDir(), "missing"), plumbing.ZeroHash); ok {
		t.Error("keyFor accepted a path that does not exist")
	}
}

func TestFileDiffMemo_ShouldStayUnderBudgetAndCountEvictions_When10000Files(t *testing.T) {
	const files = 10_000
	budget := int64(256 << 10)
	m := newFileDiffMemo(budget, func() time.Time { return time.Unix(0, 1.7e18).Add(time.Hour) })
	for i := range files {
		m.store(FileDiffMemoKey{Path: fmt.Sprintf("/w/dir/file%05d.go", i), Size: 1, MtimeNs: 1.7e18, CtimeNs: 1.7e18, Inode: uint64(i)}, EditCounts{})
	}
	if got := m.bytes.Load(); got > budget {
		t.Errorf("bytes = %d, want <= budget %d", got, budget)
	}
	c := m.counters()
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

// A same-size rewrite inside the second the index entry was recorded keeps the
// truncated mtime equal, and a later index rewrite (go-git does not smudge racy
// entries) pushes indexMtime past it. Only the nanosecond part tells them apart.
func TestUnstagedTargetList_ShouldFlagSameSizeRewrite_WhenOnlyNanosecondsDiffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	recorded := time.Date(2026, 1, 1, 12, 0, 0, 100, time.UTC)
	if err := os.WriteFile(path, []byte("bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, recorded.Add(700*time.Microsecond), recorded.Add(700*time.Microsecond)); err != nil {
		t.Fatal(err)
	}
	m := indexMeta{
		name:       "f.txt",
		indexHash:  plumbing.ComputeHash(plumbing.BlobObject, []byte("aaaa")),
		size:       4,
		modifiedAt: recorded,
	}
	got := (&GoGitVCSReader{}).unstagedTargetList(dir, []indexMeta{m}, recorded.Add(10*time.Second), nil)
	if len(got) != 1 || got[0].name != "f.txt" {
		t.Fatalf("targets = %+v, want f.txt flagged as changed", got)
	}

	// Same bytes as the index, only the mtime moved within the second: not a change.
	if err := os.WriteFile(path, []byte("aaaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, recorded.Add(700*time.Microsecond), recorded.Add(700*time.Microsecond)); err != nil {
		t.Fatal(err)
	}
	if got := (&GoGitVCSReader{}).unstagedTargetList(dir, []indexMeta{m}, recorded.Add(10*time.Second), nil); len(got) != 0 {
		t.Fatalf("targets = %+v, want none for identical content", got)
	}
}

// A file whose nanoseconds differ from the index but is otherwise stat-clean and
// outside the racy window is hashed every scan; that cost is counted.
func TestUnstagedTargetList_ShouldCountForcedHashes_WhenOnlyNanosMoved(t *testing.T) {
	dir := t.TempDir()
	recorded := time.Date(2026, 1, 1, 12, 0, 0, 100, time.UTC)
	write := func(name string, nanos time.Duration) indexMeta {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("aaaa"), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := recorded.Add(nanos)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
		return indexMeta{name: name, indexHash: plumbing.ComputeHash(plumbing.BlobObject, []byte("aaaa")), size: 4, modifiedAt: recorded}
	}
	moved := write("moved.txt", 700*time.Microsecond)
	same := write("same.txt", 0) // identical nanoseconds: no hash forced
	g := &GoGitVCSReader{}
	for scan := 1; scan <= 2; scan++ {
		g.unstagedTargetList(dir, []indexMeta{moved, same}, recorded.Add(10*time.Second), nil)
		if got := g.NanosForcedHashes(); got != int64(scan) {
			t.Fatalf("scan %d: forced hashes = %d, want %d (moved.txt only)", scan, got, scan)
		}
	}
}
