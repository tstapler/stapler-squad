//go:build windows

package services

func diskFreeBytes(_ string) (uint64, error) {
	return ^uint64(0), nil
}
