// Command e2eslice is the ADR-009 observed-dependency toolchain: manifest merge/check, registry coverage,
// diff-to-spec selection, selection replay, BUILD generation and shard planning (Epic 12, spike S14).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	e2eDir       = "tests/e2e"
	manifestPath = "tests/e2e/slice-manifest.json"
	globalsPath  = "tools/bzl/e2e-global-inputs.txt"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, out, errw io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "usage: e2eslice <check-manifest|check-completeness|check-slow-suites|check-quarantine|merge|list-groups|specs-of|registry-coverage|select|replay|universal-share|gen-build|plan-shards|check-global-inputs|check-no-binary-dep> [flags]")
		return 2
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(errw)
	root := fs.String("root", ".", "repository root")
	asJSON := fs.Bool("json", false, "machine-readable output")
	mp := fs.String("manifest", manifestPath, "slice manifest, relative to -root")
	gp := fs.String("global-inputs", globalsPath, "global inputs list, relative to -root")
	outFile := fs.String("out", "", "output file")
	commit := fs.String("commit", "", "merge: generated_at commit")
	n := fs.Int("n", 100, "replay: commits")
	rev := fs.String("rev", "main", "replay: revision")
	closureMode := fs.String("closure", "none", "none|static: widen each spec's packages by the static import closure")
	root1 := fs.String("universal-root", modulePath+"/server", "universal-share: package every spec observes")
	shards := fs.Int("shards", 5, "plan-shards: runner count")
	durations := fs.String("durations", "", "plan-shards: durations.json {target: seconds}")
	onlyManaged := fs.Bool("only-managed", false, "select/replay: treat only manifest specs as existing (partial-manifest sample runs)")
	groupSize := fs.Int("group-size", 0, "override specs per target (1 measures per-spec share on a sample)")
	group := fs.String("group", "", "specs-of: target name")
	gran := fs.String("granularity", "line", "select-synth: package|file|line")
	capsDir := fs.String("caps", "", "replay-fine: directory of per-spec e2ecapture JSON")
	check := fs.Bool("check", false, "gen-build: fail if the committed file differs")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	rest := fs.Args()
	if *groupSize > 0 {
		maxGroup = *groupSize
	}
	R := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(*root, p)
	}
	fail := func(err error) int { fmt.Fprintln(errw, "e2eslice:", err); return 1 }
	emit := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintln(out, string(b))
	}

	switch args[0] {
	case "check-manifest":
		m, err := readManifest(R(*mp))
		if err != nil {
			return fail(err)
		}
		specs, err := listSpecs(*root, e2eDir)
		if err != nil {
			return fail(err)
		}
		problems := checkManifest(m, specs)
		for _, p := range problems {
			fmt.Fprintln(errw, "manifest:", p)
		}
		if len(problems) > 0 {
			return 1
		}
		fmt.Fprintf(out, "manifest ok: %d specs\n", len(m.Specs))
	case "merge":
		var caps []Entry
		for _, f := range rest {
			b, err := os.ReadFile(f)
			if err != nil {
				return fail(err)
			}
			var e []Entry
			if err := json.Unmarshal(b, &e); err != nil {
				var one Entry
				if err2 := json.Unmarshal(b, &one); err2 != nil {
					return fail(fmt.Errorf("%s: %w", f, err))
				}
				e = []Entry{one}
			}
			caps = append(caps, e...)
		}
		if err := merge(*commit, caps).write(R(*mp)); err != nil {
			return fail(err)
		}
	case "registry-coverage":
		idx, err := loadRegistry(*root)
		if err != nil {
			return fail(err)
		}
		specs, err := listSpecs(*root, e2eDir)
		if err != nil {
			return fail(err)
		}
		cov, err := registryCoverage(*root, specs, idx)
		if err != nil {
			return fail(err)
		}
		if *outFile != "" {
			b, _ := json.MarshalIndent(cov, "", "  ")
			if err := os.WriteFile(R(*outFile), append(b, '\n'), 0o644); err != nil {
				return fail(err)
			}
		}
		if *asJSON {
			emit(cov)
		} else {
			fmt.Fprintf(out, "specs %d\nwith feature id %d\nwith resolvable id %d\nall ids resolve %d\nwith existing source file %d (go %d, web %d)\ndistinct ids %d, unresolved %d\n",
				cov.Specs, cov.WithFeatureID, cov.WithResolvableID, cov.AllIDsResolve, cov.WithExistingFile, cov.WithGoFile, cov.WithWebFile, cov.DistinctIDs, len(cov.UnresolvedIDs))
		}
	case "select", "replay":
		sel, err := buildSelector(*root, *mp, *gp, *closureMode, *onlyManaged)
		if err != nil {
			return fail(err)
		}
		if args[0] == "select" {
			files := rest
			if len(files) == 0 {
				files = readLines(stdin)
			}
			s := sel.Select(files)
			if *asJSON {
				emit(map[string]any{"all": s.All, "groups": s.Groups, "unmanaged": s.Unmanaged, "share": s.Share()})
				break
			}
			if s.All {
				fmt.Fprintln(out, allMarker)
			}
			for _, g := range s.Groups {
				fmt.Fprintln(out, "//tests/e2e:"+g)
			}
			break
		}
		cs, err := gitLog(*root, *rev, *n)
		if err != nil {
			return fail(err)
		}
		st := replay(cs, sel.Select)
		emit(st)
	case "replay-fine":
		caps, err := loadFineCaptures(R(*capsDir))
		if err != nil {
			return fail(err)
		}
		globs, err := loadGlobs(R(*gp))
		if err != nil {
			return fail(err)
		}
		specs, err := listSpecs(*root, e2eDir)
		if err != nil {
			return fail(err)
		}
		cs, err := gitLog(*root, *rev, *n)
		if err != nil {
			return fail(err)
		}
		var res []FineStats
		for _, g := range []Granularity{GranPackage, GranFile, GranLine, GranLineStrict} {
			res = append(res, replayFine(cs, newFineSelector(caps, specs, globs, g, gitHunks(*root))))
		}
		emit(res)
	case "select-synth":
		// select-synth -caps DIR -granularity G FILE[:A-B]...: the specs a synthetic one-file diff selects.
		caps, err := loadFineCaptures(R(*capsDir))
		if err != nil {
			return fail(err)
		}
		globs, err := loadGlobs(R(*gp))
		if err != nil {
			return fail(err)
		}
		specs, err := listSpecs(*root, e2eDir)
		if err != nil {
			return fail(err)
		}
		var files []string
		hunks := map[string][]Line{}
		for _, a := range rest {
			f, rng, _ := strings.Cut(a, ":")
			files = append(files, f)
			if lo, hi, ok := strings.Cut(rng, "-"); ok {
				l, _ := strconv.Atoi(lo)
				h, _ := strconv.Atoi(hi)
				hunks[f] = append(hunks[f], Line{l, h})
			}
		}
		fs := newFineSelector(caps, specs, globs, Granularity(*gran), func(string, []string) map[string][]Line { return hunks })
		s := fs.Select(Commit{SHA: "synthetic", Files: files})
		emit(map[string]any{"all": s.All, "specs": s.Hit})
	case "list-groups", "specs-of":
		sel, err := buildSelector(*root, *mp, *gp, "none", false)
		if err != nil {
			return fail(err)
		}
		for _, g := range sel.groups {
			switch {
			case args[0] == "list-groups":
				fmt.Fprintln(out, "//tests/e2e:"+g.Name)
			case g.Name == strings.TrimPrefix(*group, "//tests/e2e:"):
				for _, sp := range g.Specs {
					fmt.Fprintln(out, strings.TrimPrefix(sp, e2eDir+"/"))
				}
			}
		}
	case "check-quarantine":
		rows, err := readQuarantine(R(e2eDir + "/quarantine.json"))
		if err != nil {
			return fail(err)
		}
		build, _ := os.ReadFile(R(e2eDir + "/BUILD.bazel"))
		bad := checkQuarantine(rows, string(build))
		for _, b := range bad {
			fmt.Fprintln(errw, "quarantine:", b)
		}
		if len(bad) > 0 {
			return 1
		}
	case "check-slow-suites":
		b, err := os.ReadFile(R("docs/reference/bazel-slow-suites.md"))
		if err != nil {
			return fail(err)
		}
		bad := checkSlowSuites(string(b))
		for _, m := range bad {
			fmt.Fprintln(errw, "slow suite:", m)
		}
		if len(bad) > 0 {
			return 1
		}
	case "check-completeness":
		idx, err := loadRegistry(*root)
		if err != nil {
			return fail(err)
		}
		specs, err := listSpecs(*root, e2eDir)
		if err != nil {
			return fail(err)
		}
		cov, err := registryCoverage(*root, specs, idx)
		if err != nil {
			return fail(err)
		}
		var recorded Coverage
		if b, err := os.ReadFile(R(e2eDir + "/feature-gaps.json")); err != nil || json.Unmarshal(b, &recorded) != nil {
			return fail(fmt.Errorf("tests/e2e/feature-gaps.json missing or invalid (run registry-coverage -out)"))
		}
		grown := growthOfGaps(cov.UnresolvedIDs, recorded.UnresolvedIDs)
		for _, id := range grown {
			fmt.Fprintln(errw, "feature id resolves nowhere and is not in feature-gaps.json:", id)
		}
		if len(grown) > 0 {
			return 1
		}
	case "universal-share":
		return universalShare(*root, *rev, *n, *root1, out, errw)
	case "gen-build":
		m, err := readManifest(R(*mp))
		if err != nil {
			return fail(err)
		}
		globs, err := loadGlobs(R(*gp))
		if err != nil {
			return fail(err)
		}
		text := genBuild(*root, m, globs, loadQuarantine(R(e2eDir+"/quarantine.json")))
		dst := R(e2eDir + "/BUILD.bazel")
		if *check {
			cur, err := os.ReadFile(dst)
			if err != nil || string(cur) != text {
				fmt.Fprintln(errw, "tests/e2e/BUILD.bazel is out of date; run: e2eslice gen-build")
				return 1
			}
			break
		}
		if err := os.WriteFile(dst, []byte(text), 0o644); err != nil {
			return fail(err)
		}
	case "plan-shards":
		targets := rest
		if len(targets) == 0 {
			targets = readLines(stdin)
		}
		d := map[string]float64{}
		if *durations != "" {
			b, err := os.ReadFile(*durations)
			if err != nil {
				return fail(err)
			}
			if err := json.Unmarshal(b, &d); err != nil {
				return fail(err)
			}
		}
		emit(map[string]any{"include": planShards(targets, d, *shards)})
	case "check-global-inputs":
		globs, err := loadGlobs(R(*gp))
		if err != nil {
			return fail(err)
		}
		missing := checkGlobalInputs(*root, globs)
		for _, g := range missing {
			fmt.Fprintln(errw, "global input matches no file:", g)
		}
		if len(missing) > 0 {
			return 1
		}
	case "check-no-binary-dep":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return fail(err)
		}
		bad := checkNoBinaryDep(string(b))
		for _, l := range bad {
			fmt.Fprintln(errw, "e2e slice depends on the binary seam:", l)
		}
		if len(bad) > 0 {
			return 1
		}
	default:
		fmt.Fprintln(errw, "unknown subcommand", args[0])
		return 2
	}
	return 0
}

