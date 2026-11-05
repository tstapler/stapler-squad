package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

// UniversalFiles returns, per production Go change, the files (renamed to the current tree) that sit directly in a
// universal-slice package. Changes that touch no production Go are dropped; changes that touch only packages outside
// the universal set are kept with an empty set, because they stay in the T9b denominator.
func UniversalFiles(r Resolver, changes []Change) []map[string]bool {
	var out []map[string]bool
	for _, c := range changes {
		prod := false
		files := map[string]bool{}
		for _, p := range c.Paths {
			n, isTest, ok := r.Resolve(p)
			if !ok || isTest || !strings.HasSuffix(p, ".go") {
				continue
			}
			prod = true
			if universalDirs[n] {
				if renamed, moved := r.Renames[p]; moved {
					p = renamed
				}
				files[p] = true
			}
		}
		if prod {
			out = append(out, files)
		}
	}
	return out
}

// SoleGain counts the changes whose universal files all lie in set (and that touch at least one): moving the set out
// of the universal packages removes exactly these changes from the T9b numerator.
func SoleGain(changes []map[string]bool, set map[string]bool) int {
	n := 0
	for _, files := range changes {
		if len(files) == 0 {
			continue
		}
		all := true
		for f := range files {
			if !set[f] {
				all = false
				break
			}
		}
		if all {
			n++
		}
	}
	return n
}

// Family is the file-name stem before the first underscore ("backlog_service.go" -> "backlog"), the grouping the
// package already uses informally.
func Family(file string) string {
	b := strings.TrimSuffix(path.Base(file), ".go")
	if i := strings.Index(b, "_"); i > 0 {
		b = b[:i]
	}
	return path.Dir(file) + "/" + b
}

type touchRow struct {
	Name  string
	Gain  int
	Files int
}

// FamilyGains ranks families (and single files when perFile) by SoleGain.
func FamilyGains(changes []map[string]bool, perFile bool) []touchRow {
	groups := map[string]map[string]bool{}
	for _, files := range changes {
		for f := range files {
			k := Family(f)
			if perFile {
				k = f
			}
			if groups[k] == nil {
				groups[k] = map[string]bool{}
			}
			groups[k][f] = true
		}
	}
	var rows []touchRow
	for k, set := range groups {
		rows = append(rows, touchRow{Name: k, Gain: SoleGain(changes, set), Files: len(set)})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Gain != rows[j].Gain {
			return rows[i].Gain > rows[j].Gain
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

func runTouch(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("touch", flag.ContinueOnError)
	commits := fs.Int("commits", 500, "number of main commits to replay")
	files := fs.String("files", "", "comma-separated module-relative files: print the T9b gain of moving exactly these")
	perFile := fs.Bool("per-file", false, "rank single files instead of name families")
	top := fs.Int("top", 25, "rows to print")
	root := fs.String("root", workspace(), "repository root")
	if fs.Parse(args) != nil {
		return 2
	}
	m, err := LoadModel(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	changes, res, _, err := historyWindow(*root, m, *commits, readBaseline(*root), "main")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sets := UniversalFiles(res, changes)
	hit := 0
	for _, s := range sets {
		if len(s) > 0 {
			hit++
		}
	}
	fmt.Fprintf(stdout, "prod_changes=%d universal_hits=%d touch=%.4f\n", len(sets), hit, float64(hit)/float64(max(len(sets), 1)))
	if *files != "" {
		set := map[string]bool{}
		for _, f := range strings.Split(*files, ",") {
			set[f] = true
		}
		g := SoleGain(sets, set)
		fmt.Fprintf(stdout, "gain=%d (%.2f pp)\n", g, 100*float64(g)/float64(max(len(sets), 1)))
		return 0
	}
	for i, r := range FamilyGains(sets, *perFile) {
		if i >= *top {
			break
		}
		fmt.Fprintf(stdout, "%4d  %5.2f pp  %3d files  %s\n", r.Gain, 100*float64(r.Gain)/float64(max(len(sets), 1)), r.Files, r.Name)
	}
	return 0
}
