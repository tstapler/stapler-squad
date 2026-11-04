package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/imports"
)

// RelocateSpec moves named top-level declarations between files of one package (no package change): the Story 13.4/13.5
// "relocate first" commit that lets a cluster move afterwards as a pure rename.
type RelocateSpec struct {
	Root        string
	Dir         string
	ToFile      string   // destination file name inside Dir (created when absent)
	Decls       []string // "Name" for a func/type/const/var, "Type.Method" for one method
	WithMethods bool     // also move every method of each listed type
	AnyKind     bool     // allow declarations to cross between _test.go and non-test files
	DryRun      bool
}

type cut struct {
	start, end int // byte range removed from the source file
	text       string
	imports    []*ast.ImportSpec
	file       string
}

// declFound records one located declaration for removal.
type declFound struct {
	name string
	cut  cut
}

func isTestFile(name string) bool { return strings.HasSuffix(name, "_test.go") }

// RunRelocate performs the move and writes a report to out.
func RunRelocate(spec RelocateSpec, out io.Writer) error {
	dir := filepath.Join(spec.Root, spec.Dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, d := range spec.Decls {
		want[d] = false
	}
	destTest := isTestFile(spec.ToFile)
	cuts := map[string][]cut{} // by source file
	srcs := map[string][]byte{}
	var pkgName string
	order := []string{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || n == spec.ToFile {
			continue
		}
		path := filepath.Join(dir, n)
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return err
		}
		if strings.HasSuffix(f.Name.Name, "_test") {
			continue
		}
		pkgName = f.Name.Name
		found := findDecls(fset, f, src, want, spec.WithMethods)
		if len(found) == 0 {
			continue
		}
		if !spec.AnyKind && isTestFile(n) != destTest {
			return fmt.Errorf("%s declares %s: test and non-test declarations cannot be relocated into %s", n, found[0].name, spec.ToFile)
		}
		srcs[n] = src
		for _, d := range found {
			d.cut.imports = f.Imports
			d.cut.file = n
			cuts[n] = append(cuts[n], d.cut)
			fmt.Fprintf(out, "relocate %s from %s to %s\n", d.name, n, spec.ToFile)
		}
		order = append(order, n)
	}
	var missing []string
	for d, ok := range want {
		if !ok {
			missing = append(missing, d)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("declarations not found in %s: %s", spec.Dir, strings.Join(missing, ", "))
	}
	if spec.DryRun {
		return nil
	}
	return applyRelocation(dir, spec.ToFile, pkgName, order, cuts, srcs)
}

func applyRelocation(dir, toFile, pkgName string, order []string, cuts map[string][]cut, srcs map[string][]byte) error {
	destPath := filepath.Join(dir, toFile)
	dest, err := os.ReadFile(destPath)
	if os.IsNotExist(err) {
		dest = []byte("package " + pkgName + "\n")
	} else if err != nil {
		return err
	}
	var body bytes.Buffer
	body.Write(dest)
	var allImports []*ast.ImportSpec
	for _, n := range order {
		cs := cuts[n]
		sort.Slice(cs, func(i, j int) bool { return cs[i].start < cs[j].start })
		src := srcs[n]
		var kept bytes.Buffer
		prev := 0
		for _, c := range cs {
			kept.Write(src[prev:c.start])
			prev = c.end
			body.WriteString("\n" + c.text + "\n")
			allImports = append(allImports, c.imports...)
		}
		kept.Write(src[prev:])
		if err := writeFormatted(filepath.Join(dir, n), kept.Bytes(), nil); err != nil {
			return err
		}
	}
	return writeFormatted(destPath, body.Bytes(), allImports)
}

// writeFormatted writes src, first seeding it with extra imports, then removing unused ones and formatting.
func writeFormatted(path string, src []byte, extra []*ast.ImportSpec) error {
	if len(extra) > 0 {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for _, im := range extra {
			p := strings.Trim(im.Path.Value, "\"")
			name := ""
			if im.Name != nil {
				name = im.Name.Name
			}
			if name == "_" || name == "." {
				continue
			}
			astutil.AddNamedImport(fset, f, name, p)
		}
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, f); err != nil {
			return err
		}
		src = buf.Bytes()
	}
	out, err := imports.Process(path, src, &imports.Options{Comments: true, TabIndent: true, TabWidth: 8, FormatOnly: false})
	if err != nil {
		return fmt.Errorf("%s: %w\n%s", path, err, src)
	}
	return os.WriteFile(path, out, 0o644)
}

