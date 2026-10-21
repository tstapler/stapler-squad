//go:build unix

package unfinished

// Story 4.2.1 spike: index TREE (cache-tree) extension and stat-signature
// feasibility, using go-git-written indexes only (hard policy: no native git).
// Findings are recorded in ADR-006's addendum; the experiments stay as a
// regression record of what go-git writes and tolerates.

import (
	"bytes"
	"crypto/sha1" // #nosec G505 -- the index trailer is SHA-1 by format, not a security use
	"encoding/binary"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// spikeIndexSig is the candidate IndexStatSig (mtime ns, size, inode).
type spikeIndexSig struct {
	mtimeNs int64
	size    int64
	inode   uint64
}

func readSpikeIndexSig(t *testing.T, path string) spikeIndexSig {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return spikeIndexSig{fi.ModTime().UnixNano(), fi.Size(), fi.Sys().(*syscall.Stat_t).Ino}
}

func decodeSpikeIndex(t *testing.T, raw []byte) *index.Index {
	t.Helper()
	idx := &index.Index{}
	if err := index.NewDecoder(bytes.NewReader(raw)).Decode(idx); err != nil {
		t.Fatalf("index.Decode: %v", err)
	}
	return idx
}

func TestIndexSpike_ShouldHaveNoCacheTree_WhenIndexWrittenByGoGit(t *testing.T) {
	repo, dir := testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"a.txt": "a\n", "d/b.txt": "b\n"}, "init", testrepo.Epoch)
	indexPath := filepath.Join(dir, ".git", "index")

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	clean := decodeSpikeIndex(t, raw)
	if clean.Cache != nil {
		t.Fatalf("go-git wrote a TREE extension after Add+Commit (%d entries); the 4.2.1 finding no longer holds", len(clean.Cache.Entries))
	}
	if bytes.Contains(raw, []byte("TREE")) {
		t.Error("raw index bytes contain a TREE signature although Decode found none")
	}

	// A staged change rewrites the index; still no extension.
	testrepo.WriteFiles(t, dir, map[string]string{"a.txt": "changed\n"})
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("a.txt"); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if staged := decodeSpikeIndex(t, raw); staged.Cache != nil || len(staged.Entries) != 2 {
		t.Errorf("staged index: Cache=%v entries=%d, want nil and 2", staged.Cache, len(staged.Entries))
	}
}

// A handcrafted TREE extension (the shape git itself writes) is parsed into
// index.Tree, so an absent extension and a present one are both tolerated.
func TestIndexSpike_ShouldParseTreeExtension_WhenPresentAndTolerateAbsence(t *testing.T) {
	repo, dir := testrepo.NewRepo(t)
	head := testrepo.Commit(t, repo, map[string]string{"a.txt": "a\n"}, "init", testrepo.Epoch)
	commit, err := repo.CommitObject(head)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}

	payload := append([]byte("\x001 0\n"), commit.TreeHash[:]...) // root: path "", 1 entry, 0 subtrees
	ext := append([]byte("TREE"), binary.BigEndian.AppendUint32(nil, uint32(len(payload)))...)
	ext = append(ext, payload...)
	body := append(append([]byte(nil), raw[:len(raw)-sha1.Size]...), ext...)
	sum := sha1.Sum(body) // #nosec G401 -- format-mandated checksum
	crafted := decodeSpikeIndex(t, append(body, sum[:]...))

	if crafted.Cache == nil || len(crafted.Cache.Entries) != 1 {
		t.Fatalf("Cache = %+v, want one root entry", crafted.Cache)
	}
	if got := crafted.Cache.Entries[0]; got.Hash != commit.TreeHash || got.Entries != 1 || got.Trees != 0 || got.Path != "" {
		t.Errorf("root cache-tree entry = %+v, want hash %s entries 1 trees 0", got, commit.TreeHash)
	}
	if absent := decodeSpikeIndex(t, raw); absent.Cache != nil || len(absent.Entries) != len(crafted.Entries) {
		t.Errorf("index without extension: Cache=%v entries=%d", absent.Cache, len(absent.Entries))
	}
}

func TestIndexSpike_ShouldShowFalseEqualSignature_WhenRewriteRestoresMtimeAndKeepsSize(t *testing.T) {
	repo, dir := testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"a.txt": "aaaa\n"}, "init", testrepo.Epoch)
	indexPath := filepath.Join(dir, ".git", "index")
	before := readSpikeIndexSig(t, indexPath)
	rawBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	// Same-length content change: the rewritten index has identical size.
	testrepo.WriteFiles(t, dir, map[string]string{"a.txt": "bbbb\n"})
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("a.txt"); err != nil {
		t.Fatal(err)
	}
	rawAfter, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	after := readSpikeIndexSig(t, indexPath)
	if bytes.Equal(rawBefore, rawAfter) {
		t.Fatal("index content did not change")
	}
	if after.size != before.size {
		t.Fatalf("size differs (%d vs %d); fixture must keep the index length", before.size, after.size)
	}
	t.Logf("go-git rewrite keeps inode: %v (before=%d after=%d)", after.inode == before.inode, before.inode, after.inode)

	// Where timestamps are coarse or restored, (mtime, size, inode) cannot
	// tell the two contents apart: the false-equal case the racy guard covers.
	if err := os.Chtimes(indexPath, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if restored := readSpikeIndexSig(t, indexPath); after.inode != before.inode || restored != before {
		t.Skipf("signature differs (inode %d->%d); the false-equal case needs an in-place rewrite", before.inode, after.inode)
	}
	if racyIndexSigTrusted(fi.ModTime(), fi.ModTime().Add(500*time.Millisecond), 2*time.Second) {
		t.Error("racy guard trusted a signature parsed 0.5s after the mtime")
	}
	if !racyIndexSigTrusted(fi.ModTime(), fi.ModTime().Add(10*time.Second), 2*time.Second) {
		t.Error("racy guard distrusted a signature parsed 10s after the mtime")
	}
}

// racyIndexSigTrusted is the spike's model of the recommended guard: an equal
// signature only proves "unchanged" when the index mtime is older than the
// parse time by more than window (git's racy-index rule).
func racyIndexSigTrusted(mtime, parsedAt time.Time, window time.Duration) bool {
	return parsedAt.Sub(mtime) > window
}
