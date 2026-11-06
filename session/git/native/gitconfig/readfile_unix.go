//go:build unix

package gitconfig

import "syscall"

// openFlags adds O_NONBLOCK so opening a path swapped to a FIFO between a check and the open
// cannot block; the regular-file check is then made on the open descriptor.
const openFlags = syscall.O_NONBLOCK
