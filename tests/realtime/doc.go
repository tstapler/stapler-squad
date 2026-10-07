// Package realtime holds the only tests permitted to call time.Sleep (ADR-003).
//
// The notimesleeptest analyzer exempts this directory. Put a test here only when
// real wall-clock elapsed time is the behaviour under test and cannot be driven
// by a fake clock, channel, or require.Eventually. Otherwise, fix the test.
package realtime
