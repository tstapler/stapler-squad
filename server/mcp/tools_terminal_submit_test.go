package mcp

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/tstapler/stapler-squad/session"
)

func resultJSON(res *mcpgo.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	tc, _ := res.Content[0].(mcpgo.TextContent)
	return tc.Text
}

// fakeSubmitTarget records each SendKeys argument so tests can assert content
// and Enter travel as separate writes (BUG-031).
type fakeSubmitTarget struct {
	mu         sync.Mutex
	sendCalls  []string
	failOnCall int // -1 means never fail
	block      chan struct{}
}

func newFakeSubmitTarget() *fakeSubmitTarget { return &fakeSubmitTarget{failOnCall: -1} }

func (f *fakeSubmitTarget) SendKeys(keys string) error {
	f.mu.Lock()
	idx := len(f.sendCalls)
	f.sendCalls = append(f.sendCalls, keys)
	f.mu.Unlock()
	if f.block != nil {
		<-f.block
	}
	if f.failOnCall == idx {
		return errors.New("fake send failure")
	}
	return nil
}

func (f *fakeSubmitTarget) HasUpdated() (bool, bool) { return false, false }

func (f *fakeSubmitTarget) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sendCalls...)
}

func newSubmitTestHandlers(t *testing.T, fake *fakeSubmitTarget) *terminalHandlers {
	t.Helper()
	inst := &session.Instance{Title: "s1", Status: session.Active, Program: "claude"}
	return &terminalHandlers{
		store:      &stubStore{instances: []*session.Instance{inst}},
		scrollback: makeScrollbackMgr(t),
		writeLim:   newTokenBucket(10, 10),
		targetFor:  func(*session.Instance) submitTarget { return fake },
	}
}

func assertTwoSeparateWrites(t *testing.T, fake *fakeSubmitTarget, content string) {
	t.Helper()
	got := fake.calls()
	if len(got) != 2 || got[0] != content || got[1] != session.EnterKeySequence {
		t.Fatalf("SendKeys calls = %q, want [%q, %q]", got, content, session.EnterKeySequence)
	}
}

func TestWriteToSession_PressEnter_SendsContentAndEnterSeparately(t *testing.T) {
	fake := newFakeSubmitTarget()
	th := newSubmitTestHandlers(t, fake)
	res, err := th.writeToSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1", "input": "hello"}))
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := parseResult(t, res)["success"].(bool); !ok {
		t.Fatalf("expected success, got %v", parseResult(t, res))
	}
	assertTwoSeparateWrites(t, fake, "hello")
}

func TestWriteToSession_NoPressEnter_SingleWriteWithoutEnter(t *testing.T) {
	fake := newFakeSubmitTarget()
	th := newSubmitTestHandlers(t, fake)
	_, err := th.writeToSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1", "input": "hello", "press_enter": false}))
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.calls(); len(got) != 1 || got[0] != "hello" {
		t.Fatalf("SendKeys calls = %q, want [\"hello\"]", got)
	}
}

func TestRunCommand_SendsCommandAndEnterSeparately(t *testing.T) {
	fake := newFakeSubmitTarget()
	th := newSubmitTestHandlers(t, fake)
	// timeout_seconds=1 bounds the output-stability poll that follows the send.
	_, err := th.runCommand(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1", "command": "echo hi", "timeout_seconds": float64(1)}))
	if err != nil {
		t.Fatal(err)
	}
	assertTwoSeparateWrites(t, fake, "echo hi")
}

func TestSteerSession_PTYFallback_SendsMessageAndEnterSeparately(t *testing.T) {
	fake := newFakeSubmitTarget()
	th := newSubmitTestHandlers(t, fake)
	res, err := th.steerSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1", "message": "focus"}))
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := parseResult(t, res)["success"].(bool); !ok {
		t.Fatalf("expected success, got %v", parseResult(t, res))
	}
	assertTwoSeparateWrites(t, fake, "focus")
}

func TestHandlers_FailedEnterWrite_SurfacesAsError(t *testing.T) {
	cases := map[string]func(*terminalHandlers) (string, error){
		"write_to_session": func(th *terminalHandlers) (string, error) {
			r, err := th.writeToSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1", "input": "x"}))
			return resultJSON(r), err
		},
		"run_command": func(th *terminalHandlers) (string, error) {
			r, err := th.runCommand(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1", "command": "x"}))
			return resultJSON(r), err
		},
		"steer_session": func(th *terminalHandlers) (string, error) {
			r, err := th.steerSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1", "message": "x"}))
			return resultJSON(r), err
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeSubmitTarget()
			fake.failOnCall = 1 // content succeeds, Enter fails
			out, err := call(newSubmitTestHandlers(t, fake))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, ErrInternalError) || strings.Contains(out, `"success":true`) {
				t.Errorf("expected INTERNAL_ERROR and no success, got %s", out)
			}
			if got := fake.calls(); len(got) != 2 {
				t.Errorf("SendKeys calls = %q, want 2 (content ok, Enter failed)", got)
			}
		})
	}
}

func TestWriteToSession_ContentWriteFailure_NeverSendsEnter(t *testing.T) {
	fake := newFakeSubmitTarget()
	fake.failOnCall = 0
	th := newSubmitTestHandlers(t, fake)
	if _, err := th.writeToSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1", "input": "x"})); err != nil {
		t.Fatal(err)
	}
	if got := fake.calls(); len(got) != 1 {
		t.Errorf("SendKeys calls = %q, want exactly 1", got)
	}
}

func TestWriteToSession_BlockedPTY_ReturnsTimeout(t *testing.T) {
	old := ptyWriteTimeout
	ptyWriteTimeout = 50 * time.Millisecond
	t.Cleanup(func() { ptyWriteTimeout = old })

	fake := newFakeSubmitTarget()
	fake.block = make(chan struct{})
	t.Cleanup(func() { close(fake.block) })
	th := newSubmitTestHandlers(t, fake)

	res, err := th.writeToSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1", "input": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if out := resultJSON(res); !strings.Contains(out, "PTY_WRITE_TIMEOUT") {
		t.Errorf("expected PTY_WRITE_TIMEOUT, got %s", out)
	}
}

// TestMCPPackage_NoSingleWriteSendKeysPlusEnter extends the BUG-031 structural
// guard (session's TestSessionPackage_NoDirectSendKeysPlusEnterConcatenation)
// to server/mcp: SendKeys must never receive content fused with Enter, either
// via `x + EnterKeySequence` or session.BuildSubmittableInput*(...).
func TestMCPPackage_NoSingleWriteSendKeysPlusEnter(t *testing.T) {
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
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
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "SendKeys" {
				return true
			}
			if mentionsSubmitFusion(call.Args[0]) {
				t.Errorf("%s: SendKeys with content fused to Enter (BUG-031); use session.SubmitDriverContent", fset.Position(call.Pos()))
			}
			return true
		})
	}
}

func mentionsSubmitFusion(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Ident:
			found = found || v.Name == "EnterKeySequence"
		case *ast.SelectorExpr:
			found = found || v.Sel.Name == "EnterKeySequence" ||
				v.Sel.Name == "BuildSubmittableInput" || v.Sel.Name == "BuildSubmittableInputAndSubmit"
		}
		return true
	})
	return found
}
