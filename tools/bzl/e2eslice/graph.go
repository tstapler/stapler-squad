package main

import (
	"bufio"
	"os/exec"
	"sort"
	"strings"
)

// Graph is the in-module import graph (non-test imports), the static closure Gazelle's go_library deps encode.
type Graph map[string][]string

// parseGraph reads `go list -e -f '{{.ImportPath}} {{join .Imports " "}}'` output, keeping in-module edges.
func parseGraph(out string) Graph {
	g := Graph{}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		g[f[0]] = nil
		for _, imp := range f[1:] {
			if imp == modulePath || strings.HasPrefix(imp, modulePath+"/") {
				g[f[0]] = append(g[f[0]], imp)
			}
		}
	}
	return g
}

func loadGraph(repoRoot string) (Graph, error) {
	cmd := exec.Command("go", "list", "-e", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./...")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return parseGraph(string(out)), nil
}

// Closure returns roots plus everything they import transitively, sorted.
func (g Graph) Closure(roots []string) []string {
	seen := map[string]bool{}
	stack := append([]string(nil), roots...)
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[p] {
			continue
		}
		seen[p] = true
		stack = append(stack, g[p]...)
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
