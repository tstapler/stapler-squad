package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// MoveSpec describes one behaviour-preserving package split: the listed files of FromDir (non-test and test) become
// package ToDir. References are re-pointed with go/types, never by name matching.
type MoveSpec struct {
	Root    string
	FromDir string
	ToDir   string
	Files   []string
	DryRun  bool
	// Lower moves a cluster the staying files depend on (a test-support package): staying files are re-pointed at
	// the new package instead of reported, and a moved file that uses a stay-behind symbol is an error.
	Lower bool
	// AllowCollisions skips the capitalisation-collision refusal: the result does not compile and is only good for
	// `scoping replay` (SC6 measurement of a cluster whose boundary has not been prepared yet).
	AllowCollisions bool
}

type edit struct {
	start, end int
	text       string
}

type fileEdits struct {
	src    []byte
	edits  []edit
	addImp map[string]string // import path -> name ("" = default)
}

type mover struct {
	spec     MoveSpec
	modPath  string
	origPath string
	newPath  string
	origName string
	newName  string
	moved    map[string]bool   // absolute paths of moved files
	declared map[string]string // origin package-level name -> absolute file declaring it (source-loaded origin only)
	rename   map[string]string // unexported origin symbol -> capitalised name
	files    map[string]*fileEdits
	uses     []useRef
	members  map[string]string // declaration position of an unexported method/field -> capitalised name
	memRefs  []memberRef
	notes    []string
}

// memberRef is one identifier occurrence of an unexported method or field of the origin package.
type memberRef struct {
	file       string
	start, end int
	key        string
}

// useRef is one identifier occurrence that refers to a package-level object of the origin package.
type useRef struct {
	file     string
	start    int
	end      int
	name     string
	declFile string
}

func trimVariant(p string) string {
	if i := strings.Index(p, " ["); i >= 0 {
		p = p[:i]
	}
	return strings.TrimSuffix(p, "_test")
}

func capitalise(s string) string {
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// RunMove applies spec and returns the human-readable notes (boundary symbols capitalised, files rewritten).
func RunMove(spec MoveSpec, out io.Writer) error {
	m := &mover{spec: spec, declared: map[string]string{}, members: map[string]string{}, rename: map[string]string{}, files: map[string]*fileEdits{}, moved: map[string]bool{}}
	if err := m.load(); err != nil {
		return err
	}
	for _, n := range m.notes {
		fmt.Fprintln(out, n)
	}
	return nil
}

func (m *mover) load() error {
	root, err := filepath.Abs(m.spec.Root)
	if err != nil {
		return err
	}
	m.spec.Root = root
	cfg := &packages.Config{
		Dir: root, Tests: true,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedModule,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return err
	}
	for _, p := range pkgs {
		if p.Module != nil && p.Module.Main {
			m.modPath = p.Module.Path
			break
		}
	}
	if m.modPath == "" {
		return fmt.Errorf("main module not found")
	}
	m.origPath = m.modPath + "/" + m.spec.FromDir
	m.newPath = m.modPath + "/" + m.spec.ToDir
	m.origName = filepath.Base(m.spec.FromDir)
	m.newName = filepath.Base(m.spec.ToDir)
	for _, f := range m.spec.Files {
		m.moved[filepath.Join(root, m.spec.FromDir, f)] = true
	}
	m.indexOrigin(pkgs)
	seen := map[string]bool{}
	for _, p := range pkgs { // test variants first so test files get their type info
		if p.ForTest == "" {
			continue
		}
		m.scan(p, seen)
	}
	for _, p := range pkgs {
		m.scan(p, seen)
	}
	for f := range m.moved {
		if !seen[f] {
			return fmt.Errorf("file %s not found in any loaded package", f)
		}
	}
	return m.apply()
}

func (m *mover) fileOf(p *packages.Package, f *ast.File) string {
	return p.Fset.Position(f.Pos()).Filename
}

// scan walks every file of p once, recording origin-package references and the edits they imply.
func (m *mover) scan(p *packages.Package, seen map[string]bool) {
	if p.TypesInfo == nil {
		return
	}
	for _, f := range p.Syntax {
		path := m.fileOf(p, f)
		if seen[path] || !strings.HasPrefix(path, m.spec.Root+"/") || !strings.HasSuffix(path, ".go") {
			continue
		}
		seen[path] = true
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fe := &fileEdits{src: src, addImp: map[string]string{}}
		m.files[path] = fe
		inOrigin := trimVariant(p.PkgPath) == m.origPath
		off := func(pos token.Pos) int { return p.Fset.Position(pos).Offset }
		selIdents := map[token.Pos]bool{} // x.Sel is never a bare package-level reference
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr: // importer: session.Name
				selIdents[x.Sel.Pos()] = true
				if inOrigin && m.moved[path] {
					m.noteMember(p, x.Sel)
				}
				qual, ok := x.X.(*ast.Ident)
				if !ok {
					return true
				}
				pn, ok := p.TypesInfo.Uses[qual].(*types.PkgName)
				if !ok || pn.Imported().Path() != m.origPath {
					return true
				}
				decl := m.declFile(p, p.TypesInfo.Uses[x.Sel])
				if decl != "" && m.moved[decl] {
					fe.edits = append(fe.edits, edit{off(qual.Pos()), off(qual.End()), m.newName})
					fe.addImp[m.newPath] = ""
				}
				if decl != "" && !m.moved[decl] && m.moved[path] {
					m.noteBoundary(p, x.Sel, qual, path, fe, off)
				}
			case *ast.Ident:
				if inOrigin {
					m.recordMember(p, x, path, off)
				}
				if !inOrigin || selIdents[x.Pos()] {
					return true
				}
				m.originIdent(p, x, path, fe, off)
			}
			return true
		})
	}
}

