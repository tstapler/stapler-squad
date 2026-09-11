package session

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
	"time"
)

// fakePaneSubmitter is a scripted paneSubmitter: it records every SendKeys
// argument in call order (so a test can assert content and Enter travelled as
// two separate writes, not one concatenated string) and can be told to fail
// on a specific call index.
type fakePaneSubmitter struct {
	sendCalls  []string
	failOnCall int // -1 (default) means never fail
	updated    bool
}

func (f *fakePaneSubmitter) SendKeys(keys string) error {
	idx := len(f.sendCalls)
	f.sendCalls = append(f.sendCalls, keys)
	if f.failOnCall == idx {
		return errors.New("fake send failure")
	}
	return nil
}

func (f *fakePaneSubmitter) HasUpdated() (bool, bool) {
	updated := f.updated
	f.updated = false
	return updated, false
}

func newFakePaneSubmitter() *fakePaneSubmitter {
	return &fakePaneSubmitter{failOnCall: -1}
}

// TestSubmitDriverContent_SendsContentAndEnterAsSeparateWrites is the direct
// regression test for BUG-031 at the consolidated call site: content and the
// submit keystroke must never be concatenated into a single SendKeys call —
// this is the exact pattern that made Claude Code's TUI fold the Enter into a
// paste block instead of submitting it for long content.
func TestSubmitDriverContent_SendsContentAndEnterAsSeparateWrites(t *testing.T) {
	t.Parallel()
	inst := newFakePaneSubmitter()
	inst.updated = false // settle immediately

	const content = "some long driver-generated prompt text"
	if err := SubmitDriverContent(context.Background(), inst, content, time.Millisecond, 20*time.Millisecond); err != nil {
		t.Fatalf("SubmitDriverContent returned unexpected error: %v", err)
	}

	if len(inst.sendCalls) != 2 {
		t.Fatalf("SendKeys called %d times, want exactly 2 (content, then Enter) — got %#v", len(inst.sendCalls), inst.sendCalls)
	}
	if inst.sendCalls[0] != content {
		t.Errorf("first SendKeys call = %q, want exactly the content with no suffix", inst.sendCalls[0])
	}
	if inst.sendCalls[1] != EnterKeySequence {
		t.Errorf("second SendKeys call = %q, want exactly EnterKeySequence sent on its own", inst.sendCalls[1])
	}
}

// TestSubmitDriverContent_ContentSendFailure_NeverSendsEnter asserts a failed
// content write short-circuits before Enter is attempted.
func TestSubmitDriverContent_ContentSendFailure_NeverSendsEnter(t *testing.T) {
	t.Parallel()
	inst := newFakePaneSubmitter()
	inst.failOnCall = 0

	err := SubmitDriverContent(context.Background(), inst, "content", time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error from the failed content send")
	}
	if len(inst.sendCalls) != 1 {
		t.Errorf("SendKeys called %d times, want exactly 1 (Enter must not be sent after a content failure) — got %#v", len(inst.sendCalls), inst.sendCalls)
	}
}

// TestSubmitDriverContent_SubmitKeystrokeFailure_ReportsError asserts a failed
// Enter write (after a successful content write) still surfaces as an error.
func TestSubmitDriverContent_SubmitKeystrokeFailure_ReportsError(t *testing.T) {
	t.Parallel()
	inst := newFakePaneSubmitter()
	inst.failOnCall = 1

	err := SubmitDriverContent(context.Background(), inst, "content", time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error from the failed submit keystroke")
	}
	if len(inst.sendCalls) != 2 {
		t.Errorf("SendKeys called %d times, want exactly 2 (content succeeded, Enter attempted and failed) — got %#v", len(inst.sendCalls), inst.sendCalls)
	}
}

// TestSessionPackage_NoDirectSendKeysPlusEnterConcatenation is a structural
// regression guard for BUG-031: it fails if any session/*.go file (other than
// pane_submit.go, the one sanctioned place) calls inst.SendKeys(x +
// EnterKeySequence) — the single-write pattern that makes Claude Code's TUI
// paste-detector swallow the submit keystroke for long content. Before this
// test, that exact pattern was independently reintroduced at two call sites
// (session_driver.go's initial-prompt and backlog-nudge sends) after having
// already been fixed once in autonomous_driver.go — reverting either of the
// SubmitDriverContent call sites in this diff must make this test fail.
func TestSessionPackage_NoDirectSendKeysPlusEnterConcatenation(t *testing.T) {
	t.Parallel()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob session/*.go: %v", err)
	}

	fset := token.NewFileSet()
	for _, path := range matches {
		if filepath.Base(path) == "pane_submit.go" || filepath.Ext(path) != ".go" {
			continue
		}
		if isGoTestFileName(path) {
			// Test fakes/helpers are allowed to build arbitrary strings; only
			// production call sites are in scope for this guard.
			continue
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "SendKeys" {
				return true
			}
			bin, ok := call.Args[0].(*ast.BinaryExpr)
			if !ok || bin.Op != token.ADD {
				return true
			}
			if identNamed(bin.X, "EnterKeySequence") || identNamed(bin.Y, "EnterKeySequence") {
				pos := fset.Position(call.Pos())
				t.Errorf("%s: SendKeys(x + EnterKeySequence) single-write concatenation found — "+
					"this is the BUG-031 pattern; use SubmitDriverContent (pane_submit.go) instead", pos)
			}
			return true
		})
	}
}

func identNamed(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func isGoTestFileName(path string) bool {
	base := filepath.Base(path)
	return len(base) > len("_test.go") && base[len(base)-len("_test.go"):] == "_test.go"
}
