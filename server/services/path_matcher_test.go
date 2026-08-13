package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// ──────────────────────────────────────────────────────────────────────────────
// ExpandPath
// ──────────────────────────────────────────────────────────────────────────────

func TestExpandPath_Tilde(t *testing.T) {
	home, _ := os.UserHomeDir()

	got := ExpandPath("~")
	if got != home {
		t.Errorf("~ → %q; want %q", got, home)
	}
}

func TestExpandPath_TildeSlash(t *testing.T) {
	home, _ := os.UserHomeDir()

	got := ExpandPath("~/projects/foo")
	want := filepath.Join(home, "projects/foo")
	if got != want {
		t.Errorf("~/projects/foo → %q; want %q", got, want)
	}
}

func TestExpandPath_DollarHOME(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("HOME", home)

	got := ExpandPath("$HOME")
	if got != home {
		t.Errorf("$HOME → %q; want %q", got, home)
	}
}

func TestExpandPath_DollarHOMESubdir(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("HOME", home)

	got := ExpandPath("$HOME/subdir")
	want := filepath.Clean(home + "/subdir")
	if got != want {
		t.Errorf("$HOME/subdir → %q; want %q", got, want)
	}
}

func TestExpandPath_DotDot(t *testing.T) {
	got := ExpandPath("/foo/../bar")
	if got != "/bar" {
		t.Errorf("/foo/../bar → %q; want /bar", got)
	}
}

func TestExpandPath_DoubleSlash(t *testing.T) {
	got := ExpandPath("/tmp//test")
	if got != "/tmp/test" {
		t.Errorf("/tmp//test → %q; want /tmp/test", got)
	}
}

func TestExpandPath_NonPath(t *testing.T) {
	// A plain flag should pass through unchanged.
	got := ExpandPath("-rf")
	if got != "-rf" {
		t.Errorf("-rf → %q; want -rf", got)
	}
}

