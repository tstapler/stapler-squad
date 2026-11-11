// Command e2e-compare is the P7 comparator (Task 7.5b): the Playwright pass count against the Bazel-built
// binary must equal the make-built binary's, and the sorted /metrics name sets must be identical.
//
// Running Playwright is a workflow step (`cd tests/e2e && TEST_SERVER_BINARY=<bin> bun run test` with the JSON
// reporter); this command only judges the outputs. With --from-env the four paths come from E2E_BAZEL_REPORT,
// E2E_MAKE_REPORT, E2E_BAZEL_METRICS and E2E_MAKE_METRICS. Exit 0 only when both comparisons hold.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
)

var metricName = regexp.MustCompile(`^[a-z_]+`)

// PassCount reads a Playwright JSON report and returns stats.expected (tests that passed as expected).
func PassCount(r io.Reader) (int, error) {
	var rep struct {
		Stats *struct {
			Expected int `json:"expected"`
		} `json:"stats"`
	}
	if err := json.NewDecoder(r).Decode(&rep); err != nil {
		return 0, err
	}
	if rep.Stats == nil {
		return 0, fmt.Errorf("report has no stats object")
	}
	return rep.Stats.Expected, nil
}

// MetricNames returns the sorted unique metric names of Prometheus text (`grep -oE '^[a-z_]+' | sort -u`).
func MetricNames(r io.Reader) []string {
	set := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		if m := metricName.FindString(sc.Text()); m != "" {
			set[m] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Diff returns names only in a and only in b.
func Diff(a, b []string) (onlyA, onlyB []string) {
	ia, ib := map[string]bool{}, map[string]bool{}
	for _, x := range a {
		ia[x] = true
	}
	for _, x := range b {
		ib[x] = true
	}
	for _, x := range a {
		if !ib[x] {
			onlyA = append(onlyA, x)
		}
	}
	for _, x := range b {
		if !ia[x] {
			onlyB = append(onlyB, x)
		}
	}
	return
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func readFile[T any](path string, f func(io.Reader) (T, error)) (T, error) {
	fh, err := os.Open(path)
	var zero T
	if err != nil {
		return zero, err
	}
	defer fh.Close()
	return f(fh)
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("e2e-compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fromEnv := fs.Bool("from-env", false, "take the four paths from E2E_* environment variables")
	br := fs.String("bazel-report", "", "Playwright JSON report of the Bazel-binary run")
	mr := fs.String("make-report", "", "Playwright JSON report of the make-binary run")
	bm := fs.String("bazel-metrics", "", "/metrics text from the Bazel-built binary")
	mm := fs.String("make-metrics", "", "/metrics text from the make-built binary")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *fromEnv {
		*br, *mr, *bm, *mm = os.Getenv("E2E_BAZEL_REPORT"), os.Getenv("E2E_MAKE_REPORT"), os.Getenv("E2E_BAZEL_METRICS"), os.Getenv("E2E_MAKE_METRICS")
	}
	if *br == "" || *mr == "" || *bm == "" || *mm == "" {
		fmt.Fprintln(stderr, "e2e-compare: need all of bazel/make report and metrics paths")
		return 2
	}
	nb, err1 := readFile(*br, PassCount)
	nm, err2 := readFile(*mr, PassCount)
	mb, err3 := readFile(*bm, func(r io.Reader) ([]string, error) { return MetricNames(r), nil })
	mk, err4 := readFile(*mm, func(r io.Reader) ([]string, error) { return MetricNames(r), nil })
	for _, e := range []error{err1, err2, err3, err4} {
		if e != nil {
			fmt.Fprintln(stderr, "e2e-compare:", e)
			return 2
		}
	}
	onlyB, onlyM := Diff(mb, mk)
	equal := len(onlyB) == 0 && len(onlyM) == 0 && len(mb) > 0
	fmt.Fprintf(stdout, "e2e_pass_bazel=%d e2e_pass_make=%d metrics_equal=%t\n", nb, nm, equal)
	for _, n := range onlyB {
		fmt.Fprintln(stdout, "metric only in bazel build:", n)
	}
	for _, n := range onlyM {
		fmt.Fprintln(stdout, "metric only in make build:", n)
	}
	if nb != nm || nb == 0 || !equal {
		return 1
	}
	return 0
}
