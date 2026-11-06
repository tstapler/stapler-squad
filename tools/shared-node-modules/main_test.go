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

// Regression: target == webAppDir (the plain-install and pre-existing-node_modules-verify
// paths in Run) must omit --cwd. cmd.Dir already places bun in webAppDir; Bun resolves
// --cwd relative to its own process cwd, so passing both doubled the path
// (web-app/web-app) and failed with ENOENT ("failed to change directory to \"web-app\"").
func TestBunInstallCmdOmitsCwdWhenTargetIsWebAppDir(t *testing.T) {
	cmd := bunInstallCmd(context.Background(), "web-app", "web-app")

	require.Equal(t, "web-app", cmd.Dir)
	require.NotContains(t, cmd.Args, "--cwd")
}

func TestBunInstallCmdPassesCwdWhenTargetDiffers(t *testing.T) {
	cmd := bunInstallCmd(context.Background(), "web-app", "/store")

	require.Equal(t, "web-app", cmd.Dir)
	require.Contains(t, cmd.Args, "--cwd")
	require.Contains(t, cmd.Args, "/store")
}