func TestExpandPath_UnknownVar(t *testing.T) {
	// An unset variable expands to an empty string via os.ExpandEnv.
	t.Setenv("__CLAUDE_SQUAD_UNSET__", "")
	got := ExpandPath("$__CLAUDE_SQUAD_UNSET__/foo")
	// os.ExpandEnv("") → "" so result is "/foo" cleaned → "/foo"
	if got != "/foo" {
		t.Errorf("$UNSET/foo → %q; want /foo", got)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// ClassifyPath
// ──────────────────────────────────────────────────────────────────────────────

func TestClassifyPath_Root(t *testing.T) {
	c := ClassifyPath("/", classifier.ClassificationContext{})
	if c&PathRoot == 0 {
		t.Error("/ should be PathRoot")
	}
	if c&PathHome != 0 {
		t.Error("/ should not be PathHome")
	}
}

func TestClassifyPath_Home(t *testing.T) {
	home, _ := os.UserHomeDir()
	c := ClassifyPath(home, classifier.ClassificationContext{})
	if c&PathHome == 0 {
		t.Errorf("%q should be PathHome", home)
	}
	if c&PathRoot != 0 {
		t.Errorf("%q should not be PathRoot", home)
	}
}

func TestClassifyPath_HomeSuffix(t *testing.T) {
	home, _ := os.UserHomeDir()
	// A subdirectory of home is NOT home itself.
	c := ClassifyPath(filepath.Join(home, "projects"), classifier.ClassificationContext{})
	if c&PathHome != 0 {
		t.Error("home/projects should not be PathHome")
	}
}

func TestClassifyPath_SystemDirs(t *testing.T) {
	cases := []string{
		"/etc", "/etc/passwd",
		"/usr", "/usr/bin/bash",
		"/bin", "/bin/sh",
		"/sbin", "/lib", "/lib64",
		"/boot", "/dev", "/sys", "/proc", "/run",
	}
	for _, p := range cases {
		c := ClassifyPath(p, classifier.ClassificationContext{})
		if c&PathSystemDir == 0 {
			t.Errorf("%q should be PathSystemDir", p)
		}
	}
}

func TestClassifyPath_NotSystemDir(t *testing.T) {
	cases := []string{"/tmp", "/home/user/etc", "/var/log"}
	for _, p := range cases {
		c := ClassifyPath(p, classifier.ClassificationContext{})
		if c&PathSystemDir != 0 {
			t.Errorf("%q should not be PathSystemDir", p)
		}
	}
}

func TestClassifyPath_TempDir(t *testing.T) {
	cases := []string{"/tmp", "/tmp/ai-setup-test", "/var/tmp", "/var/tmp/work"}
	for _, p := range cases {
		c := ClassifyPath(p, classifier.ClassificationContext{})
		if c&PathTempDir == 0 {
			t.Errorf("%q should be PathTempDir", p)
		}
	}
}

func TestClassifyPath_TempDir_TMPDIR(t *testing.T) {
	t.Setenv("TMPDIR", "/private/tmp")
	c := ClassifyPath("/private/tmp/mytest", classifier.ClassificationContext{})
	if c&PathTempDir == 0 {
		t.Error("/private/tmp/mytest should be PathTempDir when TMPDIR=/private/tmp")
	}
}

func TestClassifyPath_Cwd(t *testing.T) {
	ctx := classifier.ClassificationContext{Cwd: "/home/user/project"}
	c := ClassifyPath("/home/user/project/src/main.go", ctx)
	if c&PathCwd == 0 {
		t.Error("path inside Cwd should be PathCwd")
	}
}

func TestClassifyPath_CwdMiss(t *testing.T) {
	ctx := classifier.ClassificationContext{Cwd: "/home/user/project"}
	c := ClassifyPath("/home/user/other", ctx)
	if c&PathCwd != 0 {
		t.Error("path outside Cwd should not be PathCwd")
	}
}

func TestClassifyPath_GitRepo(t *testing.T) {
	ctx := classifier.ClassificationContext{RepoRoot: "/home/user/project"}
	c := ClassifyPath("/home/user/project/server/main.go", ctx)
	if c&PathGitRepo == 0 {
		t.Error("path inside RepoRoot should be PathGitRepo")
	}
}

func TestClassifyPath_MultipleBits(t *testing.T) {
	// A path can match multiple classifications at once.
	ctx := classifier.ClassificationContext{Cwd: "/tmp/myproject", RepoRoot: "/tmp/myproject"}
	c := ClassifyPath("/tmp/myproject/main.go", ctx)
	if c&PathTempDir == 0 {
		t.Error("should be PathTempDir")
	}
	if c&PathCwd == 0 {
		t.Error("should be PathCwd")
	}
	if c&PathGitRepo == 0 {
		t.Error("should be PathGitRepo")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// PathMatcher.Matches
// ──────────────────────────────────────────────────────────────────────────────

func TestPathMatcher_MatchesRoot(t *testing.T) {
	pm := &PathMatcher{ArgIndex: -1, MatchIf: PathRoot}
	if !pm.Matches([]string{"-rf", "/"}, classifier.ClassificationContext{}) {
		t.Error("should match / as root")
	}
}

func TestPathMatcher_NoMatchSubdir(t *testing.T) {
	pm := &PathMatcher{ArgIndex: -1, MatchIf: PathRoot | PathHome}
	if pm.Matches([]string{"-rf", "/tmp/ai-setup-test"}, classifier.ClassificationContext{}) {
		t.Error("/tmp/ai-setup-test should not match PathRoot|PathHome")
	}
}

func TestPathMatcher_MatchesHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	pm := &PathMatcher{ArgIndex: -1, MatchIf: PathRoot | PathHome}
	if !pm.Matches([]string{"-rf", home}, classifier.ClassificationContext{}) {
		t.Errorf("home dir %q should match PathHome", home)
	}
}

func TestPathMatcher_ExpandedHomePath(t *testing.T) {
	home, _ := os.UserHomeDir()
	// ExpandedArgs already has expansion applied (done in ExtractAllCommands).
	expanded := []string{"-rf", home}
	pm := &PathMatcher{ArgIndex: -1, MatchIf: PathHome}
	if !pm.Matches(expanded, classifier.ClassificationContext{}) {
		t.Error("expanded home path should match PathHome")
	}
}

func TestPathMatcher_RejectIf(t *testing.T) {
	// RejectIf: if the path is in /tmp, the rule should NOT match.
	pm := &PathMatcher{ArgIndex: -1, RejectIf: PathTempDir, MatchIf: PathRoot | PathHome | PathTempDir}
	// /tmp/test is temp dir — RejectIf should veto.
	if pm.Matches([]string{"-rf", "/tmp/test"}, classifier.ClassificationContext{}) {
		t.Error("RejectIf PathTempDir should veto /tmp/test")
	}
}

func TestPathMatcher_ArgIndex(t *testing.T) {
	home, _ := os.UserHomeDir()
	// ArgIndex: 0 = first non-flag arg (which would be home).
	// Non-flag args of ["-rf", home, "/tmp/something"] → [home, "/tmp/something"]
	pm := &PathMatcher{ArgIndex: 0, MatchIf: PathHome}
	if !pm.Matches([]string{"-rf", home, "/tmp/something"}, classifier.ClassificationContext{}) {
		t.Error("ArgIndex=0 should select the first non-flag arg (home)")
	}
}

func TestPathMatcher_ArgIndexOutOfRange(t *testing.T) {
	pm := &PathMatcher{ArgIndex: 5, MatchIf: PathRoot}
	if pm.Matches([]string{"-rf", "/"}, classifier.ClassificationContext{}) {
		t.Error("ArgIndex out of range should return false")
	}
}

func TestPathMatcher_NoCandidates(t *testing.T) {
	// All args are flags — no path candidates.
	pm := &PathMatcher{ArgIndex: -1, MatchIf: PathRoot}
	if pm.Matches([]string{"-r", "-f"}, classifier.ClassificationContext{}) {
		t.Error("no non-flag args should return false")
	}
}

func TestPathMatcher_ZeroMatchIf(t *testing.T) {
	// MatchIf == 0 means the check is skipped (always passes if no RejectIf triggered).
	pm := &PathMatcher{ArgIndex: -1} // both MatchIf and RejectIf are zero
	// Any non-flag arg present → passes.
	if !pm.Matches([]string{"-rf", "/anything"}, classifier.ClassificationContext{}) {
		t.Error("zero MatchIf should always pass when a non-flag arg is present")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Integration: PathMatcher in the classifier (rm rule)
// ──────────────────────────────────────────────────────────────────────────────

func TestClassifier_RmRf_ExpandedHome_Denied(t *testing.T) {
	home, _ := os.UserHomeDir()
	c := classifier.NewRuleBasedClassifier()
	ctx := classifier.ClassificationContext{}

	// Provide the literal home path (as if $HOME was already expanded by shell).
	// The AST-based audit in AuditCommand detects rm -rf on the actual home dir.
	cmd := "rm -rf " + home
	r := c.Classify(classifier.PermissionRequestPayload{
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{"command": cmd},
	}, ctx)
	if r.Decision != classifier.AutoDeny {
		t.Errorf("rm -rf <home> should be AutoDeny, got %v (rule=%s)", r.Decision, r.RuleID)
	}
	if r.RuleID != "audit-rm-rf-critical-path" {
		t.Errorf("expected rule audit-rm-rf-critical-path, got %s", r.RuleID)
	}
}

func TestClassifier_RmRf_HomeSubdir_NotDenied(t *testing.T) {
	home, _ := os.UserHomeDir()
	c := classifier.NewRuleBasedClassifier()
	ctx := classifier.ClassificationContext{}

	cmd := "rm -rf " + filepath.Join(home, "projects/old-branch")
	r := c.Classify(classifier.PermissionRequestPayload{
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{"command": cmd},
	}, ctx)
	if r.Decision == classifier.AutoDeny && r.RuleID == "seed-deny-rm-rf-root" {
		t.Errorf("rm -rf <home subdir> should not be blocked by rm-rf-root rule, got classifier.AutoDeny")
	}
}

func TestClassifier_RmRf_TmpDir_NotDenied(t *testing.T) {
	c := classifier.NewRuleBasedClassifier()
	ctx := classifier.ClassificationContext{}

	cases := []string{
		"rm -rf /tmp/ai-setup-test",
		"rm -rf /tmp/somedir",
		"rm -rf /var/tmp/workdir",
	}
	for _, cmd := range cases {
		r := c.Classify(classifier.PermissionRequestPayload{
			ToolName:  "Bash",
			ToolInput: map[string]interface{}{"command": cmd},
		}, ctx)
		if r.Decision == classifier.AutoDeny && r.RuleID == "seed-deny-rm-rf-root" {
			t.Errorf("cmd %q should not be blocked by rm-rf-root rule", cmd)
		}
	}
}

// ExpandedArgs integration: verify that $HOME in a command is expanded.
func TestClassifier_RmRf_DollarHOME_Denied(t *testing.T) {
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(home, "/") {
		t.Skip("home dir is not absolute, skipping")
	}
	c := classifier.NewRuleBasedClassifier()
	ctx := classifier.ClassificationContext{}

	r := c.Classify(classifier.PermissionRequestPayload{
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{"command": "rm -rf $HOME"},
	}, ctx)
	if r.Decision != classifier.AutoDeny {
		t.Errorf("rm -rf $HOME should be classifier.AutoDeny (requires $HOME expansion), got %v (rule=%s)", r.Decision, r.RuleID)
	}
}

func TestClassifier_RmRf_DollarHOMESubdir_NotDenied(t *testing.T) {
	c := classifier.NewRuleBasedClassifier()
	ctx := classifier.ClassificationContext{}

	r := c.Classify(classifier.PermissionRequestPayload{
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{"command": "rm -rf $HOME/subdir"},
	}, ctx)
	if r.Decision == classifier.AutoDeny && r.RuleID == "seed-deny-rm-rf-root" {
		t.Errorf("rm -rf $HOME/subdir should not be blocked by rm-rf-root rule (requires $HOME expansion)")
	}
}

func TestClassifier_RmRf_RelativeDot_FromRoot_Denied(t *testing.T) {
	c := classifier.NewRuleBasedClassifier()
	ctx := classifier.ClassificationContext{Cwd: "/"}

	r := c.Classify(classifier.PermissionRequestPayload{
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{"command": "rm -rf ."},
	}, ctx)
	if r.Decision != classifier.AutoDeny {
		t.Errorf("rm -rf . from / should be AutoDeny, got %v (rule=%s)", r.Decision, r.RuleID)
	}
}

func TestClassifier_RmRf_RelativeDotDot_FromHomeSubdir_Denied(t *testing.T) {
	home, _ := os.UserHomeDir()
	c := classifier.NewRuleBasedClassifier()
	ctx := classifier.ClassificationContext{Cwd: filepath.Join(home, "projects")}

	r := c.Classify(classifier.PermissionRequestPayload{
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{"command": "rm -rf .."},
	}, ctx)
	if r.Decision != classifier.AutoDeny {
		t.Errorf("rm -rf .. from %s/projects should be AutoDeny (resolves to home), got %v (rule=%s)", home, r.Decision, r.RuleID)
	}
}

func TestClassifier_RmRf_RelativeDot_FromSafeDir_NotDenied(t *testing.T) {
	c := classifier.NewRuleBasedClassifier()
	ctx := classifier.ClassificationContext{Cwd: "/tmp/myproject"}

	r := c.Classify(classifier.PermissionRequestPayload{
		ToolName:  "Bash",
		ToolInput: map[string]interface{}{"command": "rm -rf ."},
	}, ctx)
	if r.Decision == classifier.AutoDeny && r.RuleID == "audit-rm-rf-critical-path" {
		t.Errorf("rm -rf . from /tmp/myproject should not be blocked by critical-path audit")
	}
}
