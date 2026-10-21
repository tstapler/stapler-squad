package unfinished

// Model-based oracle for Story 4.2.3: seeded operation sequences, each step
// comparing the verdict-cache walk with a full walk. The sequences cover the
// invalidators a directory mtime cannot show (in-place and ancestor .gitignore
// changes, index-entry removal, mtime-preserving creates) plus symlink
// directories and nested repositories. Every sequence asserts engagement.

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const modelSteps = 14

var modelNames = []string{"n1.txt", "keep.log", "x.log", "y.o", "z.go", "build.bin"}

var modelIgnores = []string{"*.log\n", "*.o\n!keep.o\n", "!keep.log\n", "build.bin\n", "*.log\n!keep.log\n", "\n"}

// modelRun is one seeded sequence over a fresh tree.
type modelRun struct {
	t   testing.TB
	f   *walkFixture
	rng *rand.Rand
	gen int64
	seq int
}

func newModelRun(t testing.TB, seed int64, omitAncestorChain bool) *modelRun {
	f := newWalkFixture(t)
	if omitAncestorChain {
		f.g.scanSkip.untracked.Store(f.root, &untrackedCache{omitAncestorChain: true})
	}
	m := &modelRun{t: t, f: f, rng: rand.New(rand.NewSource(seed)), gen: 1}
	for _, rel := range []string{"root.go", "a/x.go", "a/b/y.go", "a/b/c/z.go", "d/w.go", "e/v.go", ".gitignore"} {
		content := "x\n"
		if rel == ".gitignore" {
			content = "*.log\n"
		}
		f.write(rel, content)
		f.track(rel)
	}
	f.write("a/b/c/keep.log", "k\n")
	f.write("d/build.bin", "b\n")
	f.ageAll()
	f.refreshMatcher()
	return m
}

func (m *modelRun) dirs() []string {
	var dirs []string
	require.NoError(m.t, filepath.WalkDir(m.f.root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	}))
	sort.Strings(dirs)
	return dirs
}

func (m *modelRun) files() []string {
	var files []string
	require.NoError(m.t, filepath.WalkDir(m.f.root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	}))
	sort.Strings(files)
	return files
}

func (m *modelRun) pick(list []string) (string, bool) {
	if len(list) == 0 {
		return "", false
	}
	return list[m.rng.Intn(len(list))], true
}

func (m *modelRun) fresh() string { m.seq++; return "g" + strconv.Itoa(m.seq) }

// step applies one random operation and reports whether it changed gitignore rules.
func (m *modelRun) step() (op string, rulesChanged bool) {
	dirs := m.dirs()
	switch m.rng.Intn(13) {
	case 0:
		if p, ok := m.pick(m.files()); ok {
			_ = os.WriteFile(p, []byte("edited "+m.fresh()+"\n"), 0o644)
		}
		return "edit", false
	case 1, 2:
		d, _ := m.pick(dirs)
		m.f.write(m.f.rel(filepath.Join(d, modelNames[m.rng.Intn(len(modelNames))])), "u\n")
		return "add-untracked", false
	case 3:
		if p, ok := m.pick(m.files()); ok {
			_ = os.Remove(p)
		}
		return "delete-file", false
	case 4:
		if p, ok := m.pick(m.files()); ok {
			_ = os.Rename(p, filepath.Join(filepath.Dir(p), m.fresh()+".txt"))
		}
		return "rename-file", false
	case 5:
		d, _ := m.pick(dirs)
		_ = os.MkdirAll(filepath.Join(d, m.fresh()), 0o755)
		return "mkdir", false
	case 6:
		if d, ok := m.pick(dirs[1:]); ok {
			_ = os.RemoveAll(d)
		}
		return "rmdir", false
	case 7:
		if d, ok := m.pick(dirs[1:]); ok {
			_ = os.Rename(d, filepath.Join(filepath.Dir(d), m.fresh()))
		}
		return "rename-dir", false
	case 8:
		d, _ := m.pick(dirs)
		_ = os.WriteFile(filepath.Join(d, ".gitignore"), []byte(modelIgnores[m.rng.Intn(len(modelIgnores))]), 0o644)
		return "gitignore-edit", true
	case 9:
		d, _ := m.pick(dirs)
		g := filepath.Join(d, ".gitignore")
		if _, err := os.Stat(g); err == nil {
			_ = os.Remove(g)
		} else {
			_ = os.WriteFile(g, []byte(modelIgnores[m.rng.Intn(len(modelIgnores))]), 0o644)
		}
		return "gitignore-create-delete", true
	case 10:
		names := make([]string, 0, len(m.f.indexed))
		for n := range m.f.indexed {
			names = append(names, n)
		}
		sort.Strings(names)
		if n, ok := m.pick(names); ok {
			delete(m.f.indexed, n)
			m.gen++
			m.f.setIndex(m.gen)
		}
		return "index-remove", false
	case 11:
		d, _ := m.pick(dirs)
		fi, err := os.Stat(d)
		require.NoError(m.t, err)
		m.f.write(m.f.rel(filepath.Join(d, m.fresh()+".txt")), "p\n")
		_ = os.Chtimes(d, fi.ModTime(), fi.ModTime()) // rsync -a / tar x: mtime preserved, ctime moves
		return "mtime-preserving-create", false
	default:
		d, _ := m.pick(dirs)
		if m.rng.Intn(2) == 0 {
			target, _ := m.pick(dirs)
			_ = os.Symlink(target, filepath.Join(d, m.fresh()))
			return "symlink-dir", false
		}
		nested := filepath.Join(d, m.fresh())
		m.f.write(m.f.rel(filepath.Join(nested, ".git", "config")), "c\n")
		m.f.write(m.f.rel(filepath.Join(nested, "file.txt")), "f\n")
		return "nested-repo", false
	}
}

