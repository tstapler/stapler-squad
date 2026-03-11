package session

import (
	"testing"
)

// TestStatusDetector_TestsFailingDetection verifies that test failures are correctly detected
func TestStatusDetector_TestsFailingDetection(t *testing.T) {
	t.Skip("StatusTestsFailing patterns are disabled to prevent false positives - see status_detector.go:421")
	detector := NewStatusDetector()

	tests := []struct {
		name           string
		output         string
		expectedStatus DetectedStatus
		shouldMatch    bool
	}{
		// Go test failures
		{
			name:           "go_test_fail_uppercase",
			output:         "FAIL\tgithub.com/example/package\t0.123s",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "go_test_fail_verbose",
			output:         "--- FAIL: TestMyFunction (0.00s)",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "go_test_failed_lowercase",
			output:         "test execution failed",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},

		// Python pytest failures
		{
			name:           "pytest_failed_test",
			output:         "FAILED tests/test_example.py::test_function",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "pytest_summary_failed",
			output:         "pytest: 5 passed, 3 failed in 2.34s",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "pytest_count_failed",
			output:         "===== 3 failed, 10 passed in 5.23s =====",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},

		// JavaScript Jest failures
		{
			name:           "jest_fail_test_file",
			output:         "FAIL src/components/Button.test.js",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "jest_summary_failed",
			output:         "Tests: 2 failed, 8 passed, 10 total",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "jest_failing_count",
			output:         "Ran all test suites. 3 tests failing.",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},

		// Generic test failure patterns
		{
			name:           "generic_tests_failed",
			output:         "5 tests failed",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "generic_test_failed_singular",
			output:         "1 test failed",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "generic_tests_failing",
			output:         "Warning: 3 tests failing",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "generic_test_suite_failed",
			output:         "test suite failed",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "generic_all_tests_failed",
			output:         "Build failed: all tests failed",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},

		// Test error count indicators
		{
			name:           "failures_count",
			output:         "Failures: 3",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "errors_count",
			output:         "Test Results - Errors: 5",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "failed_count",
			output:         "Failed: 2",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},

		// Case insensitivity tests
		{
			name:           "case_insensitive_fail",
			output:         "fail: TestMyFunction",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},
		{
			name:           "case_insensitive_failed",
			output:         "Failed test: test_example",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    true,
		},

		// Edge cases - should NOT match
		{
			name:           "passing_tests",
			output:         "PASS\tgithub.com/example/package\t0.123s",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    false,
		},
		{
			name:           "success_message",
			output:         "All tests passed successfully",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    false,
		},
		{
			name:           "no_tests_run",
			output:         "No tests found",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    false,
		},
		{
			name:           "compilation_success",
			output:         "Build succeeded with no errors",
			expectedStatus: StatusTestsFailing,
			shouldMatch:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := detector.Detect([]byte(tt.output))

			if tt.shouldMatch {
				if status != tt.expectedStatus {
					t.Errorf("Expected status %s for output %q, got %s",
						tt.expectedStatus.String(), tt.output, status.String())
				}
			} else {
				if status == tt.expectedStatus {
					t.Errorf("Should NOT match status %s for output %q, but it did",
						tt.expectedStatus.String(), tt.output)
				}
			}
		})
	}
}

// TestStatusDetector_TestsFailingWithContext verifies context reporting for test failures
func TestStatusDetector_TestsFailingWithContext(t *testing.T) {
	t.Skip("StatusTestsFailing patterns are disabled to prevent false positives - see status_detector.go:421")
	detector := NewStatusDetector()

	tests := []struct {
		name            string
		output          string
		expectedStatus  DetectedStatus
		expectedPattern string
	}{
		{
			name:            "go_test_fail_with_context",
			output:          "FAIL\tgithub.com/example/package\t0.123s",
			expectedStatus:  StatusTestsFailing,
			expectedPattern: "go_test_fail",
		},
		{
			name:            "pytest_fail_with_context",
			output:          "FAILED tests/test_example.py::test_function",
			expectedStatus:  StatusTestsFailing,
			expectedPattern: "pytest_fail",
		},
		{
			name:            "jest_fail_with_context",
			output:          "FAIL src/components/Button.test.js",
			expectedStatus:  StatusTestsFailing,
			expectedPattern: "jest_fail",
		},
		{
			name:            "generic_fail_with_context",
			output:          "5 tests failed",
			expectedStatus:  StatusTestsFailing,
			expectedPattern: "generic_test_fail",
		},
		{
			name:            "error_count_with_context",
			output:          "Failures: 3",
			expectedStatus:  StatusTestsFailing,
			expectedPattern: "test_error_count",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, context := detector.DetectWithContext([]byte(tt.output))

			if status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus.String(), status.String())
			}

			// Verify pattern name is in context
			if context == "" {
				t.Error("Expected non-empty context")
			}
		})
	}
}

