//go:build !darwin

package tcc

// Classify always reports not-protected: TCC exists only on macOS.
func Classify(string) Result { return Result{} }
