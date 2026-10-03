//go:build linux

package tmux

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// findSocketOwnerPID finds which process holds sockPath's unix socket open,
// via /proc -- pure Go, no cgo, no shelling out. /proc/net/unix lists every
// unix domain socket in the kernel with its inode and (for a bound socket)
// its filesystem path; /proc/<pid>/fd/<fd> is a symlink reading
// "socket:[<inode>]" for a socket file descriptor, so matching sockPath's
// inode against every process's fd symlinks finds the owner.
func findSocketOwnerPID(sockPath string) (int32, bool) {
	inode, ok := unixSocketInode(sockPath)
	if !ok {
		return 0, false
	}
	target := "socket:[" + inode + "]"

	procEntries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, false
	}
	for _, entry := range procEntries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // not a PID directory (e.g. /proc/self, /proc/net)
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // process exited, or no permission to read its fds
		}
		for _, fd := range fds {
			if link, err := os.Readlink(filepath.Join(fdDir, fd.Name())); err == nil && link == target {
				return int32(pid), true
			}
		}
	}
	return 0, false
}

// unixSocketInode looks up sockPath's inode number from /proc/net/unix, the
// kernel's own table of every unix domain socket. Column layout (see
// Documentation/networking/af_unix.rst): Num RefCount Protocol Flags Type
// St Inode [Path] -- Path is present only for a bound (not just connected)
// socket, which is exactly the tmux-server-listening case this exists for.
func unixSocketInode(sockPath string) (string, bool) {
	f, err := os.Open("/proc/net/unix")
	if err != nil {
		return "", false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // header line
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 {
			continue
		}
		if fields[7] == sockPath {
			return fields[6], true
		}
	}
	return "", false
}
