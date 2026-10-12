package gogit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// Refs are read here, not through go-git's ref storage: go-git trims and parses ref files and
// packed-refs leniently, where git rejects ("ignoring broken ref", "unexpected line in
// packed-refs") and so answers differently. Anything not exactly in the form git itself writes is
// fallback-eligible. go-git is used only for objects.

// maxRefFileSize bounds a loose ref or HEAD file; real ones are under 100 bytes.
const maxRefFileSize = 4096

// maxPackedRefsSize bounds packed-refs (a repository with a million refs is about 80 MiB).
const maxPackedRefsSize = 256 << 20

var hex40 = regexp.MustCompile(`^[0-9a-fA-F]{40}`)

// rawRef is the parsed content of a loose ref file or HEAD.
type rawRef struct {
	target string        // set for a symbolic ref
	hash   plumbing.Hash // set for a direct ref
}

func (r rawRef) symbolic() bool { return r.target != "" }

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

// parseRefContent mirrors git's parse_loose_ref_contents: "ref:" followed by whitespace and a
// valid ref name, or a 40-digit hex SHA followed by end of file or whitespace.
func parseRefContent(b []byte) (rawRef, bool) {
	if bytes.HasPrefix(b, []byte("ref:")) {
		rest := bytes.TrimLeft(b[4:], " \t\n\r\v\f")
		name := string(bytes.TrimRight(rest, " \t\n\r\v\f"))
		if !validRefName(name, true) || strings.ContainsAny(name, "\x00") {
			return rawRef{}, false
		}
		return rawRef{target: name}, true
	}
	if !hex40.Match(b) || (len(b) > 40 && !isSpace(b[40])) {
		return rawRef{}, false
	}
	h := plumbing.NewHash(string(b[:40]))
	if h.IsZero() {
		return rawRef{}, false // git treats a null object id as a broken ref
	}
	return rawRef{hash: h}, true
}

// validateHeadref is git's validate_headref, the test is_git_directory applies: "ref:" plus
// whitespace plus a name starting refs/, or a hex SHA.
func validateHeadref(b []byte) bool {
	if bytes.HasPrefix(b, []byte("ref:")) {
		return bytes.HasPrefix(bytes.TrimLeft(b[4:], " \t\n\r\v\f"), []byte("refs/"))
	}
	return hex40.Match(b)
}

// refDir is where the file for a ref lives: per-worktree refs and top-level names are in the
// worktree's own git dir, everything else is shared.
func (v *repoView) refDir(name string) string {
	if !strings.HasPrefix(name, "refs/") {
		return v.lay.gitDir
	}
	for _, p := range perWorktreeRefPrefixes {
		if strings.HasPrefix(name, p) {
			return v.lay.gitDir
		}
	}
	return v.lay.commonDir
}

// perWorktreeRefPrefixes live in the per-worktree git dir; every other ref is shared.
var perWorktreeRefPrefixes = []string{"refs/bisect/", "refs/worktree/", "refs/rewritten/"} //nolint:gochecknoglobals read-only

// readRef reads one ref by name without following it: loose file first, then packed-refs.
func (v *repoView) readRef(name string) (ref rawRef, found bool, err error) {
	if !validRefName(name, true) {
		return rawRef{}, false, nil
	}
	path := filepath.Join(v.refDir(name), filepath.FromSlash(name))
	fi, statErr := os.Lstat(path)
	switch {
	case statErr == nil && fi.Mode()&fs.ModeSymlink != 0:
		return rawRef{}, false, fallbackEligible(fmt.Errorf("ref %s is a symlink", name))
	case statErr == nil && fi.Mode().IsRegular():
		data, readErr := readSmallFile(path)
		if readErr == nil {
			ref, ok := parseRefContent(data)
			if !ok {
				return rawRef{}, false, fallbackEligible(fmt.Errorf("%w: ref %s is not in a form git writes (a writer mid-update, or broken)", backend.ErrTornRead, name))
			}
			return ref, true, nil
		}
		if !errors.Is(readErr, fs.ErrNotExist) {
			return rawRef{}, false, fallbackEligible(readErr)
		}
	case statErr != nil && !errors.Is(statErr, fs.ErrNotExist) && !isNotDir(statErr):
		return rawRef{}, false, fallbackEligible(statErr)
	}
	// a directory, or no loose file: packed-refs decides
	if !strings.HasPrefix(name, "refs/") {
		return rawRef{}, false, nil
	}
	packed, err := v.packedRefs()
	if err != nil {
		return rawRef{}, false, err
	}
	if h, ok := packed[name]; ok {
		return rawRef{hash: h}, true, nil
	}
	return rawRef{}, false, nil
}

