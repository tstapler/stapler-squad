package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotateServiceLogIfLarge_CopyTruncatesWhenOversized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.log")
	content := strings.Repeat("x", serviceLogMaxBytes+1)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to seed log file: %v", err)
	}

	rotateServiceLogIfLarge(path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("original log missing after rotation: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected original log truncated to 0 bytes, got %d", info.Size())
	}

	rotated, err := os.ReadFile(path + ".old")
	if err != nil {
		t.Fatalf("expected rotated .old file: %v", err)
	}
	if string(rotated) != content {
		t.Fatalf("rotated content mismatch: got %d bytes, want %d", len(rotated), len(content))
	}
}

func TestRotateServiceLogIfLarge_AgesOutPreviousOldGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.log")
	first := strings.Repeat("a", serviceLogMaxBytes+1)
	if err := os.WriteFile(path, []byte(first), 0600); err != nil {
		t.Fatalf("failed to seed first log file: %v", err)
	}
	rotateServiceLogIfLarge(path)

	second := strings.Repeat("b", serviceLogMaxBytes+1)
	if err := os.WriteFile(path, []byte(second), 0600); err != nil {
		t.Fatalf("failed to seed second log file: %v", err)
	}
	rotateServiceLogIfLarge(path)

	gen1, err := os.ReadFile(path + ".old.1")
	if err != nil {
		t.Fatalf("expected first rotation preserved as .old.1: %v", err)
	}
	if string(gen1) != first {
		t.Fatalf(".old.1 should hold the first rotation's content, got %d bytes want %d", len(gen1), len(first))
	}

	gen0, err := os.ReadFile(path + ".old")
	if err != nil {
		t.Fatalf("expected second rotation as .old: %v", err)
	}
	if string(gen0) != second {
		t.Fatalf(".old should hold the second rotation's content, got %d bytes want %d", len(gen0), len(second))
	}
}

func TestRotateServiceLogIfLarge_NoopWhenUnderThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "service.log")
	if err := os.WriteFile(path, []byte("small"), 0600); err != nil {
		t.Fatalf("failed to seed log file: %v", err)
	}

	rotateServiceLogIfLarge(path)

	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Fatalf("expected no .old file for a small log, err=%v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "small" {
		t.Fatalf("expected untouched small log, got %q, err=%v", data, err)
	}
}
