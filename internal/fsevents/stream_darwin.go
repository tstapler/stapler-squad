//go:build darwin

package fsevents

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	cfStringEncodingUTF8 = 0x08000100

	createFlagNoDefer    = 0x00000002
	createFlagWatchRoot  = 0x00000004
	createFlagFileEvents = 0x00000010

	// runLoopSlice bounds how long a Stop issued before the loop is running
	// (CFRunLoopStop is a no-op then) can go unnoticed.
	runLoopSlice = 0.25
)

// streamContext mirrors FSEventStreamContext; info carries the registry id.
type streamContext struct {
	_    int // version, must be 0
	info uintptr
	_    [3]uintptr // retain, release, copyDescription: unused, must be nil
}

var (
	loadOnce sync.Once
	loadErr  error

	cfStringCreateWithCString func(alloc uintptr, s *byte, enc uint32) uintptr
	cfArrayCreate             func(alloc uintptr, values *uintptr, n int, cb uintptr) uintptr
	cfRelease                 func(ref uintptr)
	cfRunLoopGetCurrent       func() uintptr
	cfRunLoopRunInMode        func(mode uintptr, seconds float64, returnAfterHandled uint8) int32
	cfRunLoopStop             func(rl uintptr)
	cfUUIDCreateString        func(alloc, uuid uintptr) uintptr
	cfStringGetCString        func(s uintptr, buf *byte, size int, enc uint32) uint8

	fsStreamCreate      func(alloc, cb uintptr, ctx *streamContext, paths uintptr, since uint64, latency float64, flags uint32) uintptr
	fsStreamSchedule    func(stream, rl, mode uintptr)
	fsStreamStart       func(stream uintptr) uint8
	fsStreamStop        func(stream uintptr)
	fsStreamInvalidate  func(stream uintptr)
	fsStreamRelease     func(stream uintptr)
	fsStreamFlushSync   func(stream uintptr)
	fsGetCurrentEventID func() uint64
	fsCopyUUIDForDevice func(dev int32) uintptr

	cfTypeArrayCallBacks uintptr // address of the kCFTypeArrayCallBacks struct
	cfRunLoopDefaultMode uintptr // value of kCFRunLoopDefaultMode

	callbackPtr uintptr

	registryMu sync.Mutex
	registry   = map[uintptr]*Stream{}
	nextID     atomic.Uintptr
)

// ptrFromAddr avoids the uintptr->unsafe.Pointer conversion go vet rejects for
// addresses that come from C rather than from Go memory.
func ptrFromAddr(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

func load() error {
	loadOnce.Do(func() {
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = fmt.Errorf("fsevents: dlopen CoreFoundation: %w", err)
			return
		}
		cs, err := purego.Dlopen("/System/Library/Frameworks/CoreServices.framework/CoreServices", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = fmt.Errorf("fsevents: dlopen CoreServices: %w", err)
			return
		}
		purego.RegisterLibFunc(&cfStringCreateWithCString, cf, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&cfArrayCreate, cf, "CFArrayCreate")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		purego.RegisterLibFunc(&cfRunLoopGetCurrent, cf, "CFRunLoopGetCurrent")
		purego.RegisterLibFunc(&cfRunLoopRunInMode, cf, "CFRunLoopRunInMode")
		purego.RegisterLibFunc(&cfRunLoopStop, cf, "CFRunLoopStop")
		purego.RegisterLibFunc(&cfUUIDCreateString, cf, "CFUUIDCreateString")
		purego.RegisterLibFunc(&cfStringGetCString, cf, "CFStringGetCString")

		purego.RegisterLibFunc(&fsStreamCreate, cs, "FSEventStreamCreate")
		purego.RegisterLibFunc(&fsStreamSchedule, cs, "FSEventStreamScheduleWithRunLoop")
		purego.RegisterLibFunc(&fsStreamStart, cs, "FSEventStreamStart")
		purego.RegisterLibFunc(&fsStreamStop, cs, "FSEventStreamStop")
		purego.RegisterLibFunc(&fsStreamInvalidate, cs, "FSEventStreamInvalidate")
		purego.RegisterLibFunc(&fsStreamRelease, cs, "FSEventStreamRelease")
		purego.RegisterLibFunc(&fsStreamFlushSync, cs, "FSEventStreamFlushSync")
		purego.RegisterLibFunc(&fsGetCurrentEventID, cs, "FSEventsGetCurrentEventId")
		purego.RegisterLibFunc(&fsCopyUUIDForDevice, cs, "FSEventsCopyUUIDForDevice")

		if cfTypeArrayCallBacks, err = purego.Dlsym(cf, "kCFTypeArrayCallBacks"); err != nil {
			loadErr = fmt.Errorf("fsevents: kCFTypeArrayCallBacks: %w", err)
			return
		}
		modeSym, err := purego.Dlsym(cf, "kCFRunLoopDefaultMode")
		if err != nil {
			loadErr = fmt.Errorf("fsevents: kCFRunLoopDefaultMode: %w", err)
			return
		}
		cfRunLoopDefaultMode = *(*uintptr)(ptrFromAddr(modeSym))
		// NewCallback slots are never freed, so create exactly one.
		callbackPtr = purego.NewCallback(streamCallback)
	})
	return loadErr
}

// streamCallback is the FSEventStreamCallback. It runs on the run-loop thread
// that called CFRunLoopRunInMode, which is a Go-created, locked thread.
func streamCallback(_ uintptr, info uintptr, numEvents uintptr, paths uintptr, flags uintptr, ids uintptr) {
	registryMu.Lock()
	s := registry[info]
	registryMu.Unlock()
	if s == nil {
		return
	}
	evs := decodeEvents(int(numEvents), ptrFromAddr(paths), ptrFromAddr(flags), ptrFromAddr(ids))
	s.cfg.dispatch(evs)
}

