package services

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

const (
	ignorePathSep = "/"
	gitignoreName = ".gitignore"
	// maxGitignoreLine skips pathological lines instead of failing the walk (bufio.Scanner would stop).
	maxGitignoreLine = 64 * 1024
	// maxGitignoreBytes bounds how much of one .gitignore is read.
	maxGitignoreBytes = 1 << 20
)

// IgnoreMatcher applies .gitignore rules incrementally: the walker calls Enter as it
// reaches each directory and Leave when done, so rules load on demand through the
// confined fs.FS, honour ctx, and cost O(directory depth) rather than a pre-pass over
// the tree. Divergences from git: .git/info/exclude and global excludes are not read
// (search never enters .git), and a "**" inside a path segment never matches (go-git).
type IgnoreMatcher struct {
	patterns []gitignore.Pattern
	levels   []int // len(patterns) before each Enter, restored by Leave
}

// Enter loads dir's .gitignore (when entries lists a regular file by that name) and
// pushes its rules. A symlinked .gitignore is ignored, as git does. Every Enter must be
// paired with Leave, including when it returns an error.
func (m *IgnoreMatcher) Enter(ctx context.Context, fsys fs.FS, dir string, entries []fs.DirEntry) error {
	m.levels = append(m.levels, len(m.patterns))
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() != gitignoreName || !e.Type().IsRegular() {
			continue
		}
		m.load(fsys, dir)
		break
	}
	return nil
}

// Leave pops the rules pushed by the matching Enter.
func (m *IgnoreMatcher) Leave() {
	n := len(m.levels) - 1
	m.patterns = m.patterns[:m.levels[n]]
	m.levels = m.levels[:n]
}

// Ignored reports whether rel (slash-separated, root-relative) is excluded by the rules in force.
func (m *IgnoreMatcher) Ignored(rel string, isDir bool) bool {
	if len(m.patterns) == 0 {
		return false
	}
	parts := strings.Split(rel, ignorePathSep)
	for i := len(m.patterns) - 1; i >= 0; i-- {
		if r := m.patterns[i].Match(parts, isDir); r > gitignore.NoMatch {
			return r == gitignore.Exclude
		}
	}
	return false
}

func (m *IgnoreMatcher) load(fsys fs.FS, dir string) {
	name := gitignoreName
	var domain []string
	if dir != "." {
		name = dir + ignorePathSep + gitignoreName
		domain = strings.Split(dir, ignorePathSep)
	}
	f, err := fsys.Open(name)
	if err != nil {
		return // unreadable or escaping rules are ignored, never fatal
	}
	defer func() { _ = f.Close() }()
	readIgnoreLines(io.LimitReader(f, maxGitignoreBytes), func(line string) {
		m.patterns = append(m.patterns, gitignore.ParsePattern(line, domain))
	})
}

// readIgnoreLines calls fn for each meaningful line: comments, blank lines and lines
// over maxGitignoreLine are dropped (go-git's ParsePattern does not drop the first two itself).
func readIgnoreLines(r io.Reader, fn func(string)) {
	br := bufio.NewReaderSize(r, maxGitignoreLine)
	skipping := false
	for {
		chunk, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			skipping = true
			continue
		}
		if !skipping {
			line := strings.TrimRight(string(chunk), "\r\n")
			if !strings.HasPrefix(line, "#") && strings.TrimSpace(line) != "" {
				fn(line)
			}
		}
		skipping = false
		if err != nil {
			return
		}
	}
}
