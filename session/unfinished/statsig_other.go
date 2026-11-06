//go:build !unix

package unfinished

import "os"

// statPathSig is the portable fallback: no ctime and no inode, so a signature
// is weaker; the racy guard and the age bound still apply.
func statPathSig(path string) (sig statSig, ok bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return statSig{}, false
	}
	return statSig{mtimeNs: fi.ModTime().UnixNano(), size: fi.Size()}, true
}

func lstatPathSig(path string) (sig statSig, ok bool) {
	fi, err := os.Lstat(path)
	if err != nil {
		return statSig{}, false
	}
	return statSig{mtimeNs: fi.ModTime().UnixNano(), size: fi.Size()}, true
}
