// Package driver stands in for the real session/driver package at its import path, so analyzers keyed on that path fire on seeds.
package driver

func StartSessionDriver(inst any, allowedPath string) {}
