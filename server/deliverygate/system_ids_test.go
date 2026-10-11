package deliverygate

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// reviewedDynamicIDForms are the non-literal first arguments of
// NewNotificationEvent that Spike 1.3d classified as a session identity form
// (UUID, title, tmux name, raw hook id) or a backlog item id (which the
// resolver treats as NotASession through the item_id stamp). A new form fails
// the scan until it is reviewed and added here.
var reviewedDynamicIDForms = map[string]string{
	"callerUUID":           "session UUID (MCP backlog tool)",
	"inst.UUID":            "session UUID",
	"instance.UUID":        "session UUID",
	"snap.UUID":            "session UUID (snapshot)",
	"sessionUUID":          "session UUID",
	"uuid":                 "session UUID (crash producer, NewSessionCrashEvent)",
	"sessionID":            "session UUID, title, tmux name or raw hook id (Spike 1.3d identity forms); bulk-reset:<scope> ids are in the system prefix set",
	"resolvedID":           "review-queue key resolved through FindInstance, else the raw title key",
	"resolvedSessionID":    "SendNotification's resolved UUID, else the raw hook id",
	"is.BacklogItemID":     "backlog item id (item_id stamp: NotASession)",
	"item.ID":              "backlog item id (item_id stamp: NotASession)",
	"itemID":               "backlog item id (item_id stamp: NotASession)",
	"quotaGateNotifierKey": "constant \"backlog-quota-gate\" (in the system set)",
}

// T-IX-15: every producer's first ID argument is a known form.
func TestProducers_ShouldUseKnownIDForm_WhenScanningEveryNewNotificationEventCall(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	found := map[string][]string{} // expression text -> locations
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case "gen", "node_modules", "web-app", ".git", "third_party", "testdata", ".claude":
				// .claude/worktrees holds full nested git worktrees of this same
				// module (one per in-progress agent task) — real directories on
				// disk, but not part of this module's package graph. Walking into
				// them re-scans their own, possibly-stale copy of this file against
				// reviewedDynamicIDForms, which only covers the current source.
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(src, []byte("NewNotificationEvent(")) {
			return nil
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 || calleeName(call.Fun) != "NewNotificationEvent" {
				return true
			}
			if strings.HasSuffix(filepath.ToSlash(path), "pkg/events/types.go") {
				return true // the definition's own helpers
			}
			var buf bytes.Buffer
			_ = printer.Fprint(&buf, fset, call.Args[0])
			expr := buf.String()
			loc := path + ":" + strconv.Itoa(fset.Position(call.Pos()).Line)
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				s, _ := strconv.Unquote(lit.Value)
				if s != "" && !IsSystemID(s) {
					t.Errorf("%s: literal id %q is not in the closed system-ID set", loc, s)
				}
				return true
			}
			found[expr] = append(found[expr], loc)
			return true
		})
		return nil
	})
	var unknown []string
	for expr, locs := range found {
		if _, ok := reviewedDynamicIDForms[expr]; !ok {
			unknown = append(unknown, expr+"  <-  "+strings.Join(locs, ", "))
		}
	}
	sort.Strings(unknown)
	for _, u := range unknown {
		t.Errorf("unreviewed dynamic id expression: %s", u)
	}
}

func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		return f.Sel.Name
	case *ast.Ident:
		return f.Name
	}
	return ""
}
