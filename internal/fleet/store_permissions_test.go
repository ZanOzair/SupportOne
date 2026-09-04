//go:build !windows

package fleet

import (
	"os"
	"path/filepath"
	"testing"
)

// The store creates records 0600 so that one account's fleet reports are not
// readable by every account on the host.
//
// The assertion lives here rather than beside the store's other tests because
// it is about POSIX permission bits, and Windows does not have them: Go's file
// mode there reflects the read-only attribute and nothing else, so a 0600 file
// reports 0666 and the check would be asserting something the platform never
// promised. The protection on Windows is the directory's ACL, which is the
// operator's to set and not this package's to claim.
func TestRecordsAreNotWorldReadable(t *testing.T) {
	s := store(t)

	if _, err := s.Put(report("Reception PC"), now); err != nil {
		t.Fatalf("Put: %v", err)
	}

	info, err := os.Stat(filepath.Join(s.Dir(), MachineID("Reception PC")+".json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("permissions = %v, want %v", got, os.FileMode(0o600))
	}
}
