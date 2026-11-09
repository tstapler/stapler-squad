// Command scoping measures how package layout drives Bazel invalidation cost (ADR-010, Epic 13): the T9
// baseline/replay over the real import graph and the test-inversion guard.
//
//	scoping replay    [--commits N] [--write]   T9 components as JSON; --write stores tools/bzl/scoping/baseline.json
//	scoping move      --from D --to D --files a.go,..  git mv a cluster into a new package and re-point references
//	scoping relocate  --dir D --to f.go --decls a,T.m  move declarations between files of one package (no package change)
//	scoping deps      --dir D --decls a,b       same-package declarations a, b reference transitively (helper extraction)
//	scoping leaves    --dir D [--min-loc N]  files nothing else in D uses, grouped into movable clusters
//	scoping touch     [--commits N] [--files a,b|--per-file]  T9b gain of moving files (changes that touch only them)
//	scoping criteria [--lower] --relocated N --uses N --importers a,b --natural a   SC1/SC3 verdicts (lower-package rule)
//	scoping t9        [--commits N]              T9a-T9f PASS|FAIL|UNMEASURED against baseline.json
//	scoping inversions [--json]                  exits 1 when a package's tests import a package that depends on it
//	scoping budget    [--file F] [--base REF]   per-package size/import caps (ratchet); --init a,b prints caps from the tree
//	scoping boundaries [--file F]               packages that must not depend (transitively) on others
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	baselineRel = "tools/bzl/scoping/baseline.json"
	prSampleRel = "tools/bzl/baseline/pr-sample.json"
)

// Report is the baseline.json / replay output; key names are the ones plan.md Epic 13 cites.
type Report struct {
	CommitSHA              string         `json:"commit_sha"`
	LogRefSHA              string         `json:"log_ref_sha"`
	NChanges               int            `json:"n_changes"`
	GoTouchingMedian       float64        `json:"go_touching_median"`
	T9aTestLOCShare500     float64        `json:"t9a_test_loc_share_500"`
	T9bUniversalTouch100   float64        `json:"t9b_universal_touch_100"`
	T9bUniversalTouch500   float64        `json:"t9b_universal_touch_500"`
	T9cMeanLibLOCShare     float64        `json:"t9c_mean_lib_loc_share"`
	T9cChangesWithLowerLib float64        `json:"t9c_changes_with_lower_lib_loc"`
	T9dServicesLOC         int            `json:"t9d_services_loc"`
	T9dSessionLOC          int            `json:"t9d_session_loc"`
	T9dTestInversions      int            `json:"t9d_test_inversions"`
	T9eTargetModelShare    float64        `json:"t9e_target_model_share"`
	PRCount                int            `json:"pr_count"`
	PerChangeLibLOC        map[string]int `json:"per_change_lib_loc,omitempty"`
}

func git(root string, args ...string) (string, error) {
	//nolint:norawexec dev-time measurement tool, short-lived and waited on
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// ParseRenames reads `git diff -M --name-status` output into an old-path to new-path map.
func ParseRenames(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		f := strings.Split(line, "\t")
		if len(f) == 3 && strings.HasPrefix(f[0], "R") {
			out[f[1]] = f[2]
		}
	}
	return out
}

