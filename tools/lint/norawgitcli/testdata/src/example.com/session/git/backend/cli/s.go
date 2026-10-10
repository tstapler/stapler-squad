// Package sanctioned holds git CLI calls the analyzer must allow.
package sanctioned

import (
	"context"
	"os/exec"
)

type runner interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

func ok(ctx context.Context, r runner) {
	_ = exec.Command("git", "status")
	_, _ = r.Run(ctx, "/repo", "git", "status")
}
