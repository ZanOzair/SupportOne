//go:build !windows

package schedule

import (
	"os"
	"path/filepath"
	"testing"
)

// A monthly report is written 0600: it is about someone's own machine and
// belongs to them, not to every account on the host.
//
// The assertion lives here rather than beside the other tests because it is
// about POSIX permission bits, and Windows does not have them: Go's file mode
// there reflects the read-only attribute and nothing else, so a 0600 file
// reports 0666 and this would be asserting something the platform never
// promised. On Windows the protection is the folder's ACL, which is whoever
// chose the folder's to set and not this package's to claim.
func TestReportsAreNotWorldReadable(t *testing.T) {
	dir := t.TempDir()

	got, err := Write(snapshot(), Options{Dir: dir, Bundle: bundle(t)})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	for _, path := range []string{got.HTML, got.JSON} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s has permissions %v, want %v", filepath.Base(path), perm, os.FileMode(0o600))
		}
	}
}
