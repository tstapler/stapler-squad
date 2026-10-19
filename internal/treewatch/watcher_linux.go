//go:build linux

package treewatch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/internal/fsevents"
)

// Watcher is a recursive inotify watch over one directory tree.
type Watcher struct {
	cfg  Config
	w    *fsnotify.Watcher
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// Start watches cfg.Root and every directory below it, and keeps the set
// current as directories are created. It fails with ErrWatchLimit if the tree
// does not fit under the inotify limit.
func Start(cfg Config) (*Watcher, error) {
	if _, err := os.Stat(cfg.Root); err != nil {
		return nil, err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := addTree(w, cfg.Root); err != nil {
		_ = w.Close()
		return nil, err
	}
	t := &Watcher{cfg: cfg, w: w, stop: make(chan struct{}), done: make(chan struct{})}
	go t.run()
	return t, nil
}

// Stop ends the watch and waits for the event loop to exit. Safe to repeat.
func (t *Watcher) Stop() {
	t.once.Do(func() {
		close(t.stop)
		_ = t.w.Close()
	})
	<-t.done
}

// addTree watches root and its subdirectories except skipDirs. Directories
// that vanish mid-walk are not an error.
func addTree(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if skipDir(d.Name()) && path != root {
			return filepath.SkipDir
		}
		if err := w.Add(path); err != nil {
			if errors.Is(err, syscall.ENOSPC) {
				return fmt.Errorf("%w: %s", ErrWatchLimit, path)
			}
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		return nil
	})
}

func (t *Watcher) run() {
	defer close(t.done)
	var batch []fsevents.Event
	flush := func() {
		if len(batch) > 0 && t.cfg.OnEvents != nil {
			t.cfg.OnEvents(batch)
		}
		batch = nil
	}
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	for {
		select {
		case <-t.stop:
			return
		case ev, ok := <-t.w.Events:
			if !ok {
				return
			}
			if e, ok := t.translate(ev); ok {
				if len(batch) == 0 {
					timer.Reset(batchWindow)
				}
				batch = append(batch, e)
			}
		case err, ok := <-t.w.Errors:
			if !ok {
				return
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				if len(batch) == 0 {
					timer.Reset(batchWindow)
				}
				batch = append(batch, fsevents.Event{Path: t.cfg.Root, Flags: fsevents.KernelDropped})
			}
		case <-timer.C:
			flush()
		}
	}
}

// translate maps an inotify event to the shared shape and extends the watch
// to newly created directories. Pure attribute (chmod) changes are dropped.
func (t *Watcher) translate(ev fsnotify.Event) (fsevents.Event, bool) {
	var f fsevents.EventFlags
	switch {
	case ev.Has(fsnotify.Create):
		f |= fsevents.ItemCreated
		if isDir(ev.Name) {
			f |= fsevents.ItemIsDir
			if !skipDir(filepath.Base(ev.Name)) {
				if err := addTree(t.w, ev.Name); errors.Is(err, ErrWatchLimit) {
					// Coverage is now incomplete: tell the caller to rescan.
					f |= fsevents.KernelDropped
				}
			}
		}
	case ev.Has(fsnotify.Write):
		f |= fsevents.ItemModified
	case ev.Has(fsnotify.Remove):
		f |= fsevents.ItemRemoved
	case ev.Has(fsnotify.Rename):
		f |= fsevents.ItemRenamed
	default:
		return fsevents.Event{}, false
	}
	return fsevents.Event{Path: ev.Name, Flags: f}, true
}

func isDir(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.IsDir()
}
