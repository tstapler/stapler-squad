package envtest

import (
	"fmt"
	"os"
)

// IsolateClaudeCLI points every `claude` process the test binary spawns
// (directly, or through tmux, which inherits this process's environment) at
// an empty config dir and a dead loopback API endpoint, and returns a func
// that restores the previous environment.
//
// Without it, session/restart tests that launch the real `claude` binary run
// it with the developer's ~/.claude credentials and MCP servers: real
// api.anthropic.com traffic, real `npx` MCP launches, and real token spend.
// Call it first thing in TestMain, before any tmux server or subprocess starts.
func IsolateClaudeCLI() (restore func(), err error) {
	dir, err := os.MkdirTemp("", "ssq-test-claude-config-")
	if err != nil {
		return nil, fmt.Errorf("envtest: create isolated claude config dir: %w", err)
	}
	set := map[string]string{
		"CLAUDE_CONFIG_DIR": dir,
		// Port 9 (discard) on loopback: connections are refused immediately.
		"ANTHROPIC_BASE_URL":                       "http://127.0.0.1:9",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	}
	prev := map[string]*string{}
	save := func(k string) {
		if v, ok := os.LookupEnv(k); ok {
			prev[k] = &v
		} else {
			prev[k] = nil
		}
	}
	for k, v := range set {
		save(k)
		_ = os.Setenv(k, v)
	}
	// Ambient variables that would hand a real credential to a spawned `claude`.
	for _, k := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"} {
		save(k)
		_ = os.Unsetenv(k)
	}
	return func() {
		for k, v := range prev {
			if v == nil {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, *v)
			}
		}
		_ = os.RemoveAll(dir)
	}, nil
}
