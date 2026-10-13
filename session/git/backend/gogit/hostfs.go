package gogit

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"
)

// hostFS is a read-only billy filesystem rooted at a real directory that lets the operating
// system resolve symlinks. go-billy's osfs.New is a "soft chroot" that re-resolves every symlink
// itself and refuses any whose target leaves the root ("chroot boundary crossed"), but git
// follows them (a .git symlinked to a git directory elsewhere, objects/ or refs/ symlinked to
// shared storage, a config symlinked into a dotfiles repo). The only guard kept is the lexical
// one: a name may not climb above the root with "..". It never writes: every mutating method
// fails with billy.ErrReadOnly.
type hostFS struct{ root string }

var _ billy.Filesystem = hostFS{}

// resolve maps a root-relative name to a host path, as billy's chroot does for the lexical part.
func (h hostFS) resolve(name string) (string, error) {
	rel := strings.TrimLeft(filepath.ToSlash(name), "/")
	if c := path.Clean(rel); c == ".." || strings.HasPrefix(c, "../") {
		return "", billy.ErrCrossedBoundary
	}
	return filepath.Join(h.root, filepath.FromSlash(rel)), nil
}

func (h hostFS) Root() string                         { return h.root }
func (h hostFS) Join(elem ...string) string           { return filepath.Join(elem...) }
func (h hostFS) Open(name string) (billy.File, error) { return h.OpenFile(name, os.O_RDONLY, 0) }

func (h hostFS) OpenFile(name string, flag int, _ os.FileMode) (billy.File, error) {
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
		return nil, billy.ErrReadOnly
	}
	p, err := h.resolve(name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	return hostFile{f}, nil
}

func (h hostFS) Stat(name string) (os.FileInfo, error) {
	p, err := h.resolve(name)
	if err != nil {
		return nil, err
	}
	return os.Stat(p)
}

func (h hostFS) Lstat(name string) (os.FileInfo, error) {
	p, err := h.resolve(name)
	if err != nil {
		return nil, err
	}
	return os.Lstat(p)
}

func (h hostFS) Readlink(name string) (string, error) {
	p, err := h.resolve(name)
	if err != nil {
		return "", err
	}
	return os.Readlink(p)
}

func (h hostFS) ReadDir(name string) ([]os.FileInfo, error) {
	p, err := h.resolve(name)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	infos := make([]os.FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			if os.IsNotExist(err) {
				continue // removed between the listing and the stat
			}
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (h hostFS) Chroot(name string) (billy.Filesystem, error) {
	p, err := h.resolve(name)
	if err != nil {
		return nil, err
	}
	return hostFS{root: p}, nil
}

func (hostFS) Create(string) (billy.File, error)           { return nil, billy.ErrReadOnly }
func (hostFS) Rename(string, string) error                 { return billy.ErrReadOnly }
func (hostFS) Remove(string) error                         { return billy.ErrReadOnly }
func (hostFS) MkdirAll(string, os.FileMode) error          { return billy.ErrReadOnly }
func (hostFS) Symlink(string, string) error                { return billy.ErrReadOnly }
func (hostFS) TempFile(string, string) (billy.File, error) { return nil, billy.ErrReadOnly }

// hostFile adapts *os.File to billy.File. Locking is a no-op: the file is only ever read.
type hostFile struct{ *os.File }

func (hostFile) Lock() error   { return nil }
func (hostFile) Unlock() error { return nil }
