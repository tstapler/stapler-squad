package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DepsReport lists, for each named declaration, the other package-level declarations of the same package that it
// references transitively (syntactic; a local variable that shadows a package-level name is counted, so it errs high).
// It answers "can this test helper leave the package without dragging the package's types along".
type DepsReport struct {
	Decl     string
	File     string
	Closure  []string // "name (file)" of every transitively referenced package-level declaration
	TestOnly bool     // every referenced declaration is declared in a _test.go file
}

func collectTopLevel(dir string) (map[string]string, map[string][]ast.Node, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", dir, err)
	}
	files := map[string]string{}
	nodes := map[string][]ast.Node{}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		if strings.HasSuffix(f.Name.Name, "_test") {
			continue
		}
		for _, d := range f.Decls {
			addTopLevel(d, e.Name(), files, nodes)
		}
	}
	return files, nodes, nil
}

func addTopLevel(d ast.Decl, file string, files map[string]string, nodes map[string][]ast.Node) {
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil {
			files[d.Name.Name] = file
			nodes[d.Name.Name] = append(nodes[d.Name.Name], d)
			return
		}
		// A method belongs to its receiver type: referencing the type pulls the method bodies in.
		rt := recvTypeName(d.Recv.List[0].Type)
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

// DeclDeps computes the transitive same-package references of each named declaration.
func DeclDeps(dir string, decls []string) ([]DepsReport, error) {
	files, nodes, err := collectTopLevel(dir)
	if err != nil {
		return nil, err
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
					if id, ok := x.(*ast.Ident); ok && !seen[id.Name] {
						if _, top := files[id.Name]; top {
							seen[id.Name] = true
							queue = append(queue, id.Name)
						}
					}
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
	rep, err := DeclDeps(filepath.Join(root, dir), strings.Split(decls, ","))
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
