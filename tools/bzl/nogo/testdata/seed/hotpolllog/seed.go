// Package hotpolllog is a deliberate hotpolllog violation; never import it.
package hotpolllog

import "log"

var DebugLog = log.New(nil, "DEBUG: ", 0)

// Violation logs through DebugLog inside a select case of a for loop.
func Violation(ch <-chan int) {
	for {
		select {
		case <-ch:
			DebugLog.Printf("got value")
		}
	}
}
