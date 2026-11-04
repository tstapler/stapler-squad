//go:build darwin

package tcc

import (
	"os"

	"golang.org/x/sys/unix"
)

// Classify reports whether path is TCC-protected using the real filesystem.
func Classify(path string) Result {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return Classifier{Home: home, FS: statfsInfo, Device: statDevice}.Classify(path)
}

func statfsInfo(path string) (FSInfo, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return FSInfo{}, err
	}
	return FSInfo{Type: unix.ByteSliceToString(st.Fstypename[:]), MountOn: unix.ByteSliceToString(st.Mntonname[:])}, nil
}

func statDevice(path string) (uint64, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Dev), nil //nolint:gosec // device IDs are opaque; only equality is used
}
