// Package tmux is a fake of session/tmux for analyzer fixtures.
package tmux

import "context"

type CommandRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}
