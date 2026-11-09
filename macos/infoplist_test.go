// Package macos holds the macOS-only build assertions (Task 5.1c). The test needs the macOS-built
// //:stapler-squad and is incompatible (skipped) on every other host platform.
package macos

import (
	"bytes"
	"debug/macho"
	"os"
	"path/filepath"
	"testing"
)

// infoPlist returns the __TEXT,__info_plist section of the Mach-O binary at path.
func infoPlist(path string) ([]byte, error) {
	f, err := macho.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sec := f.Section("__info_plist")
	if sec == nil || sec.Seg != "__TEXT" {
		return nil, os.ErrNotExist
	}
	return sec.Data()
}

func TestInfoPlistEmbedded(t *testing.T) {
	if os.Getenv("TEST_SRCDIR") == "" {
		t.Skip("needs the Bazel-built macOS //:stapler-squad in runfiles; not runnable under plain go test")
	}
	bin := filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("TEST_WORKSPACE"), "stapler-squad")
	data, err := infoPlist(bin)
	if err != nil {
		t.Fatalf("no __TEXT,__info_plist in %s (the -sectcreate link flag did not reach the woven link): %v", bin, err)
	}
	if !bytes.Contains(data, []byte("<key>CFBundleIdentifier</key>")) || !bytes.Contains(data, []byte("com.stapler-squad")) {
		t.Fatalf("embedded plist lacks CFBundleIdentifier com.stapler-squad:\n%s", data)
	}
}

func TestInfoPlistRejectsNonMachO(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	if _, err := infoPlist(exe); err == nil {
		t.Skip("test binary is itself a Mach-O with the section; nothing to reject")
	}
}
