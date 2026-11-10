//go:build windows

package sessionbundle

func diskFreeBytes(_ string) (uint64, error) {
	return ^uint64(0), nil
}
