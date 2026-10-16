package unfinished

// Guards "assertions unchanged" while test fixtures move to internal/testrepo.
// For every Test*/Benchmark* function in the migrated files, the assertion
// calls (assert.*, require.*, t.Errorf/Fatalf/Error/Fatal...) are serialised
// position-independently, hashed, and compared with
// testdata/assertion_ast_hashes.golden.
//
// A changed hash must be justified in the story report, then the golden line
// edited by hand. WRITE_ASSERTION_AST_GOLDEN=1 rewrites the whole file and is
// for seeding only.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const assertionGoldenPath = "testdata/assertion_ast_hashes.golden"

// assertionHashedFiles lists, relative to session/unfinished, every test file
// whose fixtures are migrated off native git (the story inventory plus the
// same-package caller objectcache_test.go and the relocated index benchmark).
var assertionHashedFiles = []string{
	"gogit_vcs_reader_gitignore_test.go",
	"gogit_vcs_reader_limits_test.go",
	"gogit_vcs_reader_objectcache_test.go",
	"state_test.go",
	"vcsreader_bench_test.go",
	"vcsreader_index_bench_test.go",
	"vcsreader_test.go",
	"gogitstore/crash_subprocess_test.go",
	"gogitstore/gogitstore_test.go",
	"gogitstore/mmap_adversarial_test.go",
	"gogitstore/mmap_realrepo_test.go",
	"gogitstore/mmap_stage2_test.go",
	"gogitstore/mmap_truncation_test.go",
	"gogitstore/mmapindex_test.go",
	"gogitstore/registry_eviction_test.go",
	"gogitstore/soak_test.go",
}

// removedAssertionTests allow-lists golden keys that may be absent from the
// migrated tree (renamed or deleted tests), each with a reason.
var removedAssertionTests = []struct{ key, reason string }{
	{"gogitstore|gogitstore|TestGitRunErr_WedgedSubprocess_FailsFastInsteadOfHangingForever", "Task 0.0.3e: tested the native-git subprocess wrapper gitRunErr (fake git binary on PATH); the wrapper is deleted with native git"},
	{"gogitstore|gogitstore|TestGitRunErrCtx_PreemptedByShorterParentDeadline", "Task 0.0.3e: tested gitRunErrCtx's parent-deadline preemption of a wedged git subprocess; the wrapper is deleted with native git"},
}

var assertionMethodNames = map[string]bool{
	"Errorf": true, "Fatalf": true, "Error": true, "Fatal": true,
}

func isAssertionCall(c *ast.CallExpr) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if id, ok := sel.X.(*ast.Ident); ok && (id.Name == "assert" || id.Name == "require") {
		return true
	}
	return assertionMethodNames[sel.Sel.Name]
}

var (
	posType     = reflect.TypeOf(token.NoPos)
	objectType  = reflect.TypeOf((*ast.Object)(nil))
	scopeType   = reflect.TypeOf((*ast.Scope)(nil))
	commentType = reflect.TypeOf((*ast.CommentGroup)(nil))
)

// canon writes a position-, comment- and resolution-independent encoding of v.
func canon(sb *strings.Builder, v reflect.Value) {
	switch v.Kind() {
	case reflect.Interface, reflect.Ptr:
		if v.IsNil() {
			sb.WriteString("nil;")
			return
		}
		if v.Type() == objectType || v.Type() == scopeType || v.Type() == commentType {
			return
		}
		canon(sb, v.Elem())
	case reflect.Struct:
		sb.WriteString(v.Type().Name())
		sb.WriteByte('{')
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).Type == posType {
				continue
			}
			canon(sb, v.Field(i))
		}
		sb.WriteByte('}')
	case reflect.Slice:
		fmt.Fprintf(sb, "[%d:", v.Len())
		for i := 0; i < v.Len(); i++ {
			canon(sb, v.Index(i))
		}
		sb.WriteByte(']')
	case reflect.String:
		fmt.Fprintf(sb, "%q;", v.String())
	case reflect.Bool:
		fmt.Fprintf(sb, "%t;", v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fmt.Fprintf(sb, "%d;", v.Int())
	}
}

func assertionHash(fn *ast.FuncDecl) string {
	var sb strings.Builder
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && isAssertionCall(c) {
			canon(&sb, reflect.ValueOf(c))
			sb.WriteByte('\n')
		}
		return true
	})
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// computeAssertionHashes returns "dir|package|Func" -> hash for all files.
func computeAssertionHashes(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, rel := range assertionHashedFiles {
		f, err := parser.ParseFile(token.NewFileSet(), filepath.FromSlash(rel), nil, 0)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("parse %s: %v", rel, err)
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil {
				continue
			}
			if !strings.HasPrefix(fn.Name.Name, "Test") && !strings.HasPrefix(fn.Name.Name, "Benchmark") {
				continue
			}
			out[dir+"|"+f.Name.Name+"|"+fn.Name.Name] = assertionHash(fn)
		}
	}
	return out
}

func readAssertionGolden(t *testing.T) map[string]string {
	t.Helper()
	fh, err := os.Open(filepath.FromSlash(assertionGoldenPath))
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer fh.Close()
	golden := map[string]string{}
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, hash, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatalf("malformed golden line %q", line)
		}
		golden[key] = hash
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read golden: %v", err)
	}
	return golden
}

func writeAssertionGolden(t *testing.T, hashes map[string]string) {
	t.Helper()
	keys := make([]string, 0, len(hashes))
	for k := range hashes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString("# key = dir|package|TestFunc; value = sha256 of its assertion-call ASTs.\n")
	sb.WriteString("# Edit a line by hand only with a justification in the story report.\n")
	for _, k := range keys {
		sb.WriteString(k + "\t" + hashes[k] + "\n")
	}
	if err := os.WriteFile(filepath.FromSlash(assertionGoldenPath), []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
}

func TestMigratedTests_ShouldKeepAssertionASTHashes_WhenFixturesMovedToTestrepo(t *testing.T) {
	current := computeAssertionHashes(t)
	if os.Getenv("WRITE_ASSERTION_AST_GOLDEN") == "1" {
		writeAssertionGolden(t, current)
	}
	golden := readAssertionGolden(t)

	allowed := map[string]bool{}
	for _, r := range removedAssertionTests {
		if r.reason == "" {
			t.Fatalf("allow-list entry %q has no reason", r.key)
		}
		allowed[r.key] = true
	}

	for key, want := range golden {
		got, ok := current[key]
		switch {
		case !ok && allowed[key]:
		case !ok:
			t.Errorf("%s is in the golden but no longer exists; allow-list it in removedAssertionTests with a reason", key)
		case got != want:
			t.Errorf("assertion AST changed for %s: golden %s, now %s", key, want, got)
		}
	}
}
