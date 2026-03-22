package mux

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// SessionInfo contains metadata about a tmux session
type SessionInfo struct {
	Name         string
	CreatedAt    time.Time
	LastActivity time.Time
	Path         string
	Windows      int
	Attached     bool
}

// ListStaplerSquadSessionsWithInfo returns sessions with full metadata
func ListStaplerSquadSessionsWithInfo() ([]SessionInfo, error) {
	// Format: name|created|activity|path|windows|attached
	cmd := exec.Command("tmux", "list-sessions", "-F",
		"#{session_name}|#{session_created}|#{session_activity}|#{session_path}|#{session_windows}|#{session_attached}")
	output, err := cmd.Output()
	if err != nil {
		// tmux returns error if no sessions exist
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	var sessions []SessionInfo
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}

		name := parts[0]
		// Only include sessions that match our naming convention (new or legacy prefix)
		if !strings.HasPrefix(name, "staplersquad_") && !strings.HasPrefix(name, "claudesquad_") {
			continue
		}

		created, _ := strconv.ParseInt(parts[1], 10, 64)
		activity, _ := strconv.ParseInt(parts[2], 10, 64)
		windows, _ := strconv.Atoi(parts[4])
		attached := parts[5] == "1"

		sessions = append(sessions, SessionInfo{
			Name:         name,
			CreatedAt:    time.Unix(created, 0),
			LastActivity: time.Unix(activity, 0),
			Path:         parts[3],
			Windows:      windows,
			Attached:     attached,
		})
	}

	return sessions, nil
}

// InteractiveSessionPicker displays an interactive picker for session selection
func InteractiveSessionPicker() (string, error) {
	sessions, err := ListStaplerSquadSessionsWithInfo()
	if err != nil {
		return "", err
	}

	if len(sessions) == 0 {
		return "", fmt.Errorf("no stapler-squad tmux sessions found")
	}

	// Check if we have a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("interactive picker requires a terminal")
	}

	// Put terminal in raw mode for key reading
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	selected := 0
	reader := bufio.NewReader(os.Stdin)

	for {
		// Clear screen and move cursor to top
		fmt.Print("\033[2J\033[H")

		// Print header
		fmt.Println("\033[1;36m┌─────────────────────────────────────────────────────────────────────────┐\033[0m")
		fmt.Println("\033[1;36m│\033[0m  \033[1mSelect a session to attach\033[0m                                            \033[1;36m│\033[0m")
		fmt.Println("\033[1;36m│\033[0m  \033[2mUse ↑/↓ or j/k to navigate, Enter to select, q to quit\033[0m               \033[1;36m│\033[0m")
		fmt.Println("\033[1;36m└─────────────────────────────────────────────────────────────────────────┘\033[0m")
		fmt.Println()

		// Print sessions
		for i, s := range sessions {
			prefix := "  "
			nameStyle := "\033[0m"
			if i == selected {
				prefix = "\033[1;32m▶ \033[0m"
				nameStyle = "\033[1;32m"
			}

			// Format the session name (remove prefix for display)
			displayName := strings.TrimPrefix(strings.TrimPrefix(s.Name, "staplersquad_"), "claudesquad_")
			if len(displayName) > 40 {
				displayName = displayName[:37] + "..."
			}

			// Format path (abbreviate home directory and long paths)
			displayPath := abbreviatePath(s.Path)

			// Format time
			timeAgo := formatTimeAgo(s.LastActivity)

			// Attached indicator
			attachedStr := ""
			if s.Attached {
				attachedStr = " \033[1;33m[attached]\033[0m"
			}

			fmt.Printf("%s%s%-40s\033[0m%s\n", prefix, nameStyle, displayName, attachedStr)
			if i == selected {
				fmt.Printf("     \033[2mPath: %s\033[0m\n", displayPath)
				fmt.Printf("     \033[2mLast activity: %s\033[0m\n", timeAgo)
				fmt.Println()
			}
		}

		// Read key
		b, err := reader.ReadByte()
		if err != nil {
			return "", err
		}

		switch b {
		case 'q', 'Q', 27: // q, Q, or ESC
			// Check if it's an escape sequence (arrow keys)
			if b == 27 {
				// Try to read more bytes for escape sequence
				reader.ReadByte() // [
				arrow, _ := reader.ReadByte()
				switch arrow {
				case 'A': // Up arrow
					if selected > 0 {
						selected--
					}
				case 'B': // Down arrow
					if selected < len(sessions)-1 {
						selected++
					}
				default:
					// Plain ESC - quit
					fmt.Print("\033[2J\033[H") // Clear screen
					return "", fmt.Errorf("cancelled")
				}
			} else {
				// q or Q - quit
				fmt.Print("\033[2J\033[H") // Clear screen
				return "", fmt.Errorf("cancelled")
			}
		case 'j', 'J': // vim down
			if selected < len(sessions)-1 {
				selected++
			}
		case 'k', 'K': // vim up
			if selected > 0 {
				selected--
			}
		case 13: // Enter
			fmt.Print("\033[2J\033[H") // Clear screen
			return sessions[selected].Name, nil
		}
	}
}

// abbreviatePath shortens a path for display
func abbreviatePath(path string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(path, home) {
		path = "~" + path[len(home):]
	}

	// Abbreviate .stapler-squad paths
	if strings.Contains(path, ".stapler-squad/worktrees/") {
		parts := strings.Split(path, ".stapler-squad/worktrees/")
		if len(parts) > 1 {
			path = "~/.stapler-squad/worktrees/" + filepath.Base(parts[1])
		}
	}

	if len(path) > 50 {
		path = "..." + path[len(path)-47:]
	}

	return path
}

// formatTimeAgo formats a time as relative duration
func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return "just now"
	} else if duration < time.Hour {
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	} else {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
