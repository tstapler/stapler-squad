package crew

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDetectTestRunner_Go(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a test file so detection succeeds
	if err := os.WriteFile(filepath.Join(dir, "foo_test.go"), []byte("package test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runner, err := DetectTestRunner(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner == nil {
		t.Fatal("expected Go runner, got nil")
	}
	if runner.Name != "go" {
		t.Errorf("expected Name=go, got %s", runner.Name)
	}
	if runner.Command != "go test ./..." {
		t.Errorf("expected 'go test ./...', got %s", runner.Command)
	}
}

func TestDetectTestRunner_GoNoTests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// No *_test.go files — should return nil (no false positive)
	runner, err := DetectTestRunner(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner != nil {
		t.Errorf("expected nil runner for Go project with no test files, got %v", runner)
	}
}

func TestDetectTestRunner_NodeNpmPlaceholder(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"name":"test","scripts":{"test":"echo \"Error: no test specified\" && exit 1"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644); err != nil {
		t.Fatal(err)
	}
	runner, err := DetectTestRunner(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner != nil {
		t.Errorf("expected nil runner for npm placeholder, got %v", runner)
	}
}

func TestDetectTestRunner_NodeWithTest(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"name":"test","scripts":{"test":"jest"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644); err != nil {
		t.Fatal(err)
	}
	runner, err := DetectTestRunner(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner == nil {
		t.Fatal("expected node runner, got nil")
	}
	if runner.Name != "node" {
		t.Errorf("expected Name=node, got %s", runner.Name)
	}
}

func TestDetectTestRunner_None(t *testing.T) {
	dir := t.TempDir()
	runner, err := DetectTestRunner(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner != nil {
		t.Errorf("expected nil runner for empty dir, got %v", runner)
	}
}

func TestRunSweep_NoRunner(t *testing.T) {
	dir := t.TempDir()
	result, err := RunSweep(context.Background(), dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != SweepStatusNoTestsFound {
		t.Errorf("expected NoTestsFound, got %v", result.Status)
	}
}

func TestRunSweep_Timeout(t *testing.T) {
	dir := t.TempDir()
	runner := &TestRunner{
		Name:    "test",
		Command: "sleep 999",
		Timeout: 100 * time.Millisecond,
	}
	result, err := RunSweep(context.Background(), dir, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != SweepStatusTimeout {
		t.Errorf("expected Timeout, got %v", result.Status)
	}
}

func TestRunSweep_Pass(t *testing.T) {
	dir := t.TempDir()
	runner := &TestRunner{
		Name:    "test",
		Command: "exit 0",
		Timeout: 10 * time.Second,
	}
	result, err := RunSweep(context.Background(), dir, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != SweepStatusPass {
		t.Errorf("expected Pass, got %v", result.Status)
	}
}

func TestRunSweep_Fail(t *testing.T) {
	dir := t.TempDir()
	runner := &TestRunner{
		Name:    "test",
		Command: "sh -c 'echo FAIL; exit 1'",
		Timeout: 10 * time.Second,
	}
	result, err := RunSweep(context.Background(), dir, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != SweepStatusFail {
		t.Errorf("expected Fail, got %v", result.Status)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
}

func TestRunSweep_ANSIStripped(t *testing.T) {
	dir := t.TempDir()
	runner := &TestRunner{
		Name:    "test",
		Command: `sh -c 'printf "\x1b[31mRED\x1b[0m OUTPUT"; exit 1'`,
		Timeout: 10 * time.Second,
	}
	result, err := RunSweep(context.Background(), dir, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.TestOutput, "\x1b") {
		t.Error("ANSI sequences not stripped from test output")
	}
	if !strings.Contains(result.TestOutput, "RED OUTPUT") {
		t.Error("visible text was stripped along with ANSI")
	}
}

func TestComputeFailureHash_Stable(t *testing.T) {
	tests1 := []string{"TestFoo", "TestBar", "TestBaz"}
	tests2 := []string{"TestBaz", "TestFoo", "TestBar"} // different order
	hash1 := computeFailureHash(tests1)
	hash2 := computeFailureHash(tests2)
	if hash1 != hash2 {
		t.Error("failure hash not stable across orderings")
	}
}

func TestComputeFailureHash_Empty(t *testing.T) {
	hash := computeFailureHash(nil)
	if hash != "" {
		t.Errorf("expected empty hash for nil tests, got %s", hash)
	}
}