// Stream watches directory trees via FSEvents with FileEvents|NoDefer|WatchRoot.
type Stream struct {
	cfg Config
	id  uintptr

	mu      sync.Mutex
	started bool
	stopped bool
	handle  uintptr // FSEventStreamRef, valid while running
	rl      uintptr // run loop ref, valid while running
	stopReq atomic.Bool
	done    chan struct{}
}

// NewStream validates cfg and loads CoreServices; it does not start watching.
func NewStream(cfg Config) (*Stream, error) {
	if len(cfg.Paths) == 0 {
		return nil, errors.New("fsevents: no paths")
	}
	if err := load(); err != nil {
		return nil, err
	}
	return &Stream{cfg: cfg, id: nextID.Add(1), done: make(chan struct{})}, nil
}

// Start begins delivery and returns once the stream is running.
func (s *Stream) Start() error {
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return errors.New("fsevents: stream already started or stopped")
	}
	s.started = true
	s.mu.Unlock()

	registryMu.Lock()
	registry[s.id] = s
	registryMu.Unlock()

	ready := make(chan error, 1)
	go s.run(ready)
	if err := <-ready; err != nil {
		s.unregister()
		return err
	}
	return nil
}

func (s *Stream) unregister() {
	registryMu.Lock()
	delete(registry, s.id)
	registryMu.Unlock()
}

// run owns the run-loop thread for the stream's whole life.
func (s *Stream) run(ready chan<- error) {
	defer close(s.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	arr, err := s.makePathArray()
	if err != nil {
		ready <- err
		return
	}
	defer cfRelease(arr)

	ctx := streamContext{info: s.id}
	h := fsStreamCreate(0, callbackPtr, &ctx, arr, s.cfg.SinceWhen, s.cfg.Latency.Seconds(),
		createFlagFileEvents|createFlagNoDefer|createFlagWatchRoot)
	if h == 0 {
		ready <- errors.New("fsevents: FSEventStreamCreate failed")
		return
	}
	rl := cfRunLoopGetCurrent()
	fsStreamSchedule(h, rl, cfRunLoopDefaultMode)
	if fsStreamStart(h) == 0 {
		fsStreamInvalidate(h)
		fsStreamRelease(h)
		ready <- errors.New("fsevents: FSEventStreamStart failed")
		return
	}
	s.mu.Lock()
	s.handle, s.rl = h, rl
	s.mu.Unlock()
	ready <- nil

	for !s.stopReq.Load() {
		cfRunLoopRunInMode(cfRunLoopDefaultMode, runLoopSlice, 0)
	}

	s.mu.Lock()
	s.handle, s.rl = 0, 0
	s.mu.Unlock()
	fsStreamStop(h)
	fsStreamInvalidate(h)
	fsStreamRelease(h)
}

func (s *Stream) makePathArray() (uintptr, error) {
	strs := make([]uintptr, 0, len(s.cfg.Paths))
	defer func() {
		for _, r := range strs {
			cfRelease(r)
		}
	}()
	for _, p := range s.cfg.Paths {
		b, err := syscall.BytePtrFromString(p)
		if err != nil {
			return 0, fmt.Errorf("fsevents: path %q: %w", p, err)
		}
		r := cfStringCreateWithCString(0, b, cfStringEncodingUTF8)
		if r == 0 {
			return 0, fmt.Errorf("fsevents: CFString for %q", p)
		}
		strs = append(strs, r)
	}
	// CFArrayCreate retains each element, so releasing strs on return is correct.
	arr := cfArrayCreate(0, &strs[0], len(strs), cfTypeArrayCallBacks)
	if arr == 0 {
		return 0, errors.New("fsevents: CFArrayCreate failed")
	}
	return arr, nil
}

// Stop ends the stream and blocks until the run-loop goroutine has released
// every CF object. Idempotent; a never-started stream is a no-op.
func (s *Stream) Stop() {
	s.mu.Lock()
	if s.stopped || !s.started {
		s.stopped = true
		s.mu.Unlock()
		return
	}
	s.stopped = true
	rl := s.rl
	s.mu.Unlock()

	s.stopReq.Store(true)
	if rl != 0 {
		cfRunLoopStop(rl) // wakes the slice early; the flag covers the not-yet-running case
	}
	<-s.done
	s.unregister()
}

// FlushSync blocks until all events up to now have been delivered, giving
// read-your-writes before a query. No-op if the stream isn't running.
func (s *Stream) FlushSync() {
	s.mu.Lock()
	h := s.handle
	s.mu.Unlock()
	if h != 0 {
		fsStreamFlushSync(h)
	}
}

// CurrentEventID wraps FSEventsGetCurrentEventId; 0 if CoreServices can't load.
func CurrentEventID() EventID {
	if load() != nil {
		return 0
	}
	return fsGetCurrentEventID()
}

// VolumeUUID returns the UUID of the volume containing path, or "" when it is
// unavailable (e.g. network volumes).
func VolumeUUID(path string) string {
	if load() != nil {
		return ""
	}
	var st syscall.Stat_t
	if syscall.Stat(path, &st) != nil {
		return ""
	}
	uuid := fsCopyUUIDForDevice(st.Dev)
	if uuid == 0 {
		return ""
	}
	defer cfRelease(uuid)
	str := cfUUIDCreateString(0, uuid)
	if str == 0 {
		return ""
	}
	defer cfRelease(str)
	buf := make([]byte, 64)
	if cfStringGetCString(str, &buf[0], len(buf), cfStringEncodingUTF8) == 0 {
		return ""
	}
	for i, c := range buf {
		if c == 0 {
			return string(buf[:i])
		}
	}
	return ""
}
