package tmux

import (
	"strings"
	"testing"
)

func TestBannerFilter_IsBanner(t *testing.T) {
	filter := NewBannerFilter()

	tests := []struct {
		name     string
		line     string
		isBanner bool
	}{
		{
			name:     "empty line",
			line:     "",
			isBanner: false,
		},
		{
			name:     "whitespace only",
			line:     "   ",
			isBanner: false,
		},
		{
			name:     "regular terminal output",
			line:     "Hello, world!",
			isBanner: false,
		},
		{
			name:     "command prompt",
			line:     "user@host:~$ ls -la",
			isBanner: false,
		},
		{
			name:     "error message",
			line:     "Error: file not found",
			isBanner: false,
		},
		{
			name:     "tmux timestamp banner simple",
			line:     "14:23 5-Jan-24",
			isBanner: true,
		},
		{
			name:     "tmux full status line",
			line:     `[session-name] 0:bash* "localhost" 14:23 5-Jan-24`,
			isBanner: true,
		},
		{
			name:     "tmux window indicator",
			line:     "[0] 1:vim- 2:bash* 3:top#",
			isBanner: true,
		},
		{
			name:     "tmux status with pipes",
			line:     "[session] | window:0 | 14:23",
			isBanner: true,
		},
		{
			name:     "code output with timestamp",
			line:     "Timestamp: 2024-01-05T14:23:00Z",
			isBanner: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filter.IsBanner(tt.line)
			if got != tt.isBanner {
				t.Errorf("IsBanner(%q) = %v, want %v", tt.line, got, tt.isBanner)
			}
		})
	}
}

func TestBannerFilter_FilterBanners(t *testing.T) {
	filter := NewBannerFilter()

	input := []string{
		"Hello, world!",
		"14:23 5-Jan-24",
		"This is meaningful output",
		`[session-name] 0:bash* "localhost" 14:23 5-Jan-24`,
		"More meaningful content",
		"",
		"Final line",
	}

	expectedFiltered := []string{
		"Hello, world!",
		"This is meaningful output",
		"More meaningful content",
		"",
		"Final line",
	}
	expectedCount := 2

	filtered, count := filter.FilterBanners(input)

	if count != expectedCount {
		t.Errorf("FilterBanners() removed %d banners, want %d", count, expectedCount)
	}

	if len(filtered) != len(expectedFiltered) {
		t.Errorf("FilterBanners() returned %d lines, want %d", len(filtered), len(expectedFiltered))
	}

	for i, line := range filtered {
		if i >= len(expectedFiltered) {
			break
		}
		if line != expectedFiltered[i] {
			t.Errorf("FilterBanners() line %d = %q, want %q", i, line, expectedFiltered[i])
		}
	}
}

func TestBannerFilter_FilterBannersFromText(t *testing.T) {
	filter := NewBannerFilter()

	input := `Hello, world!
14:23 5-Jan-24
This is meaningful output
[session-name] 0:bash* "localhost" 14:23 5-Jan-24
More meaningful content

Final line`

	expectedOutput := `Hello, world!
This is meaningful output
More meaningful content

Final line`
	expectedCount := 2

	filtered, count := filter.FilterBannersFromText(input)

	if count != expectedCount {
		t.Errorf("FilterBannersFromText() removed %d banners, want %d", count, expectedCount)
	}

	if filtered != expectedOutput {
		t.Errorf("FilterBannersFromText() output mismatch\nGot:\n%s\n\nWant:\n%s", filtered, expectedOutput)
	}
}

func TestBannerFilter_HasMeaningfulContent(t *testing.T) {
	filter := NewBannerFilter()

	tests := []struct {
		name        string
		text        string
		hasMeaning  bool
	}{
		{
			name:       "empty text",
			text:       "",
			hasMeaning: false,
		},
		{
			name:       "only whitespace",
			text:       "   \n  \n   ",
			hasMeaning: false,
		},
		{
			name:       "only banners",
			text:       "14:23 5-Jan-24\n[session] | window:0 | 14:23",
			hasMeaning: false,
		},
		{
			name:       "mixed content with banners",
			text:       "14:23 5-Jan-24\nHello, world!\n[session] 0:bash*",
			hasMeaning: true,
		},
		{
			name:       "only meaningful content",
			text:       "Hello, world!\nThis is output",
			hasMeaning: true,
		},
		{
			name:       "single meaningful line",
			text:       "Error: something went wrong",
			hasMeaning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filter.HasMeaningfulContent(tt.text)
			if got != tt.hasMeaning {
				t.Errorf("HasMeaningfulContent(%q) = %v, want %v", tt.text, got, tt.hasMeaning)
			}
		})
	}
}

