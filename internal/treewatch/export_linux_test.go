//go:build linux

package treewatch

import "github.com/fsnotify/fsnotify"

func chmodEvent(name string) fsnotify.Event { return fsnotify.Event{Name: name, Op: fsnotify.Chmod} }
