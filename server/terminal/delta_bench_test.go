package terminal

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// generateANSIContent creates realistic terminal output with ANSI escape sequences.
// This simulates what a Claude Code session produces: colored text, cursor movements,
// progress bars, etc.
func generateANSIContent(lines, cols int) []byte {
	var buf bytes.Buffer
	colors := []string{
		"\033[31m", // Red
		"\033[32m", // Green
		"\033[33m", // Yellow
		"\033[34m", // Blue
		"\033[35m", // Magenta
		"\033[36m", // Cyan
		"\033[0m",  // Reset
	}
	for i := 0; i < lines; i++ {
		color := colors[i%len(colors)]
		// Simulate a typical terminal line: colored prefix + content + reset
		line := fmt.Sprintf("%s[%04d]%s %s\n",
			color,
			i,
			"\033[0m",
			strings.Repeat("x", cols-10),
		)
		buf.WriteString(line)
	}
	return buf.Bytes()
}

// generateProgressBar simulates a progress bar update (common in Claude Code output).
func generateProgressBar(percent int, width int) []byte {
	filled := (percent * width) / 100
	bar := fmt.Sprintf("\r\033[K\033[32m[%s%s]\033[0m %3d%%",
		strings.Repeat("█", filled),
		strings.Repeat("░", width-filled),
		percent,
	)
	return []byte(bar)
}

// BenchmarkDeltaGenerator_LargeANSI_100KB measures delta generation with a realistic
// 100KB ANSI payload. This is the production-like code path for large terminal sessions.
//
// We pre-generate numVariants slightly-different payloads so every iteration exercises
// real delta computation rather than the trivial no-change fast path (which would be hit
// on every iteration after the first if a single static buffer were reused).
// Each variant differs only on the last row, simulating new terminal output arriving —
// exactly the steady-state production pattern during active streaming.
func BenchmarkDeltaGenerator_LargeANSI_100KB(b *testing.B) {
	const cols, rows = 200, 50
	const targetSize = 100 * 1024
	const numVariants = 50

	// Pre-generate numVariants payloads. Stable rows are identical across variants;
	// the last row encodes the variant index so the delta generator always sees a change.
	stableBlock := generateANSIContent(rows-1, cols)
	variants := make([][]byte, numVariants)
	for v := range variants {
		lastLine := []byte(fmt.Sprintf("\033[32m[frame %04d] new output\033[0m\n", v))
		frame := append(append([]byte(nil), stableBlock...), lastLine...)
		// Pad to exactly targetSize by repeating stableBlock bytes.
		for len(frame) < targetSize {
			frame = append(frame, stableBlock...)
		}
		variants[v] = frame[:targetSize]
	}

	dg := NewDeltaGenerator(cols, rows)

	b.SetBytes(int64(targetSize))
	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		delta := dg.GenerateDelta(variants[i%numVariants])
		if delta == nil {
			b.Fatal("GenerateDelta returned nil")
		}
	}
}

// BenchmarkDeltaGenerator_RapidSequential measures performance under rapid sequential
// small updates — the common case during active streaming (each tmux poll produces a
// small diff of changed lines).
func BenchmarkDeltaGenerator_RapidSequential(b *testing.B) {
	const cols, rows = 120, 50
	const numUpdates = 100

	// Pre-generate a series of slightly-different content slices to simulate streaming.
	// Content differs by one line per update (simulating new log output being appended).
	updates := make([][]byte, numUpdates)
	for i := range updates {
		var buf bytes.Buffer
		for line := 0; line < rows; line++ {
			if line == rows-1 {
				// Last line changes each update (new output)
				fmt.Fprintf(&buf, "\033[32mUpdate %04d: new output line\033[0m\n", i)
			} else {
				fmt.Fprintf(&buf, "Stable line %04d: some content here\n", line)
			}
		}
		updates[i] = buf.Bytes()
	}

	dg := NewDeltaGenerator(cols, rows)

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		update := updates[i%numUpdates]
		delta := dg.GenerateDelta(update)
		if delta == nil {
			b.Fatal("GenerateDelta returned nil")
		}
		// Note: b.SetBytes not used here since input size varies per iteration
	}
}

// BenchmarkDeltaGenerator_FullScreen_WithCompression measures full-screen update
// performance after the compression dictionary has been warmed up (deltasSinceFullSync
// will trigger periodic full sync after fullSyncInterval iterations).
func BenchmarkDeltaGenerator_FullScreen_WithCompression(b *testing.B) {
	const cols, rows = 220, 50

	content := generateANSIContent(rows, cols)

	dg := NewDeltaGenerator(cols, rows)

	// Warm up the delta generator so the compression dictionary is populated.
	// fullSyncInterval is 50 deltas; warmupIters exceeds it by 5 to guarantee
	// at least one full sync cycle before timing begins.
	const warmupIters = 55 // fullSyncInterval(50) + 5
	for i := 0; i < warmupIters; i++ {
		dg.GenerateDelta(content)
	}

	b.SetBytes(int64(len(content)))
	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		delta := dg.GenerateDelta(content)
		if delta == nil {
			b.Fatal("GenerateDelta returned nil")
		}
	}
}

// BenchmarkStateGenerator_LargeScreen_200x50 measures state generation for a larger
// terminal size than the existing BenchmarkStateGeneration (which uses default 80x24).
// Larger terminals produce proportionally larger state objects and more serialization work.
func BenchmarkStateGenerator_LargeScreen_200x50(b *testing.B) {
	const cols, rows = 200, 50

	content := generateANSIContent(rows, cols)

	sg := NewStateGenerator(cols, rows)

	b.SetBytes(int64(len(content)))
	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		state := sg.GenerateState(content)
		if state == nil {
			b.Fatal("GenerateState returned nil")
		}
	}
}

// BenchmarkDeltaGenerator_ProgressBar measures the common case of rapid single-line
// updates — e.g., a progress bar updating on the last terminal line.
func BenchmarkDeltaGenerator_ProgressBar(b *testing.B) {
	const cols, rows = 120, 24

	// Pre-generate 100 progress bar frames
	frames := make([][]byte, 100)
	base := generateANSIContent(rows-1, cols) // Stable lines above the progress bar
	for i := range frames {
		frame := make([]byte, len(base)) //nolint:prealloc // final size unknown before calling generateProgressBar
		copy(frame, base)
		frame = append(frame, generateProgressBar(i, 50)...)
		frames[i] = frame
	}

	dg := NewDeltaGenerator(cols, rows)

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		frame := frames[i%len(frames)]
		delta := dg.GenerateDelta(frame)
		if delta == nil {
			b.Fatal("GenerateDelta returned nil")
		}
	}
}
