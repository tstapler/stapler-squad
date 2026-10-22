package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/tstapler/stapler-squad/session/git/native"
)

// OpKind names one scripted event.
type OpKind string

const (
	OpCommit    OpKind = "commit"
	OpEdit      OpKind = "edit-tracked"
	OpCreate    OpKind = "create-untracked"
	OpDelete    OpKind = "delete-untracked"
	OpGitignore OpKind = "gitignore"
	OpMoveBase  OpKind = "move-base"
	OpIdle      OpKind = "idle"
)

// Clock is the pacing seam. The driver never reads wall-clock time: schedule
// offsets and commit timestamps are logical (logicalEpoch + offset), so op
// logs depend only on seed, config and corpus bytes. A real run injects a
// clock that sleeps; tests inject one that returns immediately.
type Clock interface {
	WaitUntil(ctx context.Context, offset time.Duration) error
}

// immediateClock never waits.
type immediateClock struct{}

func (immediateClock) WaitUntil(ctx context.Context, _ time.Duration) error { return ctx.Err() }

// logicalEpoch is the base for commit timestamps.
var logicalEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC) //nolint:gochecknoglobals // immutable constant-like time value; Go has no const time.Time

// Target is one directory the driver may touch: a copied repo or one of its
// linked worktrees. Name is relative to the corpus root so logs from two
// corpora at different paths compare equal.
type Target struct {
	Name string
	Repo string // owning repo name
	Dir  string
	Main bool // the copied repo itself (not a linked worktree)
}

// TargetsFromManifest lists every repo and linked worktree in the corpus.
func TargetsFromManifest(root string, m Manifest) []Target {
	var ts []Target
	rel := func(p string) string {
		r, err := filepath.Rel(root, p)
		if err != nil {
			return p
		}
		return filepath.ToSlash(r)
	}
	for _, r := range m.Repos {
		ts = append(ts, Target{Name: rel(r.Path), Repo: r.Name, Dir: r.Path, Main: true})
		for _, w := range r.Worktrees {
			ts = append(ts, Target{Name: rel(w), Repo: r.Name, Dir: w})
		}
	}
	return ts
}

// DriverConfig parameterises one deterministic run.
type DriverConfig struct {
	Seed     uint64
	Duration time.Duration // logical length of the run
	Tick     time.Duration // idle tick spacing
	Targets  []Target
	// CommitsPerDay is each repo's observed cadence (from the cadence
	// subcommand). It drives commit and base-ref-move event rates. Never
	// derive it from a profile.
	CommitsPerDay        map[string]float64
	DefaultCommitsPerDay float64
	Speedup              float64 // multiplies the cadence; 1 = observed rate
	FSOpsPerHour         float64 // per repo: edit/create/delete/gitignore events
	Clock                Clock
	Out                  io.Writer // op log, one JSON object per line; may be nil
}

// Op is one scheduled (and, after Run, executed) event.
type Op struct {
	Seq      int    `json:"seq"`
	OffsetMS int64  `json:"offset_ms"`
	Kind     OpKind `json:"kind"`
	Target   string `json:"target,omitempty"`
	Salt     uint64 `json:"salt"`
	Detail   string `json:"detail,omitempty"`

	repo string
}

// Schedule builds the fixed, seeded op sequence. It is a pure function of the
// config: same seed and config, same schedule. Each repo draws from its own
// stream so adding a repo does not reshuffle the others.
func Schedule(cfg DriverConfig) []Op {
	byRepo := map[string][]Target{}
	var repos []string
	for _, t := range cfg.Targets {
		if _, ok := byRepo[t.Repo]; !ok {
			repos = append(repos, t.Repo)
		}
		byRepo[t.Repo] = append(byRepo[t.Repo], t)
	}
	sort.Strings(repos)
	speed := cfg.Speedup
	if speed <= 0 {
		speed = 1
	}

	var ops []Op
	for _, repo := range repos {
		ts := byRepo[repo]
		sort.Slice(ts, func(i, j int) bool { return ts[i].Name < ts[j].Name })
		rng := rand.New(rand.NewPCG(cfg.Seed, hashString(repo)))

		perDay, ok := cfg.CommitsPerDay[repo]
		if !ok {
			perDay = cfg.DefaultCommitsPerDay
		}
		commitRate := perDay * speed / float64(24*time.Hour) // events per ns
		for _, off := range poisson(rng, commitRate, cfg.Duration) {
			ops = append(ops, Op{OffsetMS: off, Kind: OpCommit, Target: pick(rng, ts).Name, Salt: rng.Uint64(), repo: repo})
		}
		// Fetch events (the base branch advancing) arrive at the same cadence.
		for _, off := range poisson(rng, commitRate, cfg.Duration) {
			ops = append(ops, Op{OffsetMS: off, Kind: OpMoveBase, Target: mainOf(ts).Name, Salt: rng.Uint64(), repo: repo})
		}
		fsRate := cfg.FSOpsPerHour / float64(time.Hour)
		for _, off := range poisson(rng, fsRate, cfg.Duration) {
			ops = append(ops, Op{OffsetMS: off, Kind: fsKind(rng.IntN(100)), Target: pick(rng, ts).Name, Salt: rng.Uint64(), repo: repo})
		}
	}
	if cfg.Tick > 0 {
		for off := cfg.Tick; off <= cfg.Duration; off += cfg.Tick {
			ops = append(ops, Op{OffsetMS: off.Milliseconds(), Kind: OpIdle})
		}
	}
	sort.SliceStable(ops, func(i, j int) bool {
		a, b := ops[i], ops[j]
		if a.OffsetMS != b.OffsetMS {
			return a.OffsetMS < b.OffsetMS
		}
		if a.repo != b.repo {
			return a.repo < b.repo
		}
		return a.Kind < b.Kind
	})
	for i := range ops {
		ops[i].Seq = i + 1
	}
	return ops
}

