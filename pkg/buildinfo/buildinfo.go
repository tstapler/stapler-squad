// Package buildinfo holds the build's version/branch/commit/worktree
// provenance so it can be read from anywhere in the binary — the web UI's
// version display (server/server.go's /api/server-info) and the CLI's
// `version` command (main.go) — without server importing main (main
// already imports server) or duplicating the values.
//
// Version is set both here and as main.version via the same Makefile
// LDFLAGS invocation (see Makefile's LDFLAGS comment) so a local dev build
// never has the two diverge; main() also copies main.version into Version
// at startup as a fallback, since GoReleaser's release build only sets
// main.version (`-X main.version={{.Version}}`, kept separate deliberately
// — see Makefile's LDFLAGS comment) and never touches this package, so
// Version would otherwise be empty on a released binary.
//
// Branch, Commit, and Worktree are set ONLY by the Makefile — GoReleaser's
// release build never sets them. That's deliberate: they answer "what
// checkout/branch did THIS install come from", which only matters for a
// local dev build someone is about to run as their service, not a tagged
// release (see scripts/install-service.sh's provenance banner, the actual
// reason this package exists).
package buildinfo

//nolint:gochecknoglobals // ldflags' `-X pkg.Var=value` can only target package-level vars; there is no other mechanism to inject build-time values into a Go binary.
var (
	Version  string
	Branch   string
	Commit   string
	Worktree string
)
