//go:build !unix

package adminstore

import "errors"

// ErrStoreLocked is never returned here: without flock there is no lock to
// contend for, and the gateway ships for Linux only. See lock_unix.go.
var ErrStoreLocked = errors.New("adminstore: the state file is in use by another running instance")

func lockStateFile(string) error { return nil }
