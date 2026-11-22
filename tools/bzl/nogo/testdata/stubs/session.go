// Package session stands in for the real session package at its import path, so analyzers keyed on that path fire on seeds.
package session

import "context"

type Instance struct{}

type Storage struct{}

func (s *Storage) UpdateItemSessionEnded(ctx context.Context, itemID string, code int) error {
	return nil
}