// TestStatusDetector_TestsFailingPriority verifies priority ordering
func TestStatusDetector_TestsFailingPriority(t *testing.T) {
	t.Skip("StatusTestsFailing patterns are disabled to prevent false positives - see status_detector.go:421")
	detector := NewStatusDetector()

	tests := []struct {
		name           string
		output         string
		expectedStatus DetectedStatus
		description    string
	}{
		{
			name:           "error_overrides_test_fail",
			output:         "ERROR: Failed to connect\nFAIL: tests failed",
			expectedStatus: StatusError,
			description:    "Errors should take priority over test failures",
		},
		{
			name:           "test_fail_overrides_success",
			output:         "Task completed\nFAIL: tests failed",
			expectedStatus: StatusTestsFailing,
			description:    "Test failures should override success messages",
		},
		{
			name:           "test_fail_overrides_approval",
			output:         "Do you want to proceed?\nFAIL: tests failed",
			expectedStatus: StatusTestsFailing,
			description:    "Test failures should override approval prompts",
		},
		{
			name:           "test_fail_overrides_input",
			output:         "Please enter your name:\nFAIL: tests failed",
			expectedStatus: StatusTestsFailing,
			description:    "Test failures should override input prompts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := detector.Detect([]byte(tt.output))

			if status != tt.expectedStatus {
				t.Errorf("%s: Expected status %s, got %s",
					tt.description, tt.expectedStatus.String(), status.String())
			}
		})
	}
}

// TestStatusDetector_TestsFailingMultiline verifies detection in multiline output
func TestStatusDetector_TestsFailingMultiline(t *testing.T) {
	t.Skip("StatusTestsFailing patterns are disabled to prevent false positives - see status_detector.go:421")
	detector := NewStatusDetector()

	output := `
Running test suite...
===================

Test Results:
-------------
TestFunction1 ... PASS
TestFunction2 ... FAIL
TestFunction3 ... PASS

Summary: 2 passed, 1 failed
`

	status := detector.Detect([]byte(output))

	if status != StatusTestsFailing {
		t.Errorf("Expected StatusTestsFailing for multiline output with test failures, got %s",
			status.String())
	}
}

// TestStatusDetector_TestsFailingRealWorldExamples verifies real-world test output
func TestStatusDetector_TestsFailingRealWorldExamples(t *testing.T) {
	t.Skip("StatusTestsFailing patterns are disabled to prevent false positives - see status_detector.go:421")
	detector := NewStatusDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name: "go_verbose_test_output",
			output: `=== RUN   TestMyFunction
--- FAIL: TestMyFunction (0.00s)
    myfile_test.go:123: Expected 5, got 3
FAIL
FAIL	github.com/example/package	0.123s`,
		},
		{
			name: "pytest_verbose_output",
			output: `============================= test session starts ==============================
platform darwin -- Python 3.9.7, pytest-7.1.2
collected 10 items

tests/test_example.py::test_add PASSED                                   [ 10%]
tests/test_example.py::test_subtract FAILED                              [ 20%]

================================== FAILURES ===================================
__________________________ test_subtract _________________________________
    def test_subtract():
>       assert subtract(5, 3) == 2
E       assert 3 == 2

tests/test_example.py:15: AssertionError
=========================== short test summary info ===========================
FAILED tests/test_example.py::test_subtract - assert 3 == 2
========================= 1 failed, 1 passed in 0.12s =========================`,
		},
		{
			name: "jest_verbose_output",
			output: `FAIL src/components/Button.test.js
  ● Button component › renders correctly

    expect(received).toHaveTextContent()

    Expected: "Click me"
    Received: "Click"

      12 |     render(<Button>Click me</Button>);
      13 |     const button = screen.getByRole('button');
    > 14 |     expect(button).toHaveTextContent('Click me');
         |                    ^
      15 |   });

Test Suites: 1 failed, 5 passed, 6 total
Tests:       1 failed, 15 passed, 16 total`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := detector.Detect([]byte(tt.output))

			if status != StatusTestsFailing {
				t.Errorf("Expected StatusTestsFailing for real-world output, got %s",
					status.String())
			}
		})
	}
}

// TestStatusDetector_TestsFailingPatternNames verifies pattern names are available
func TestStatusDetector_TestsFailingPatternNames(t *testing.T) {
	t.Skip("StatusTestsFailing patterns are disabled to prevent false positives - see status_detector.go:421")
	detector := NewStatusDetector()

	patternNames := detector.GetPatternNames(StatusTestsFailing)

	expectedPatterns := []string{
		"go_test_fail",
		"pytest_fail",
		"jest_fail",
		"generic_test_fail",
		"test_error_count",
	}

	if len(patternNames) != len(expectedPatterns) {
		t.Errorf("Expected %d test failure patterns, got %d",
			len(expectedPatterns), len(patternNames))
	}

	// Verify all expected patterns are present
	patternMap := make(map[string]bool)
	for _, name := range patternNames {
		patternMap[name] = true
	}

	for _, expected := range expectedPatterns {
		if !patternMap[expected] {
			t.Errorf("Expected pattern '%s' not found in pattern names", expected)
		}
	}
}

// TestStatusDetector_TestsFailingString verifies String() method
func TestStatusDetector_TestsFailingString(t *testing.T) {
	status := StatusTestsFailing
	expected := "Tests Failing"

	if status.String() != expected {
		t.Errorf("Expected StatusTestsFailing.String() to return %q, got %q",
			expected, status.String())
	}
}
