package vnc

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// WindowTracker polls the virtual display for a Chrome browser window using
// xdotool and calls registered callbacks when a window appears or disappears.
type WindowTracker struct {
	displayN          int
	onWindowDetected  func(windowID string)
	onWindowLost      func()
	lastWindowID      string
}

// NewWindowTracker creates a WindowTracker for the given X11 display number.
// onWindowDetected is called (in the polling goroutine) when a new Chrome window
// appears or the tracked window ID changes. onWindowLost is called when the
// previously tracked window disappears.
func NewWindowTracker(displayN int, onWindowDetected func(windowID string), onWindowLost func()) *WindowTracker {
	return &WindowTracker{
		displayN:         displayN,
		onWindowDetected: onWindowDetected,
		onWindowLost:     onWindowLost,
	}
}

// Start launches the window tracking goroutine. It exits when ctx is cancelled.
// The goroutine polls at 500ms when no window is tracked, or 2s when a window
// is stable, matching the plan specification.
func (wt *WindowTracker) Start(ctx context.Context) {
	go wt.run(ctx)
}

// run is the main polling loop. It must select on ctx.Done() to avoid leaks.
func (wt *WindowTracker) run(ctx context.Context) {
	// Start with fast polling until we detect a window.
	interval := 500 * time.Millisecond

	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			wt.poll(ctx)
			// Adjust polling interval based on whether we have a tracked window.
			if wt.lastWindowID != "" {
				interval = 2 * time.Second
			} else {
				interval = 500 * time.Millisecond
			}
			timer.Reset(interval)
		}
	}
}

// poll executes one xdotool query, parses results, and fires callbacks on change.
func (wt *WindowTracker) poll(ctx context.Context) {
	// Use a 1s timeout for the xdotool subprocess itself.
	pollCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	windowID := wt.findBestWindow(pollCtx)

	switch {
	case windowID == "" && wt.lastWindowID != "":
		// Window was lost.
		wt.lastWindowID = ""
		if wt.onWindowLost != nil {
			wt.onWindowLost()
		}

	case windowID != "" && wt.lastWindowID == "":
		// New window detected.
		wt.lastWindowID = windowID
		if wt.onWindowDetected != nil {
			wt.onWindowDetected(windowID)
		}

	case windowID != "" && windowID != wt.lastWindowID:
		// Window ID changed (browser restarted with a new window).
		wt.lastWindowID = windowID
		if wt.onWindowDetected != nil {
			wt.onWindowDetected(windowID)
		}
	}
	// If windowID == wt.lastWindowID (including both empty), no change.
}

// findBestWindow searches the virtual display for Chrome windows and returns
// the window ID of the largest one (by area). Returns "" if none are found.
func (wt *WindowTracker) findBestWindow(ctx context.Context) string {
	displayEnv := fmt.Sprintf("DISPLAY=:%d", wt.displayN)

	cmd := safeexec.CommandContextPG(ctx, "xdotool", "search", "--onlyvisible", "--classname", "google-chrome")
	cmd.Env = append(os.Environ(), displayEnv)

	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return ""
	}

	lines := strings.Fields(strings.TrimSpace(string(out)))
	if len(lines) == 0 {
		return ""
	}

	if len(lines) == 1 {
		return lines[0]
	}

	// Multiple windows: pick the one with the largest area.
	return wt.largestWindow(ctx, lines, displayEnv)
}

// largestWindow returns the window ID with the greatest width*height among ids.
// Falls back to the first ID if geometry cannot be determined for any window.
func (wt *WindowTracker) largestWindow(ctx context.Context, ids []string, displayEnv string) string {
	bestID := ids[0]
	bestArea := 0

	for _, id := range ids {
		area := wt.windowArea(ctx, id, displayEnv)
		if area > bestArea {
			bestArea = area
			bestID = id
		}
	}

	return bestID
}

// windowArea returns the pixel area (width*height) of the given window, or 0 on error.
// Each call gets its own 300ms deadline derived from ctx so that a single slow
// xdotool invocation cannot consume the entire 1s poll budget.
func (wt *WindowTracker) windowArea(ctx context.Context, windowID string, displayEnv string) int {
	callCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	cmd := safeexec.CommandContextPG(callCtx, "xdotool", "getwindowgeometry", windowID)
	cmd.Env = append(os.Environ(), displayEnv)

	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	// xdotool getwindowgeometry output example:
	//   Window 12345678
	//     Position: 0,0 (screen: 0)
	//     Geometry: 1280x800
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Geometry:") {
			continue
		}
		// Parse "Geometry: WxH"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		dims := strings.SplitN(parts[1], "x", 2)
		if len(dims) != 2 {
			continue
		}
		w, errW := strconv.Atoi(dims[0])
		h, errH := strconv.Atoi(dims[1])
		if errW != nil || errH != nil {
			continue
		}
		return w * h
	}

	return 0
}
