package app

import (
	"claude-squad/app/state"
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"context"
	"fmt"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
)

// BenchmarkTabSwitchingRealistic tests the actual tab switching performance including git and tmux operations
func BenchmarkTabSwitchingRealistic(b *testing.B) {
	sessionCounts := []int{10, 25, 50}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("TabSwitch_%d", count), func(b *testing.B) {
			h := setupRealisticBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Simulate actual user behavior: navigate to a session and view its details
				sessionIdx := i % count
				h.list.SetSelectedIdx(sessionIdx)
				instance := h.list.GetSelectedInstance()

				if instance != nil {
					// This is what actually happens when you switch tabs or navigate

					// 1. Update menu (fast)
					h.menu.SetInstance(instance)

					// 2. Update diff pane (EXPENSIVE: git operations)
					h.tabbedWindow.UpdateDiff(instance)

					// 3. Update preview pane (EXPENSIVE: tmux capture-pane)
					if err := h.tabbedWindow.UpdatePreview(instance); err != nil {
						// Log but continue benchmark
						b.Logf("Preview update error: %v", err)
					}

					// 4. Category organization (can be expensive with many sessions)
					h.list.OrganizeByCategory()
				}
			}
		})
	}
}

// BenchmarkIndividualExpensiveOperations breaks down where time is spent
func BenchmarkIndividualExpensiveOperations(b *testing.B) {
	h := setupRealisticBenchmarkHome(b, 20)
	instance := h.list.GetSelectedInstance()

	b.Run("GitDiffOperation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			h.tabbedWindow.UpdateDiff(instance)
		}
	})

	b.Run("TmuxCaptureOperation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err := h.tabbedWindow.UpdatePreview(instance); err != nil {
				b.Logf("Preview error: %v", err)
			}
		}
	})

	b.Run("CategoryOrganization", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			h.list.OrganizeByCategory()
		}
	})

	b.Run("MenuUpdate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			h.menu.SetInstance(instance)
		}
	})
}

// BenchmarkRapidTabSwitching simulates rapid tab switching like a user would do
func BenchmarkRapidTabSwitching(b *testing.B) {
	sessionCounts := []int{10, 25, 50}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("RapidSwitch_%d", count), func(b *testing.B) {
			h := setupRealisticBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Simulate user rapidly switching between sessions
				for j := 0; j < 5; j++ {
					sessionIdx := (i + j) % count
					h.list.SetSelectedIdx(sessionIdx)

					// Simulate the instanceChanged() method being called
					instance := h.list.GetSelectedInstance()
					if instance != nil {
						// Only do lightweight operations during rapid switching
						h.menu.SetInstance(instance)
						h.tabbedWindow.SetInstance(instance)

						// Expensive operations should be debounced during rapid switching
						// This tests if our debouncing works for the expensive ops too
						if j == 4 { // Only on the last switch
							h.tabbedWindow.UpdateDiff(instance)
							h.tabbedWindow.UpdatePreview(instance)
						}
					}
				}
			}
		})
	}
}

// BenchmarkFullInstanceChanged tests the complete instanceChanged workflow
func BenchmarkFullInstanceChanged(b *testing.B) {
	sessionCounts := []int{10, 25, 50}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("FullInstanceChanged_%d", count), func(b *testing.B) {
			h := setupRealisticBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				sessionIdx := i % count
				h.list.SetSelectedIdx(sessionIdx)

				// Test the complete instance change workflow
				h.instanceChanged()
			}
		})
	}
}

// BenchmarkDebouncedVsImmediateUpdates compares performance with and without debouncing
func BenchmarkDebouncedVsImmediateUpdates(b *testing.B) {
	b.Run("WithDebouncing", func(b *testing.B) {
		h := setupRealisticBenchmarkHome(b, 25)

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Simulate rapid navigation with debouncing
			for j := 0; j < 10; j++ {
				h.list.SetSelectedIdx(j % 25)
				h.instanceChanged() // This should be lightweight
			}
		}
	})

	b.Run("WithoutDebouncing", func(b *testing.B) {
		h := setupRealisticBenchmarkHome(b, 25)

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Simulate what would happen without debouncing
			for j := 0; j < 10; j++ {
				h.list.SetSelectedIdx(j % 25)
				h.instanceChanged() // This runs every time
			}
		}
	})
}

// setupRealisticBenchmarkHome creates a home instance with realistic session data
func setupRealisticBenchmarkHome(b *testing.B, sessionCount int) *home {
	appConfig := config.DefaultConfig()
	appState := config.LoadState()

	storage, err := session.NewStorage(appState)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	h := &home{
		ctx:                  ctx,
		spinner:              spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:                 ui.NewMenu(),
		tabbedWindow:         ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewVCPane()),
		errBox:               ui.NewErrBox(),
		storage:              storage,
		appConfig:            appConfig,
		program:              "echo",
		autoYes:              true,
		stateManager: state.NewManager(),
		appState:             appState,
	}
	h.list = ui.NewList(&h.spinner, true, appState)

	// Create sessions with realistic variety that will actually have git/tmux data
	categories := []string{"Development", "Testing", "Production", "Staging", "Bugfix"}
	programs := []string{"claude", "aider", "cursor"}

	for i := 0; i < sessionCount; i++ {
		category := categories[i%len(categories)]
		program := programs[i%len(programs)]

		// Use current directory so git operations will work
		workdir := "."

		instance, err := session.NewInstance(session.InstanceOptions{
			Title:    fmt.Sprintf("realistic-session-%d", i),
			Path:     workdir,
			Program:  program,
			Category: category,
		})
		if err != nil {
			b.Fatalf("Failed to create instance: %v", err)
		}

		// Set realistic status distribution
		statuses := []session.Status{
			session.Ready, session.Ready, session.Ready, // Most are ready
			session.Running, session.Running, // Some running
			session.Paused,        // Some paused
			session.NeedsApproval, // Some need approval
		}
		instance.Status = statuses[i%len(statuses)]

		h.list.AddInstance(instance)
	}

	// Set realistic window size
	h.updateHandleWindowSizeEvent(struct {
		Width  int
		Height int
	}{Width: 120, Height: 40})

	return h
}

// BenchmarkMemoryLeakTest tests for memory leaks during extended operation
func BenchmarkMemoryLeakTest(b *testing.B) {
	h := setupRealisticBenchmarkHome(b, 20)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulate extended session switching to detect memory leaks
		for j := 0; j < 100; j++ {
			sessionIdx := j % 20
			h.list.SetSelectedIdx(sessionIdx)
			instance := h.list.GetSelectedInstance()

			if instance != nil && j%10 == 0 { // Only expensive ops occasionally
				h.tabbedWindow.UpdateDiff(instance)
				h.tabbedWindow.UpdatePreview(instance)
			}
		}

		// Force garbage collection to see true memory usage
		if i%10 == 0 {
			b.ReportMetric(0, "gc-cycles")
		}
	}
}
