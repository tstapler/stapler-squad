package tmux

import (
	"testing"
)

// TestPromptDetection tests the detection of prompts from various AI programs
func TestPromptDetection(t *testing.T) {
	tests := []struct {
		name     string
		program  string
		content  string
		expected bool
	}{
		// Claude Code prompt detection
		{
			name:     "Claude approval prompt - standard",
			program:  ProgramClaude,
			content:  "I'll help you implement the user authentication system.\n\nNo, and tell Claude what to do differently\n\nWould you like me to proceed?",
			expected: true,
		},
		{
			name:     "Claude approval prompt - multiline context",
			program:  ProgramClaude,
			content:  "```python\ndef authenticate_user(username, password):\n    # Implementation here\n    pass\n```\n\nNo, and tell Claude what to do differently\n\nThis implementation uses bcrypt for password hashing.",
			expected: true,
		},
		{
			name:     "Claude approval prompt - with ANSI colors",
			program:  ProgramClaude,
			content:  "\x1b[32mSuccess:\x1b[0m File created successfully\n\nNo, and tell Claude what to do differently\n\n\x1b[33mNext steps:\x1b[0m Review the changes",
			expected: true,
		},
		{
			name:     "Claude working without prompt",
			program:  ProgramClaude,
			content:  "I'm analyzing the code structure...\n\nThe authentication system looks good. Here are the changes:\n\n```python\ndef login(user):\n    return validate_credentials(user)\n```",
			expected: false,
		},
		{
			name:     "Claude thinking/processing",
			program:  ProgramClaude,
			content:  "Let me examine the current implementation...\n\nAnalyzing dependencies...\n\nChecking for potential issues...",
			expected: false,
		},
		{
			name:     "Claude error without prompt",
			program:  ProgramClaude,
			content:  "Error: Could not find the specified file\n\nPlease check the file path and try again.",
			expected: false,
		},
		{
			name:     "Claude approval prompt - proceed dialog",
			program:  ProgramClaude,
			content:  "I'll help you with this task.\n\nDo you want to proceed?\n❯ 1. Yes\n  2. No",
			expected: true,
		},
		{
			name:     "Claude approval prompt - file read permission",
			program:  ProgramClaude,
			content:  "╭──────────────────────────────────────────────────────────────────────────╮\n│ Bash command                                                             │\n│                                                                          │\n│   cd /path/to/project && ls -la                                         │\n│                                                                          │\n│ Do you want to proceed?                                                  │\n│   1. Yes                                                                 │\n│ ❯ 2. Yes, allow reading from claude-squad/ from this project            │\n│   3. No, and tell Claude what to do differently (esc)                   │\n╰──────────────────────────────────────────────────────────────────────────╯",
			expected: true,
		},
		{
			name:     "Claude approval prompt - file write permission",
			program:  ProgramClaude,
			content:  "I need to modify some files.\n\n❯ 1. Yes\n  2. Yes, allow writing to the project directory\n  3. No, and tell Claude what to do differently",
			expected: true,
		},
		{
			name:     "Claude approval prompt - generic allow once",
			program:  ProgramClaude,
			content:  "This operation requires permissions.\n\n❯ 1. Yes, allow once\n  2. Yes, allow all edits during this session\n  3. No, and tell Claude what to do differently",
			expected: true,
		},

		// Aider prompt detection
		{
			name:     "Aider approval prompt - standard",
			program:  ProgramAider,
			content:  "I want to modify the following files:\n- app.py\n- config.json\n\n(Y)es/(N)o/(D)on't ask again",
			expected: true,
		},
		{
			name:     "Aider approval prompt - with file diff",
			program:  ProgramAider,
			content:  "Here are the proposed changes:\n\n@@ -10,3 +10,7 @@\n def main():\n+    print('Hello world')\n     return 0\n\n(Y)es/(N)o/(D)on't ask again",
			expected: true,
		},
		{
			name:     "Aider working without prompt",
			program:  ProgramAider,
			content:  "Applying changes to app.py...\n\nChanges applied successfully!\n\nThe file has been updated with your requested modifications.",
			expected: false,
		},
		{
			name:     "Aider analysis without prompt",
			program:  ProgramAider,
			content:  "I'll analyze the codebase structure...\n\nFound 15 Python files\nFound 3 configuration files\n\nReady for your next request.",
			expected: false,
		},

		// Gemini prompt detection
		{
			name:     "Gemini approval prompt",
			program:  ProgramGemini,
			content:  "I need to access the file system to make these changes.\n\nYes, allow once\n\nThis will modify the following files:",
			expected: true,
		},
		{
			name:     "Gemini working without prompt",
			program:  ProgramGemini,
			content:  "Generating code based on your requirements...\n\n```go\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}\n```",
			expected: false,
		},

		// Edge cases and variations
		{
			name:     "Empty content",
			program:  ProgramClaude,
			content:  "",
			expected: false,
		},
		{
			name:     "Only whitespace",
			program:  ProgramClaude,
			content:  "   \n\t\n   ",
			expected: false,
		},
		{
			name:     "Partial prompt text",
			program:  ProgramClaude,
			content:  "No, and tell Claude what",
			expected: false,
		},
		{
			name:     "Wrong program - Claude text with Aider program",
			program:  ProgramAider,
			content:  "No, and tell Claude what to do differently",
			expected: false,
		},
		{
			name:     "Case sensitivity test",
			program:  ProgramClaude,
			content:  "no, and tell claude what to do differently",
			expected: false,
		},
		{
			name:     "Multiple prompts in content",
			program:  ProgramClaude,
			content:  "First response here.\n\nNo, and tell Claude what to do differently\n\nSecond response with prompt again.\n\nNo, and tell Claude what to do differently",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a tmux instance with the test program
			tmux := &TmuxSession{program: tt.program}

			// Call the prompt detection logic directly
			hasPrompt := tmux.detectPromptInContent(tt.content)

			if hasPrompt != tt.expected {
				t.Errorf("detectPromptInContent() = %v, expected %v\nContent: %q", hasPrompt, tt.expected, tt.content)
			}
		})
	}
}

