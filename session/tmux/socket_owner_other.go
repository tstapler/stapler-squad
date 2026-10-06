//go:build !(darwin && cgo) && !linux

package tmux

// findSocketOwnerPID stub for darwin+!cgo or any non-darwin/linux OS.
// Deliberately returns "not found" rather than shelling out to lsof as a
// substitute -- the caller already treats that as "fall back to Binary()".
func findSocketOwnerPID(sockPath string) (int32, bool) {
	return 0, false
}
