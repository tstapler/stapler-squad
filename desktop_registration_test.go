package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateDesktopFile_should_IncludeMimeTypeSchemeHandler_When_Rendered(t *testing.T) {
	content := desktopFileContent("stapler-squad")

	if !strings.Contains(content, "MimeType=x-scheme-handler/ssq;") {
		t.Errorf("desktop file content missing MimeType line, got:\n%s", content)
	}
	if !strings.Contains(content, "Exec=stapler-squad --open-url %u") {
		t.Errorf("desktop file content missing Exec line, got:\n%s", content)
	}
}

func TestRegisterLinuxScheme_should_SkipDuplicateRegistration_When_AlreadyRegistered(t *testing.T) {
	origQuery := xdgMimeQueryFunc
	origDefault := xdgMimeDefaultFunc
	origUpdateDB := updateDesktopDatabaseFunc
	t.Cleanup(func() {
		xdgMimeQueryFunc = origQuery
		xdgMimeDefaultFunc = origDefault
		updateDesktopDatabaseFunc = origUpdateDB
	})

	xdgMimeQueryFunc = func(_ context.Context) (string, error) {
		return linuxDesktopFileName, nil
	}
	defaultCalled := false
	xdgMimeDefaultFunc = func(_ context.Context) error {
		defaultCalled = true
		return nil
	}
	updateDBCalled := false
	updateDesktopDatabaseFunc = func(_ context.Context, _ string) error {
		updateDBCalled = true
		return nil
	}

	dir := t.TempDir()
	if err := registerLinuxScheme(context.Background(), dir, "stapler-squad"); err != nil {
		t.Fatalf("registerLinuxScheme() error = %v, want nil", err)
	}

	if defaultCalled {
		t.Error("expected xdg-mime default NOT to be called when already registered")
	}
	if updateDBCalled {
		t.Error("expected update-desktop-database NOT to be called when already registered")
	}

	// The .desktop file itself is still (re)written — that part is always
	// safe/idempotent regardless of registration state.
	if _, err := os.Stat(filepath.Join(dir, linuxDesktopFileName)); err != nil {
		t.Errorf("expected .desktop file to exist: %v", err)
	}
}

func TestRegisterLinuxScheme_should_RegisterViaXdgMime_When_NotYetRegistered(t *testing.T) {
	origQuery := xdgMimeQueryFunc
	origDefault := xdgMimeDefaultFunc
	origUpdateDB := updateDesktopDatabaseFunc
	t.Cleanup(func() {
		xdgMimeQueryFunc = origQuery
		xdgMimeDefaultFunc = origDefault
		updateDesktopDatabaseFunc = origUpdateDB
	})

	xdgMimeQueryFunc = func(_ context.Context) (string, error) {
		return "some-other-handler.desktop", nil
	}
	defaultCalled := false
	xdgMimeDefaultFunc = func(_ context.Context) error {
		defaultCalled = true
		return nil
	}
	updateDBCalled := false
	updateDesktopDatabaseFunc = func(_ context.Context, _ string) error {
		updateDBCalled = true
		return nil
	}

	dir := t.TempDir()
	if err := registerLinuxScheme(context.Background(), dir, "stapler-squad"); err != nil {
		t.Fatalf("registerLinuxScheme() error = %v, want nil", err)
	}

	if !defaultCalled {
		t.Error("expected xdg-mime default to be called when not yet registered")
	}
	if !updateDBCalled {
		t.Error("expected update-desktop-database to be called when not yet registered")
	}
}
