//go:build darwin && cgo

package procinfo

/*
#include <libproc.h>
#include <string.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// openFilesCgo returns paths of open regular files for the given PID using
// the native macOS proc_pidinfo(PROC_PIDLISTFDS) + proc_pidfdinfo(PROC_PIDFDVNODEPATHINFO)
// syscalls. This avoids shelling out to lsof.
//
// Returns a non-nil error only for unexpected syscall failures. Returns an
// empty slice (no error) when the process has exited or access is denied —
// matching the graceful-degradation contract of ProcessInspector.OpenFiles.
func openFilesCgo(pid int32) ([]string, error) {
	cpid := C.int(pid)

	// First call with nil buffer: returns the total byte size needed for the FD list.
	sz := C.proc_pidinfo(cpid, C.PROC_PIDLISTFDS, 0, nil, 0)
	if sz < 0 {
		return nil, fmt.Errorf("proc_pidinfo LISTFDS size failed for pid %d", pid)
	}
	if sz == 0 {
		return []string{}, nil
	}

	// Allocate and populate the FD list.
	count := int(sz) / int(C.sizeof_struct_proc_fdinfo)
	fds := make([]C.struct_proc_fdinfo, count)
	actual := C.proc_pidinfo(cpid, C.PROC_PIDLISTFDS, 0,
		unsafe.Pointer(&fds[0]), sz)
	if actual <= 0 {
		return nil, fmt.Errorf("proc_pidinfo LISTFDS data failed for pid %d", pid)
	}
	actualCount := int(actual) / int(C.sizeof_struct_proc_fdinfo)

	var paths []string
	for i := 0; i < actualCount; i++ {
		// Skip non-vnode FDs (sockets, pipes, etc.).
		if fds[i].proc_fdtype != C.uint(C.PROX_FDTYPE_VNODE) {
			continue
		}

		var vnodeInfo C.struct_vnode_fdinfowithpath
		r := C.proc_pidfdinfo(cpid, fds[i].proc_fd, C.PROC_PIDFDVNODEPATHINFO,
			unsafe.Pointer(&vnodeInfo), C.int(C.sizeof_struct_vnode_fdinfowithpath))
		if r <= 0 {
			// FD may have been closed between LISTFDS and this call — skip it.
			continue
		}

		path := C.GoString((*C.char)(unsafe.Pointer(&vnodeInfo.pvip.vip_path[0])))
		if path != "" {
			paths = append(paths, path)
		}
	}

	return paths, nil
}