// indexOrigin records which file declares each package-level name of the origin package. Importers see the origin
// through export data whose positions are not source paths, so declaration files are looked up by name instead.
func (m *mover) indexOrigin(pkgs []*packages.Package) {
	for _, p := range pkgs {
		if trimVariant(p.PkgPath) != m.origPath || p.TypesInfo == nil {
			continue
		}
		for id, obj := range p.TypesInfo.Defs {
			if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == m.origPath && obj.Parent() == obj.Pkg().Scope() {
				m.declared[id.Name] = p.Fset.Position(id.Pos()).Filename
			}
		}
	}
}

func (m *mover) declFile(_ *packages.Package, obj types.Object) string {
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != m.origPath || obj.Parent() != obj.Pkg().Scope() {
		return ""
	}
	return m.declared[obj.Name()]
}

func memberKey(p *packages.Package, obj types.Object) string {
	pos := p.Fset.Position(obj.Pos())
	return fmt.Sprintf("%s:%d", pos.Filename, pos.Offset)
}

// unexportedMember reports whether obj is an unexported method or struct field declared in the origin package.
func (m *mover) unexportedMember(obj types.Object) bool {
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != m.origPath || obj.Exported() {
		return false
	}
	switch o := obj.(type) {
	case *types.Func:
		sig, ok := o.Type().(*types.Signature)
		return ok && sig.Recv() != nil
	case *types.Var:
		return o.IsField()
	}
	return false
}

// noteMember marks an unexported method or field of a staying type, used by a moved file, for capitalisation.
func (m *mover) noteMember(p *packages.Package, sel *ast.Ident) {
	obj := p.TypesInfo.Uses[sel]
	if !m.unexportedMember(obj) || m.moved[p.Fset.Position(obj.Pos()).Filename] || isTestFile(filepath.Base(p.Fset.Position(obj.Pos()).Filename)) {
		return
	}
	key := memberKey(p, obj)
	if _, done := m.members[key]; !done {
		up := capitalise(sel.Name)
		m.members[key] = up
		m.notes = append(m.notes, fmt.Sprintf("capitalise member %s -> %s", sel.Name, up))
	}
}

func (m *mover) recordMember(p *packages.Package, id *ast.Ident, path string, off func(token.Pos) int) {
	obj := p.TypesInfo.Uses[id]
	if obj == nil {
		obj = p.TypesInfo.Defs[id]
	}
	if m.unexportedMember(obj) {
		m.memRefs = append(m.memRefs, memberRef{file: path, start: off(id.Pos()), end: off(id.End()), key: memberKey(p, obj)})
	}
}

// noteBoundary handles a moved file that already imported the origin package and uses a stay-behind symbol.
func (m *mover) noteBoundary(p *packages.Package, sel, qual *ast.Ident, path string, fe *fileEdits, off func(token.Pos) int) {
	if sel.IsExported() {
		return
	}
	m.notes = append(m.notes, fmt.Sprintf("unexported symbol %s used across the boundary by %s (already qualified)", sel.Name, filepath.Base(path)))
}

// originIdent handles an identifier inside the origin package: a moved file using a stay-behind package-level
// symbol is qualified; a stay-behind file using a moved symbol is reported (the cluster must not be used by the core).
// Every reference to an unexported package-level symbol is remembered so a later capitalisation reaches all of them.
func (m *mover) originIdent(p *packages.Package, id *ast.Ident, path string, fe *fileEdits, off func(token.Pos) int) {
	obj := p.TypesInfo.Uses[id]
	if obj == nil {
		obj = p.TypesInfo.Defs[id]
	}
	decl := m.declFile(p, obj)
	if decl == "" {
		return
	}
	if !id.IsExported() {
		m.uses = append(m.uses, useRef{file: path, start: off(id.Pos()), end: off(id.End()), name: id.Name, declFile: decl})
	}
	if p.TypesInfo.Defs[id] != nil {
		return
	}
	switch {
	case m.spec.Lower && m.moved[path] && !m.moved[decl]:
		m.notes = append(m.notes, fmt.Sprintf("ERROR: %s (moved) uses stay-behind symbol %s: a lower package cannot import its origin", filepath.Base(path), id.Name))
	case m.spec.Lower && !m.moved[path] && m.moved[decl]:
		name := id.Name
		if !id.IsExported() {
			name = capitalise(name)
			m.rename[id.Name] = name
		}
		fe.edits = append(fe.edits, edit{off(id.Pos()), off(id.End()), m.newName + "." + name})
		fe.addImp[m.newPath] = ""
	case m.moved[path] && !m.moved[decl] && isTestFile(filepath.Base(decl)):
		// A stay-behind test helper cannot be imported: the moved file keeps its name and the new package gets its own copy.
		m.notes = append(m.notes, fmt.Sprintf("TWIN %s (used by %s)", id.Name, filepath.Base(path)))
	case m.moved[path] && !m.moved[decl]:
		name := id.Name
		if !id.IsExported() {
			up := capitalise(name)
			m.rename[name] = up
			m.notes = append(m.notes, fmt.Sprintf("capitalise %s -> %s (used by %s)", name, up, filepath.Base(path)))
			name = up
		}
		fe.edits = append(fe.edits, edit{off(id.Pos()), off(id.End()), m.origName + "." + name})
		fe.addImp[m.origPath] = ""
	case !m.moved[path] && m.moved[decl]:
		m.notes = append(m.notes, fmt.Sprintf("WARNING: %s (staying) uses moved symbol %s", filepath.Base(path), id.Name))
	}
}

