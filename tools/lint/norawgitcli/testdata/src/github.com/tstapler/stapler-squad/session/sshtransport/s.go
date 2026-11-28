// Package sshtransport is a fake of session/sshtransport for analyzer fixtures.
package sshtransport

import "context"

type SSHRunner struct{}

func (*SSHRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	return nil, nil
}
