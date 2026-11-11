package main

import (
	"fmt"
	"go/ast"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// ClosureReport is the declaration-level cut for moving a cluster of files whose staying test files use it.
type ClosureReport struct {
	// Moves maps a staying file to the declarations that depend on the cluster and must move with it.
	Moves map[string][]string
	// Exports lists staying non-test declarations the moved code uses (they get capitalised in the core).
	Exports []string
	// Free lists staying test declarations the moved code uses that reach no non-test declaration (svctest candidates).
	Free []string
	// Bound lists staying test declarations the moved code uses that reach the core's own declarations.
	Bound []string
	// UsedBy maps a boundary declaration to "moved users/staying users".
	UsedBy map[string]string
	// Why maps a staying declaration that moves to the cluster-file declarations it references directly.
	Why map[string][]string
}

type declGraph struct {
	files map[string]string   // top-level name -> declaring file (base name)
	refs  map[string][]string // top-level name -> referenced top-level names
}

func loadDeclGraph(root, dir string) (*declGraph, error) {
	cfg := &packages.Config{
		Dir: root, Tests: true,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
	}
	pkgs, err := packages.Load(cfg, "./"+dir)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", dir, err)
	}
	var p *packages.Package
	for _, c := range pkgs {
		if !strings.HasSuffix(c.PkgPath, "_test") && strings.HasSuffix(c.ID, ".test]") {
			p = c
		}
	}
	if p == nil {
		return nil, fmt.Errorf("no test variant of %s loaded", dir)
	}
	files := map[string]string{}
	nodes := map[string][]ast.Node{}
	for _, f := range p.Syntax {
		base := filepath.Base(p.Fset.Position(f.Pos()).Filename)
		for _, d := range f.Decls {
			addTopLevel(d, base, files, nodes)
		}
	}
	g := &declGraph{files: files, refs: map[string][]string{}}
	for name, ns := range nodes {
		seen := map[string]bool{}
		for _, n := range ns {
			ast.Inspect(n, func(x ast.Node) bool {
				id, ok := x.(*ast.Ident)
				if !ok {
					return true
				}
				obj := p.TypesInfo.Uses[id]
				if obj == nil || obj.Pkg() != p.Types || obj.Parent() != p.Types.Scope() || id.Name == name {
					return true
				}
				seen[id.Name] = true
				return true
			})
		}
		for r := range seen {
			g.refs[name] = append(g.refs[name], r)
		}
		sort.Strings(g.refs[name])
	}
	return g, nil
}

func (g *declGraph) reachesNonTest(name string) bool {
	seen := map[string]bool{name: true}
	queue := []string{name}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, r := range g.refs[cur] {
			if seen[r] {
				continue
			}
			seen[r] = true
			if !isTestFile(g.files[r]) {
				return true
			}
			queue = append(queue, r)
		}
	}
	return false
}

// movedSet is the declaration-level fixpoint: the named files' declarations, every staying declaration that uses a
// moved one, and every staying test-only declaration whose users are all moved (it follows its users).
func (g *declGraph) movedSet(movedFile map[string]bool, also []string) map[string]bool {
	moved := map[string]bool{}
	for n, f := range g.files {
		if movedFile[f] && n != "_" && n != "init" {
			moved[n] = true
		}
	}
	for _, n := range also {
		moved[n] = true
	}
	for changed := true; changed; {
		changed = false
		users := map[string][]string{}
		for u, rs := range g.refs {
			for _, r := range rs {
				users[r] = append(users[r], u)
			}
		}
		for n, rs := range g.refs {
			if moved[n] || n == "_" || n == "init" {
				continue
			}
			for _, r := range rs {
				if moved[r] {
					moved[n] = true
					changed = true
					break
				}
			}
		}
		for n := range g.files {
			if moved[n] || n == "TestMain" || !isTestFile(g.files[n]) || len(users[n]) == 0 {
				continue
			}
			all := true
			for _, u := range users[n] {
				all = all && moved[u]
			}
			if all {
				moved[n] = true
				changed = true
			}
		}
	}
	return moved
}

// Closure computes the declaration-level boundary for moving the named files of dir: every staying declaration that
// (transitively) uses a moved declaration moves with it, and the declarations the moved code still uses are classified.
func Closure(root, dir string, moveFiles, also []string) (*ClosureReport, error) {
	g, err := loadDeclGraph(root, dir)
	if err != nil {
		return nil, err
	}
	movedFile := map[string]bool{}
	for _, f := range moveFiles {
		movedFile[f] = true
	}
	moved := g.movedSet(movedFile, also)
	rep := &ClosureReport{Moves: map[string][]string{}, UsedBy: map[string]string{}, Why: map[string][]string{}}
	for n := range moved {
		if f := g.files[n]; !movedFile[f] {
			rep.Moves[f] = append(rep.Moves[f], n)
			for _, r := range g.refs[n] {
				if movedFile[g.files[r]] {
					rep.Why[n] = append(rep.Why[n], r)
				}
			}
		}
	}
	for f := range rep.Moves {
		sort.Strings(rep.Moves[f])
	}
	used := map[string]bool{}
	for n := range moved {
		for _, r := range g.refs[n] {
			if !moved[r] {
				used[r] = true
			}
		}
	}
	for n := range used {
		switch {
		case !isTestFile(g.files[n]):
			rep.Exports = append(rep.Exports, n)
		case g.reachesNonTest(n):
			rep.Bound = append(rep.Bound, n)
		default:
			rep.Free = append(rep.Free, n)
		}
		mv, st := 0, 0
		var who []string
		for u, rs := range g.refs {
			for _, r := range rs {
				if r != n {
					continue
				}
				if moved[u] {
					mv++
					who = append(who, u)
				} else {
					st++
				}
			}
		}
		sort.Strings(who)
		rep.UsedBy[n] = fmt.Sprintf("%d/%d [%s]", mv, st, strings.Join(who, " "))
	}
	sort.Strings(rep.Exports)
	sort.Strings(rep.Free)
	sort.Strings(rep.Bound)
	return rep, nil
}

