//go:build !unix

package unfinished

// statFileStamp reports ok=false: without ctime and inode the memo cannot rule
// out a timestamp-preserving rewrite, so it is disabled rather than trusted.
func statFileStamp(string) (fileStamp, bool) { return fileStamp{}, false }