func readLines(r io.Reader) []string {
	b, _ := io.ReadAll(r)
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func loadQuarantine(p string) map[string]bool {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var rows []struct {
		Spec string `json:"spec"`
	}
	_ = json.Unmarshal(b, &rows)
	q := map[string]bool{}
	for _, r := range rows {
		q[r.Spec] = true
	}
	return q
}

func buildSelector(root, mp, gp, closureMode string, onlyManaged bool) (*Selector, error) {
	abs := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(root, p)
	}
	m, err := readManifest(abs(mp))
	if err != nil {
		return nil, fmt.Errorf("%w (no manifest: run the capture first, see Task 12.1a)", err)
	}
	globs, err := loadGlobs(abs(gp))
	if err != nil {
		return nil, err
	}
	specs, err := listSpecs(root, e2eDir)
	if err != nil {
		return nil, err
	}
	if onlyManaged {
		specs = nil
		for _, e := range m.Specs {
			specs = append(specs, e.Spec)
		}
	}
	var closure func([]string) []string
	if closureMode == "static" {
		g, err := loadGraph(root)
		if err != nil {
			return nil, err
		}
		closure = g.Closure
	}
	return newSelector(m, globs, specs, closure), nil
}

// universalShare is the manifest-free lower bound of ADR-009's design: every spec boots the server, so every
// spec's slice contains the static closure of the server package. A commit touching that closure selects all
// specs. Prints how many relevant commits do, with the per-directory breakdown.
func universalShare(root, rev string, n int, universalRoot string, out, errw io.Writer) int {
	g, err := loadGraph(root)
	if err != nil {
		fmt.Fprintln(errw, "e2eslice:", err)
		return 1
	}
	cl := map[string]bool{}
	for _, p := range g.Closure([]string{universalRoot}) {
		cl[p] = true
	}
	cs, err := gitLog(root, rev, n)
	if err != nil {
		fmt.Fprintln(errw, "e2eslice:", err)
		return 1
	}
	rel, all, webOnly := 0, 0, 0
	for _, c := range cs {
		if !relevant(c.Files) {
			continue
		}
		rel++
		hit := false
		for _, f := range c.Files {
			if strings.HasSuffix(f, ".go") && isProdGoSurface(f) && cl[pkgOf(f)] {
				hit = true
			}
		}
		if hit {
			all++
		} else if !hasGo(c.Files) {
			webOnly++
		}
	}
	b, _ := json.Marshal(map[string]any{"commits": len(cs), "relevant": rel, "closure_packages": len(cl), "module_packages": len(g),
		"select_all_by_go_closure": all, "web_only_undetermined": webOnly, "go_closure_share": ratio(all, rel)})
	fmt.Fprintln(out, string(b))
	return 0
}

func hasGo(files []string) bool {
	for _, f := range files {
		if strings.HasSuffix(f, ".go") && isProdGoSurface(f) {
			return true
		}
	}
	return false
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