func fsKind(roll int) OpKind {
	switch {
	case roll < 40:
		return OpEdit
	case roll < 65:
		return OpCreate
	case roll < 80:
		return OpDelete
	default:
		return OpGitignore
	}
}

func pick(rng *rand.Rand, ts []Target) Target { return ts[rng.IntN(len(ts))] }

func mainOf(ts []Target) Target {
	for _, t := range ts {
		if t.Main {
			return t
		}
	}
	return ts[0]
}

func hashString(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// poisson returns event offsets (ms, ascending, within d) of a Poisson process
// with the given rate in events per nanosecond.
func poisson(rng *rand.Rand, ratePerNS float64, d time.Duration) []int64 {
	if ratePerNS <= 0 {
		return nil
	}
	var out []int64
	t := 0.0
	for {
		t += -math.Log(1-rng.Float64()) / ratePerNS
		if t > float64(d) {
			return out
		}
		out = append(out, int64(t)/int64(time.Millisecond))
	}
}

// OpLog is the executed op sequence.
type OpLog struct{ Ops []Op }

// Lines renders the log as JSON lines.
func (l OpLog) Lines() []string {
	lines := make([]string, len(l.Ops))
	for i, op := range l.Ops {
		b, _ := json.Marshal(op) // Op has only marshalable fields
		lines[i] = string(b)
	}
	return lines
}

// Hash is the SHA-256 of the log lines.
func (l OpLog) Hash() string {
	h := sha256.New()
	for _, line := range l.Lines() {
		_, _ = io.WriteString(h, line+"\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Run applies the schedule through go-git and file writes. An op that cannot
// apply is logged with a "skip:" detail rather than aborting the run.
func Run(ctx context.Context, cfg DriverConfig) (OpLog, error) {
	if len(cfg.Targets) == 0 {
		return OpLog{}, errors.New("driver: no targets")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = immediateClock{}
	}
	dirs := map[string]string{}
	for _, t := range cfg.Targets {
		dirs[t.Name] = t.Dir
	}
	state := &runState{created: map[string][]string{}}
	var log OpLog
	for _, op := range Schedule(cfg) {
		if err := clock.WaitUntil(ctx, time.Duration(op.OffsetMS)*time.Millisecond); err != nil {
			return log, err
		}
		op.Detail = state.apply(op, dirs[op.Target])
		log.Ops = append(log.Ops, op)
		if cfg.Out != nil {
			b, _ := json.Marshal(op)
			if _, err := fmt.Fprintf(cfg.Out, "%s\n", b); err != nil {
				return log, err
			}
		}
	}
	return log, nil
}

type runState struct {
	created map[string][]string // target -> untracked files this run created, oldest first
}

const workloadDir = ".ssq-workload"

func (s *runState) apply(op Op, dir string) string {
	if op.Kind == OpIdle {
		return ""
	}
	detail, err := s.dispatch(op, dir)
	if err != nil {
		return "skip: " + err.Error()
	}
	return detail
}

func (s *runState) dispatch(op Op, dir string) (string, error) {
	when := logicalEpoch.Add(time.Duration(op.OffsetMS) * time.Millisecond)
	switch op.Kind {
	case OpCommit:
		return commitInWorktree(dir, op, when)
	case OpEdit:
		return editTracked(dir, op)
	case OpCreate:
		rel := fmt.Sprintf("%s/untracked-%d.txt", workloadDir, op.Seq)
		if err := writeRel(dir, rel, fmt.Sprintf("seq %d salt %d\n", op.Seq, op.Salt)); err != nil {
			return "", err
		}
		s.created[op.Target] = append(s.created[op.Target], rel)
		return rel, nil
	case OpDelete:
		q := s.created[op.Target]
		if len(q) == 0 {
			return "", errors.New("no untracked file to delete yet")
		}
		rel := q[0]
		s.created[op.Target] = q[1:]
		return rel, os.Remove(filepath.Join(dir, filepath.FromSlash(rel)))
	case OpGitignore:
		return editGitignore(dir, op)
	case OpMoveBase:
		return moveBase(dir, op, when)
	case OpIdle:
		// Idle ops only advance logical time; they touch no files.
	}
	return "", fmt.Errorf("unknown op kind %q", op.Kind)
}

func writeRel(dir, rel, content string) error {
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

func appendRel(dir, rel, line string) error {
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(f, line); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func openRepo(dir string) (*git.Repository, error) {
	return native.OpenRepo(dir)
}

func signature(when time.Time) *object.Signature {
	return &object.Signature{Name: "scanner-workload", Email: "scanner-workload@invalid", When: when}
}

func commitInWorktree(dir string, op Op, when time.Time) (string, error) {
	repo, err := openRepo(dir)
	if err != nil {
		return "", err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	rel := fmt.Sprintf("%s/commit-%d.txt", workloadDir, op.Seq)
	if err := writeRel(dir, rel, fmt.Sprintf("seq %d salt %d\n", op.Seq, op.Salt)); err != nil {
		return "", err
	}
	if _, err := wt.Add(rel); err != nil {
		return "", err
	}
	h, err := wt.Commit(fmt.Sprintf("workload: commit %d", op.Seq), &git.CommitOptions{Author: signature(when), Committer: signature(when)})
	if err != nil {
		return "", err
	}
	return h.String(), nil
}

var editableExt = map[string]bool{".go": true, ".md": true, ".txt": true, ".ts": true, ".tsx": true, ".json": true, ".yml": true, ".yaml": true, ".sh": true, ".py": true, ".rs": true} //nolint:gochecknoglobals // read-only lookup table

const maxEditBytes = 1 << 20

// editTracked appends a line to a tracked text file chosen from the index.
func editTracked(dir string, op Op) (string, error) {
	repo, err := openRepo(dir)
	if err != nil {
		return "", err
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		return "", err
	}
	var cands []string
	for _, e := range idx.Entries {
		if !editableExt[strings.ToLower(path.Ext(e.Name))] || strings.HasPrefix(e.Name, workloadDir+"/") {
			continue
		}
		if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(e.Name))); err == nil && fi.Mode().IsRegular() && fi.Size() <= maxEditBytes {
			cands = append(cands, e.Name)
		}
	}
	if len(cands) == 0 {
		return "", errors.New("no editable tracked file")
	}
	sort.Strings(cands)
	rel := cands[rand.New(rand.NewPCG(op.Salt, 1)).IntN(len(cands))]
	return rel, appendRel(dir, rel, fmt.Sprintf("\n// workload edit %d\n", op.Seq))
}

// editGitignore appends a pattern to the root .gitignore or to one inside an
// intermediate directory, chosen from the salt.
func editGitignore(dir string, op Op) (string, error) {
	rel := ".gitignore"
	if op.Salt%2 == 1 {
		rel = fmt.Sprintf("%s/sub%d/.gitignore", workloadDir, op.Salt%3)
	}
	return rel, appendRel(dir, rel, fmt.Sprintf("ssq-ignored-%d.tmp\n", op.Seq))
}

// moveBase advances the base branch with a tree-identical commit via
// Storer.SetReference, simulating a fetch. The tree is unchanged so no
// worktree turns dirty.
func moveBase(dir string, op Op, when time.Time) (string, error) {
	repo, err := openRepo(dir)
	if err != nil {
		return "", err
	}
	var ref *plumbing.Reference
	for _, name := range []string{"main", "master"} {
		if r, err := repo.Reference(plumbing.NewBranchReferenceName(name), true); err == nil {
			ref = r
			break
		}
	}
	if ref == nil {
		return "", errors.New("no main/master branch")
	}
	base, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return "", err
	}
	next := &object.Commit{
		Author:       *signature(when),
		Committer:    *signature(when),
		Message:      fmt.Sprintf("workload: fetch %d", op.Seq),
		TreeHash:     base.TreeHash,
		ParentHashes: []plumbing.Hash{base.Hash},
	}
	obj := repo.Storer.NewEncodedObject()
	if err := next.Encode(obj); err != nil {
		return "", err
	}
	h, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", err
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(ref.Name(), h)); err != nil {
		return "", err
	}
	return ref.Name().Short() + " " + h.String(), nil
}

// realClock paces a live run against the wall clock. It is the only place the
// tool reads wall-clock time, and it is never used by tests or the schedule.
type realClock struct{ start time.Time }

func newRealClock() *realClock { return &realClock{start: time.Now()} }

func (c *realClock) WaitUntil(ctx context.Context, offset time.Duration) error {
	timer := time.NewTimer(time.Until(c.start.Add(offset)))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
