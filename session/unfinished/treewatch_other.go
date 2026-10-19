//go:build !darwin && !linux

package unfinished

func newTreeWatcher(treeChangeFunc) (treeWatcher, error) { return nil, errNoTreeWatcher }
