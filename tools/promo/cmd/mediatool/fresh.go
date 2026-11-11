package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tstapler/stapler-squad/tools/promo/internal/flags"
	"github.com/tstapler/stapler-squad/tools/promo/internal/manifest"
)

type freshFlags struct {
	manifests   stringList
	expected    string
	budgets     string
	dist        string
	sha         string
	reuseFrom   string
	compareJSON string
	decideReuse bool
	summary     bool
	flagsPath   string
	prevDist    int64
}

func parseFreshFlags(args []string, stderr io.Writer, getenv func(string) string) (freshFlags, error) {
	var f freshFlags
	fs := flag.NewFlagSet("fresh", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Var(&f.manifests, "manifest", "manifest file (repeatable; shards are merged)")
	fs.StringVar(&f.expected, "expected", "", "expected-media JSON (website/expected-media.json)")
	fs.StringVar(&f.budgets, "budgets", "tools/promo/budgets.json", "budgets file")
	fs.StringVar(&f.dist, "dist", "", "built site dir; enables the existence and total-size checks")
	fs.StringVar(&f.sha, "sha", getenv("GITHUB_SHA"), "commit the media must match (default $GITHUB_SHA)")
	fs.StringVar(&f.reuseFrom, "reuse-from", "", "recorded commit to reuse media from (content-only deploys)")
	fs.StringVar(&f.compareJSON, "compare-json", "", "output of gh api repos/{owner}/{repo}/compare/<recorded>...<sha> --paginate")
	fs.BoolVar(&f.decideReuse, "decide-reuse", false, "print record or reuse and exit")
	fs.BoolVar(&f.summary, "summary", false, "append the table to $GITHUB_STEP_SUMMARY")
	fs.StringVar(&f.flagsPath, "flags", flags.Path, "promo-flags.json in the working tree (push, schedule and dispatch runs)")
	fs.Int64Var(&f.prevDist, "prev-dist-bytes", -1, "previous run's dist size, for the summary trend")
	return f, fs.Parse(args)
}

// readCompare loads the compare result; ok is false when no path was given.
func readCompare(path string) (c manifest.Compare, ok bool, err error) {
	if path == "" {
		return c, false, nil
	}
	fh, err := os.Open(path)
	if err != nil {
		return c, false, err
	}
	defer fh.Close()
	c, err = manifest.ParseCompare(fh)
	return c, err == nil, err
}

func runFresh(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	f, err := parseFreshFlags(args, stderr, getenv)
	if err != nil {
		return 2
	}
	code, err := fresh(f, stdout, getenv("GITHUB_STEP_SUMMARY"))
	if err != nil {
		fmt.Fprintln(stderr, "mediatool fresh:", err)
		return 2
	}
	return code
}

func fresh(f freshFlags, stdout io.Writer, summaryPath string) (int, error) {
	m, err := loadMerged(f.manifests)
	if err != nil {
		return 0, err
	}
	b, err := manifest.ReadBudgets(f.budgets)
	if err != nil {
		return 0, err
	}
	compare, haveCompare, err := readCompare(f.compareJSON)
	if err != nil {
		return 0, err
	}
	var cmp *manifest.Compare
	if haveCompare {
		cmp = &compare
	}
	if f.decideReuse {
		return decide(f, m, b, cmp, stdout)
	}
	if f.expected == "" {
		return 0, fmt.Errorf("-expected is required outside -decide-reuse")
	}
	exp, err := manifest.ReadExpected(f.expected)
	if err != nil {
		return 0, err
	}
	fl, err := flags.FromTree(f.flagsPath)
	if err != nil {
		return 0, err
	}
	problems := manifest.Fresh(manifest.FreshInput{Manifest: m, Expected: exp, Budgets: b, DistDir: f.dist, SHA: f.sha, ReuseFrom: f.reuseFrom, Compare: cmp})
	if f.summary {
		if err := emitSummary(m, f.dist, f.prevDist, stdout, summaryPath); err != nil {
			return 0, err
		}
	}
	return report(problems, fl, f.reuseFrom, stdout), nil
}

func decide(f freshFlags, m manifest.MediaManifest, b manifest.Budgets, cmp *manifest.Compare, stdout io.Writer) (int, error) {
	if cmp == nil {
		return 0, fmt.Errorf("-decide-reuse needs -compare-json")
	}
	if f.reuseFrom == "" && m.Commit == "" {
		return 0, fmt.Errorf("-decide-reuse needs -reuse-from or a manifest with a commit")
	}
	if manifest.DecideReuse(*cmp, b.RecordInputPaths).Reusable {
		fmt.Fprintln(stdout, "reuse")
	} else {
		fmt.Fprintln(stdout, "record")
	}
	return 0, nil
}

func report(problems []manifest.Problem, fl flags.Flags, reuseFrom string, stdout io.Writer) int {
	for _, p := range problems {
		fmt.Fprintln(stdout, p)
	}
	if len(problems) == 0 {
		if reuseFrom != "" {
			fmt.Fprintf(stdout, "reused media from %s\n", reuseFrom)
		}
		return 0
	}
	if !fl.FreshnessEnforce {
		fmt.Fprintln(stdout, "report-only: freshnessEnforce is false in .github/promo-flags.json")
		return 0
	}
	return 1
}
