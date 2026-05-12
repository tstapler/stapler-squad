// Package a contains test fixtures for the hotpolllog analyzer.
package a

import (
	"log"
)

// DebugLog is a package-level variable that mimics log.DebugLog.
var DebugLog = log.New(nil, "DEBUG: ", 0)

// BAD1: DebugLog.Printf called directly in a select case inside a for loop.
func bad1(ch <-chan int) {
	for {
		select {
		case <-ch:
			DebugLog.Printf("got value") // want `hot-log call`
		}
	}
}

// BAD2: DebugLog.Printf (no package qualifier) in a select case inside a for loop.
func bad2(ch <-chan string) {
	for {
		select {
		case data := <-ch:
			_ = data
			DebugLog.Printf("got: %s", data) // want `hot-log call`
		}
	}
}

// BAD3: Qualified pkg.DebugLog.Printf inside a range-based for loop.
type mockPkg struct {
	DebugLog *log.Logger
}

var logPkg = &mockPkg{DebugLog: log.New(nil, "DEBUG: ", 0)}

func bad3(items []int, ch <-chan int) {
	for range items {
		select {
		case <-ch:
			logPkg.DebugLog.Printf("iteration") // want `hot-log call`
		}
	}
}

// BAD4: Even with a nil guard the call still fires every iteration — removing
// it is the fix, not guarding.
func bad4(ch <-chan int) {
	for {
		select {
		case <-ch:
			if DebugLog != nil {
				DebugLog.Printf("guarded but still bad") // want `hot-log call`
			}
		}
	}
}

// GOOD1: DebugLog.Printf is outside the select statement (after it), so it
// does not fire on every channel receive — this is fine.
func good1(ch <-chan int) {
	for {
		select {
		case <-ch:
		}
		DebugLog.Printf("after select, not in case")
	}
}

// GOOD2: DebugLog.Printf outside any loop entirely.
func good2() {
	DebugLog.Printf("top-level call")
}

// BAD5: InfoLog.Printf inside a select case of a for loop. InfoLog serializes
// concurrent goroutines on the stdlib log mutex (full I/O duration, not just
// channel send), causing mutex-contention events identical to DebugLog's block events.
var InfoLog = log.New(nil, "INFO: ", 0)

func bad5(ch <-chan int) {
	for {
		select {
		case <-ch:
			InfoLog.Printf("info inside select") // want `hot-log call`
		}
	}
}

// GOOD4: select without a surrounding for loop — not a hot-poll pattern.
func good4(ch <-chan int) {
	select {
	case <-ch:
		DebugLog.Printf("select without for")
	}
}
