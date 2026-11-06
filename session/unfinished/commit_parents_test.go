package unfinished

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

const (
	cpTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	cpP1   = "1111111111111111111111111111111111111111"
	cpP2   = "2222222222222222222222222222222222222222"
	cpP3   = "3333333333333333333333333333333333333333"
	cpSig  = "committer T <t@example.com> 1700000000 +0100\n"
	cpAut  = "author A <a@example.com> 1600000000 -0230\n"
)

// commitParentsCorpus is the single table of raw commit payloads shared by
// every parents adapter's differential test (stock, parentMemo, fork).
func commitParentsCorpus() map[string]string {
	hdr := "tree " + cpTree + "\n"
	return map[string]string{
		"zero parents":         hdr + cpAut + cpSig + "\nroot\n",
		"one parent":           hdr + "parent " + cpP1 + "\n" + cpAut + cpSig + "\nmsg\n",
		"two parents":          hdr + "parent " + cpP1 + "\nparent " + cpP2 + "\n" + cpAut + cpSig + "\nmerge\n",
		"octopus":              hdr + "parent " + cpP1 + "\nparent " + cpP2 + "\nparent " + cpP3 + "\n" + cpAut + cpSig + "\noctopus\n",
		"gpgsig multi-line":    hdr + "parent " + cpP1 + "\n" + cpAut + cpSig + "gpgsig -----BEGIN PGP SIGNATURE-----\n \n iQEz\n parent " + cpP2 + "\n -----END PGP SIGNATURE-----\n\nsigned\n",
		"mergetag":             hdr + "parent " + cpP1 + "\nparent " + cpP2 + "\n" + cpAut + cpSig + "mergetag object " + cpP2 + "\n type commit\n tag v1\n\nmerge tag\n",
		"empty message":        hdr + "parent " + cpP1 + "\n" + cpAut + cpSig + "\n",
		"blank then parent":    hdr + cpAut + cpSig + "\nparent " + cpP1 + "\nnot a header\n",
		"no trailing newline":  hdr + "parent " + cpP1 + "\n" + cpAut + cpSig + "\nlast",
		"no committer":         hdr + "parent " + cpP1 + "\n" + cpAut + "\nmsg\n",
		"no author":            hdr + "parent " + cpP1 + "\n" + cpSig + "\nmsg\n",
		"committer first only": hdr + "parent " + cpP1 + "\n" + cpSig,
		"bad timestamp":        hdr + "parent " + cpP1 + "\nauthor A <a@e> 1 +0000\ncommitter T <t@e> notanumber +0000\n\nm\n",
		"no timezone":          hdr + "parent " + cpP1 + "\n" + "committer T <t@e> 1234567890\n\nm\n",
		"late parent":          hdr + cpAut + "parent " + cpP1 + "\n" + cpSig + "\nm\n",
		"extra header first":   hdr + "parent " + cpP1 + "\nencoding ISO-8859-1\n" + cpSig + "\nm\n",
		"headers only":         hdr + "parent " + cpP1 + "\n" + cpAut + cpSig,
	}
}

func rawCommitObject(payload string) plumbing.EncodedObject {
	o := &plumbing.MemoryObject{}
	o.SetType(plumbing.CommitObject)
	_, _ = o.Write([]byte(payload))
	return o
}

func TestCommitParentsStock_ShouldMatchDecodeCommit_WhenCorpusOfShapes(t *testing.T) {
	st := memory.NewStorage()
	for name, payload := range commitParentsCorpus() {
		o := rawCommitObject(payload)
		if _, err := st.SetEncodedObject(o); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want, wantErr := object.DecodeCommit(st, o)
		got, when, err := stockCommitParents.Parents(st, o.Hash(), nil)
		if (err != nil) != (wantErr != nil) {
			t.Fatalf("%s: err = %v, DecodeCommit err = %v", name, err, wantErr)
		}
		if wantErr != nil {
			continue
		}
		if len(got) != len(want.ParentHashes) {
			t.Fatalf("%s: parents = %v, want %v", name, got, want.ParentHashes)
		}
		for i := range got {
			if got[i] != want.ParentHashes[i] {
				t.Errorf("%s: parent %d = %s, want %s", name, i, got[i], want.ParentHashes[i])
			}
		}
		if when != want.Committer.When.Unix() {
			t.Errorf("%s: when = %d, want %d", name, when, want.Committer.When.Unix())
		}
	}
}

func TestCommitParentsStock_ShouldNotReturnParentText_WhenGpgsigContinuationHasParentLine(t *testing.T) {
	st := memory.NewStorage()
	payload := "tree " + cpTree + "\n" + cpAut + cpSig +
		"gpgsig -----BEGIN-----\n parent " + cpP2 + "\n parent " + cpP3 + "\n -----END-----\n\nparent " + cpP3 + "\n"
	o := rawCommitObject(payload)
	_, _ = st.SetEncodedObject(o)
	got, _, err := stockCommitParents.Parents(st, o.Hash(), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("parents = %v err = %v, want none", got, err)
	}
}

