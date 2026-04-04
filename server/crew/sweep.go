package crew

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/queue"
)

// truncateLast truncates s to the last n characters.
func truncateLast(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// DetectTestRunner detects the project's test runner by inspecting the working directory.
// Returns nil, nil if no runner is detected (distinct from an error).
func DetectTestRunner(dir string) (*TestRunner, error) {
	// Priority 1: Go
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		// Verify at least one *_test.go file exists to avoid false positives
		hasTests, err := hasGoTestFiles(dir)
		if err != nil {
			return nil, fmt.Errorf("checking for Go test files: %w", err)
		}
		if hasTests {
			return &TestRunner{Name: "go", Command: "go test ./...", Timeout: 120 * time.Second}, nil
		}
	}

	// Priority 2: Rust
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		return &TestRunner{Name: "rust", Command: "cargo test", Timeout: 300 * time.Second}, nil
	}

	// Priority 3: Python
	for _, manifest := range []string{"pyproject.toml", "pytest.ini", "setup.py", "setup.cfg"} {
		if _, err := os.Stat(filepath.Join(dir, manifest)); err == nil {
			return &TestRunner{Name: "python", Command: "python -m pytest", Timeout: 180 * time.Second}, nil
		}
	}

	// Priority 4: Node/JS
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		runner, err := detectNodeRunner(dir)
		if err != nil {
			return nil, fmt.Errorf("detecting node runner: %w", err)
		}
		if runner != nil {
			return runner, nil
		}
	}

	// Priority 5: Makefile
	if _, err := os.Stat(filepath.Join(dir, "Makefile")); err == nil {
		if hasMakeTestTarget(dir) {
			return &TestRunner{Name: "make", Command: "make test", Timeout: 180 * time.Second}, nil
		}
	}

	// No runner detected
	return nil, nil
}

// hasGoTestFiles returns true if any *_test.go files exist anywhere under dir.
func hasGoTestFiles(dir string) (bool, error) {
	found := false
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if found {
			return filepath.SkipAll
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), "_test.go") {
			found = true
			return filepath.SkipAll
		}
		// Skip vendor and hidden dirs
		if info.IsDir() && (info.Name() == "vendor" || strings.HasPrefix(info.Name(), ".")) {
			return filepath.SkipDir
		}
		return nil
	})
	return found, err
}

// detectNodeRunner reads package.json to determine the test command and package manager.
func detectNodeRunner(dir string) (*TestRunner, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, nil
	}

	// Look for "test" script field
	content := string(data)
	if !strings.Contains(content, `"test"`) {
		return nil, nil
	}

	// Extract test script value
	testScript := extractJSONStringField(content, "test")
	if testScript == "" {
		return nil, nil
	}

	// Skip npm placeholder: "echo "Error: no test specified" && exit 1"
	// The script value extraction may be truncated at inner quotes, so we check
	// both the extracted script and the raw content for the npm default placeholder.
	if strings.HasPrefix(testScript, "echo ") {
		return nil, nil
	}
	if strings.Contains(content, "Error: no test specified") {
		return nil, nil
	}
	if testScript == "exit 1" || testScript == "exit 0" {
		return nil, nil
	}

	// Detect package manager from lockfiles
	cmd := detectNodeCommand(dir)
	return &TestRunner{Name: "node", Command: cmd, Timeout: 120 * time.Second}, nil
}

// extractJSONStringField does a simple (not full JSON) extraction of a string field value.
// It searches for the pattern `"field":` to avoid matching field names that appear as values.
func extractJSONStringField(json, field string) string {
	// Search for `"field"` followed (with optional whitespace) by `:` and a string value.
	// Iterate through all occurrences in case the field name appears as a value first.
	key := `"` + field + `"`
	searchFrom := 0
	for {
		idx := strings.Index(json[searchFrom:], key)
		if idx < 0 {
			return ""
		}
		absIdx := searchFrom + idx
		rest := strings.TrimSpace(json[absIdx+len(key):])
		// The next non-space character must be ':' for this to be a key.
		if len(rest) == 0 || rest[0] != ':' {
			searchFrom = absIdx + len(key)
			continue
		}
		rest = strings.TrimSpace(rest[1:]) // skip ':'
		if len(rest) == 0 || rest[0] != '"' {
			searchFrom = absIdx + len(key)
			continue
		}
		// Extract the string value
		rest = rest[1:]
		end := strings.Index(rest, `"`)
		if end < 0 {
			return ""
		}
		return rest[:end]
	}
}

// detectNodeCommand returns the test command based on lockfile presence.
func detectNodeCommand(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "bun.lockb")); err == nil {
		return "bun test"
	}
	if _, err := os.Stat(filepath.Join(dir, "pnpm-lock.yaml")); err == nil {
		return "pnpm test"
	}
	if _, err := os.Stat(filepath.Join(dir, "yarn.lock")); err == nil {
		return "yarn test"
	}
	return "npm test"
}

// hasMakeTestTarget checks if the Makefile has a 'test' target.
func hasMakeTestTarget(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "Makefile"))
	if err != nil {
		return false
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "test:") || trimmed == "test" {
			return true
		}
	}
	return false
}

