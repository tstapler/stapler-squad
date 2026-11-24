package fsops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/fsops/fsopstest"
)

// goldenPaths covers traversal, absolute, NUL, .git and every symlink class of the resolver fixture.
var goldenPaths = []string{
	"", ".", "./", "src/a.go", "./src/a.go", "src//a.go", "src/", "src/../src/a.go", "src/new.go",
	"..", "../", "../outside", "src/../..", "src/../../outside", "/etc/passwd", "/", "a\x00b",
	".git", ".git/config", ".git/hooks", "x/.git/y", "src/.git", ".GIT", "src/.git/config",
	"AGENTS.md", "docs/alias.md", "shared", "shared/x.txt", "pkg/shared/../shared/x.txt", "chain1", "chain1/",
	"leak", "leak_rel", "dir_leak", "dir_leak/secret", "dangling", "dangling_inside", "gitlink", "gitlink/config",
	"linkdir", "linkdir/x", "loop1", "loop2/x", "missing", "missing/deeper", "CLAUDE.md/extra",
}

// goldenDump renders what the resolver and the confined root decide for goldenPaths in both modes.
// testdata/resolver_golden.txt was recorded from the pre-move server/services implementation.
func goldenDump(t *testing.T) string {
	t.Helper()
	dir, outside := fsopstest.NewResolverFixture(t)
	root, err := OpenWorktreeRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	scrub := strings.NewReplacer(dir, "<DIR>", outside, "<OUT>")

	var b strings.Builder
	for _, mode := range []AccessMode{AccessRead, AccessWrite} {
		for _, p := range goldenPaths {
			resolved, viaLink, err := ResolveInWorktreeLinks(root, p, mode)
			fmt.Fprintf(&b, "mode=%d path=%q => ", mode, p)
			if err != nil {
				fmt.Fprintf(&b, "ERR %s\n", describeGoldenErr(err, scrub))
				continue
			}
			fmt.Fprintf(&b, "resolved=%q via=%v", string(resolved), viaLink)
			_, serr := root.Stat(resolved)
			fmt.Fprintf(&b, " stat=%s", describeGoldenErr(serr, scrub))
			if mode == AccessWrite {
				werr := atomicReplace(root, resolved, []byte("golden\n"))
				fmt.Fprintf(&b, " replace=%s", describeGoldenErr(werr, scrub))
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func describeGoldenErr(err error, scrub *strings.Replacer) string {
	if err == nil {
		return "ok"
	}
	var ce *connect.Error
	if errors.As(err, &ce) {
		return fmt.Sprintf("connect:%s %s details=%d", ce.Code(), scrub.Replace(ce.Message()), len(ce.Details()))
	}
	if errors.Is(err, os.ErrNotExist) {
		return "notexist"
	}
	return scrub.Replace(err.Error())
}

func TestResolver_ShouldMatchGoldenRecordedFromPreMoveImplementation(t *testing.T) {
	t.Parallel()
	got := goldenDump(t)
	want, err := os.ReadFile(filepath.Join("testdata", "resolver_golden.txt"))
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
