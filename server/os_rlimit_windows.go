//go:build windows

package server

func openFDLimit() (int64, bool) { return 0, false }
