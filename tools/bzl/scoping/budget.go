package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Budget caps one package: non-test files, non-test LOC, direct imports of other module packages (ADR-010 Story 13.6).
type Budget struct {
	Pkg                string
	Files, LOC, Import int
}

const budgetRaiseTag = "scoping-budget-raise:"

// ParseBudget reads `pkg files=N loc=N imports=N` lines; `#` starts a comment.
func ParseBudget(r io.Reader) ([]Budget, error) {
	var out []Budget
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		b := Budget{Pkg: f[0]}
		for _, kv := range f[1:] {
			k, v, ok := strings.Cut(kv, "=")
			num, err := strconv.Atoi(v)
			if !ok || err != nil {
				return nil, fmt.Errorf("budget line %d: bad field %q", n, kv)
			}
			switch k {
			case "files":
				b.Files = num
			case "loc":
				b.LOC = num
			case "imports":
				b.Import = num
			default:
				return nil, fmt.Errorf("budget line %d: unknown key %q", n, k)
			}
		}
		out = append(out, b)
	}
	return out, sc.Err()
}

// Measure computes the budgeted quantities for each package directory with go list.
func Measure(root string, pkgs []string) (map[string]Budget, error) {
	args := []string{"list", "-e", "-json"}
	for _, p := range pkgs {
		args = append(args, "./"+p)
	}
	cmd := exec.Command("go", args...) //nolint:norawexec short-lived offline CLI; Run waits for exit
	cmd.Dir = root
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list: %w: %s", err, strings.TrimSpace(errb.String()))
	}
	lp, err := decodeList(&out)
	if err != nil {
		return nil, err
	}
	res := map[string]Budget{}
	for _, p := range lp {
		d := RelDir(p.ImportPath)
		b := Budget{Pkg: d, Files: len(p.GoFiles)}
		for _, f := range p.GoFiles {
			b.LOC += countLines(filepath.Join(p.Dir, f))
		}
		seen := map[string]bool{}
		for _, i := range p.Imports {
			if id := RelDir(i); inModule(i) && id != d && !seen[id] {
				seen[id] = true
				b.Import++
			}
		}
		res[d] = b
	}
	return res, nil
}

// OverBudget lists every package that exceeds a cap, or is missing.
func OverBudget(caps []Budget, got map[string]Budget) []string {
	var msgs []string
	for _, c := range caps {
		g, ok := got[c.Pkg]
		if !ok {
			msgs = append(msgs, fmt.Sprintf("%s: package not found (delete its budget line if it was removed)", c.Pkg))
			continue
		}
		if g.Files > c.Files {
			msgs = append(msgs, fmt.Sprintf("%s: %d non-test files, budget %d", c.Pkg, g.Files, c.Files))
		}
		if g.LOC > c.LOC {
			msgs = append(msgs, fmt.Sprintf("%s: %d non-test LOC, budget %d", c.Pkg, g.LOC, c.LOC))
		}
		if g.Import > c.Import {
			msgs = append(msgs, fmt.Sprintf("%s: %d direct internal imports, budget %d", c.Pkg, g.Import, c.Import))
		}
	}
	sort.Strings(msgs)
	return msgs
}

// RaisedCaps lists the caps of next that are higher than in prev (a new package line is not a raise).
func RaisedCaps(prev, next []Budget) []string {
	old := map[string]Budget{}
	for _, b := range prev {
		old[b.Pkg] = b
	}
	var out []string
	for _, n := range next {
		o, ok := old[n.Pkg]
		if !ok {
			continue
		}
		if n.Files > o.Files {
			out = append(out, fmt.Sprintf("%s files %d -> %d", n.Pkg, o.Files, n.Files))
		}
		if n.LOC > o.LOC {
			out = append(out, fmt.Sprintf("%s loc %d -> %d", n.Pkg, o.LOC, n.LOC))
		}
		if n.Import > o.Import {
			out = append(out, fmt.Sprintf("%s imports %d -> %d", n.Pkg, o.Import, n.Import))
		}
	}
	return out
}

// checkRaises fails a branch that raised a cap against base without a `scoping-budget-raise: <reason>` commit line.
func checkRaises(root, file, base string, next []Budget, stdout io.Writer) (bool, error) {
	prevTxt, err := git(root, "show", base+":"+file)
	if err != nil {
		fmt.Fprintf(stdout, "budget ratchet: %s has no %s; raise check skipped\n", base, file)
		return true, nil
	}
	prev, err := ParseBudget(strings.NewReader(prevTxt))
	if err != nil {
		return false, err
	}
	raised := RaisedCaps(prev, next)
	if len(raised) == 0 {
		return true, nil
	}
	msgs, err := git(root, "log", base+"..HEAD", "--format=%B")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(msgs, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), budgetRaiseTag) && strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), budgetRaiseTag)) != "" {
			return true, nil
		}
	}
	fmt.Fprintf(stdout, "budget raised without a `%s <reason>` commit message line: %s\n", budgetRaiseTag, strings.Join(raised, "; "))
	return false, nil
}

func runBudget(args []string, stdout io.Writer) int {
	file := "tools/bzl/scoping/budget.txt"
	root := workspace()
	base, initPkgs := "", ""
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--file":
			file = args[i+1]
		case "--root":
			root = args[i+1]
		case "--base":
			base = args[i+1]
		case "--init":
			initPkgs = args[i+1]
		}
	}
	if initPkgs != "" {
		return printInitialBudget(root, strings.Split(initPkgs, ","), stdout)
	}
	f, err := os.Open(filepath.Join(root, file))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer f.Close()
	caps, err := ParseBudget(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	pkgs := make([]string, len(caps))
	for i, c := range caps {
		pkgs[i] = c.Pkg
	}
	got, err := Measure(root, pkgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ok := true
	for _, m := range OverBudget(caps, got) {
		fmt.Fprintln(stdout, "over budget:", m)
		ok = false
	}
	if base != "" {
		good, err := checkRaises(root, file, base, caps, stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		ok = ok && good
	}
	if !ok {
		return 1
	}
	return 0
}

// printInitialBudget writes caps at the measured values rounded up by 3% (the plan's initial values).
func printInitialBudget(root string, pkgs []string, stdout io.Writer) int {
	got, err := Measure(root, pkgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	up := func(n int) int { return (n*103 + 99) / 100 }
	for _, p := range pkgs {
		g := got[p]
		fmt.Fprintf(stdout, "%s files=%d loc=%d imports=%d\n", p, up(g.Files), up(g.LOC), up(g.Import))
	}
	return 0
}
