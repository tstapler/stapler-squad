package unfinished

import (
	"context"
	"os/exec"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// gitCmd returns a read-only git command rooted at dir.
// --no-optional-locks is always injected so the command never acquires
// index.lock, preventing contention with concurrent git operations in the
// same repo.
func gitCmd(ctx context.Context, dir string, args ...string) *exec.Cmd {
	all := make([]string, 0, 2+len(args))
	all = append(all, "--no-optional-locks", "-C", dir)
	all = append(all, args...)
	return safeexec.CommandContext(ctx, "git", all...)
}
