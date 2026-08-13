package services

import (
	"context"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/internal/claudehooks"
)

func TestGetHookStatus_ReflectsGlobalSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc := newCreateTestService(t, createTestStorage(t))

	// Nothing installed yet.
	resp, err := svc.GetHookStatus(context.Background(), connect.NewRequest(&sessionv1.GetHookStatusRequest{}))
	if err != nil {
		t.Fatalf("GetHookStatus: %v", err)
	}
	if resp.Msg.RulesInstalled {
		t.Error("rules should not be installed on a fresh HOME")
	}

	// Install the rules hook directly into the temp HOME's settings, then re-check.
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := claudehooks.InstallRules(settings, "/x/ssq-hooks"); err != nil {
		t.Fatalf("seed InstallRules: %v", err)
	}
	resp, err = svc.GetHookStatus(context.Background(), connect.NewRequest(&sessionv1.GetHookStatusRequest{}))
	if err != nil {
		t.Fatalf("GetHookStatus #2: %v", err)
	}
	if !resp.Msg.RulesInstalled {
		t.Error("rules should be detected after seeding settings.json")
	}
}

func TestInstallHooks_MissingBinary_ReportsManualFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", filepath.Join(home, "empty-bin")) // ensure ssq-hooks is not discoverable

	svc := newCreateTestService(t, createTestStorage(t))

	resp, err := svc.InstallHooks(context.Background(), connect.NewRequest(&sessionv1.InstallHooksRequest{
		InstallRules: true,
	}))
	if err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	if resp.Msg.Status.RulesInstalled {
		t.Error("rules must not be reported installed when the binary is unavailable")
	}
	if len(resp.Msg.Messages) == 0 {
		t.Error("expected a manual-fallback message when ssq-hooks is missing")
	}
}
