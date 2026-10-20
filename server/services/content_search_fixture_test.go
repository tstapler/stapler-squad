package services

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureConfig tunes genFixtureTree; zero values mean the documented defaults.
type fixtureConfig struct {
	gitignores int
}

type fixtureOption func(*fixtureConfig)

// WithGitignores spreads n small .gitignore files across the tree (default 1).
func WithGitignores(n int) fixtureOption {
	return func(c *fixtureConfig) { c.gitignores = n }
}

const (
	fixtureSeed        = 42
	fixtureDirCount    = 2000
	fixtureIgnoredFile = 20000
)

// genFixtureTree writes a deterministic tree (seed 42) of `files` files over
// about 2,000 directories, 5% binary, plus node_modules/ holding 20,000
// ignored files and a root .gitignore. It returns the root.
func genFixtureTree(tb testing.TB, files int, opts ...fixtureOption) string {
	tb.Helper()
	cfg := fixtureConfig{gitignores: 1}
	for _, o := range opts {
		o(&cfg)
	}
	root := tb.TempDir()
	rng := rand.New(rand.NewSource(fixtureSeed))

	dirs := make([]string, fixtureDirCount)
	for i := range dirs {
		dirs[i] = filepath.Join(root, fmt.Sprintf("d%02d", i%40), fmt.Sprintf("s%03d", i))
		mustMkdir(tb, dirs[i])
	}
	writeFixtureFile(tb, filepath.Join(root, ".gitignore"), []byte("node_modules/\n*.log\n"))
	for i := 1; i < cfg.gitignores; i++ {
		d := dirs[(i*7)%len(dirs)]
		writeFixtureFile(tb, filepath.Join(d, ".gitignore"), []byte("*.tmp\nbuild/\n"))
	}

	nm := filepath.Join(root, "node_modules")
	mustMkdir(tb, nm)
	for i := 0; i < fixtureIgnoredFile; i++ {
		sub := filepath.Join(nm, fmt.Sprintf("pkg%03d", i%200))
		if i < 200 {
			mustMkdir(tb, sub)
		}
		writeFixtureFile(tb, filepath.Join(sub, fmt.Sprintf("f%05d.js", i)), fixtureText(rng, 4))
	}

	for i := 0; i < files; i++ {
		d := dirs[rng.Intn(len(dirs))]
		var data []byte
		name := fmt.Sprintf("f%06d", i)
		if i%20 == 0 {
			data = make([]byte, 512+rng.Intn(2048))
			rng.Read(data)
			data[0] = 0 // NUL in the first block marks it binary
			name += ".bin"
		} else {
			data = fixtureText(rng, 5+rng.Intn(60))
			name += ".txt"
		}
		writeFixtureFile(tb, filepath.Join(d, name), data)
	}
	return root
}

var fixtureWords = []string{"alpha", "beta", "gamma", "delta", "func", "return", "TODO", "value", "index", "config"}

func fixtureText(rng *rand.Rand, lines int) []byte {
	var b bytes.Buffer
	for l := 0; l < lines; l++ {
		n := 3 + rng.Intn(8)
		for w := 0; w < n; w++ {
			b.WriteString(fixtureWords[rng.Intn(len(fixtureWords))])
			b.WriteByte(' ')
		}
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func mustMkdir(tb testing.TB, p string) {
	tb.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		tb.Fatal(err)
	}
}

func writeFixtureFile(tb testing.TB, p string, data []byte) {
	tb.Helper()
	if err := os.WriteFile(p, data, 0o644); err != nil {
		tb.Fatal(err)
	}
}

// treeDigest hashes relative paths and contents in walk order.
func treeDigest(tb testing.TB, root string) [32]byte {
	tb.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func TestGenFixtureTree_ShouldBeByteIdentical_When_RunTwiceWithSeed42(t *testing.T) {
	if testing.Short() {
		t.Skip("creates ~140k files")
	}
	a := genFixtureTree(t, 50000, WithGitignores(200))
	b := genFixtureTree(t, 50000, WithGitignores(200))
	if treeDigest(t, a) != treeDigest(t, b) {
		t.Fatal("two generations differ")
	}
	var total, bin, ignored, gi int
	_ = filepath.WalkDir(a, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(a, p)
		switch {
		case filepath.Base(p) == ".gitignore":
			gi++
		case strings.HasPrefix(rel, "node_modules/"):
			ignored++
		default:
			total++
			if filepath.Ext(p) == ".bin" {
				bin++
			}
		}
		return nil
	})
	if total != 50000 || ignored != fixtureIgnoredFile || bin != 2500 || gi != 200 {
		t.Fatalf("total=%d ignored=%d bin=%d gitignores=%d", total, ignored, bin, gi)
	}
}
