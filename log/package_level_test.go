package log

import (
	"bytes"
	"context"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"
)

// callerPC returns the PC of its caller, mirroring what logAt captures for a
// real log.Info/Warn/Debug/Error call (skip=3 there; skip=2 here since this
// helper itself is one frame shallower).
func callerPC() uintptr {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	return pcs[0]
}

func TestResolvePackage_ResolvesCallingTestPackage(t *testing.T) {
	pc := callerPC()
	pkg := resolvePackage(pc)
	if !strings.HasSuffix(pkg, "log") {
		t.Errorf("resolvePackage(pc of this test) = %q, want a path ending in \"log\" (this package), not e.g. an unrelated frame", pkg)
	}
}

func TestResolvePackage_ZeroPCReturnsEmpty(t *testing.T) {
	if got := resolvePackage(0); got != "" {
		t.Errorf("resolvePackage(0) = %q, want \"\"", got)
	}
}

func TestLookupPackageLevel_WalksUpHierarchy(t *testing.T) {
	overrides := map[string]slog.Level{
		"session":     slog.LevelWarn,
		"session/git": slog.LevelDebug,
	}

	tests := []struct {
		pkg       string
		wantLevel slog.Level
		wantFound bool
	}{
		{"session/git", slog.LevelDebug, true},          // exact match wins
		{"session/git/subpkg", slog.LevelDebug, true},   // walks up to "session/git"
		{"session/tmux", slog.LevelWarn, true},           // falls back to "session"
		{"server/services", 0, false},                    // no ancestor configured
	}
	for _, tt := range tests {
		level, found := lookupPackageLevel(overrides, tt.pkg)
		if found != tt.wantFound || (found && level != tt.wantLevel) {
			t.Errorf("lookupPackageLevel(%q) = (%v, %v), want (%v, %v)", tt.pkg, level, found, tt.wantLevel, tt.wantFound)
		}
	}
}

func TestPackageLevelHandler_OverrideAllowsDebugForOnePackageOnly(t *testing.T) {
	t.Cleanup(ClearAllPackageLevels)

	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewPackageLevelHandler(base)

	pc := callerPC()
	thisPkg := resolvePackage(pc)

	// Global level defaults to INFO (set in this package's init()); a Debug
	// record for a package with no override must still be dropped.
	SetRuntimeLevel(INFO)
	r := slog.NewRecord(time.Now(), slog.LevelDebug, "should be dropped", pc)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output without an override, got: %s", buf.String())
	}

	// Override this package to DEBUG — the same call should now come through.
	SetPackageLevel(thisPkg, DEBUG)
	r2 := slog.NewRecord(time.Now(), slog.LevelDebug, "should be logged", pc)
	if err := h.Handle(context.Background(), r2); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "should be logged") {
		t.Errorf("expected debug record to pass with package override, got: %s", buf.String())
	}
}
