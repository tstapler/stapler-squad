// Package ignorewalk collects a worktree's gitignore patterns without
// descending into directories the patterns already ignore.
package ignorewalk

import (
	"bufio"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

const (
	gitDir      = ".git"
	infoExclude = gitDir + "/info/exclude"
	ignoreFile  = ".gitignore"
)

// ReadPatterns is a drop-in for gitignore.ReadPatterns(fs, nil) that returns the
// same patterns in the same priority order, minus those inside ignored subtrees.
//
// go-git's version checks each subdirectory only against that directory's own
// patterns, never its ancestors', so a root "/web-app/node_modules" entry never
// stops the walk and every node_modules directory is listed. Here the check uses
// every pattern collected so far, as git itself does.
func ReadPatterns(fs billy.Filesystem) ([]gitignore.Pattern, error) {
	ps := readFile(fs, nil, infoExclude)
	return walk(fs, nil, ps)
}

func walk(fs billy.Filesystem, dir []string, ps []gitignore.Pattern) ([]gitignore.Pattern, error) {
	ps = append(ps, readFile(fs, dir, ignoreFile)...)

	infos, err := fs.ReadDir(fs.Join(dir...))
	if err != nil {
		return ps, err
	}
	for _, fi := range infos {
		if !fi.IsDir() || fi.Name() == gitDir {
			continue
		}
		sub := append(append(make([]string, 0, len(dir)+1), dir...), fi.Name())
		if gitignore.NewMatcher(ps).Match(sub, true) {
			continue
		}
		if ps, err = walk(fs, sub, ps); err != nil {
			return ps, err
		}
	}
	return ps, nil
}

// readFile parses one ignore file; a missing or unreadable file yields no patterns,
// matching go-git's tolerance.
func readFile(fs billy.Filesystem, dir []string, name string) []gitignore.Pattern {
	f, err := fs.Open(fs.Join(append(append([]string(nil), dir...), name)...))
	if err != nil {
		return nil
	}
	defer f.Close()

	var ps []gitignore.Pattern
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		s := sc.Text()
		if !strings.HasPrefix(s, "#") && len(strings.TrimSpace(s)) > 0 {
			ps = append(ps, gitignore.ParsePattern(s, dir))
		}
	}
	return ps
}
