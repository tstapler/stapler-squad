package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBunInstallCmdClearsInheritedUserAgent(t *testing.T) {
	t.Setenv("npm_config_user_agent", "npm/10.0.0 node/v22 linux x64")
	t.Setenv("KEEP_ME", "1")

	cmd := bunInstallCmd(context.Background(), "web-app", "/store")

	require.NotContains(t, cmd.Env, "npm_config_user_agent=npm/10.0.0 node/v22 linux x64")
	for _, kv := range cmd.Env {
		require.NotContains(t, kv, "npm_config_user_agent=")
	}
	require.Contains(t, cmd.Env, "KEEP_ME=1")
}
