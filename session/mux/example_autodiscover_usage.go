// Example usage of AutoDiscovery - not included in build
// +build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/tstapler/stapler-squad/session/mux"
)

func main() {
	// Create auto-discovery with filesystem watching and fallback
	ad := mux.NewAutoDiscoveryWithFallback()
	defer ad.Stop()

	// Register callback for session changes
	ad.OnSessionChange(func(session *mux.DiscoveredSession, isNew bool) {
		if isNew {
			log.Printf("New session discovered: %s (PID: %d, Command: %s)",
				session.SocketPath,
				session.Metadata.PID,
				session.Metadata.Command)
		} else {
			log.Printf("Session removed: %s", session.SocketPath)
		}
	})

	// Start auto-discovery
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	done, err := ad.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start auto-discovery: %v", err)
	}

	// Report mode
	if ad.WatcherActive() {
		fmt.Println("Auto-discovery mode: Filesystem watching (immediate)")
	} else {
		fmt.Println("Auto-discovery mode: Polling fallback (2s interval)")
	}

	// Wait for sessions
	fmt.Println("Watching for claude-mux sessions...")
	fmt.Println("Press Ctrl+C to exit")

	// Periodically report discovered sessions
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			fmt.Println("Auto-discovery stopped")
			return

		case <-ticker.C:
			sessions := ad.GetSessions()
			fmt.Printf("Active sessions: %d\n", len(sessions))
			for _, s := range sessions {
				fmt.Printf("  - %s (Command: %s, PID: %d)\n",
					s.SocketPath, s.Metadata.Command, s.Metadata.PID)
			}

		case <-ctx.Done():
			fmt.Println("Timeout reached")
			return
		}
	}
}