// compare returns "" when the cached walk equals the full walk.
func (m *modelRun) compare(fresh bool) string {
	var skip, full []string
	if err := m.f.g.walkUntrackedSkip(m.f.root, m.f.indexed, m.f.matcher, m.f.tg, func(abs string) { skip = append(skip, abs) }); err != nil {
		return "skip walk error: " + err.Error()
	}
	if err := walkUntracked(m.f.root, m.f.indexed, m.f.matcher, func(abs string) { full = append(full, abs) }); err != nil {
		return "full walk error: " + err.Error()
	}
	has, err := m.f.g.hasUntrackedSkip(m.f.root, m.f.indexed, m.f.matcher, m.f.tg, fresh)
	if err != nil {
		return "has error: " + err.Error()
	}
	if fmt.Sprint(skip) != fmt.Sprint(full) {
		return fmt.Sprintf("walk differs:\n skip=%v\n full=%v", skip, full)
	}
	if has != (len(full) > 0) {
		return fmt.Sprintf("hasUntracked=%v but full walk found %d", has, len(full))
	}
	return ""
}

// runModelSequence returns the first mismatch ("" if none) and whether the
// cache engaged at least once.
func runModelSequence(t testing.TB, seed int64, omitAncestorChain bool) (mismatch string, engaged bool) {
	m := newModelRun(t, seed, omitAncestorChain)
	if msg := m.compare(false); msg != "" {
		return "cold: " + msg, false
	}
	if msg := m.compare(false); msg != "" {
		return "warm: " + msg, false
	}
	for i := range modelSteps {
		op, _ := m.step()
		// The model's matcher is always current (any op may delete or rename a
		// .gitignore); production instead gives a rebuilt matcher a new
		// identity, which resets the cache.
		m.f.refreshMatcher()
		// Random forced walks model DirtyGen overflow/error; dropped events
		// (the common case) leave fresh false and the keys alone must hold.
		if msg := m.compare(m.rng.Intn(4) == 0); msg != "" {
			return fmt.Sprintf("seed %d step %d (%s): %s", seed, i, op, msg), false
		}
	}
	return "", m.f.stats().DirsSkipped > 0
}

func TestUntrackedSkip_ShouldEqualFullWalk_WhenSeededModelRuns1000SequencesWithEventDrops(t *testing.T) {
	const sequences, shards = 1000, 10
	for shard := range shards {
		t.Run(fmt.Sprintf("shard%d", shard), func(t *testing.T) {
			t.Parallel()
			for seed := int64(shard*sequences/shards + 1); seed <= int64((shard+1)*sequences/shards); seed++ {
				mismatch, engaged := runModelSequence(t, seed, false)
				require.Empty(t, mismatch, "seed %d", seed)
				require.True(t, engaged, "seed %d: the cache never served a directory, so the comparison was vacuous", seed)
			}
		})
	}
}

// Mutation check on the oracle: with the ancestor markers removed from the key,
// the same generator must find a divergence, proving the oracle can fail.
func TestUntrackedModel_ShouldFail_WhenAncestorMarkerRemovedFromKey(t *testing.T) {
	t.Parallel()
	for seed := int64(1); seed <= 2000; seed++ {
		if mismatch, _ := runModelSequence(t, seed, true); mismatch != "" {
			t.Logf("mutant caught at seed %d: %.120s", seed, mismatch)
			return
		}
	}
	assert.Fail(t, "removing the ancestor .gitignore markers from the key went unnoticed by 2000 sequences")
}

var _ = time.Second