func loadPRs(root string) ([]Change, error) {
	b, err := os.ReadFile(filepath.Join(root, prSampleRel))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Partial []struct {
			Number int      `json:"number"`
			Paths  []string `json:"paths"`
		} `json:"partial"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	var prs []Change
	for _, p := range doc.Partial {
		prs = append(prs, Change{ID: fmt.Sprint(p.Number), Paths: p.Paths})
	}
	return prs, nil
}

func workspace() string {
	if d := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); d != "" {
		return d
	}
	d, _ := os.Getwd()
	return d
}

func measure(root string, commits int, base *Report, ref string) (*Report, error) {
	m, err := LoadModel(root)
	if err != nil {
		return nil, err
	}
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	changes, res, refSHA, err := historyWindow(root, m, commits, base, ref)
	if err != nil {
		return nil, err
	}
	w500 := Replay(m, res, changes)
	w100 := Replay(m, res, changes[:min(100, len(changes))])
	prs, err := loadPRs(root)
	if err != nil {
		return nil, err
	}
	target, merged, npr := PRMedians(m, res, prs)
	r := &Report{
		CommitSHA: strings.TrimSpace(head), LogRefSHA: refSHA, NChanges: w500.NChanges,
		GoTouchingMedian: merged, T9aTestLOCShare500: w500.TestLOCShare,
		T9bUniversalTouch100: w100.UniversalTouch, T9bUniversalTouch500: w500.UniversalTouch,
		T9cMeanLibLOCShare: w500.LibLOCShare, T9dServicesLOC: m.LOC["server/services"], T9dSessionLOC: m.LOC["session"],
		T9dTestInversions: len(m.Inversions()), T9eTargetModelShare: target, PRCount: npr,
		PerChangeLibLOC: w500.LibLOCByChange,
	}
	if base != nil {
		r.T9cChangesWithLowerLib = LowerShare(base.PerChangeLibLOC, w500.LibLOCByChange, total(m.LOC))
	}
	return r, nil
}

func readBaseline(root string) *Report {
	b, err := os.ReadFile(filepath.Join(root, baselineRel))
	if err != nil {
		return nil
	}
	var r Report
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

func runReplay(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	commits := fs.Int("commits", 500, "number of main commits to replay")
	write := fs.Bool("write", false, "write "+baselineRel+" (refuses to overwrite)")
	root := fs.String("root", workspace(), "repository root")
	ref := fs.String("ref", "main", "history ref to replay (a stored baseline pins its own)")
	if fs.Parse(args) != nil {
		return 2
	}
	var base *Report
	if !*write {
		base = readBaseline(*root)
	}
	r, err := measure(*root, *commits, base, *ref)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if *write {
		p := filepath.Join(*root, baselineRel)
		if _, err := os.Stat(p); err == nil {
			fmt.Fprintln(os.Stderr, "baseline.json exists; delete it deliberately to re-baseline")
			return 1
		}
		var buf bytes.Buffer
		e := json.NewEncoder(&buf)
		e.SetIndent("", "  ")
		if e.Encode(r) != nil || os.WriteFile(p, buf.Bytes(), 0o644) != nil {
			fmt.Fprintln(os.Stderr, "write baseline failed")
			return 1
		}
		return 0
	}
	if enc.Encode(r) != nil {
		return 1
	}
	return 0
}

// runT9 replays against the stored baseline and prints the T9 verdicts; exit 1 when any component fails.
func runT9(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("t9", flag.ContinueOnError)
	commits := fs.Int("commits", 500, "number of main commits to replay")
	root := fs.String("root", workspace(), "repository root")
	if fs.Parse(args) != nil {
		return 2
	}
	base := readBaseline(*root)
	if base == nil {
		fmt.Fprintln(os.Stderr, "no baseline.json; run `scoping replay --write` first")
		return 2
	}
	r, err := measure(*root, *commits, base, "")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if PrintT9(stdout, r, DefaultT9Targets, T9fLine(*root)) {
		return 0
	}
	return 1
}

func runInversions(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("inversions", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	allow := fs.String("allow", "", "allowlist file of known inversions (a ratchet: new ones fail, resolved ones must be removed)")
	root := fs.String("root", workspace(), "repository root")
	if fs.Parse(args) != nil {
		return 2
	}
	m, err := LoadModel(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	inv := m.Inversions()
	if *allow != "" {
		return ratchetInversions(*root, *allow, inv, stdout)
	}
	if *asJSON {
		_ = json.NewEncoder(stdout).Encode(inv)
	}
	for _, i := range inv {
		if !*asJSON {
			fmt.Fprintf(stdout, "inversion: tests of %s import %s, which depends on %s\n", i.Pkg, i.Imported, i.Pkg)
		}
	}
	if len(inv) > 0 {
		return 1
	}
	return 0
}

func runMove(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("move", flag.ContinueOnError)
	spec := MoveSpec{}
	files := fs.String("files", "", "comma-separated file names (non-test and test) in --from to move")
	fs.StringVar(&spec.FromDir, "from", "", "origin package directory, relative to the root")
	fs.StringVar(&spec.ToDir, "to", "", "new package directory, relative to the root")
	fs.StringVar(&spec.Root, "root", workspace(), "repository root")
	fs.BoolVar(&spec.DryRun, "dry-run", false, "report the plan without touching files")
	fs.BoolVar(&spec.AllowCollisions, "allow-collisions", false, "measurement only: skip the capitalisation-collision refusal (the result does not compile)")
	fs.BoolVar(&spec.Lower, "lower", false, "the staying files depend on the cluster (test-support package): re-point them instead of reporting")
	if fs.Parse(args) != nil || spec.FromDir == "" || spec.ToDir == "" || *files == "" {
		fmt.Fprintln(os.Stderr, "usage: scoping move --from DIR --to DIR --files a.go,b.go [--dry-run]")
		return 2
	}
	spec.Files = strings.Split(*files, ",")
	if err := RunMove(spec, stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runRelocate(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("relocate", flag.ContinueOnError)
	spec := RelocateSpec{}
	decls := fs.String("decls", "", "comma-separated declarations: Name, or Type.Method")
	fs.StringVar(&spec.Dir, "dir", "", "package directory, relative to the root")
	fs.StringVar(&spec.ToFile, "to", "", "destination file name inside --dir (created when absent)")
	fs.StringVar(&spec.Root, "root", workspace(), "repository root")
	fs.BoolVar(&spec.AnyKind, "any-kind", false, "allow moving declarations between _test.go and non-test files")
	fs.BoolVar(&spec.WithMethods, "with-methods", false, "also move every method of each listed type")
	fs.BoolVar(&spec.DryRun, "dry-run", false, "report the plan without touching files")
	if fs.Parse(args) != nil || spec.Dir == "" || spec.ToFile == "" || *decls == "" {
		fmt.Fprintln(os.Stderr, "usage: scoping relocate --dir DIR --to file.go --decls A,B,T.M [--with-methods] [--dry-run]")
		return 2
	}
	spec.Decls = strings.Split(*decls, ",")
	if err := RunRelocate(spec, stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func ratchetInversions(root, allowFile string, inv []Inversion, stdout io.Writer) int {
	b, err := os.ReadFile(filepath.Join(root, allowFile))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	violations, stale := Ratchet(inv, ParseAllowlist(string(b)))
	for _, v := range violations {
		fmt.Fprintf(stdout, "new inversion: tests of %s import %s, which depends on %s\n", v.Pkg, v.Imported, v.Pkg)
	}
	for _, s := range stale {
		fmt.Fprintf(stdout, "stale allowlist entry (resolved, delete it from %s): %s %s\n", allowFile, s.Pkg, s.Imported)
	}
	if len(violations)+len(stale) > 0 {
		return 1
	}
	return 0
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: scoping replay|inversions [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "replay":
		os.Exit(runReplay(os.Args[2:], os.Stdout))
	case "inversions":
		os.Exit(runInversions(os.Args[2:], os.Stdout))
	case "move":
		os.Exit(runMove(os.Args[2:], os.Stdout))
	case "relocate":
		os.Exit(runRelocate(os.Args[2:], os.Stdout))
	case "closure":
		os.Exit(runClosure(os.Args[2:], os.Stdout))
	case "peel":
		os.Exit(runPeel(os.Args[2:], os.Stdout))
	case "deps":
		os.Exit(runDeps(os.Args[2:], os.Stdout))
	case "budget":
		os.Exit(runBudget(os.Args[2:], os.Stdout))
	case "boundaries":
		os.Exit(runBoundaries(os.Args[2:], os.Stdout))
	case "leaves":
		os.Exit(runLeaves(os.Args[2:], os.Stdout))
	case "touch":
		os.Exit(runTouch(os.Args[2:], os.Stdout))
	case "criteria":
		os.Exit(runCriteria(os.Args[2:], os.Stdout))
	case "t9":
		os.Exit(runT9(os.Args[2:], os.Stdout))
	default:
		fmt.Fprintln(os.Stderr, "unknown subcommand", os.Args[1])
		os.Exit(2)
	}
}

// historyWindow reads the replayed commits and the rename map that carries their paths into the current tree. A stored
// baseline pins both the history ref and the tree the renames start from.
func historyWindow(root string, m *Model, commits int, base *Report, ref string) ([]Change, Resolver, string, error) {
	if base != nil && base.LogRefSHA != "" {
		ref = base.LogRefSHA
	}
	refSHA, err := git(root, "rev-parse", ref)
	if err != nil {
		return nil, Resolver{}, "", err
	}
	logOut, err := git(root, "log", ref, fmt.Sprintf("-%d", commits), "--name-only", "--pretty=@@%H")
	if err != nil {
		return nil, Resolver{}, "", err
	}
	res := Resolver{Model: m}
	if base != nil && base.CommitSHA != "" {
		diff, err := git(root, "diff", "-M", "--name-status", base.CommitSHA, "HEAD")
		if err != nil {
			return nil, Resolver{}, "", err
		}
		res.Renames = ParseRenames(diff)
	}
	return ParseGitLog(strings.NewReader(logOut)), res, strings.TrimSpace(refSHA), nil
}
