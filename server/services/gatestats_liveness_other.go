//go:build !linux && !darwin

package services

import "time"

// processStart is unavailable here, so ProcessWriterAlive falls back to signal 0 alone.
func processStart(int) (time.Time, bool) { return time.Time{}, false }
