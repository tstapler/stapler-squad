//go:build !unix

package gogit

import "os"

// fileIDs reports nothing on platforms without Unix ownership, so every locate falls back to
// the CLI there.
func fileIDs(os.FileInfo) (dev uint64, uid uint32, ok bool) { return 0, 0, false }

func effectiveUID() int { return -1 }
