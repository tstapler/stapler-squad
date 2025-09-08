package app

import (
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

// BenchmarkNavigationPerformance benchmarks the performance of navigating between sessions
// This test specifically measures the time taken for instanceChanged() operations
func BenchmarkNavigationPerformance(b *testing.B) {
	// Create test config and app state
	appConfig := config.DefaultConfig()
	appState := config.LoadState()

	// Create storage with state
	storage, err := session.NewStorage(appState)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	// Create test home instance
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
		state:                stateDefault,
		appState:             appState,
		selectionUpdateDelay: 150 * time.Millisecond,
	}
	h.list = ui.NewList(&h.spinner, true, appState)

	// Create mock instances with different categories for realistic scenario
	categories := []string{"Development", "Testing", "Production", "Staging", "Uncategorized"}
	numInstances := 50 // Test with reasonable number of instances

	for i := 0; i < numInstances; i++ {
		category := categories[i%len(categories)]
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:    fmt.Sprintf("test-session-%d", i),
			Path:     ".",
			Program:  "echo",
			Category: category,
		})
		if err != nil {
			b.Fatalf("Failed to create instance: %v", err)
		}

		// Add to list without finalizer for benchmark
		h.list.AddInstance(instance)
	}

	// Set initial window size for realistic rendering
	h.updateHandleWindowSizeEvent(struct {
		Width  int
		Height int
	}{Width: 120, Height: 40})

	b.ResetTimer()

	// Benchmark navigation operations
	for i := 0; i < b.N; i++ {
		// Simulate rapid navigation through sessions
		for j := 0; j < 10; j++ {
			// Navigate down
			h.list.Down()
			h.instanceChanged()

			// Navigate up
			h.list.Up()
			h.instanceChanged()
		}
	}
}

// BenchmarkInstanceChangedComponents benchmarks individual components of instanceChanged
func BenchmarkInstanceChangedComponents(b *testing.B) {
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
		state:                stateDefault,
		appState:             appState,
		selectionUpdateDelay: 150 * time.Millisecond,
	}
	h.list = ui.NewList(&h.spinner, true, appState)

	// Create test instances
	for i := 0; i < 20; i++ {
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   fmt.Sprintf("bench-session-%d", i),
			Path:    ".",
			Program: "echo",
		})
		if err != nil {
			b.Fatalf("Failed to create instance: %v", err)
		}
		h.list.AddInstance(instance)
	}

	h.updateHandleWindowSizeEvent(struct {
		Width  int
		Height int
	}{Width: 120, Height: 40})

	selected := h.list.GetSelectedInstance()

	// Benchmark category organization
	b.Run("OrganizeByCategory", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			h.list.OrganizeByCategory()
		}
	})

	// Benchmark diff updates
	b.Run("UpdateDiff", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			h.tabbedWindow.UpdateDiff(selected)
		}
	})

	// Benchmark preview updates
	b.Run("UpdatePreview", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			h.tabbedWindow.UpdatePreview(selected)
		}
	})

	// Benchmark menu updates
	b.Run("SetInstance", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			h.menu.SetInstance(selected)
		}
	})
}

// BenchmarkListRendering benchmarks the performance of list rendering
func BenchmarkListRendering(b *testing.B) {
	appState := config.LoadState()
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&s, false, appState)

	// Create test instances with various categories and statuses
	categories := []string{"Development", "Testing", "Production", "Staging"}
	statuses := []session.Status{session.Ready, session.Running, session.Paused, session.NeedsApproval}

	for i := 0; i < 100; i++ {
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:    fmt.Sprintf("render-test-%d", i),
			Path:     ".",
			Program:  "echo",
			Category: categories[i%len(categories)],
		})
		if err != nil {
			b.Fatalf("Failed to create instance: %v", err)
		}
		instance.Status = statuses[i%len(statuses)]
		list.AddInstance(instance)
	}

	list.SetSize(80, 30)

	b.ResetTimer()

	// Benchmark list rendering
	for i := 0; i < b.N; i++ {
		_ = list.String()
	}
}
