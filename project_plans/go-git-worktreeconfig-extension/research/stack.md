# Stack
- Consumer: stapler-squad `go.mod:232` `replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.3-ssq.1` (fork branch `ssq/v5`, tip 98880159, admin access verified).
- Bug site: `repository_extensions.go` `verifyExtensions` (fork, v5.19.3-ssq.1, lines 48-53 and 79-121).
- Upstream main (go-git/go-git, v6 line) already lowercased `extensionsValidForV0` keys ("Keys must be lowercase ..."); it is not backported to v5.19.3, and upstream main still needs a storer `ExtensionChecker` to accept `worktreeconfig`. The v5 fork has no such hook.
- Release mechanics: push fix to `ssq/v5`, tag `v5.19.3-ssq.2`, bump `replace` + `go mod tidy`, see docs/how-to/roll-back-go-git-fork.md.
