//go:build darwin && cgo

// Package main embeds Info.plist via CGO linking for macOS TCC permissions.
package main

import "C"
