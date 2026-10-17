//go:build unix

package adminstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// ErrStoreLocked means another process holds the state file.
//
// The store keeps the whole state in memory and writes it back whole, with
// usage debits batched between writes. Two processes on one file therefore do
// not share a wallet, they each keep one: a payment credited by one is
// overwritten by the other's next save, or credited twice, once in each. That
// is what a redeploy that left the old container running beside the new one
// did. One writer is the only safe number, so the second one refuses to start.
var ErrStoreLocked = errors.New("adminstore: the state file is in use by another running instance")

// held is this process's lock per state file. A process may open the same file
// more than once (the tests reopen to prove persistence); only a second process
// is the danger, and an flock taken twice by one process would refuse itself.
var (
	heldMu sync.Mutex
	held   = map[string]*os.File{}
)

// lockWait is how long a starting instance waits for the previous one to let
// go: long enough for a redeploy's graceful shutdown, short enough that a
// stale container left running is reported rather than waited on forever.
var lockWait = 30 * time.Second

func lockStateFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	heldMu.Lock()
	defer heldMu.Unlock()
	if _, ok := held[abs]; ok {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return fmt.Errorf("adminstore: create dir: %w", err)
	}
	f, err := os.OpenFile(abs+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("adminstore: open lock: %w", err)
	}
	deadline := time.Now().Add(lockWait)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			held[abs] = f
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			f.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) {
				return ErrStoreLocked
			}
			return fmt.Errorf("adminstore: lock: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
