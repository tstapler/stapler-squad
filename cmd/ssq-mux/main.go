// ssq-mux is a PTY multiplexer that enables bidirectional terminal access
// from multiple sources (e.g., IntelliJ terminal + claude-squad web UI).
//
// Usage:
//
//	ssq-mux [options] <command> [args...]
//	ssq-mux --attach <session-name>
//	ssq-mux --list
//
// Options:
//
//	-n, --name <name>       Custom session name (default: auto-generated from directory)
//	-a, --attach [session]  Attach to existing tmux session (interactive picker if no session specified)
//	-l, --list              List available claude-squad tmux sessions
//
// Example:
//
//	ssq-mux claude
//	ssq-mux -n "api-refactor" claude
//	ssq-mux --name "feature-xyz" aider --model gpt-4
//	ssq-mux --list                           # Show available sessions
//	ssq-mux --attach staplersquad_ext_12345   # Reattach to existing session
//
// The multiplexer creates a Unix domain socket at /tmp/ssq-mux-<PID>.sock
// that claude-squad can discover and connect to for terminal streaming.
//
// Session names are automatically generated from the current directory, command, and PID,
// e.g., "staplersquad_ext_myproject_claude_12345". The PID suffix ensures uniqueness when
// running multiple sessions in the same directory. You can override this with -n/--name.
//
// Reattaching to sessions:
//
// After a restart, tmux sessions may still be running but without the streaming socket.
// Use --list to see available sessions and --attach to reconnect:
//
//	ssq-mux --list
//	ssq-mux --attach staplersquad_ext_myproject_claude_12345
//
// Setup for seamless use with IntelliJ:
//
//	# Option 1: Shell alias
//	alias claude='ssq-mux claude'
//
//	# Option 2: PATH override (create ~/bin/claude wrapper)
//	#!/bin/bash
//	exec ssq-mux /usr/local/bin/claude "$@"
package main

import (
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/session/mux"

	"golang.org/x/term"
)

func main() {
	// Parse flags manually to allow flexible command positioning
	var sessionName string
	var attachSession string
	var attachMode bool // true if --attach was specified (even without session name)
	var listSessions bool
	var command string
	var args []string

	i := 1
	for i < len(os.Args) {
		arg := os.Args[i]
		if arg == "-n" || arg == "--name" {
			if i+1 >= len(os.Args) {
				fmt.Fprintf(os.Stderr, "ssq-mux: %s requires a session name argument\n", arg)
				os.Exit(1)
			}
			sessionName = os.Args[i+1]
			i += 2
		} else if arg == "-a" || arg == "--attach" {
			attachMode = true
			// Check if next arg is a session name (not another flag)
			if i+1 < len(os.Args) && len(os.Args[i+1]) > 0 && os.Args[i+1][0] != '-' {
				attachSession = os.Args[i+1]
				i += 2
			} else {
				// No session specified - will use interactive picker
				i++
			}
		} else if arg == "-l" || arg == "--list" {
			listSessions = true
			i++
		} else if arg == "-h" || arg == "--help" {
			printUsage()
			os.Exit(0)
		} else if len(arg) > 0 && arg[0] == '-' {
			fmt.Fprintf(os.Stderr, "ssq-mux: unknown option: %s\n", arg)
			printUsage()
			os.Exit(1)
		} else {
			// First non-flag argument is the command
			command = arg
			args = os.Args[i+1:]
			break
		}
	}

	// Handle --list mode
	if listSessions {
		sessions, err := mux.ListStaplerSquadSessions()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ssq-mux: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Println("No claude-squad tmux sessions found.")
			fmt.Println("Sessions are created when you run: ssq-mux <command>")
		} else {
			fmt.Println("Available claude-squad tmux sessions:")
			for _, s := range sessions {
				fmt.Printf("  %s\n", s)
			}
			fmt.Println("\nTo reattach: ssq-mux --attach <session-name>")
		}
		os.Exit(0)
	}

	// Handle --attach mode
	if attachMode {
		// If no session specified, use interactive picker
		if attachSession == "" {
			selected, err := mux.InteractiveSessionPicker()
			if err != nil {
				fmt.Fprintf(os.Stderr, "ssq-mux: %v\n", err)
				os.Exit(1)
			}
			attachSession = selected
		}

		// Check if stdin is a terminal
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprintf(os.Stderr, "ssq-mux: stdin is not a terminal\n")
			os.Exit(1)
		}

		// Set terminal to raw mode for proper PTY forwarding
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "ssq-mux: failed to set raw mode: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

		// Run the attach
		exitCode, err := mux.RunAttach(attachSession)
		if err != nil {
			// Restore terminal before printing error
			_ = term.Restore(int(os.Stdin.Fd()), oldState)
			fmt.Fprintf(os.Stderr, "ssq-mux: %v\n", err)
			os.Exit(1)
		}

		os.Exit(exitCode)
	}

	// Normal mode: require a command
	if command == "" {
		printUsage()
		os.Exit(1)
	}

	// Check if stdin is a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "ssq-mux: stdin is not a terminal\n")
		os.Exit(1)
	}

	// Set terminal to raw mode for proper PTY forwarding
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ssq-mux: failed to set raw mode: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	// Run the multiplexer with optional custom session name
	exitCode, err := mux.RunWithName(command, args, sessionName)
	if err != nil {
		// Restore terminal before printing error
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		fmt.Fprintf(os.Stderr, "ssq-mux: %v\n", err)
		os.Exit(1)
	}

	os.Exit(exitCode)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: ssq-mux [options] <command> [args...]\n")
	fmt.Fprintf(os.Stderr, "       ssq-mux --attach [session-name]\n")
	fmt.Fprintf(os.Stderr, "       ssq-mux --list\n")
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fmt.Fprintf(os.Stderr, "  -n, --name <name>       Custom session name (default: auto-generated)\n")
	fmt.Fprintf(os.Stderr, "  -a, --attach [session]  Attach to tmux session (interactive picker if omitted)\n")
	fmt.Fprintf(os.Stderr, "  -l, --list              List available claude-squad tmux sessions\n")
	fmt.Fprintf(os.Stderr, "  -h, --help              Show this help message\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  ssq-mux claude                           # Start new session\n")
	fmt.Fprintf(os.Stderr, "  ssq-mux -n \"api-refactor\" claude         # Custom session name\n")
	fmt.Fprintf(os.Stderr, "  ssq-mux --attach                         # Interactive session picker\n")
	fmt.Fprintf(os.Stderr, "  ssq-mux --attach staplersquad_ext_12345   # Attach to specific session\n")
	fmt.Fprintf(os.Stderr, "  ssq-mux --list                           # List available sessions\n")
	fmt.Fprintf(os.Stderr, "\nSession names are auto-generated based on directory and command,\n")
	fmt.Fprintf(os.Stderr, "e.g., \"staplersquad_ext_myproject_claude_1234\".\n")
	fmt.Fprintf(os.Stderr, "\nAfter a restart, use --attach to reconnect to orphaned sessions.\n")
}
