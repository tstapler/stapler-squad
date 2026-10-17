package services

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/sync/errgroup"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

const (
	minSearchPattern       = 2
	maxSearchPattern       = 1024
	defaultSearchResults   = 1000
	maxSearchWorkers       = 4
	searchWorkerQueueDepth = 2 // queued jobs/results per worker
)

type (
	MatchMode int
	CaseMode  int
	WordMode  int
)

const (
	MatchLiteral MatchMode = iota + 1
	MatchRegex
)

const (
	CaseSensitive CaseMode = iota + 1
	CaseInsensitive
)

const (
	WordAny WordMode = iota + 1
	WordWhole
)

// SearchScope narrows which files are searched.
type SearchScope struct {
	IncludeGlobs   []string
	ExcludeGlobs   []string
	IncludeIgnored bool
}

// SearchLimits caps what one search emits.
type SearchLimits struct {
	MaxResults int
}

// SearchOptions is a validated search request; build it with NewSearchOptions.
type SearchOptions struct {
	Pattern string
	Match   MatchMode
	Case    CaseMode
	Word    WordMode
	Scope   SearchScope
	Limits  SearchLimits
}

// SearchStats summarizes one finished walk.
type SearchStats struct {
	FilesScanned      int
	BinarySkipped     int
	LargeFilesSkipped int
	Truncated         bool
	Duration          time.Duration
}

// ContentSearcher greps file contents under a confined root. emit receives the matches
// of one file at a time and is never called concurrently.
type ContentSearcher interface {
	Search(ctx context.Context, root fs.FS, opts SearchOptions, emit func([]SearchMatch) error) (SearchStats, error)
}

// NewSearchOptions is the single parse-at-boundary validation point for a search request.
func NewSearchOptions(in *sessionv1.SearchOptions) (SearchOptions, error) {
	opts := SearchOptions{
		Pattern: in.GetPattern(),
		Match:   MatchLiteral,
		Case:    CaseSensitive,
		Word:    WordAny,
		Scope: SearchScope{
			IncludeGlobs:   in.GetScope().GetIncludeGlobs(),
			ExcludeGlobs:   in.GetScope().GetExcludeGlobs(),
			IncludeIgnored: in.GetScope().GetIncludeIgnored(),
		},
		Limits: SearchLimits{MaxResults: int(in.GetLimits().GetMaxResults())},
	}
	if in.GetMatchMode() == sessionv1.SearchMatchMode_SEARCH_MATCH_MODE_REGEX {
		opts.Match = MatchRegex
	}
	if in.GetCaseMode() == sessionv1.SearchCaseMode_SEARCH_CASE_MODE_INSENSITIVE {
		opts.Case = CaseInsensitive
	}
	if in.GetWordMode() == sessionv1.SearchWordMode_SEARCH_WORD_MODE_WHOLE {
		opts.Word = WordWhole
	}
	if err := opts.validate(); err != nil {
		return SearchOptions{}, err
	}
	return opts, nil
}

func invalidArg(format string, a ...any) error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(format, a...))
}

func (o SearchOptions) validate() error {
	if len(o.Pattern) > maxSearchPattern {
		return invalidArg("pattern must be at most %d bytes", maxSearchPattern)
	}
	for _, g := range append(append([]string{}, o.Scope.IncludeGlobs...), o.Scope.ExcludeGlobs...) {
		if !doublestar.ValidatePattern(g) {
			return invalidArg("malformed glob %q", g)
		}
	}
	if _, err := o.compile(); err != nil {
		return err // reported before the length floor so "(" names its RE2 problem
	}
	if len(o.Pattern) < minSearchPattern {
		return invalidArg("pattern must be at least %d characters", minSearchPattern)
	}
	return nil
}

// compile builds the one RE2 matcher every mode shares: literals are quoted, case
// folding and word boundaries are flags around it.
func (o SearchOptions) compile() (*regexp.Regexp, error) {
	expr := o.Pattern
	if o.Match != MatchRegex {
		expr = regexp.QuoteMeta(expr)
	}
	if o.Word == WordWhole {
		expr = `\b(?:` + expr + `)\b`
	}
	if o.Case == CaseInsensitive {
		expr = `(?i)` + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, invalidArg("invalid pattern: %v", err)
	}
	return re, nil
}

func (o SearchOptions) maxResults() int {
	if o.Limits.MaxResults <= 0 || o.Limits.MaxResults > defaultSearchResults {
		return defaultSearchResults
	}
	return o.Limits.MaxResults
}

// searchWorkerCount leaves CPU for terminal streaming in the same process.
func searchWorkerCount(gomaxprocs int) int {
	return max(1, min(maxSearchWorkers, gomaxprocs/2))
}

// walkSearcher is the in-process ContentSearcher: one streaming tree walk feeding a
// bounded worker pool. No path list is built, so memory is independent of file count.
type walkSearcher struct{ workers int }

// NewWalkSearcher returns the production searcher sized for the current GOMAXPROCS.
func NewWalkSearcher() ContentSearcher {
	return walkSearcher{workers: searchWorkerCount(runtime.GOMAXPROCS(0))}
}

type searchCounters struct {
	scanned, binary, large atomic.Int64
}

func (c *searchCounters) stats(truncated bool, start time.Time) SearchStats {
	return SearchStats{
		FilesScanned:      int(c.scanned.Load()),
		BinarySkipped:     int(c.binary.Load()),
		LargeFilesSkipped: int(c.large.Load()),
		Truncated:         truncated,
		Duration:          time.Since(start),
	}
}

type fileResult struct {
	matches []SearchMatch
}

// scanEnv is what every scan worker shares.
type scanEnv struct {
	root     fs.FS
	re       *regexp.Regexp
	counters *searchCounters
}

