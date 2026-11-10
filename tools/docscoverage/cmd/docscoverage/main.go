// Command docscoverage checks that registry features carry verified docs links
// and applies the per-feature ratchet (ADR-005).
//
//	docscoverage [-repo d] [-base sha|none] [-enforce=bool] [-flags file]   check and report
//	docscoverage baseline [-repo d]                                    write grandfather sidecars
//	docscoverage enforce [-repo d] [-base sha|none]                    print docsCoverageEnforce at the base
//
// Report-only unless docsCoverageEnforce is true in .github/promo-flags.json
// (or -enforce is passed). An unresolvable base never blocks. Exit codes: 0
// pass or report-only, 1 enforced violations, 2 tool error.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/check"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/registry"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/report"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/resolve"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
)

const (
	budgetsPath = "tools/docscoverage/budgets.json"
	flagsPath   = ".github/promo-flags.json"
	originMain  = "origin/main"
	noBase      = "none" // explicit -base value: report-only, for events with no base
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv)) }

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) > 0 && args[0] == "baseline" {
		return runBaseline(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "enforce" {
		return runEnforce(args[1:], stdout, stderr)
	}
	return runCheck(args, stdout, stderr, getenv)
}

// runEnforce prints true or false: whether docsCoverageEnforce is on at -base.
// CI reads it through here so the flag never comes from the PR's own tree.
func runEnforce(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("docscoverage enforce", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o checkOpts
	fs.StringVar(&o.repo, "repo", ".", "repository root")
	fs.StringVar(&o.base, "base", "", "base commit; same values as the check command")
	fs.StringVar(&o.flagsFile, "flags", flagsPath, "rollout flags file, relative to -repo")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	on, err := resolveEnforce(o)
	if err != nil {
		fmt.Fprintln(stderr, "docscoverage:", err)
		return 2
	}
	fmt.Fprintln(stdout, on)
	return 0
}

type checkOpts struct {
	repo, base, flagsFile string
	enforce               string
}

func runCheck(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("docscoverage", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o checkOpts
	fs.StringVar(&o.repo, "repo", ".", "repository root")
	fs.StringVar(&o.base, "base", "", "base commit (CI: HEAD^1 on pull_request, github.event.before on push); default merge-base with origin/main")
	fs.StringVar(&o.enforce, "enforce", "", "true|false; default reads docsCoverageEnforce from the flags file")
	fs.StringVar(&o.flagsFile, "flags", flagsPath, "rollout flags file, relative to -repo")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	res, opt, err := assess(o)
	if err != nil {
		fmt.Fprintln(stderr, "docscoverage:", err)
		return 2
	}
	report.Write(stdout, res, opt)
	if p := getenv("GITHUB_STEP_SUMMARY"); p != "" {
		if err := appendSummary(p, res, opt); err != nil {
			fmt.Fprintln(stderr, "docscoverage: write step summary:", err)
		}
	}
	if opt.Enforce && len(res.Violations) > 0 {
		return 1
	}
	return 0
}

func assess(o checkOpts) (check.Result, report.Options, error) {
	var opt report.Options
	in, err := loadInput(o.repo)
	if err != nil {
		return check.Result{}, opt, err
	}
	enforce, err := resolveEnforce(o)
	if err != nil {
		return check.Result{}, opt, err
	}
	base, notice := loadBase(o)
	in.Base = base
	if notice != "" {
		enforce = false
		opt.Notices = append(opt.Notices, notice)
	}
	opt.Enforce = enforce
	return check.Assess(in), opt, nil
}

func loadInput(repo string) (check.Input, error) {
	var in check.Input
	feat, err := registry.ReadDir(repo, registry.FeaturesDir)
	if err != nil {
		return in, err
	}
	if in.Registry, err = registry.Parse(feat); err != nil {
		return in, err
	}
	side, err := registry.ReadDir(repo, registry.SidecarDir)
	if err != nil {
		return in, err
	}
	in.Sidecars = registry.ParseSidecars(side)
	if in.Resolver, err = resolve.Load(repo); err != nil {
		return in, err
	}
	in.Budgets, err = readBudgets(repo)
	return in, err
}

func readBudgets(repo string) (check.Budgets, error) {
	b := check.Budgets{ExemptRatioCeiling: 0.1, MaxExemptIncrease: 5}
	data, err := os.ReadFile(filepath.Join(repo, budgetsPath))
	if errors.Is(err, fs.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return b, err
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return b, fmt.Errorf("%s: %w", budgetsPath, err)
	}
	return b, nil
}

// resolveEnforce: -enforce wins; else docsCoverageEnforce from the flags file
// as of the base commit, so a PR cannot flip its own gate (Cross-cutting 7a).
// A base that lacks the file means report-only (Cross-cutting 8a); a malformed
// file is an error. With no resolvable base (run report-only anyway) the
// working-tree file is parsed so a broken file still surfaces.
func resolveEnforce(o checkOpts) (bool, error) {
	switch o.enforce {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "":
	default:
		return false, fmt.Errorf("-enforce must be true or false, got %q", o.enforce)
	}
	data, err := readFlagsBlob(o)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, gitbase.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var f struct {
		DocsCoverageEnforce *bool `json:"docsCoverageEnforce"`
	}
	if err := json.Unmarshal(data, &f); err != nil || f.DocsCoverageEnforce == nil {
		return false, fmt.Errorf("%s: docsCoverageEnforce must be a boolean", o.flagsFile)
	}
	return *f.DocsCoverageEnforce, nil
}

// readFlagsBlob reads the flags file from the base blob, falling back to the
// working tree only when no base can be resolved.
func readFlagsBlob(o checkOpts) ([]byte, error) {
	if o.base != noBase {
		if r, err := gitbase.Open(o.repo); err == nil {
			base := o.base
			if base == "" {
				base, err = r.MergeBaseWith(originMain)
			}
			if err == nil {
				data, err := r.ReadBlob(base, o.flagsFile)
				var wrong *gitbase.WrongBaseError
				if !errors.As(err, &wrong) && !errors.Is(err, gitbase.ErrBaseUnresolved) {
					return data, err
				}
			}
		}
	}
	return os.ReadFile(filepath.Join(o.repo, o.flagsFile))
}

// loadBase never fails the run: any problem resolving the base yields a notice
// and a nil base, which makes the run report-only (Task 4.1.2a).
func loadBase(o checkOpts) (*check.Base, string) {
	if o.base == noBase {
		return nil, "no base for this event (workflow_dispatch, merge queue or similar); running report-only"
	}
	r, err := gitbase.Open(o.repo)
	if err != nil {
		return nil, fmt.Sprintf("base unresolved (%v); running report-only", err)
	}
	base := o.base
	if base == "" {
		if base, err = r.MergeBaseWith(originMain); err != nil {
			return nil, fmt.Sprintf("no -base given and %s is unavailable (%v); running report-only", originMain, err)
		}
	}
	feat, err := registry.ReadBase(r, base, registry.FeaturesDir)
	if err != nil {
		return nil, fmt.Sprintf("base %q unresolved (%v); running report-only", base, err)
	}
	reg, err := registry.Parse(feat)
	if err != nil {
		return nil, fmt.Sprintf("base registry unreadable (%v); running report-only", err)
	}
	side, err := registry.ReadBase(r, base, registry.SidecarDir)
	if err != nil {
		return nil, fmt.Sprintf("base sidecars unreadable (%v); running report-only", err)
	}
	return &check.Base{Registry: reg, Sidecars: registry.ParseSidecars(side)}, ""
}

func appendSummary(path string, res check.Result, opt report.Options) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	report.Write(f, res, opt)
	return nil
}
