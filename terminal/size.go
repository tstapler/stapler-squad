package terminal

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/tstapler/stapler-squad/log"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// Rate limiting for terminal size logging to prevent spam
var (
	lastLoggedSize    string
	lastLogTime       time.Time
	logCooldownPeriod = 10 * time.Second // Only log if size changes or 10 seconds have passed
)

// SizeInfo contains comprehensive terminal size information
type SizeInfo struct {
	// PTY dimensions (what the pseudo-terminal reports)
	PTYWidth  int
	PTYHeight int

	// Environment variable dimensions
	EnvWidth  int
	EnvHeight int

	// Actual dimensions we should use
	ActualWidth  int
	ActualHeight int

	// Debug information
	Method     string
	IsReliable bool
	Issues     []string
}

// Manager handles terminal size detection and management
type Manager struct {
	// Track size changes for optimization
	lastWidth  int
	lastHeight int
	lastMethod string
}

// NewManager creates a new terminal size manager
func NewManager() *Manager {
	return &Manager{}
}

// DetectSize attempts to detect the real terminal size using multiple methods
func (m *Manager) DetectSize() *SizeInfo {
	info := &SizeInfo{}

	// Method 1: Get PTY size using golang.org/x/term
	if width, height, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		info.PTYWidth = width
		info.PTYHeight = height
	} else {
		info.Issues = append(info.Issues, fmt.Sprintf("PTY size detection failed: %v", err))
	}

	// Method 2: Check environment variables (set by some terminals)
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if w, err := strconv.Atoi(cols); err == nil {
			info.EnvWidth = w
		}
	}
	if lines := os.Getenv("LINES"); lines != "" {
		if h, err := strconv.Atoi(lines); err == nil {
			info.EnvHeight = h
		}
	}

	// Method 3: Try ioctl syscall directly (more reliable on some systems)
	ioctlWidth, ioctlHeight := getTerminalSizeIOCTL()

	// Method 4: Query terminal using escape sequences (most reliable for actual visible area)
	escapeWidth, escapeHeight := queryTerminalSizeEscape()

	// Decision logic: choose the most reliable size
	info.ActualWidth, info.ActualHeight, info.Method, info.IsReliable = chooseBestSize(
		info.PTYWidth, info.PTYHeight,
		info.EnvWidth, info.EnvHeight,
		ioctlWidth, ioctlHeight,
		escapeWidth, escapeHeight,
	)

	// Detect potential issues
	info.detectIssues()

	// Only log when size actually changes
	currentSize := fmt.Sprintf("%dx%d-%s", info.ActualWidth, info.ActualHeight, info.Method)

	if currentSize != lastLoggedSize {
		// Single-line structured log with all information
		log.InfoLog.Printf("Terminal size changed: %dx%d [pty=%dx%d env=%dx%d ioctl=%dx%d escape=%dx%d method=%s reliable=%v]",
			info.ActualWidth, info.ActualHeight,
			info.PTYWidth, info.PTYHeight,
			info.EnvWidth, info.EnvHeight,
			ioctlWidth, ioctlHeight,
			escapeWidth, escapeHeight,
			info.Method, info.IsReliable)

		if len(info.Issues) > 0 {
			log.WarningLog.Printf("Terminal size issues: %v", strings.Join(info.Issues, "; "))
		}

		lastLoggedSize = currentSize
		lastLogTime = time.Now()
	}

	// Update tracking
	m.lastWidth = info.ActualWidth
	m.lastHeight = info.ActualHeight
	m.lastMethod = info.Method

	return info
}

// GetReliableSize returns the most reliable terminal dimensions
func (m *Manager) GetReliableSize() (width, height int, method string) {
	info := m.DetectSize()
	return info.ActualWidth, info.ActualHeight, info.Method
}

// HasSizeChanged returns true if the terminal size has changed since last check
func (m *Manager) HasSizeChanged() bool {
	width, height, _ := m.GetReliableSize()
	return width != m.lastWidth || height != m.lastHeight
}

// CreateWindowSizeMsg creates a BubbleTea WindowSizeMsg with current dimensions
func (m *Manager) CreateWindowSizeMsg() tea.WindowSizeMsg {
	width, height, _ := m.GetReliableSize()
	return tea.WindowSizeMsg{Width: width, Height: height}
}

// getTerminalSizeIOCTL uses direct ioctl syscall to get terminal size
func getTerminalSizeIOCTL() (int, int) {
	type winsize struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}

	ws := &winsize{}
	retCode, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)))

	if int(retCode) == -1 {
		log.WarningLog.Printf("IOCTL terminal size detection failed: %v", errno)
		return 0, 0
	}

	return int(ws.Col), int(ws.Row)
}

