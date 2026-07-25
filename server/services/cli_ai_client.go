package services

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor"
)

// CLIAgentSpec describes how to invoke an AI agent CLI in one-shot mode.
type CLIAgentSpec struct {
	// Name is the human-readable identifier used in logs and error messages.
	Name string
	// Binary is the executable name resolved via exec.LookPath.
	Binary string
	// Args returns the argv slice (excluding binary name) for one-shot mode.
	Args func() []string
	// PromptSeparator is inserted between system and user prompts. Defaults to "\n\n---\n\n".
	PromptSeparator string
	// PromptAsArg delivers the combined prompt as the last positional argument to Args()
	// instead of writing it to stdin. Required for CLIs like agy (--print "msg") and
	// opencode (run "msg") that take the prompt inline rather than from stdin.
	PromptAsArg bool
}

// knownCLIAgents lists supported AI CLI agents in preference order for
// NewBestAvailableAIClient. The first one found in PATH wins.
var knownCLIAgents = []CLIAgentSpec{
	{
		// Claude Code CLI: --print enables non-interactive one-shot mode.
		// The message is read from stdin when no positional argument is given.
		Name:            "claude",
		Binary:          "claude",
		Args:            func() []string { return []string{"--print"} },
		PromptSeparator: "\n\n---\n\n",
	},
	{
		// Gemini CLI: reads a prompt from stdin in non-interactive mode.
		Name:            "gemini",
		Binary:          "gemini",
		Args:            func() []string { return []string{} },
		PromptSeparator: "\n\n---\n\n",
	},
	{
		// Antigravity CLI: --print takes the prompt as a positional string argument (not stdin).
		// Verified: agy --print "hello" exits 0 and writes response to stdout. (v1.0.15)
		Name:            "agy",
		Binary:          "agy",
		Args:            func() []string { return []string{"--print"} },
		PromptSeparator: "\n\n---\n\n",
		PromptAsArg:     true,
	},
	{
		// OpenCode CLI: run [message..] takes the prompt as positional args, not stdin.
		Name:            "opencode",
		Binary:          "opencode",
		Args:            func() []string { return []string{"run"} },
		PromptSeparator: "\n\n",
		PromptAsArg:     true,
	},
}

// CLIAIClient implements AIClient by shelling out to a locally installed
// AI agent CLI. The agent runs in one-shot mode with the prompt on stdin.
// executor.ShortLivedCmd provides context cancellation, timeout, and audit logging.
type CLIAIClient struct {
	spec CLIAgentSpec
	bin  string // resolved absolute path from LookPath
}

// NewCLIAIClient resolves spec.Binary in PATH and returns a CLIAIClient.
// Returns an error if the binary is not found.
func NewCLIAIClient(spec CLIAgentSpec) (*CLIAIClient, error) {
	bin, err := exec.LookPath(spec.Binary)
	if err != nil {
		return nil, fmt.Errorf("CLI agent %q not found in PATH: %w", spec.Binary, err)
	}
	if spec.PromptSeparator == "" {
		spec.PromptSeparator = "\n\n---\n\n"
	}
	return &CLIAIClient{spec: spec, bin: bin}, nil
}

// Complete delivers the combined prompt to the CLI and returns stdout.
// A 55-second timeout is applied so the caller's 60-second deadline has headroom.
func (c *CLIAIClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	combined := systemPrompt + c.spec.PromptSeparator + userPrompt
	var cmd *executor.ShortLivedCmd
	if c.spec.PromptAsArg {
		argv := append(c.spec.Args(), combined)
		cmd = executor.New(ctx, c.bin, argv, executor.WithTimeout(55*time.Second))
	} else {
		cmd = executor.New(ctx, c.bin, c.spec.Args(),
			executor.WithStdin(strings.NewReader(combined)),
			executor.WithTimeout(55*time.Second),
		)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("CLI agent %q: %w", c.spec.Name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// NewBestAvailableAIClient returns the highest-priority available AIClient
// and a string identifying the backend selected. Returns (nil, "") when no
// backend is available.
//
// specs is the ordered list of CLIAgentSpec entries to probe; pass knownCLIAgents
// at production call sites. Tests may pass a custom slice to avoid PATH lookups.
//
// Priority order:
//  1. First matching CLI agent from specs (handles its own auth)
//  2. Anthropic HTTP API — fallback if anthropicAPIKey is non-empty
//
// CLI agents are preferred because they manage their own authentication and
// model selection, requiring no extra configuration in stapler-squad.
func NewBestAvailableAIClient(anthropicAPIKey string, specs []CLIAgentSpec) (AIClient, string) {
	for _, spec := range specs {
		if c, err := NewCLIAIClient(spec); err == nil {
			return c, "cli:" + spec.Name
		}
	}
	if anthropicAPIKey != "" {
		if c, err := NewAnthropicAIClientFromKey(anthropicAPIKey); err == nil {
			return c, "anthropic-api"
		}
	}
	return nil, ""
}