func (w walkSearcher) Search(ctx context.Context, root fs.FS, opts SearchOptions, emit func([]SearchMatch) error) (SearchStats, error) {
	re, err := opts.compile()
	if err != nil {
		return SearchStats{}, err
	}
	start := time.Now()
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()

	env := scanEnv{root: root, re: re, counters: &searchCounters{}}
	jobs := make(chan string, w.workers*searchWorkerQueueDepth)
	results := make(chan fileResult, w.workers*searchWorkerQueueDepth)

	g, gctx := errgroup.WithContext(wctx)
	g.Go(func() error {
		defer close(jobs)
		return newTreeWalk(root, opts).run(gctx, jobs)
	})
	var workers sync.WaitGroup
	for range w.workers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			env.worker(gctx, jobs, results)
		}()
	}
	go func() { workers.Wait(); close(results) }()

	truncated, emitErr := collectResults(results, opts.maxResults(), emit)
	cancel()
	walkErr := g.Wait()
	workers.Wait()

	stats := env.counters.stats(truncated, start)
	if err := ctx.Err(); err != nil {
		return stats, err
	}
	if emitErr != nil {
		return stats, emitErr
	}
	if errors.Is(walkErr, context.Canceled) {
		return stats, nil // our own cancel after the result cap was reached
	}
	return stats, walkErr
}

// collectResults forwards matches in arrival order until the result cap is reached.
func collectResults(results <-chan fileResult, limit int, emit func([]SearchMatch) error) (truncated bool, err error) {
	total := 0
	for res := range results {
		batch := res.matches
		if total+len(batch) >= limit {
			batch = batch[:limit-total]
			truncated = true
		}
		total += len(batch)
		if len(batch) > 0 {
			if err := emit(batch); err != nil {
				return truncated, err
			}
		}
		if truncated {
			return true, nil
		}
	}
	return false, nil
}

// worker reads each job through root (the same os.Root confinement as the walk).
func (e scanEnv) worker(ctx context.Context, jobs <-chan string, out chan<- fileResult) {
	for path := range jobs {
		if ctx.Err() != nil {
			return
		}
		res, ok := e.scan(ctx, path)
		if !ok || len(res.matches) == 0 {
			continue
		}
		select {
		case out <- res:
		case <-ctx.Done():
			return
		}
	}
}

func (e scanEnv) scan(ctx context.Context, path string) (fileResult, bool) {
	f, err := e.root.Open(path)
	if err != nil {
		return fileResult{}, false // vanished or unreadable: skip, never fatal
	}
	defer func() { _ = f.Close() }()
	scan, err := scanContent(ctx, f, path, e.re, searchFileCap)
	if err != nil {
		return fileResult{}, false
	}
	e.counters.scanned.Add(1)
	if scan.Binary {
		e.counters.binary.Add(1)
		return fileResult{}, false
	}
	if scan.Capped {
		if st, serr := f.Stat(); serr == nil && st.Size() > searchFileCap {
			e.counters.large.Add(1)
		}
	}
	return fileResult{matches: scan.Matches}, true
}

// treeWalk streams candidate file paths into jobs, depth first in sorted order. It is a
// hand-rolled recursion over fs.ReadDir (not fs.WalkDir) because the IgnoreMatcher needs
// a callback on leaving a directory to pop its rules.
type treeWalk struct {
	root   fs.FS
	opts   SearchOptions
	ignore IgnoreMatcher
}

func newTreeWalk(root fs.FS, opts SearchOptions) *treeWalk {
	return &treeWalk{root: root, opts: opts}
}

func (t *treeWalk) run(ctx context.Context, jobs chan<- string) error {
	return t.dir(ctx, ".", jobs)
}

func (t *treeWalk) dir(ctx context.Context, dir string, jobs chan<- string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := fs.ReadDir(t.root, dir)
	if err != nil && len(entries) == 0 {
		return nil // unreadable directory: skip, never fatal
	}
	if !t.opts.Scope.IncludeIgnored {
		defer t.ignore.Leave()
		if err := t.ignore.Enter(ctx, t.root, dir, entries); err != nil {
			return err
		}
	}
	for _, e := range entries {
		if err := t.entry(ctx, dir, e, jobs); err != nil {
			return err
		}
	}
	return nil
}

func (t *treeWalk) entry(ctx context.Context, dir string, e fs.DirEntry, jobs chan<- string) error {
	name := e.Name()
	rel := name
	if dir != "." {
		rel = dir + ignorePathSep + name
	}
	if name == ".git" || e.Type()&fs.ModeSymlink != 0 {
		return nil
	}
	if e.IsDir() {
		if t.skipDir(name, rel) {
			return nil
		}
		return t.dir(ctx, rel, jobs)
	}
	if !e.Type().IsRegular() || isAtomicTempName(name) || !t.wantFile(rel) {
		return nil
	}
	select {
	case jobs <- rel:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *treeWalk) skipDir(name, rel string) bool {
	if t.opts.Scope.IncludeIgnored {
		return false
	}
	return hardSkipDirs[name] || t.ignore.Ignored(rel, true)
}

func (t *treeWalk) wantFile(rel string) bool {
	if !t.opts.Scope.IncludeIgnored && t.ignore.Ignored(rel, false) {
		return false
	}
	return matchesGlobs(rel, t.opts.Scope.IncludeGlobs, true) && !matchesGlobs(rel, t.opts.Scope.ExcludeGlobs, false)
}

// matchesGlobs reports whether rel matches any glob; an empty list yields emptyValue.
func matchesGlobs(rel string, globs []string, emptyValue bool) bool {
	if len(globs) == 0 {
		return emptyValue
	}
	for _, g := range globs {
		if ok, err := doublestar.Match(g, rel); err == nil && ok {
			return true
		}
	}
	return false
}
