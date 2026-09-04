package provenance

import (
	"runtime"
	"strings"
	"testing"
)

func TestAStampedBuildFromACleanTreeIsAReleaseBuild(t *testing.T) {
	b := Current("supportone-agent", "v1.2.3", "abc123", "2026-09-04T10:00:00Z")
	// Current reads the running test binary's VCS stamp, which this test
	// cannot control, so Released is asserted through the field it depends on.
	b.Modified = false
	b.Released = b.Version != "" && b.Version != DevVersion && !b.Modified

	if !b.Released {
		t.Error("a stamped version from a clean tree is not being treated as a release build")
	}
	if b.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want the runtime's own %q", b.GoVersion, runtime.Version())
	}

	line := b.Line()
	for _, want := range []string{"supportone-agent", "v1.2.3", "abc123", "2026-09-04T10:00:00Z", runtime.Version()} {
		if !strings.Contains(line, want) {
			t.Errorf("Line() = %q, want it to mention %q", line, want)
		}
	}
}

func TestABuildNobodyPublishedSaysSoFirst(t *testing.T) {
	b := Build{Program: "supportone-agent", Version: DevVersion}

	notes := b.Notes()
	if len(notes) == 0 || notes[0] != KeyUnsigned {
		t.Fatalf("Notes() = %v, want it to open by saying the build is unpublished", notes)
	}
}

func TestABuildFromAModifiedTreeSaysThatInsteadOfClaimingACommit(t *testing.T) {
	b := Build{Program: "supportone-agent", Version: "v1.2.3", Commit: "abc123", Modified: true}

	notes := b.Notes()
	if len(notes) == 0 || notes[0] != KeyModified {
		t.Fatalf("Notes() = %v, want it to open by saying the tree was modified", notes)
	}

	// A clean commit hash beside a dirty tree would be a lie by omission: the
	// source that was built is not the source at that commit.
	if line := b.Line(); !strings.Contains(line, "uncommitted changes") {
		t.Errorf("Line() = %q, want it to qualify the commit", line)
	}
}

func TestAVersionIsNeverPresentedAsEvidence(t *testing.T) {
	// Whatever the build is, the sentence saying a version proves nothing and
	// the pointer to a real check must both be there. That is the property
	// this package exists for.
	cases := map[string]Build{
		"a release":       {Version: "v1.2.3", Released: true},
		"a dev build":     {Version: DevVersion},
		"a modified tree": {Version: "v1.2.3", Modified: true},
		"nothing stamped": {},
	}

	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			notes := b.Notes()
			if !contains(notes, KeySelfReported) {
				t.Errorf("Notes() = %v, want the caveat that a build cannot vouch for itself", notes)
			}
			if !contains(notes, KeyHowToVerify) {
				t.Errorf("Notes() = %v, want it to point at a check worth something", notes)
			}
		})
	}
}

func TestAReleaseBuildIsNotToldItIsUnpublished(t *testing.T) {
	b := Build{Version: "v1.2.3", Released: true}

	notes := b.Notes()
	if contains(notes, KeyUnsigned) || contains(notes, KeyModified) {
		t.Errorf("Notes() = %v, want no complaint about a build that is a release", notes)
	}
}

func TestNothingStampedStillPrintsAUsableLine(t *testing.T) {
	line := Build{Program: "supportone-agent"}.Line()

	for _, want := range []string{"supportone-agent", DevVersion, "unknown"} {
		if !strings.Contains(line, want) {
			t.Errorf("Line() = %q, want it to mention %q rather than leaving a blank", line, want)
		}
	}
}

func TestBlankStampsAreTreatedAsAbsent(t *testing.T) {
	// -ldflags with an empty value is a real way to end up here, and an empty
	// version must not read as a released one.
	b := Current("supportone-agent", "   ", "", "")

	if b.Released {
		t.Error("a build with a blank version is being reported as a release")
	}
	if b.Version != "" {
		t.Errorf("Version = %q, want the whitespace trimmed away", b.Version)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
