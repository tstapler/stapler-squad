package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

// The recorder must never touch the developer's live service on :8543 (Task 2.3.1g).
func TestRecordScriptIsHermetic(t *testing.T) {
	t.Parallel()
	script := readRepoFile(t, "scripts/record-website-demos.sh")
	config := readRepoFile(t, "tests/e2e/playwright.website-demos.config.ts")

	for name, text := range map[string]string{"record script": script, "playwright config": config} {
		assert.NotContains(t, text, "8543", name+" must not name the live port")
	}
	assert.Regexp(t, regexp.MustCompile(`bind\(\("127\.0\.0\.1",\s*0\)\)`), script, "PORT must be a kernel-assigned free port")
	assert.Regexp(t, regexp.MustCompile(`TMUX_SOCKET="ssq-[a-z-]+`), script, "tmux socket must be an ssq-* name")
	assert.Contains(t, script, `STAPLER_SQUAD_TMUX_SOCKET="$TMUX_SOCKET"`)
	assert.Contains(t, script, "--test-mode --test-dir")
}

// BUG-116: a second instance's startup cleanup can delete a live instance's cdp-bins dirs,
// so the server runs with a per-run HOME.
func TestRecordScriptIsolatesServerHome(t *testing.T) {
	t.Parallel()
	script := readRepoFile(t, "scripts/record-website-demos.sh")
	assert.Contains(t, script, `SERVER_HOME="$(mktemp -d)"`)
	assert.Contains(t, script, "docs/bugs/open/BUG-116")

	var serverLine string
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, `HOME="$SERVER_HOME"`) {
			serverLine = line
		}
	}
	require.NotEmpty(t, serverLine, "server must start with HOME=$SERVER_HOME")
	assert.Contains(t, serverLine, "PORT=", "HOME is set on the server launch, not globally")
}

func TestRecordScriptCleansUpServerHome(t *testing.T) {
	t.Parallel()
	assert.Contains(t, readRepoFile(t, "scripts/record-website-demos.sh"), `rm -rf "$SERVER_HOME"`)
}
