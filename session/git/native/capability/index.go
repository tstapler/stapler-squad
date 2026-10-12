package capability

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

const (
	maxIndexSize   = 512 << 20 // refuse to decode absurdly large indexes
	indexCacheSize = 64
	checksumLen    = 20 // the index ends with a SHA-1 of everything before it
)

// indexFacts are the expensive, index-derived facts: found by decoding every entry, so they are
// cached under the index's trailing checksum (which changes whenever git rewrites the index).
type indexFacts struct {
	checksum      string
	size          int64
	attributeDirs []string // directories (relative to the root) holding a tracked .gitattributes
	skipWorktree  bool     // some entry is sparse-excluded
	gitlinks      bool     // some entry is a submodule
	unsupported   bool     // a mandatory extension go-git cannot read (split index)
}

// readIndex returns the facts of <GitDir>/index, from cache when its checksum is unchanged. A
// missing index (a repository with nothing staged yet) has no facts.
func (d *Detector) readIndex(l layout) (*indexFacts, error) {
	p := filepath.Join(l.gitDir, "index")
	fi, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return &indexFacts{}, nil
	}
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxIndexSize || fi.Size() < checksumLen {
		return nil, detectErr(backend.DetectUnsupportedIndex, errOrSize(err))
	}
	sum, err := trailer(p, fi.Size())
	if err != nil {
		return nil, detectErr(backend.DetectUnsupportedIndex, err)
	}
	d.mu.Lock()
	cached, ok := d.index[p]
	d.mu.Unlock()
	if ok && cached.checksum == sum && cached.size == fi.Size() {
		return &cached, nil
	}
	facts, err := decodeIndex(p)
	if err != nil {
		return nil, detectErr(backend.DetectUnsupportedIndex, err)
	}
	facts.checksum, facts.size = sum, fi.Size()
	d.mu.Lock()
	if len(d.index) >= indexCacheSize {
		d.index = map[string]indexFacts{}
	}
	d.index[p] = *facts
	d.mu.Unlock()
	return facts, nil
}

func errOrSize(err error) error {
	if err != nil {
		return err
	}
	return errors.New("index is not a plausible regular file")
}

func trailer(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, checksumLen)
	if _, err := f.ReadAt(buf, size-checksumLen); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return string(buf), nil
}

func decodeIndex(p string) (*indexFacts, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	idx := &index.Index{}
	err = index.NewDecoder(bufio.NewReader(bytes.NewReader(data))).Decode(idx)
	if errors.Is(err, index.ErrUnknownExtension) {
		return &indexFacts{unsupported: true}, nil
	}
	if err != nil {
		return nil, err
	}
	f := &indexFacts{}
	seen := map[string]bool{}
	for _, e := range idx.Entries {
		if e.SkipWorktree {
			f.skipWorktree = true
		}
		if e.Mode == filemode.Submodule {
			f.gitlinks = true
		}
		if path.Base(e.Name) == ".gitattributes" {
			if dir := path.Dir(e.Name); !seen[dir] {
				seen[dir] = true
				f.attributeDirs = append(f.attributeDirs, dir)
			}
		}
	}
	return f, nil
}

func (f *facts) unsupportedIndex() ([]backend.Capability, error) {
	idx, err := f.indexFacts()
	if err != nil {
		return nil, err
	}
	if idx.unsupported {
		return one(backend.CapUnsupportedIndex, "mandatory index extension"), nil
	}
	return nil, nil
}

func (f *facts) submodules() ([]backend.Capability, error) {
	idx, err := f.indexFacts()
	if err != nil {
		return nil, err
	}
	if idx.gitlinks {
		return one(backend.CapSubmodule, ""), nil
	}
	return nil, nil
}

// sparse reports sparse checkout: core.sparseCheckout true, or an index with sparse-excluded entries.
func (f *facts) sparse() ([]backend.Capability, error) {
	cfg, err := f.config()
	if err != nil {
		return nil, err
	}
	if v, set, ok := cfg.Bool("core.sparsecheckout"); set && (v || !ok) {
		return one(backend.CapSparse, "core.sparseCheckout"), nil
	}
	idx, err := f.indexFacts()
	if err != nil {
		return nil, err
	}
	if idx.skipWorktree {
		return one(backend.CapSparse, "skip-worktree entries"), nil
	}
	return nil, nil
}