// RunSweep executes the test runner and returns a structured SweepResult.
func RunSweep(ctx context.Context, dir string, runner *TestRunner) (*SweepResult, error) {
	if runner == nil {
		return &SweepResult{Status: SweepStatusNoTestsFound}, nil
	}

	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, runner.Timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "sh", "-c", runner.Command)
	cmd.Dir = dir

	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	runErr := cmd.Run()
	duration := time.Since(start)

	rawOutput := outBuf.String()
	cleanOutput := StripANSI(rawOutput)
	cleanOutput = truncateLast(cleanOutput, 4000)

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	// Check for timeout
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return &SweepResult{
			Status:        SweepStatusTimeout,
			TestOutput:    cleanOutput,
			Duration:      duration,
			ExitCode:      exitCode,
			RunnerName:    runner.Name,
			RunnerCommand: runner.Command,
		}, nil
	}

	if runErr != nil && exitCode == 0 {
		// Command failed to start or other internal error
		return &SweepResult{
			Status:        SweepStatusError,
			TestOutput:    cleanOutput,
			Duration:      duration,
			ExitCode:      exitCode,
			RunnerName:    runner.Name,
			RunnerCommand: runner.Command,
		}, nil
	}

	// Parse failing tests
	failingTests := parseFailingTests(cleanOutput, runner.Name)
	failureHash := computeFailureHash(failingTests)

	status := SweepStatusPass
	if exitCode != 0 {
		status = SweepStatusFail
	}

	return &SweepResult{
		Status:        status,
		TestOutput:    cleanOutput,
		FailingTests:  failingTests,
		FailureHash:   failureHash,
		Duration:      duration,
		ExitCode:      exitCode,
		RunnerName:    runner.Name,
		RunnerCommand: runner.Command,
	}, nil
}

// parseFailingTests extracts failing test names from test output.
func parseFailingTests(output, ecosystem string) []string {
	var failing []string
	lines := strings.Split(output, "\n")

	switch ecosystem {
	case "go":
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "--- FAIL:") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					failing = append(failing, parts[2])
				}
			}
		}
	case "node":
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "✕ ") || strings.HasPrefix(line, "× ") ||
				strings.HasPrefix(line, "FAIL ") || strings.Contains(line, "● ") {
				failing = append(failing, strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line, "✕ "), "× "), "FAIL ")))
			}
		}
	case "python":
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "FAILED ") {
				name := strings.TrimPrefix(line, "FAILED ")
				// Remove " - AssertionError..." suffix if present
				if idx := strings.Index(name, " - "); idx >= 0 {
					name = name[:idx]
				}
				failing = append(failing, name)
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	deduped := failing[:0]
	for _, t := range failing {
		if t != "" && !seen[t] {
			seen[t] = true
			deduped = append(deduped, t)
		}
	}
	return deduped
}

// computeFailureHash produces a stable SHA256 hash of the sorted failing test names.
// Used for oscillation detection: same hash = same failures despite different edits.
func computeFailureHash(tests []string) string {
	if len(tests) == 0 {
		return ""
	}
	sorted := make([]string, len(tests))
	copy(sorted, tests)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return fmt.Sprintf("%x", h)
}

// CollectDiffSummary runs git diff to collect change statistics.
func CollectDiffSummary(dir string) (*queue.DiffSummary, error) {
	// Get stats (lines added/deleted)
	statCmd := exec.Command("git", "diff", "--stat", "HEAD")
	statCmd.Dir = dir
	statOut, err := statCmd.Output()
	if err != nil {
		// git diff --stat failing is not fatal (may be a clean tree or non-git dir)
		return &queue.DiffSummary{}, nil
	}

	// Get changed file names
	namesCmd := exec.Command("git", "diff", "--name-only", "HEAD")
	namesCmd.Dir = dir
	namesOut, _ := namesCmd.Output()

	summary := parseDiffStat(string(statOut))
	if len(namesOut) > 0 {
		for _, line := range strings.Split(strings.TrimSpace(string(namesOut)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				summary.ChangedFiles = append(summary.ChangedFiles, line)
			}
		}
		summary.FilesChanged = int32(len(summary.ChangedFiles))
	}

	return summary, nil
}

// parseDiffStat extracts added/deleted line counts from `git diff --stat` output.
func parseDiffStat(output string) *queue.DiffSummary {
	summary := &queue.DiffSummary{}
	lines := strings.Split(output, "\n")
	// The last non-empty line has the summary: "N files changed, M insertions(+), K deletions(-)"
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// Parse "N insertions(+)"
		var added, deleted int32
		fmt.Sscanf(parseStatValue(line, "insertion"), "%d", &added)
		fmt.Sscanf(parseStatValue(line, "deletion"), "%d", &deleted)
		summary.LinesAdded = added
		summary.LinesDeleted = deleted
		break
	}
	return summary
}

// parseStatValue extracts a numeric value from a git stat summary line.
func parseStatValue(line, keyword string) string {
	idx := strings.Index(line, keyword)
	if idx < 0 {
		return "0"
	}
	// Walk backwards to find the number
	prefix := strings.TrimSpace(line[:idx])
	parts := strings.Fields(prefix)
	if len(parts) == 0 {
		return "0"
	}
	return parts[len(parts)-1]
}
