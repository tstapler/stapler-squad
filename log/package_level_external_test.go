package log_test

import (
	"bytes"
	"log/slog"
	"runtime"
	"strings"
	"testing"

	applog "github.com/tstapler/stapler-squad/log"
)

func externalCallerPC() uintptr {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	return pcs[0]
}

// TestInfo_AttributesToCallerNotLogPackage guards the exact bug
// PackageLevelHandler depends on not regressing: applog.Info/Warn/Error/Debug
// must resolve the PC of *their caller*, not of themselves or of an internal
// wrapper. Calling slog.Info(msg, args...) directly from those package-level
// functions (the pre-fix implementation) attributes every log call in the
// whole binary to the "log" package itself, silently making every
// "session/..." or "server/..." override in STAPLER_SQUAD_LOG_LEVELS a
// no-op. This test lives in an external log_test package specifically so
// the correct answer ("log_test", this package) differs from the buggy one
// ("log", the package under test) — an internal test can't tell them apart.
func TestInfo_AttributesToCallerNotLogPackage(t *testing.T) {
	t.Cleanup(applog.ClearAllPackageLevels)

	// Pin the runtime level explicitly: this test's assertions depend on the
	// global level being at or above INFO (a Debug call would otherwise be
	// dropped for a different reason than the one this test guards against),
	// and this must not silently rely on test-execution order leaving the
	// global at its INFO default.
	origLevel := applog.GetRuntimeLevel()
	applog.SetRuntimeLevel(applog.INFO)
	t.Cleanup(func() { applog.SetRuntimeLevel(origLevel) })

	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	origDefault := applog.SetSlogDefaultForTest(slog.New(applog.NewPackageLevelHandler(base)))
	t.Cleanup(func() { applog.SetSlogDefaultForTest(origDefault) })

	pkg := applog.PackageForPC(externalCallerPC())
	if pkg == "log" {
		t.Fatalf("test setup bug: resolved this external test's own package as %q", pkg)
	}
	applog.SetPackageLevel(pkg, applog.DEBUG)

	applog.Debug("attributed to the external test package")

	if !strings.Contains(buf.String(), "attributed to the external test package") {
		t.Errorf("Debug() call was not attributed to caller package %q (frame-skip regression) — got: %s", pkg, buf.String())
	}
}
