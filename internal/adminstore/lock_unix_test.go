//go:build unix

package adminstore

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A second process on the same state file is a second wallet that overwrites
// the first: it must refuse to open, while the same process may reopen freely.
func TestASecondProcessCannotOpenTheStore(t *testing.T) {
	if path := os.Getenv("ADMINSTORE_LOCK_CHILD"); path != "" {
		lockWait = 300 * time.Millisecond
		_, err := Open(path)
		if errors.Is(err, ErrStoreLocked) {
			os.Exit(3)
		}
		os.Exit(0)
	}

	path := filepath.Join(t.TempDir(), "console.json")
	if _, err := Open(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err != nil {
		t.Fatalf("the same process could not reopen its own store: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run", "^TestASecondProcessCannotOpenTheStore$")
	cmd.Env = append(os.Environ(), "ADMINSTORE_LOCK_CHILD="+path)
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("a second process opened a locked store (err %v)", err)
	}
}
