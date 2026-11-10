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
	"strings"
)

// Boundary says package Pkg must not depend, even transitively, on a package under Forbidden (module-relative).
type Boundary struct {
	Pkg       string
	Forbidden string
}

// ParseBoundaries reads `pkg !forbidden/prefix` lines; `#` starts a comment.
func ParseBoundaries(r io.Reader) ([]Boundary, error) {
	var out []Boundary
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
		if len(f) != 2 || !strings.HasPrefix(f[1], "!") {
			return nil, fmt.Errorf("boundaries line %d: want `pkg !forbidden`, got %q", n, line)
		}
		out = append(out, Boundary{Pkg: f[0], Forbidden: strings.TrimPrefix(f[1], "!")})
	}
	return out, sc.Err()
}

// ViolatedBoundaries returns one message per boundary whose package depends on a forbidden package, using deps
// (module-relative directories reachable from the package, as `go list -deps` reports them).
func ViolatedBoundaries(bs []Boundary, deps map[string][]string) []string {
	var msgs []string
	for _, b := range bs {
		for _, d := range deps[b.Pkg] {
			if d == b.Forbidden || strings.HasPrefix(d, b.Forbidden+"/") {
				msgs = append(msgs, fmt.Sprintf("%s must not depend on %s (reaches %s)", b.Pkg, b.Forbidden, d))
				break
			}
		}
	}
	sort.Strings(msgs)
	return msgs
}

func listDeps(root, pkg string) ([]string, error) {
	cmd := exec.Command("go", "list", "-e", "-deps", "-f", "{{.ImportPath}}", "./"+pkg) //nolint:norawexec short-lived offline CLI; Run waits for exit
	cmd.Dir = root
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list -deps %s: %w: %s", pkg, err, strings.TrimSpace(errb.String()))
	}
	var deps []string
	for _, l := range strings.Fields(out.String()) {
		if inModule(l) {
			deps = append(deps, RelDir(l))
		}
	}
	return deps, nil
}

func runBoundaries(args []string, stdout io.Writer) int {
	file := "tools/bzl/scoping/boundaries.txt"
	root := workspace()
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--file":
			file = args[i+1]
		case "--root":
			root = args[i+1]
		}
	}
	f, err := os.Open(filepath.Join(root, file))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer f.Close()
	bs, err := ParseBoundaries(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	deps := map[string][]string{}
	for _, b := range bs {
		if _, done := deps[b.Pkg]; done {
			continue
		}
		d, err := listDeps(root, b.Pkg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		deps[b.Pkg] = d
	}
	msgs := ViolatedBoundaries(bs, deps)
	for _, m := range msgs {
		fmt.Fprintln(stdout, "boundary violated:", m)
	}
	if len(msgs) > 0 {
		return 1
	}
	return 0
}
