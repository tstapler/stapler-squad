// Package backend is a fake of session/git/backend: Runner is the port.
package backend

import "context"

type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

type StdoutRunner interface {
	RunStdout(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}