func findDecls(fset *token.FileSet, f *ast.File, src []byte, want map[string]bool, withMethods bool) []declFound {
	var out []declFound
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	lineStart := func(o int) int {
		for o > 0 && src[o-1] != '\n' {
			o--
		}
		return o
	}
	lineEnd := func(o int) int {
		for o < len(src) && src[o] != '\n' {
			o++
		}
		if o < len(src) {
			o++
		}
		return o
	}
	typeWanted := map[string]bool{}
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
			for _, s := range gd.Specs {
				if _, ok := want[s.(*ast.TypeSpec).Name.Name]; ok {
					typeWanted[s.(*ast.TypeSpec).Name.Name] = true
				}
			}
		}
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) == 1 {
				rt := recvTypeName(d.Recv.List[0].Type)
				key := rt + "." + name
				if _, ok := want[key]; ok {
					name = key
				} else if withMethods && typeWanted[rt] {
					want[key] = true
					name = key
				} else {
					continue
				}
			} else if _, ok := want[name]; !ok {
				continue
			}
			want[name] = true
			start := off(d.Pos())
			if d.Doc != nil {
				start = off(d.Doc.Pos())
			}
			s, e := lineStart(start), lineEnd(off(d.End()))
			out = append(out, declFound{name, cut{start: s, end: e, text: strings.TrimRight(string(src[s:e]), "\n")}})
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			out = append(out, findGenSpecs(fset, d, src, want, off, lineStart, lineEnd)...)
		}
	}
	return out
}

func recvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.IndexExpr:
		return recvTypeName(t.X)
	case *ast.IndexListExpr:
		return recvTypeName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func specNames(s ast.Spec) []string {
	switch s := s.(type) {
	case *ast.TypeSpec:
		return []string{s.Name.Name}
	case *ast.ValueSpec:
		var n []string
		for _, id := range s.Names {
			n = append(n, id.Name)
		}
		return n
	}
	return nil
}

func findGenSpecs(fset *token.FileSet, d *ast.GenDecl, src []byte, want map[string]bool, off func(token.Pos) int, lineStart, lineEnd func(int) int) []declFound {
	var hit []ast.Spec
	var names []string
	for _, s := range d.Specs {
		for _, n := range specNames(s) {
			if _, ok := want[n]; ok {
				want[n] = true
				hit = append(hit, s)
				names = append(names, n)
			}
		}
	}
	if len(hit) == 0 {
		return nil
	}
	if len(hit) == len(d.Specs) { // whole declaration goes
		start := off(d.Pos())
		if d.Doc != nil {
			start = off(d.Doc.Pos())
		}
		s, e := lineStart(start), lineEnd(off(d.End()))
		return []declFound{{strings.Join(names, ","), cut{start: s, end: e, text: strings.TrimRight(string(src[s:e]), "\n")}}}
	}
	var out []declFound
	for _, s := range hit {
		start := off(s.Pos())
		var doc *ast.CommentGroup
		switch s := s.(type) {
		case *ast.TypeSpec:
			doc = s.Doc
		case *ast.ValueSpec:
			doc = s.Doc
		}
		if doc != nil {
			start = off(doc.Pos())
		}
		cs, ce := lineStart(start), lineEnd(off(s.End()))
		body := strings.TrimSpace(string(src[off(s.Pos()):off(s.End())]))
		if vs, ok := s.(*ast.ValueSpec); ok && len(vs.Values) == 0 && d.Tok == token.CONST {
			body = "// (implicit value) " + body
		}
		text := d.Tok.String() + " " + body
		if doc != nil {
			text = strings.TrimSpace(string(src[off(doc.Pos()):off(doc.End())])) + "\n" + text
		}
		out = append(out, declFound{strings.Join(specNames(s), ","), cut{start: cs, end: ce, text: text}})
	}
	return out
}