func TestCommitParentsStock_ShouldReturnError_WhenObjectIsNotACommit(t *testing.T) {
	st := memory.NewStorage()
	blob := &plumbing.MemoryObject{}
	blob.SetType(plumbing.BlobObject)
	_, _ = blob.Write([]byte("tree " + cpTree + "\n"))
	_, _ = st.SetEncodedObject(blob)
	if _, _, err := parseCommitHeader(blob, nil); !errors.Is(err, object.ErrUnsupportedObject) {
		t.Errorf("blob: err = %v, want ErrUnsupportedObject", err)
	}
	for name, payload := range map[string]string{
		"empty":           "",
		"blank first":     "\ntree " + cpTree + "\n",
		"tree not first":  "parent " + cpP1 + "\ntree " + cpTree + "\n",
		"bad tree hash":   "tree nothex\n",
		"bad parent hash": "tree " + cpTree + "\nparent 12\n",
		"truncated":       "tree 4b82",
	} {
		if _, _, err := parseCommitHeader(rawCommitObject(payload), nil); !errors.Is(err, object.ErrMalformedCommit) {
			t.Errorf("%s: err = %v, want ErrMalformedCommit", name, err)
		}
	}
	if _, _, err := stockCommitParents.Parents(st, plumbing.NewHash(cpP1), nil); !errors.Is(err, plumbing.ErrObjectNotFound) {
		t.Errorf("missing object: err = %v, want ErrObjectNotFound", err)
	}
}

func TestCommitParentsStock_ShouldSurviveHugeHeader_WhenGpgsigOver64KiB(t *testing.T) {
	st := memory.NewStorage()
	huge := strings.Repeat("A", 100<<10)
	payload := "tree " + cpTree + "\nparent " + cpP1 + "\n" + cpAut + cpSig + "gpgsig " + huge + "\n\nm\n"
	o := rawCommitObject(payload)
	_, _ = st.SetEncodedObject(o)
	got, when, err := stockCommitParents.Parents(st, o.Hash(), nil)
	if err != nil || len(got) != 1 || got[0] != plumbing.NewHash(cpP1) || when != 1700000000 {
		t.Fatalf("got %v when=%d err=%v", got, when, err)
	}
	// A committer line itself longer than the bufio buffer takes the accumulate path.
	long := "tree " + cpTree + "\nparent " + cpP1 + "\ncommitter " + strings.Repeat("N", 10<<10) + " <e> 42 +0000\n\nm\n"
	o2 := rawCommitObject(long)
	_, _ = st.SetEncodedObject(o2)
	if _, when, err = stockCommitParents.Parents(st, o2.Hash(), nil); err != nil || when != 42 {
		t.Fatalf("long committer: when=%d err=%v", when, err)
	}
}

func TestCommitParentsStock_ShouldCopyParentsIntoCallerBuffer_AndCapPooledBuffer64KiB(t *testing.T) {
	st := memory.NewStorage()
	o := rawCommitObject("tree " + cpTree + "\nparent " + cpP1 + "\nparent " + cpP2 + "\n" + cpSig + "\nm\n")
	_, _ = st.SetEncodedObject(o)
	buf := make([]plumbing.Hash, 0, 4)
	got, _, err := stockCommitParents.Parents(st, o.Hash(), buf)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v err=%v", got, err)
	}
	if &got[0] != &buf[:1][0] {
		t.Error("result does not use the caller's buffer")
	}
	// Pooled readers are reused across calls; the first result must not change.
	big := rawCommitObject("tree " + cpTree + "\nparent " + cpP3 + "\n" + cpSig + "\n" + strings.Repeat("x", 80<<10))
	_, _ = st.SetEncodedObject(big)
	if _, _, err := stockCommitParents.Parents(st, big.Hash(), nil); err != nil {
		t.Fatal(err)
	}
	if got[0] != plumbing.NewHash(cpP1) || got[1] != plumbing.NewHash(cpP2) {
		t.Errorf("caller buffer mutated by a later call: %v", got)
	}
}

// mallocDelta runs f n times and returns the TotalAlloc and Mallocs deltas.
// The fixture spawns no goroutines, so the deltas are this goroutine's.
func mallocDelta(n int, f func()) (bytes, mallocs uint64) {
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for range n {
		f()
	}
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc, b.Mallocs - a.Mallocs
}

func TestCommitParentsStock_ShouldUseAtMost70PctBytesOfDecode_When2500Commits(t *testing.T) {
	spec, _, _ := linearForkSpec(2500, 0, 0)
	d := buildDAG(t, spec, nil)
	st := d.Repo.Storer
	var buf []plumbing.Hash
	const rounds = 3
	decodeBytes, decodeMallocs := mallocDelta(rounds, func() {
		for _, h := range d.Hashes {
			if _, err := object.GetCommit(st, h); err != nil {
				t.Fatal(err)
			}
		}
	})
	stockBytes, stockMallocs := mallocDelta(rounds, func() {
		for _, h := range d.Hashes {
			var err error
			if buf, _, err = stockCommitParents.Parents(st, h, buf); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Logf("2500 commits x%d: B decode=%d stock=%d (%.1f%%); mallocs decode=%d stock=%d",
		rounds, decodeBytes, stockBytes, 100*float64(stockBytes)/float64(decodeBytes), decodeMallocs, stockMallocs)
	if float64(stockBytes) > 0.70*float64(decodeBytes) {
		t.Errorf("stock reader allocated %d B, want <= 70%% of decode's %d B", stockBytes, decodeBytes)
	}
	if stockMallocs >= decodeMallocs {
		t.Errorf("stock reader made %d allocations, want fewer than decode's %d", stockMallocs, decodeMallocs)
	}
}

func BenchmarkCommitParents(b *testing.B) {
	spec, _, _ := linearForkSpec(2500, 0, 0)
	d := buildDAG(b, spec, nil)
	st := d.Repo.Storer
	b.Run("decode", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			for _, h := range d.Hashes {
				_, _ = object.GetCommit(st, h)
			}
		}
	})
	b.Run("stock", func(b *testing.B) {
		b.ReportAllocs()
		var buf []plumbing.Hash
		for range b.N {
			for _, h := range d.Hashes {
				buf, _, _ = stockCommitParents.Parents(st, h, buf)
			}
		}
	})
}