func TestBannerFilter_RealWorldExamples(t *testing.T) {
	filter := NewBannerFilter()

	// Real-world tmux status line examples
	realBanners := []string{
		`[staplersquad_test-session] 0:claude* "MacBook-Pro" 09:45 17-Oct-25`,
		`[work] 1:vim- 2:bash* "dev-server" 14:22 17-Oct-25`,
		`14:23 5-Jan-24`,
		`[session] | main | 16:45`,
		`[0] 0:zsh* 1:vim- 2:htop# 14:30 17-Oct-25`,
	}

	for _, banner := range realBanners {
		if !filter.IsBanner(banner) {
			t.Errorf("IsBanner(%q) = false, want true (real tmux banner not detected)", banner)
		}
	}

	// Real-world non-banner examples that should NOT be filtered
	realContent := []string{
		`$ git status`,
		`On branch main`,
		`Changes not staged for commit:`,
		`modified:   session/instance.go`,
		`Error: connection refused at 127.0.0.1:8080`,
		`INFO: Starting server on port 3000`,
		`DEBUG [2024-01-05 14:23:45] Processing request`,
		`> npm install`,
		`added 523 packages in 12.4s`,
	}

	for _, content := range realContent {
		if filter.IsBanner(content) {
			t.Errorf("IsBanner(%q) = true, want false (real content incorrectly detected as banner)", content)
		}
	}
}

func TestBannerFilter_ANSIDetection(t *testing.T) {
	filter := NewBannerFilter()

	// Test ANSI-based banner detection (most reliable)
	tests := []struct {
		name     string
		line     string
		isBanner bool
	}{
		{
			name:     "reverse video banner",
			line:     "\x1b[7m[session] 0:bash*\x1b[27m",
			isBanner: true,
		},
		{
			name:     "256-color background (status bar)",
			line:     "\x1b[48;5;240m[session] 0:bash*\x1b[0m",
			isBanner: true,
		},
		{
			name:     "combined reverse video and colors",
			line:     "\x1b[7m\x1b[38;5;250m\x1b[48;5;237m[session]\x1b[0m",
			isBanner: true,
		},
		{
			name:     "bold + reverse video",
			line:     "\x1b[1m\x1b[7m[session] 0:bash*\x1b[0m",
			isBanner: true,
		},
		{
			name:     "status bar positioning",
			line:     "\x1b[24;1H\x1b[7m[session] 0:bash*\x1b[27m",
			isBanner: true,
		},
		{
			name:     "regular colored text (not banner)",
			line:     "\x1b[31mError:\x1b[0m something failed",
			isBanner: false,
		},
		{
			name:     "bold text (not banner)",
			line:     "\x1b[1mImportant:\x1b[0m read this",
			isBanner: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filter.IsBanner(tt.line)
			if got != tt.isBanner {
				t.Errorf("IsBanner(%q) = %v, want %v", tt.line, got, tt.isBanner)
			}
		})
	}
}

func TestStripANSICodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI codes",
			input:    "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name:     "simple color code",
			input:    "\x1b[31mError\x1b[0m",
			expected: "Error",
		},
		{
			name:     "multiple codes",
			input:    "\x1b[1m\x1b[31mBold Red\x1b[0m",
			expected: "Bold Red",
		},
		{
			name:     "reverse video",
			input:    "\x1b[7m[session]\x1b[27m",
			expected: "[session]",
		},
		{
			name:     "256-color codes",
			input:    "\x1b[38;5;250m\x1b[48;5;237mColored\x1b[0m",
			expected: "Colored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSICodes(tt.input)
			if got != tt.expected {
				t.Errorf("stripANSICodes(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBannerFilter_EdgeCases(t *testing.T) {
	filter := NewBannerFilter()

	tests := []struct {
		name     string
		line     string
		isBanner bool
	}{
		{
			name:     "very long line",
			line:     strings.Repeat("a", 10000),
			isBanner: false,
		},
		{
			name:     "unicode characters",
			line:     "Hello 世界 🌍",
			isBanner: false,
		},
		{
			name:     "special characters",
			line:     "!@#$%^&*()_+-=[]{}|;':\",./<>?",
			isBanner: false,
		},
		{
			name:     "ANSI escape codes",
			line:     "\x1b[31mError\x1b[0m: failed",
			isBanner: false,
		},
		{
			name:     "tab characters",
			line:     "field1\tfield2\tfield3",
			isBanner: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filter.IsBanner(tt.line)
			if got != tt.isBanner {
				t.Errorf("IsBanner(%q) = %v, want %v", tt.line, got, tt.isBanner)
			}
		})
	}
}

func TestBannerFilter_PerformanceWithLargeInput(t *testing.T) {
	filter := NewBannerFilter()

	// Generate large input with mixed banners and content
	lines := make([]string, 10000)
	for i := 0; i < len(lines); i++ {
		if i%10 == 0 {
			lines[i] = "14:23 5-Jan-24" // Banner every 10 lines
		} else {
			lines[i] = "This is meaningful content line " + string(rune(i))
		}
	}

	// Test that filtering completes in reasonable time
	filtered, count := filter.FilterBanners(lines)

	expectedCount := 1000 // 10000 / 10
	if count != expectedCount {
		t.Errorf("FilterBanners() removed %d banners, want %d", count, expectedCount)
	}

	expectedFiltered := 9000
	if len(filtered) != expectedFiltered {
		t.Errorf("FilterBanners() returned %d lines, want %d", len(filtered), expectedFiltered)
	}
}
