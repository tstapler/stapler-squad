// claude-mux is a PTY multiplexer that enables bidirectional terminal access
// from multiple sources (e.g., IntelliJ terminal + claude-squad web UI).
//
// Usage:
//
//	claude-mux <command> [args...]
//
// Example:
//
//	claude-mux claude
//	claude-mux bash
//	claude-mux aider --model gpt-4
//
// The multiplexer creates a Unix domain socket at /tmp/claude-mux-<PID>.sock
// that claude-squad can discover and connect to for terminal streaming.
//
// Setup for seamless use with IntelliJ:
//
//	# Option 1: Shell alias
//	alias claude='claude-mux claude'
//
//	# Option 2: PATH override (create ~/bin/claude wrapper)
//	#!/bin/bash
//	exec claude-mux /usr/local/bin/claude "$@"
package main

import (
	"fmt"
	"os"

	"claude-squad/session/mux"

	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: claude-mux <command> [args...]\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  claude-mux claude\n")
		fmt.Fprintf(os.Stderr, "  claude-mux bash\n")
		fmt.Fprintf(os.Stderr, "  claude-mux aider --model gpt-4\n")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	// Check if stdin is a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "claude-mux: stdin is not a terminal\n")
		os.Exit(1)
	}

	// Set terminal to raw mode for proper PTY forwarding
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-mux: failed to set raw mode: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Run the multiplexer
	exitCode, err := mux.Run(command, args)
	if err != nil {
		// Restore terminal before printing error
		term.Restore(int(os.Stdin.Fd()), oldState)
		fmt.Fprintf(os.Stderr, "claude-mux: %v\n", err)
		os.Exit(1)
	}

	os.Exit(exitCode)
}