func (m *mover) apply() error {
	for _, n := range m.notes {
		if strings.HasPrefix(n, "ERROR: ") {
			return fmt.Errorf("%s", strings.TrimPrefix(n, "ERROR: "))
		}
	}
	for from, up := range m.rename {
		if _, taken := m.declared[up]; taken && !m.spec.AllowCollisions {
			return fmt.Errorf("capitalising %s would collide with the existing declaration %s: use that symbol (or rename one) before moving", from, up)
		}
	}
	m.renameEverywhere()
	paths := make([]string, 0, len(m.files))
	for p, fe := range m.files {
		if len(fe.edits) > 0 || m.moved[p] {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		fe := m.files[p]
		out, err := m.rewrite(p, fe)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		dst := p
		if m.moved[p] {
			dst = filepath.Join(m.spec.Root, m.spec.ToDir, filepath.Base(p))
		}
		m.notes = append(m.notes, "rewrite "+strings.TrimPrefix(dst, m.spec.Root+"/"))
		if m.spec.DryRun {
			continue
		}
		if m.moved[p] {
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := gitMv(m.spec.Root, p, dst); err != nil {
				return err
			}
		}
		if err := os.WriteFile(dst, out, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func gitMv(root, from, to string) error {
	if _, err := git(root, "ls-files", "--error-unmatch", from); err != nil {
		return os.Rename(from, to) // an untracked staging file (a fresh relocate target) has nothing to `git mv`
	}
	//nolint:norawexec dev-time codemod, short-lived and waited on
	cmd := exec.Command("git", "mv", from, to)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git mv: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

// renameEverywhere capitalises the declaration and every use of each boundary symbol, in all loaded files.
func (m *mover) renameEverywhere() {
	for _, r := range m.memRefs {
		if up, hit := m.members[r.key]; hit {
			fe := m.files[r.file]
			fe.edits = append(fe.edits, edit{r.start, r.end, up})
		}
	}
	for _, u := range m.uses {
		up, hit := m.rename[u.name]
		if !hit {
			continue
		}
		fe := m.files[u.file]
		covered := false
		for _, e := range fe.edits {
			if e.start <= u.start && u.end <= e.end {
				covered = true
			}
		}
		if !covered {
			fe.edits = append(fe.edits, edit{u.start, u.end, up})
		}
	}
}

func (m *mover) rewrite(path string, fe *fileEdits) ([]byte, error) {
	sort.Slice(fe.edits, func(i, j int) bool { return fe.edits[i].start > fe.edits[j].start })
	src := append([]byte(nil), fe.src...)
	for _, e := range fe.edits {
		src = append(src[:e.start], append([]byte(e.text), src[e.end:]...)...)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if m.moved[path] {
		switch f.Name.Name {
		case m.origName:
			f.Name.Name = m.newName
		case m.origName + "_test":
			f.Name.Name = m.newName + "_test"
		}
	}
	for imp := range fe.addImp {
		if imp == m.origPath && filepath.Dir(path) == filepath.Join(m.spec.Root, m.spec.FromDir) && !m.moved[path] {
			continue
		}
		astutil.AddImport(fset, f, imp)
	}
	m.pruneUnused(fset, f, m.origPath, m.origName)
	m.pruneUnused(fset, f, m.newPath, m.newName)
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *mover) pruneUnused(fset *token.FileSet, f *ast.File, importPath, name string) {
	for _, spec := range append([]*ast.ImportSpec(nil), f.Imports...) {
		if spec == nil || spec.Path == nil {
			continue
		}
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil || p != importPath {
			continue
		}
		use := name
		if spec.Name != nil {
			use = spec.Name.Name
		}
		if use != "_" && use != "." && !usesQualifier(f, use) {
			astutil.DeleteImport(fset, f, importPath)
		}
	}
}

func usesQualifier(f *ast.File, name string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == name && id.Obj == nil {
				found = true
			}
		}
		return !found
	})
	return found
}