func runClosure(args []string, stdout io.Writer) int {
	var dir, files, also string
	root := workspace()
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--dir":
			dir = args[i+1]
		case "--files":
			files = args[i+1]
		case "--also":
			also = args[i+1]
		case "--root":
			root = args[i+1]
		}
	}
	if dir == "" || files == "" {
		fmt.Fprintln(os.Stderr, "usage: scoping closure --dir DIR --files a.go,b.go [--also decl,decl]")
		return 2
	}
	var alsoList []string
	if also != "" {
		alsoList = strings.Split(also, ",")
	}
	rep, err := Closure(root, dir, strings.Split(files, ","), alsoList)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var fs []string
	for f := range rep.Moves {
		fs = append(fs, f)
	}
	sort.Strings(fs)
	for _, f := range fs {
		fmt.Fprintf(stdout, "MOVE %s: %s\n", f, strings.Join(rep.Moves[f], ","))
	}
	var wn []string
	for n := range rep.Why {
		wn = append(wn, n)
	}
	sort.Strings(wn)
	for _, n := range wn {
		fmt.Fprintf(stdout, "WHY %s <- %s\n", n, strings.Join(rep.Why[n], ","))
	}
	fmt.Fprintf(stdout, "EXPORT %s\n", strings.Join(rep.Exports, ","))
	fmt.Fprintf(stdout, "FREE %s\n", strings.Join(rep.Free, ","))
	fmt.Fprintf(stdout, "BOUND %s\n", strings.Join(rep.Bound, ","))
	for _, n := range append(append([]string{}, rep.Bound...), rep.Free...) {
		fmt.Fprintf(stdout, "USE %s %s\n", n, rep.UsedBy[n])
	}
	return 0
}

// PeelCandidate is a cluster declaration that staying declarations use but that does not itself depend on the anchor
// type: it can be relocated out of the cluster instead of dragging its users along.
type PeelCandidate struct {
	Name, File string
	Test       bool
	// NeedsCore is true when its closure reaches a declaration outside the cluster files that is not test-only.
	NeedsCore bool
	// Closure is the cluster declarations it needs (itself included), which must be relocated together.
	Closure []string
	Users   []string
}

// Peel lists cluster declarations (declared in the named files) referenced by non-cluster declarations whose closure
// inside the cluster never reaches the anchor declaration.
func Peel(root, dir string, clusterFiles []string, anchor string) ([]PeelCandidate, error) {
	g, err := loadDeclGraph(root, dir)
	if err != nil {
		return nil, err
	}
	inCluster := map[string]bool{}
	cf := map[string]bool{}
	for _, f := range clusterFiles {
		cf[f] = true
	}
	for n, f := range g.files {
		if cf[f] {
			inCluster[n] = true
		}
	}
	users := map[string][]string{}
	for u, rs := range g.refs {
		if inCluster[u] {
			continue
		}
		for _, r := range rs {
			if inCluster[r] {
				users[r] = append(users[r], u)
			}
		}
	}
	var out []PeelCandidate
	for n, us := range users {
		seen := map[string]bool{n: true}
		queue := []string{n}
		needsCore := false
		bound := false
		for len(queue) > 0 && !bound {
			cur := queue[0]
			queue = queue[1:]
			for _, r := range g.refs[cur] {
				if r == anchor {
					bound = true
				}
				if seen[r] {
					continue
				}
				seen[r] = true
				if inCluster[r] {
					queue = append(queue, r)
				} else if !isTestFile(g.files[r]) {
					needsCore = true
				}
			}
		}
		if bound {
			continue
		}
		c := PeelCandidate{Name: n, File: g.files[n], Test: isTestFile(g.files[n]), NeedsCore: needsCore, Users: us}
		for x := range seen {
			if inCluster[x] {
				c.Closure = append(c.Closure, x)
			}
		}
		sort.Strings(c.Closure)
		sort.Strings(c.Users)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func runPeel(args []string, stdout io.Writer) int {
	var dir, files, anchor string
	root := workspace()
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--dir":
			dir = args[i+1]
		case "--files":
			files = args[i+1]
		case "--anchor":
			anchor = args[i+1]
		case "--root":
			root = args[i+1]
		}
	}
	if dir == "" || files == "" || anchor == "" {
		fmt.Fprintln(os.Stderr, "usage: scoping peel --dir DIR --files a.go,b.go --anchor Type")
		return 2
	}
	cands, err := Peel(root, dir, strings.Split(files, ","), anchor)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, c := range cands {
		kind := "prod"
		if c.Test {
			kind = "test"
		}
		fmt.Fprintf(stdout, "PEEL %s (%s, %s, needs-core=%v) closure=[%s] users=%d\n", c.Name, c.File, kind, c.NeedsCore, strings.Join(c.Closure, ","), len(c.Users))
	}
	return 0
}
