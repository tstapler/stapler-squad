//go:build darwin && cgo

package tmux

/*
#include <libproc.h>
#include <sys/proc_info.h>
*/
import "C"
import (
	"unsafe"

	"github.com/shirou/gopsutil/v4/process"
)

// findSocketOwnerPID finds which process holds sockPath's unix domain socket
// open, using macOS's native proc_pidinfo(PROC_PIDLISTFDS) +
// proc_pidfdinfo(PROC_PIDFDSOCKETINFO) syscalls -- no shelling out to lsof.
// Mirrors session/procinfo's openfiles_darwin.go pattern (which does the
// same enumerate-FDs-then-proc_pidfdinfo dance for vnode/regular-file FDs);
// this does it for AF_UNIX socket FDs instead, which openfiles_darwin.go
// explicitly skips ("Skip non-vnode FDs (sockets, pipes, etc.)").
func findSocketOwnerPID(sockPath string) (int32, bool) {
	procs, err := process.Processes()
	if err != nil {
		return 0, false
	}
	for _, p := range procs {
		if pidHasUnixSocketBoundTo(p.Pid, sockPath) {
			return p.Pid, true
		}
	}
	return 0, false
}

// pidHasUnixSocketBoundTo reports whether pid has an open file descriptor
// for an AF_UNIX socket bound to sockPath. Degrades to false (never an
// error) for a dead process, permission denial, or a race where an FD
// closes between the list and detail calls -- matching
// session/procinfo.ProcessInspector.OpenFiles' own graceful-degradation
// contract.
func pidHasUnixSocketBoundTo(pid int32, sockPath string) bool {
	cpid := C.int(pid)

	sz := C.proc_pidinfo(cpid, C.PROC_PIDLISTFDS, 0, nil, 0)
	if sz <= 0 {
		return false
	}
	count := int(sz) / int(C.sizeof_struct_proc_fdinfo)
	fds := make([]C.struct_proc_fdinfo, count)
	actual := C.proc_pidinfo(cpid, C.PROC_PIDLISTFDS, 0, unsafe.Pointer(&fds[0]), sz)
	if actual <= 0 {
		return false
	}
	actualCount := int(actual) / int(C.sizeof_struct_proc_fdinfo)

	for i := 0; i < actualCount; i++ {
		if fds[i].proc_fdtype != C.uint(C.PROX_FDTYPE_SOCKET) {
			continue
		}

		var si C.struct_socket_fdinfo
		r := C.proc_pidfdinfo(cpid, fds[i].proc_fd, C.PROC_PIDFDSOCKETINFO,
			unsafe.Pointer(&si), C.int(C.sizeof_struct_socket_fdinfo))
		if r <= 0 {
			continue // FD closed between LISTFDS and this call
		}
		if si.psi.soi_kind != C.SOCKINFO_UN {
			continue // not an AF_UNIX socket
		}

		// soi_proto and unsi_addr are both anonymous C unions; cgo can't
		// preserve a union's named member types, so it represents each as a
		// raw byte array. Recovering the concrete struct requires an
		// unsafe.Pointer cast at each union boundary -- confirmed empirically
		// against this exact SDK (Xcode CommandLineTools), since a wrong
		// field offset here would silently read garbage rather than fail to
		// compile.
		unInfo := (*C.struct_un_sockinfo)(unsafe.Pointer(&si.psi.soi_proto[0]))
		sunAddr := (*C.struct_sockaddr_un)(unsafe.Pointer(&unInfo.unsi_addr))
		path := C.GoString((*C.char)(unsafe.Pointer(&sunAddr.sun_path[0])))
		if path == sockPath {
			return true
		}
	}
	return false
}
