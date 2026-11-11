package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

const mod = "github.com/tstapler/stapler-squad"

type Edge struct {
	UP, UF, DP, DF string
	Sym            string
	Exp            bool
	Kind           string
}

func main() {
	root := os.Args[1]
	cfg := &packages.Config{Dir: root, Tests: true, Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		panic(err)
	}
	seen := map[Edge]bool{}
	n := 0
	for _, p := range pkgs {
		if p.TypesInfo == nil || !strings.HasPrefix(p.PkgPath, mod) {
			continue
		}
		pp := strings.TrimSuffix(strings.Split(p.PkgPath, " ")[0], "_test")
		for _, f := range p.Syntax {
			upos := p.Fset.Position(f.Pos()).Filename
			ur, _ := filepath.Rel(root, upos)
			ast.Inspect(f, func(nd ast.Node) bool {
				id, ok := nd.(*ast.Ident)
				if !ok {
					return true
				}
				obj := p.TypesInfo.Uses[id]
				if obj == nil || obj.Pkg() == nil || !strings.HasPrefix(obj.Pkg().Path(), mod) {
					return true
				}
				var kind string
				switch o := obj.(type) {
				case *types.Func:
					kind = "func"
					if o.Type().(*types.Signature).Recv() != nil {
						kind = "method"
					}
				case *types.TypeName:
					kind = "type"
				case *types.Var:
					if o.IsField() {
						kind = "field"
					} else if o.Parent() == o.Pkg().Scope() {
						kind = "var"
					} else {
						return true
					}
				case *types.Const:
					if o.Parent() != o.Pkg().Scope() {
						return true
					}
					kind = "const"
				default:
					return true
				}
				dpos := p.Fset.Position(obj.Pos()).Filename
				dr, err := filepath.Rel(root, dpos)
				if err != nil || strings.HasPrefix(dr, "..") {
					return true
				}
				dp := obj.Pkg().Path()
				dp = strings.TrimSuffix(strings.Split(dp, " ")[0], "_test")
				e := Edge{pp, ur, dp, dr, obj.Name(), obj.Exported(), kind}
				if !seen[e] {
					seen[e] = true
					n++
				}
				return true
			})
		}
	}
	out := make([]Edge, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}
	json.NewEncoder(os.Stdout).Encode(out)
	fmt.Fprintln(os.Stderr, "edges", n)
}