// TestRealClaudeCodeSamples tests with actual captured output from Claude Code sessions
func TestRealClaudeCodeSamples(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name: "Real Claude Code - requesting file changes",
			content: `I'll help you implement the user authentication system. Let me create the necessary files and functions.

First, I'll create the authentication module:

` + "```python" + `
# auth.py
import bcrypt
import jwt
from datetime import datetime, timedelta

class AuthenticationError(Exception):
    pass

def hash_password(password: str) -> str:
    """Hash a password using bcrypt."""
    salt = bcrypt.gensalt()
    return bcrypt.hashpw(password.encode('utf-8'), salt).decode('utf-8')
` + "```" + `

No, and tell Claude what to do differently

Should I proceed with creating this authentication system?`,
			expected: true,
		},
		{
			name: "Real Claude Code - working on implementation",
			content: `I'm implementing the requested changes to the authentication system.

Looking at the current structure, I can see we need to:
1. Update the password hashing mechanism
2. Add JWT token generation
3. Implement session management

Let me start by examining the existing code...

The current implementation uses a simple hash function. I'll upgrade it to use bcrypt for better security.

Implementation complete! The authentication system now includes:
- Secure password hashing with bcrypt
- JWT token generation and validation
- Session management with Redis
- Rate limiting for login attempts`,
			expected: false,
		},
		{
			name: "Real Claude Code - error with prompt",
			content: `I encountered an error while trying to access the configuration file:

FileNotFoundError: [Errno 2] No such file or directory: 'config.yaml'

The system is looking for the configuration file but it doesn't exist in the expected location.

No, and tell Claude what to do differently

Would you like me to:
1. Create a default configuration file
2. Look for the config file in a different location
3. Use environment variables instead`,
			expected: true,
		},
		{
			name: "Real Claude Code - analysis complete",
			content: `Analysis of the codebase complete!

Here's what I found:
- 23 Python files
- 5 configuration files
- 12 test files
- 1 Docker configuration

Security assessment:
✅ No hardcoded passwords found
✅ Proper input validation in place
⚠️  Some API endpoints lack rate limiting
❌ Missing CSRF protection on forms

The codebase is well-structured and follows Python best practices. The main areas for improvement are in the security middleware and API rate limiting.`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmux := &TmuxSession{program: ProgramClaude}
			hasPrompt := tmux.detectPromptInContent(tt.content)

			if hasPrompt != tt.expected {
				t.Errorf("detectPromptInContent() = %v, expected %v\nContent (first 200 chars): %q...",
					hasPrompt, tt.expected, truncateString(tt.content, 200))
			}
		})
	}
}

// TestStatusUpdateLogic tests the complete status update flow
func TestStatusUpdateLogic(t *testing.T) {
	tests := []struct {
		name        string
		program     string
		content     string
		autoYes     bool
		expectedStatus string // This would map to session.Status constants
	}{
		{
			name:        "Claude prompt without autoYes should need approval",
			program:     ProgramClaude,
			content:     "Ready to make changes.\n\nNo, and tell Claude what to do differently\n\nShould I proceed?",
			autoYes:     false,
			expectedStatus: "NeedsApproval",
		},
		{
			name:        "Claude prompt with autoYes should continue running",
			program:     ProgramClaude,
			content:     "Ready to make changes.\n\nNo, and tell Claude what to do differently\n\nShould I proceed?",
			autoYes:     true,
			expectedStatus: "Running", // AutoYes would tap enter automatically
		},
		{
			name:        "Claude working without prompt should be ready",
			program:     ProgramClaude,
			content:     "Implementation complete! All files have been updated successfully.",
			autoYes:     false,
			expectedStatus: "Ready",
		},
		{
			name:        "Aider prompt without autoYes should need approval",
			program:     ProgramAider,
			content:     "I want to modify app.py with the following changes:\n\n(Y)es/(N)o/(D)on't ask again",
			autoYes:     false,
			expectedStatus: "NeedsApproval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmux := &TmuxSession{program: tt.program}

			// Test the prompt detection
			hasPrompt := tmux.detectPromptInContent(tt.content)

			// This simulates the logic from app/app.go lines 713-726
			var resultStatus string
			if hasPrompt {
				if tt.autoYes {
					resultStatus = "Running" // AutoYes would tap enter
				} else {
					resultStatus = "NeedsApproval"
				}
			} else {
				resultStatus = "Ready"
			}

			if resultStatus != tt.expectedStatus {
				t.Errorf("Status logic = %v, expected %v\nPrompt detected: %v, AutoYes: %v",
					resultStatus, tt.expectedStatus, hasPrompt, tt.autoYes)
			}
		})
	}
}

// Helper function to truncate strings for test output
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}