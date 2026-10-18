//go:build !darwin

package unfinished

func newTreeWatcher(treeChangeFunc) (treeWatcher, error) { return nil, errNoTreeWatcher }
