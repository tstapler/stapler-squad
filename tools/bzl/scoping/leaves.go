package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LeafCluster is a connected set of non-test files that no remaining file of the package uses, so it can leave the
// package without the core holding any of its declarations (SC1 = 0 non-test uses). Layer is the peeling round of
// its deepest file: a layer-2 file is used only by layer-1 files.
type LeafCluster struct {
	Files []string
	LOC   int
	Layer int
}

// fileEdges maps each non-test file to the non-test files whose declarations it references.
func (g *declGraph) fileEdges() map[string]map[string]bool {
	edges := map[string]map[string]bool{}
	for n, f := range g.files {
		if isTestFile(f) || n == "_" || n == "init" {
			continue
		}
		if edges[f] == nil {
			edges[f] = map[string]bool{}
		}
		for _, r := range g.refs[n] {
			if f2 := g.files[r]; f2 != f && !isTestFile(f2) {
				edges[f][f2] = true
			}
		}
	}
	return edges
}

// reverseEdges flips "f uses g" into "g is used by f", so peeling it finds the lower layers.
func reverseEdges(edges map[string]map[string]bool) map[string]map[string]bool {
	rev := map[string]map[string]bool{}
	for f := range edges {
		if rev[f] == nil {
			rev[f] = map[string]bool{}
		}
		for g := range edges[f] {
			if rev[g] == nil {
				rev[g] = map[string]bool{}
			}
			rev[g][f] = true
		}
	}
	return rev
}

// PeelLeaves repeatedly removes files nobody else in the package uses and groups what it removed into connected
// clusters. loc maps a file to its line count.
func PeelLeaves(edges map[string]map[string]bool, loc map[string]int) []LeafCluster {
	remaining := map[string]bool{}
	for f := range edges {
		remaining[f] = true
	}
	layer := map[string]int{}
	for round := 1; ; round++ {
		used := map[string]bool{}
		for f := range remaining {
			for g := range edges[f] {
				if remaining[g] {
					used[g] = true
				}
			}
		}
		var leaves []string
		for f := range remaining {
			if !used[f] {
				leaves = append(leaves, f)
			}
		}
		if len(leaves) == 0 {
			break
		}
		for _, f := range leaves {
			layer[f] = round
			delete(remaining, f)
		}
	}
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if parent[x] == x {
			return x
		}
		parent[x] = find(parent[x])
		return parent[x]
	}
	for f := range layer {
		parent[f] = f
	}
	for f := range layer {
		for g := range edges[f] {
			if _, ok := layer[g]; ok {
				parent[find(f)] = find(g)
			}
		}
	}
	byRoot := map[string]*LeafCluster{}
	for f, l := range layer {
		c := byRoot[find(f)]
		if c == nil {
			c = &LeafCluster{}
			byRoot[find(f)] = c
		}
		c.Files = append(c.Files, f)
		c.LOC += loc[f]
		c.Layer = max(c.Layer, l)
	}
	var out []LeafCluster
	for _, c := range byRoot {
		sort.Strings(c.Files)
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LOC != out[j].LOC {
			return out[i].LOC > out[j].LOC
		}
		return out[i].Files[0] < out[j].Files[0]
	})
	return out
}

func runLeaves(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("leaves", flag.ContinueOnError)
	bases := fs.Bool("bases", false, "peel files that use nothing else in the package (lower layers) instead of files nothing uses")
	dir := fs.String("dir", "", "package directory, relative to the root")
	minLOC := fs.Int("min-loc", 400, "smallest cluster to print (SC2)")
	root := fs.String("root", workspace(), "repository root")
	if fs.Parse(args) != nil || *dir == "" {
		fmt.Fprintln(os.Stderr, "usage: scoping leaves --dir DIR [--min-loc N]")
		return 2
	}
	g, err := loadDeclGraph(*root, *dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	edges := g.fileEdges()
	if *bases {
		edges = reverseEdges(edges)
	}
	loc := map[string]int{}
	for f := range edges {
		if b, err := os.ReadFile(filepath.Join(*root, *dir, f)); err == nil {
			loc[f] = strings.Count(string(b), "\n")
		}
	}
	for _, c := range PeelLeaves(edges, loc) {
		if c.LOC < *minLOC {
			continue
		}
		fmt.Fprintf(stdout, "LEAF loc=%d files=%d layer=%d %s\n", c.LOC, len(c.Files), c.Layer, strings.Join(c.Files, ","))
	}
	return 0
}
