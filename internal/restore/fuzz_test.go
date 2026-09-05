package restore

import "testing"

// These two read the output of the commands that make a restore point. Getting
// a wrong answer here means the record of a restore point is wrong, which is
// worse than having none: a fix would proceed believing it can be undone.

func FuzzSequenceNumber(f *testing.F) {
	f.Add([]byte("SequenceNumber : 42\n"))
	f.Add([]byte("SequenceNumber :"))
	f.Add([]byte("\x00\xff"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = sequenceNumber(data)
		_ = snapshotName(data)
	})
}
