package fixes

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Quarantine is how a fix deletes something without deleting it.
//
// A fix that clears files moves them into a quarantine directory instead of
// removing them. That is what makes "clear the temporary files" reversible:
// Restore puts every file back where it came from, and the user gets their
// machine as it was. Discard is a separate, later decision — never part of
// applying the fix.
type Quarantine struct {
	// Dir is where the moved files live until they are restored or discarded.
	Dir string

	// moved records where each quarantined file came from, so Restore can put
	// it back exactly there.
	moved []movedFile
}

type movedFile struct {
	// From is the original path.
	From string

	// To is the path inside the quarantine directory.
	To string

	// Mode is the file's permissions, so restoring does not silently widen
	// them.
	Mode os.FileMode
}

// NewQuarantine creates a quarantine directory under parent, named for the fix
// and the moment it ran so two runs never collide.
func NewQuarantine(parent, fixID string) (*Quarantine, error) {
	name := fmt.Sprintf("%s-%s", strings.ReplaceAll(fixID, ".", "-"), time.Now().UTC().Format("20060102-150405"))
	dir := filepath.Join(parent, name)

	// 0700: the quarantine holds the user's own files, and nobody else's
	// business.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("fixes: create quarantine directory: %w", err)
	}
	return &Quarantine{Dir: dir}, nil
}

// Take moves one file or directory into quarantine.
//
// A path it cannot move is reported rather than skipped: a fix that silently
// left files behind would report success for work it did not do.
func (q *Quarantine) Take(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("fixes: quarantine %s: %w", path, err)
	}

	// The destination keeps the original name, with a counter when two files
	// from different directories share one.
	target := q.destination(filepath.Base(path))
	if err := os.Rename(path, target); err != nil {
		return fmt.Errorf("fixes: move %s to quarantine: %w", path, err)
	}

	q.moved = append(q.moved, movedFile{From: path, To: target, Mode: info.Mode().Perm()})
	return nil
}

// destination picks a free name inside the quarantine directory.
func (q *Quarantine) destination(name string) string {
	target := filepath.Join(q.Dir, name)
	for i := 1; ; i++ {
		if _, err := os.Lstat(target); os.IsNotExist(err) {
			return target
		}
		target = filepath.Join(q.Dir, fmt.Sprintf("%s.%d", name, i))
	}
}

// Count returns how many entries are held.
func (q *Quarantine) Count() int { return len(q.moved) }

// Paths returns the original locations of everything held, sorted.
func (q *Quarantine) Paths() []string {
	out := make([]string, 0, len(q.moved))
	for _, m := range q.moved {
		out = append(out, m.From)
	}
	sort.Strings(out)
	return out
}

// Restore puts every quarantined entry back where it came from.
//
// It restores what it can and reports what it could not: a partial restore the
// user is told about beats a silent one. The quarantine directory is removed
// only when it is empty, so nothing is ever lost to a failed rollback.
func (q *Quarantine) Restore() error {
	var failures []string

	// Restore in reverse order, so a directory taken before its contents goes
	// back first.
	for i := len(q.moved) - 1; i >= 0; i-- {
		m := q.moved[i]

		if err := os.MkdirAll(filepath.Dir(m.From), 0o700); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", m.From, err))
			continue
		}
		if err := os.Rename(m.To, m.From); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", m.From, err))
			continue
		}
		if err := os.Chmod(m.From, m.Mode); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", m.From, err))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("fixes: %d of %d quarantined files could not be restored (they are still in %s): %s",
			len(failures), len(q.moved), q.Dir, strings.Join(failures, "; "))
	}

	q.moved = nil
	// Remove returns an error for a non-empty directory, which is the right
	// outcome: something is still in there and should not be forgotten.
	_ = os.Remove(q.Dir)
	return nil
}