// queryTerminalSizeEscape queries the terminal directly using escape sequences
// This technique moves cursor to extreme position and queries actual position
// to determine the real visible terminal area (not PTY size)
func queryTerminalSizeEscape() (int, int) {
	// Skip this method in non-interactive environments or when stdin is not a terminal
	if !isTerminal() {
		return 0, 0
	}

	// Get file descriptors for stdin/stdout
	stdinFd := int(os.Stdin.Fd())

	// Save current terminal state
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		log.WarningLog.Printf("Failed to set terminal to raw mode: %v", err)
		return 0, 0
	}
	defer term.Restore(stdinFd, oldState)

	// Move cursor to extreme position (bottom-right corner)
	_, err = os.Stdout.Write([]byte("\033[9999;9999H"))
	if err != nil {
		log.WarningLog.Printf("Failed to write cursor position escape sequence: %v", err)
		return 0, 0
	}

	// Query current cursor position
	_, err = os.Stdout.Write([]byte("\033[6n"))
	if err != nil {
		log.WarningLog.Printf("Failed to write cursor query escape sequence: %v", err)
		return 0, 0
	}

	// Read response from terminal with timeout (format: \033[row;colR)
	response := make([]byte, 32)

	// Set up a timeout for reading the response
	done := make(chan bool, 1)
	var n int
	var readErr error

	go func() {
		n, readErr = os.Stdin.Read(response)
		done <- true
	}()

	// Wait for response or timeout (increased timeout for slower terminals)
	select {
	case <-done:
		if readErr != nil {
			// Don't log read errors as warnings - they're expected in some environments
			return 0, 0
		}
	case <-time.After(200 * time.Millisecond):
		// Timeout is common in certain environments, don't spam logs
		return 0, 0
	}

	// Parse the response with better error handling
	responseStr := string(response[:n])
	if len(responseStr) < 6 {
		return 0, 0
	}

	// Handle various terminal response formats more robustly
	if !strings.HasPrefix(responseStr, "\033[") {
		// Some terminals send different escape sequences, try to handle gracefully
		return 0, 0
	}

	// Look for the 'R' terminator, but be flexible about position
	rIndex := strings.IndexByte(responseStr, 'R')
	if rIndex == -1 {
		// Invalid response format, but don't spam logs
		return 0, 0
	}

	// Extract the coordinates part between \033[ and R
	coords := responseStr[2:rIndex]
	parts := strings.Split(coords, ";")
	if len(parts) != 2 {
		log.WarningLog.Printf("Invalid cursor position format: %q", coords)
		return 0, 0
	}

	height, err := strconv.Atoi(parts[0])
	if err != nil {
		log.WarningLog.Printf("Failed to parse height from cursor position: %v", err)
		return 0, 0
	}

	width, err := strconv.Atoi(parts[1])
	if err != nil {
		log.WarningLog.Printf("Failed to parse width from cursor position: %v", err)
		return 0, 0
	}

	// Move cursor back to top-left to avoid disrupting display
	_, err = os.Stdout.Write([]byte("\033[1;1H"))
	if err != nil {
		log.WarningLog.Printf("Failed to reset cursor position: %v", err)
	}

	return width, height
}

// isTerminal checks if we're running in a terminal environment
func isTerminal() bool {
	// Check if stdin is a terminal
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	// Check if it's a character device (terminal)
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// chooseBestSize selects the most reliable terminal size from available methods
func chooseBestSize(ptyW, ptyH, envW, envH, ioctlW, ioctlH, escapeW, escapeH int) (int, int, string, bool) {
	// Priority order for automatic detection:
	// 1. Escape sequence result (most accurate for actual visible area)
	// 2. IOCTL result (more direct than term.GetSize)
	// 3. PTY size (fallback)
	// Note: Environment variables skipped as they don't represent actual visible area in tiling WMs

	if escapeW > 0 && escapeH > 0 {
		return escapeW, escapeH, "escape_sequence", true
	}

	if ioctlW > 0 && ioctlH > 0 {
		// Check if IOCTL differs significantly from PTY (indicates potential issue)
		if ptyW > 0 && ptyH > 0 {
			widthDiff := abs(ioctlW - ptyW)
			heightDiff := abs(ioctlH - ptyH)
			if widthDiff > 5 || heightDiff > 5 {
				log.WarningLog.Printf("IOCTL size (%dx%d) differs from PTY (%dx%d) by %d,%d",
					ioctlW, ioctlH, ptyW, ptyH, widthDiff, heightDiff)
			}
		}
		return ioctlW, ioctlH, "ioctl", true
	}

	if ptyW > 0 && ptyH > 0 {
		return ptyW, ptyH, "pty", false
	}

	// Fallback to reasonable defaults
	log.InfoLog.Printf("All size detection methods failed - using defaults")
	return 80, 24, "default", false
}

// detectIssues identifies potential terminal size problems
func (info *SizeInfo) detectIssues() {
	// Only report truly problematic size issues, not just "large" terminals
	// Modern displays and tiling window managers commonly use large terminal sizes

	// Check for unreasonably large dimensions (likely indicates a real problem)
	if info.ActualHeight > 300 {
		info.Issues = append(info.Issues, fmt.Sprintf("Extremely large height (%d) - possible PTY detection error", info.ActualHeight))
	}

	if info.ActualWidth > 500 {
		info.Issues = append(info.Issues, fmt.Sprintf("Extremely large width (%d) - possible PTY detection error", info.ActualWidth))
	}

	// Check for size inconsistencies between methods (only if significant)
	if info.PTYWidth > 0 && info.PTYHeight > 0 {
		widthDiff := abs(info.ActualWidth - info.PTYWidth)
		heightDiff := abs(info.ActualHeight - info.PTYHeight)

		// Only report major discrepancies (>50 chars/lines) as issues
		if widthDiff > 50 || heightDiff > 20 {
			info.Issues = append(info.Issues, fmt.Sprintf("Major size difference between detection methods: %dx%d vs %dx%d (diff: %d,%d)",
				info.ActualWidth, info.ActualHeight, info.PTYWidth, info.PTYHeight, widthDiff, heightDiff))
		}
	}
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}