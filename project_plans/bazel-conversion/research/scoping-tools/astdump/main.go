package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Decl struct {
	Name string `json:"n"`
	Kind string `json:"k"` // func,type,var,const,method
	Recv string `json:"r,omitempty"`
}
type Sel struct{ X, S string }
type File struct {
	Dir     string      `json:"dir"`
	Name    string      `json:"name"`
	Test    bool        `json:"test"`
	Pkg     string      `json:"pkg"`
	LOC     int         `json:"loc"`
	Imports []string    `json:"imports"` // as "alias path" (alias may be empty)
	Decls   []Decl      `json:"decls"`
	Idents  []string    `json:"idents"`
	Sels    [][2]string `json:"sels"`
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.IndexListExpr:
		return recvName(t.X)
	}
	return "?"
}

func main() {
	root := os.Args[1]
	var out []File
	fset := token.NewFileSet()
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			n := info.Name()
			if n == "node_modules" || n == ".git" || n == ".claude" || n == "web-app" || n == "worktrees" || n == "vendor" || strings.HasPrefix(n, ".") && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		src, _ := os.ReadFile(p)
		f, err := parser.ParseFile(fset, p, src, parser.SkipObjectResolution|parser.ParseComments)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		fd := File{Dir: filepath.Dir(rel), Name: filepath.Base(rel), Test: strings.HasSuffix(p, "_test.go"), Pkg: f.Name.Name, LOC: strings.Count(string(src), "\n")}
		for _, im := range f.Imports {
			a := ""
			if im.Name != nil {
				a = im.Name.Name
			}
			fd.Imports = append(fd.Imports, a+" "+strings.Trim(im.Path.Value, "\""))
		}
		// collect locally-bound names to avoid false positives: params, locals via simple heuristics skipped.
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.FuncDecl:
				if x.Recv != nil && len(x.Recv.List) > 0 {
					fd.Decls = append(fd.Decls, Decl{x.Name.Name, "method", recvName(x.Recv.List[0].Type)})
				} else {
					fd.Decls = append(fd.Decls, Decl{x.Name.Name, "func", ""})
				}
			case *ast.GenDecl:
				for _, s := range x.Specs {
					switch sp := s.(type) {
					case *ast.TypeSpec:
						fd.Decls = append(fd.Decls, Decl{sp.Name.Name, "type", ""})
					case *ast.ValueSpec:
						k := "var"
						if x.Tok == token.CONST {
							k = "const"
						}
						for _, n := range sp.Names {
							fd.Decls = append(fd.Decls, Decl{n.Name, k, ""})
						}
					}
				}
			}
		}
		idset := map[string]bool{}
		selset := map[[2]string]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok {
					selset[[2]string{id.Name, x.Sel.Name}] = true
				}
				ast.Inspect(x.X, func(m ast.Node) bool {
					if id, ok := m.(*ast.Ident); ok {
						idset[id.Name] = true
					}
					return true
				})
				return false
			case *ast.Ident:
				idset[x.Name] = true
			}
			return true
		})
		for k := range idset {
			fd.Idents = append(fd.Idents, k)
		}
		for k := range selset {
			fd.Sels = append(fd.Sels, k)
		}
		sort.Strings(fd.Idents)
		out = append(out, fd)
		return nil
	})
	json.NewEncoder(os.Stdout).Encode(out)
}
