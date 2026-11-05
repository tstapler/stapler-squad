package server

import (
	"context"

	"github.com/tstapler/stapler-squad/github"
)

// prwatchSelfLogin adapts github.GetCurrentUserLogin to prwatch.SelfLogin.
// It returns "" with a nil error when unauthenticated; the resolver treats
// that as unresolved.
type prwatchSelfLogin struct {
	host string // "" means github.com
}

func (p prwatchSelfLogin) Login(ctx context.Context) (string, error) {
	return github.GetCurrentUserLogin(ctx, p.host)
}
