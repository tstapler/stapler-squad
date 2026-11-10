package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGateOK(t *testing.T) {
	if !gateOK(24, 12, 24, 12) || gateOK(24.1, 20, 24, 12) || gateOK(5, 11.9, 24, 12) {
		t.Fatal("gate boundaries")
	}
}

func TestClassify(t *testing.T) {
	if ex, ok := classify(1, "Error: No tests found"); ex == "" || ok {
		t.Fatal("excluded expected")
	}
	if ex, ok := classify(0, "3 passed"); ex != "" || !ok {
		t.Fatal("pass expected")
	}
	if ex, ok := classify(1, "2 failed"); ex != "" || ok {
		t.Fatal("failed expected")
	}
}

func TestReadLoadAndMem(t *testing.T) {
	d := t.TempDir()
	l, m := filepath.Join(d, "l"), filepath.Join(d, "m")
	_ = os.WriteFile(l, []byte("3.50 2.0 1.0 1/2 3\n"), 0o644)
	_ = os.WriteFile(m, []byte("MemTotal: 1 kB\nMemAvailable: 2097152 kB\n"), 0o644)
	if readLoad(l) != 3.5 || readMemGB(m) != 2 {
		t.Fatal(readLoad(l), readMemGB(m))
	}
}

func TestPlaywrightExitAndCapName(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "a.json")
	_ = os.WriteFile(p, []byte(`{"playwright_exit": 1}`), 0o644)
	if playwrightExit(p) != 1 || playwrightExit(filepath.Join(d, "missing.json")) != 1 {
		t.Fatal("failing and missing captures count as failed")
	}
	_ = os.WriteFile(p, []byte(`{"playwright_exit": 0}`), 0o644)
	if playwrightExit(p) != 0 || capName("tests/e2e/x-y.spec.ts") != "x-y.json" {
		t.Fatal("pass / name")
	}
}
