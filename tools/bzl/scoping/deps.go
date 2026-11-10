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

// DepsReport lists, for each named declaration, the other package-level declarations of the same package that it
// references transitively (go/types resolved).
// It answers "can this test helper leave the package without dragging the package's types along".
type DepsReport struct {
	Decl     string
	File     string
	Closure  []string // "name (file)" of every transitively referenced package-level declaration
	TestOnly bool     // every referenced declaration is declared in a _test.go file
}

func addTopLevel(d ast.Decl, file string, files map[string]string, nodes map[string][]ast.Node) {
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil {
			files[d.Name.Name] = file
			nodes[d.Name.Name] = append(nodes[d.Name.Name], d)
			return
		}
		rt := recvTypeName(d.Recv.List[0].Type) // referencing the type pulls its method bodies in
		nodes[rt] = append(nodes[rt], d)
	case *ast.GenDecl:
		for _, s := range d.Specs {
			for _, n := range specNames(s) {
				files[n] = file
				nodes[n] = append(nodes[n], s)
			}
		}
	}
}

// DeclDeps computes the transitive same-package references of each named declaration, resolved with go/types
// (a local variable never counts as a package-level reference).
func DeclDeps(root, dir string, decls []string) ([]DepsReport, error) {
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
			p = c // the package plus its in-package _test.go files
		}
	}
	if p == nil {
		return nil, fmt.Errorf("no test variant of %s loaded", dir)
	}
	files := map[string]string{}     // top-level name -> file
	nodes := map[string][]ast.Node{} // top-level name -> declaring nodes (methods attach to their receiver type)
	for _, f := range p.Syntax {
		base := filepath.Base(p.Fset.Position(f.Pos()).Filename)
		for _, d := range f.Decls {
			addTopLevel(d, base, files, nodes)
		}
	}
	var out []DepsReport
	for _, d := range decls {
		if _, ok := files[d]; !ok {
			return nil, fmt.Errorf("declaration %s not found in %s", d, dir)
		}
		seen := map[string]bool{d: true}
		queue := []string{d}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, n := range nodes[cur] {
				ast.Inspect(n, func(x ast.Node) bool {
					id, ok := x.(*ast.Ident)
					if !ok {
						return true
					}
					obj := p.TypesInfo.Uses[id]
					if obj == nil || obj.Pkg() != p.Types || obj.Parent() != p.Types.Scope() || seen[id.Name] {
						return true
					}
					seen[id.Name] = true
					queue = append(queue, id.Name)
					return true
				})
			}
		}
		r := DepsReport{Decl: d, File: files[d], TestOnly: true}
		for n := range seen {
			if n == d {
				continue
			}
			r.Closure = append(r.Closure, fmt.Sprintf("%s (%s)", n, files[n]))
			if !isTestFile(files[n]) {
				r.TestOnly = false
			}
		}
		sort.Strings(r.Closure)
		out = append(out, r)
	}
	return out, nil
}

func runDeps(args []string, stdout io.Writer) int {
	var dir, decls string
	root := workspace()
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--dir":
			dir = args[i+1]
		case "--decls":
			decls = args[i+1]
		case "--root":
			root = args[i+1]
		}
	}
	if dir == "" || decls == "" {
		fmt.Fprintln(os.Stderr, "usage: scoping deps --dir DIR --decls a,b,c")
		return 2
	}
	rep, err := DeclDeps(root, dir, strings.Split(decls, ","))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, r := range rep {
		fmt.Fprintf(stdout, "%s (%s): %d deps, test-only=%v\n", r.Decl, r.File, len(r.Closure), r.TestOnly)
		for _, c := range r.Closure {
			fmt.Fprintf(stdout, "    %s\n", c)
		}
	}
	return 0
}
