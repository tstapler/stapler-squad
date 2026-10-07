package a

import "time"

// Non-test files are out of scope.
func Wait() { time.Sleep(time.Millisecond) }
