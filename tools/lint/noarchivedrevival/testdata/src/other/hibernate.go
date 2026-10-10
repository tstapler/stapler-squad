// Package other holds a file outside the automated-lifecycle set; never flagged.
package other

type Instance struct{}

func (i *Instance) Start(first bool) error { return nil }

func explicitResume(i *Instance) {
	_ = i.Start(false)
}