func isNotDir(err error) bool {
	var pe *fs.PathError
	return errors.As(err, &pe) && strings.Contains(pe.Err.Error(), "not a directory")
}

func readSmallFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxRefFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRefFileSize {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, maxRefFileSize)
	}
	return data, nil
}

// packedRefs returns the parsed packed-refs of the repository, read once per handle.
func (v *repoView) packedRefs() (map[string]plumbing.Hash, error) {
	if v.packed != nil {
		return v.packed, nil
	}
	path := filepath.Join(v.lay.commonDir, "packed-refs")
	if fi, statErr := os.Stat(path); statErr == nil && (!fi.Mode().IsRegular() || fi.Size() > maxPackedRefsSize) {
		return nil, fallbackEligible(errors.New("packed-refs is not a regular file or is implausibly large"))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			v.packed = map[string]plumbing.Hash{}
			return v.packed, nil
		}
		return nil, fallbackEligible(err)
	}
	refs, ok := parsePackedRefs(data)
	if !ok {
		return nil, fallbackEligible(errors.New("packed-refs is not in the form git writes"))
	}
	v.packed = refs
	return refs, nil
}

// parsePackedRefs accepts only what `git pack-refs` writes: LF-terminated lines, an optional
// "# pack-refs with:" header, "<sha> <refs/name>" entries (strictly sorted when the header says
// sorted, never duplicated), each optionally followed by one "^<sha>" peel line.
func parsePackedRefs(data []byte) (map[string]plumbing.Hash, bool) {
	refs := map[string]plumbing.Hash{}
	if len(data) == 0 {
		return refs, true
	}
	if data[len(data)-1] != '\n' || bytes.ContainsAny(data, "\r\x00") {
		return nil, false
	}
	lines := strings.Split(string(data[:len(data)-1]), "\n")
	sorted, previous, afterEntry := false, "", false
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "#"):
			if i != 0 || !strings.HasPrefix(line, "# pack-refs with:") {
				return nil, false
			}
			sorted = strings.Contains(line, " sorted")
		case strings.HasPrefix(line, "^"):
			if !afterEntry || len(line) != 41 || !hex40.MatchString(line[1:]) {
				return nil, false
			}
			afterEntry = false
		default:
			if len(line) < 42 || line[40] != ' ' || !hex40.MatchString(line) {
				return nil, false
			}
			name := line[41:]
			if !strings.HasPrefix(name, "refs/") || !validRefName(name, false) {
				return nil, false
			}
			if _, dup := refs[name]; dup || (sorted && name <= previous) {
				return nil, false
			}
			refs[name], previous, afterEntry = plumbing.NewHash(line[:40]), name, true
		}
	}
	return refs, true
}

// walkRefNames lists every ref name under refs/ that git's for-each-ref would consider: loose
// files (this worktree's per-worktree refs included) overriding packed entries.
func (v *repoView) walkRefNames() ([]string, error) {
	names := map[string]struct{}{}
	walk := func(root, prefix string, skipPerWorktree bool) error {
		return filepath.WalkDir(filepath.Join(root, filepath.FromSlash(prefix)), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			name := filepath.ToSlash(rel)
			if d.IsDir() {
				if skipPerWorktree && isPerWorktreePrefix(name+"/") {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("ref %s is a symlink", name)
			}
			if validRefName(name, false) {
				names[name] = struct{}{}
			}
			return nil
		})
	}
	shared := v.lay.gitDir != v.lay.commonDir
	if err := walk(v.lay.commonDir, "refs", shared); err != nil {
		return nil, fallbackEligible(fmt.Errorf("list loose refs: %w", err))
	}
	if shared {
		for _, p := range perWorktreeRefPrefixes {
			if err := walk(v.lay.gitDir, strings.TrimSuffix(p, "/"), false); err != nil {
				return nil, fallbackEligible(fmt.Errorf("list per-worktree refs: %w", err))
			}
		}
	}
	packed, err := v.packedRefs()
	if err != nil {
		return nil, err
	}
	for name := range packed {
		names[name] = struct{}{}
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	return out, nil
}

func isPerWorktreePrefix(name string) bool {
	for _, p := range perWorktreeRefPrefixes {
		if name == p || strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
