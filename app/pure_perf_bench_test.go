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

// BenchmarkPureNavigation tests navigation without any tea.Cmd delays
func BenchmarkPureNavigation(b *testing.B) {
	sessionCounts := []int{50, 100, 200, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Pure_Nav_%d", count), func(b *testing.B) {
			h := setupPureBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Test pure navigation performance
				for j := 0; j < 50; j++ {
					h.list.Down()
					// Only call the lightweight operations, no tea.Cmd
					selected := h.list.GetSelectedInstance()
					h.menu.SetInstance(selected)
					h.tabbedWindow.SetInstance(selected)
				}

				for j := 0; j < 25; j++ {
					h.list.Up()
					selected := h.list.GetSelectedInstance()
					h.menu.SetInstance(selected)
					h.tabbedWindow.SetInstance(selected)
				}
			}
		})
	}
}

// BenchmarkListNavigation tests just the list navigation performance
func BenchmarkListNavigation(b *testing.B) {
	sessionCounts := []int{50, 100, 200, 500, 1000, 2000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("List_Nav_%d", count), func(b *testing.B) {
			appState := config.LoadState()
			s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
			list := ui.NewList(&s, false, appState)

			// Create test instances
			for i := 0; i < count; i++ {
				instance, err := session.NewInstance(session.InstanceOptions{
					Title:    fmt.Sprintf("nav-session-%d", i),
					Path:     ".",
					Program:  "echo",
					Category: fmt.Sprintf("Category-%d", i%5),
				})
				if err != nil {
					b.Fatalf("Failed to create instance: %v", err)
				}
				list.AddInstance(instance)
			}

			list.SetSize(80, 30)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Test pure list navigation
				for j := 0; j < 100; j++ {
					list.Down()
				}
				for j := 0; j < 50; j++ {
					list.Up()
				}
			}
		})
	}
}

// BenchmarkInstanceOperations tests individual instance operations
func BenchmarkInstanceOperations(b *testing.B) {
	h := setupPureBenchmarkHome(b, 100)

	b.Run("GetSelectedInstance", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = h.list.GetSelectedInstance()
		}
	})

	b.Run("SetInstance_Menu", func(b *testing.B) {
		selected := h.list.GetSelectedInstance()
		for i := 0; i < b.N; i++ {
			h.menu.SetInstance(selected)
		}
	})

	b.Run("SetInstance_TabbedWindow", func(b *testing.B) {
		selected := h.list.GetSelectedInstance()
		for i := 0; i < b.N; i++ {
			h.tabbedWindow.SetInstance(selected)
		}
	})
}

// BenchmarkCategoryOperations tests category performance with many sessions
func BenchmarkCategoryOperations(b *testing.B) {
	sessionCounts := []int{100, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Categories_%d", count), func(b *testing.B) {
			appState := config.LoadState()
			s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
			list := ui.NewList(&s, false, appState)

			// Create instances with many categories
			categories := make([]string, 10)
			for i := 0; i < 10; i++ {
				categories[i] = fmt.Sprintf("Category-%d", i)
			}

			for i := 0; i < count; i++ {
				instance, err := session.NewInstance(session.InstanceOptions{
					Title:    fmt.Sprintf("cat-session-%d", i),
					Path:     ".",
					Program:  "echo",
					Category: categories[i%len(categories)],
				})
				if err != nil {
					b.Fatalf("Failed to create instance: %v", err)
				}
				list.AddInstance(instance)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				list.OrganizeByCategory()
			}
		})
	}
}

// BenchmarkFilterOperations tests filtering performance
func BenchmarkFilterOperations(b *testing.B) {
	sessionCounts := []int{100, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Filter_%d", count), func(b *testing.B) {
			appState := config.LoadState()
			s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
			list := ui.NewList(&s, false, appState)

			// Create instances with varied statuses
			statuses := []session.Status{session.Ready, session.Running, session.Paused, session.NeedsApproval}
			for i := 0; i < count; i++ {
				instance, err := session.NewInstance(session.InstanceOptions{
					Title:    fmt.Sprintf("filter-session-%d", i),
					Path:     ".",
					Program:  "echo",
					Category: "Test",
				})
				if err != nil {
					b.Fatalf("Failed to create instance: %v", err)
				}
				instance.Status = statuses[i%len(statuses)]
				list.AddInstance(instance)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				list.TogglePausedFilter()
				list.TogglePausedFilter()
				list.SearchByTitle("test")
				list.ExitSearchMode()
			}
		})
	}
}

// BenchmarkSessionCreationRate tests how fast we can create sessions
func BenchmarkSessionCreationRate(b *testing.B) {
	appState := config.LoadState()
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&s, false, appState)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:    fmt.Sprintf("creation-session-%d", i),
			Path:     ".",
			Program:  "echo",
			Category: "Test",
		})
		if err != nil {
			b.Fatalf("Failed to create instance: %v", err)
		}
		list.AddInstance(instance)
	}
}

// setupPureBenchmarkHome creates a minimal home instance for pure performance testing
func setupPureBenchmarkHome(b *testing.B, sessionCount int) *home {
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
		tabbedWindow:         ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane()),
		errBox:               ui.NewErrBox(),
		storage:              storage,
		appConfig:            appConfig,
		program:              "echo",
		autoYes:              true,
		stateManager: state.NewManager(),
		appState:             appState,
	}
	h.list = ui.NewList(&h.spinner, true, appState)

	// Create sessions efficiently
	for i := 0; i < sessionCount; i++ {
		category := fmt.Sprintf("Category-%d", i%5)

		instance, err := session.NewInstance(session.InstanceOptions{
			Title:    fmt.Sprintf("pure-bench-session-%d", i),
			Path:     ".",
			Program:  "echo",
			Category: category,
		})
		if err != nil {
			b.Fatalf("Failed to create instance: %v", err)
		}

		instance.Status = session.Status(i % 4) // Cycle through statuses
		h.list.AddInstance(instance)
	}

	// Set minimal window size
	h.updateHandleWindowSizeEvent(struct {
		Width  int
		Height int
	}{Width: 80, Height: 20})

	return h
}
