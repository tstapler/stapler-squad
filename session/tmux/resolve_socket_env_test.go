package tmux

import (
	"strings"
	"testing"
)

func TestResolveSocket_EnvPrecedence(t *testing.T) {
	t.Parallel()
	env := func(v string) func(string) string {
		return func(k string) string {
			if k != EnvSocketVar {
				t.Errorf("unexpected env lookup %q", k)
			}
			return v
		}
	}
	tests := []struct {
		name     string
		explicit string
		env      string
		testMode bool
		want     Socket
	}{
		{"unset keeps default", "", "", false, ""},
		{"unset keeps test socket", "", "", true, Socket(testSocketOnce())},
		{"env selects private socket", "", "ssq-manual-1", false, "ssq-manual-1"},
		{"explicit beats env", "wt-socket", "ssq-manual-1", false, "wt-socket"},
		{"env beats test socket", "", "ssq-manual-1", true, "ssq-manual-1"},
		{"invalid env ignored", "", "a/b", false, ""},
		{"invalid env falls to test socket", "", "bad name", true, Socket(testSocketOnce())},
		{"explicit passes through with invalid env", "wt-socket", "../x", false, "wt-socket"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveSocket(tc.explicit, env(tc.env), tc.testMode); got != tc.want {
				t.Fatalf("resolveSocket = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveSocket_EnvYieldsDashL(t *testing.T) {
	t.Parallel()
	got := resolveSocket("", func(string) string { return "ssq-manual-1" }, false).Args("ls")
	if strings.Join(got, " ") != "-L ssq-manual-1 ls" {
		t.Fatalf("args = %v", got)
	}
}

func TestValidateEnvSocket(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		ok    bool
	}{
		{"simple", "ssq-manual-1", true},
		{"dots underscores", "a.b_c-D9", true},
		{"max length", strings.Repeat("a", maxEnvSocketLen), true},
		{"empty", "", false},
		{"too long", strings.Repeat("a", maxEnvSocketLen+1), false},
		{"slash", "a/b", false},
		{"absolute path", "/tmp/sock", false},
		{"dot", ".", false},
		{"dotdot", "..", false},
		{"space", "a b", false},
		{"shell meta", "a;rm", false},
		{"dollar", "$HOME", false},
		{"newline", "a\nb", false},
		{"leading dash", "-x", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateEnvSocket(tc.input); (err == nil) != tc.ok {
				t.Fatalf("ValidateEnvSocket(%q) err=%v, want ok=%v", tc.input, err, tc.ok)
			}
		})
	}
}
