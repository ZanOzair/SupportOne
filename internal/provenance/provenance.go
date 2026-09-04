// Package provenance says what a build is, and what saying so is worth.
//
// The distinction matters more than it looks. A program reporting its own
// version is reporting a string that was compiled into it, and a changed copy
// reports whatever it was changed to report. That is not a flaw to work
// around — no binary can vouch for itself — but it is a thing to say out loud,
// beside the version, rather than leaving a reader to assume the number is
// evidence.
//
// So this package prints two things: what the build claims, and where to go to
// check the claim.
package provenance

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// DevVersion is what an unstamped build reports. A build carrying it was made
// by someone running `go build`, not by the release pipeline.
const DevVersion = "dev"

// Build is what a binary knows about how it was made.
type Build struct {
	// Program is the command's name, e.g. "supportone-agent".
	Program string

	// Version, Commit and Date are stamped at release time. They are what the
	// build says about itself.
	Version string
	Commit  string
	Date    string

	// GoVersion is read from the runtime rather than stamped, so it is
	// accurate even for a build nobody stamped.
	GoVersion string

	// Modified reports that the source tree had uncommitted changes when this
	// was built.
	Modified bool

	// Released reports whether this looks like a build from the release
	// pipeline rather than someone's working copy.
	Released bool
}

// Current describes the running binary.
//
// version, commit and date come from the caller's -ldflags variables; the rest
// is read from the build itself.
func Current(program, version, commit, date string) Build {
	b := Build{
		Program:   program,
		Version:   strings.TrimSpace(version),
		Commit:    strings.TrimSpace(commit),
		Date:      strings.TrimSpace(date),
		GoVersion: runtime.Version(),
	}

	// Two sources, because neither covers every build. An ordinary `go build`
	// carries Go's VCS stamp; a release build does not, because the stamp is
	// read from the working tree as each binary is linked and the release
	// script writes into that tree as it goes. What a release carries instead
	// is the version, and `git describe --dirty` puts the answer in it.
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.modified" {
				b.Modified = setting.Value == "true"
			}
		}
	}
	if strings.HasSuffix(b.Version, "-dirty") {
		b.Modified = true
	}

	// A release is stamped with a real version, from a clean tree. Any of
	// those missing means this came from somewhere else, and the honest
	// answer to "is this a release build" is no.
	b.Released = b.Version != "" && b.Version != DevVersion && !b.Modified
	return b
}

// Line is the one-line summary: what this build says it is.
func (b Build) Line() string {
	version := b.Version
	if version == "" {
		version = DevVersion
	}

	commit := b.Commit
	if commit == "" {
		commit = "unknown"
	}
	if b.Modified {
		// A clean commit hash on a tree that was not clean would be a lie by
		// omission: the source built is not the source at that commit.
		commit += ", with uncommitted changes"
	}

	date := b.Date
	if date == "" {
		date = "unknown"
	}
	return fmt.Sprintf("%s %s (commit %s, built %s, %s)", b.Program, version, commit, date, b.GoVersion)
}

// Message keys this package resolves through internal/i18n.
const (
	// KeySelfReported is the sentence that keeps the version honest.
	KeySelfReported = "build.self_reported"

	// KeyUnsigned is for a build nobody published.
	KeyUnsigned = "build.unsigned"

	// KeyModified is for a build made from a tree with uncommitted changes.
	KeyModified = "build.modified"

	// KeyHowToVerify points at the check that is actually worth something.
	KeyHowToVerify = "build.how_to_verify"
)

// Notes returns the message keys that should be printed under Line, in order.
//
// A released build gets the caveat and the instructions; anything else is told
// what it is first, because "this is not a published build" is the more useful
// fact about it.
func (b Build) Notes() []string {
	notes := make([]string, 0, 3)

	if !b.Released {
		if b.Modified {
			notes = append(notes, KeyModified)
		} else {
			notes = append(notes, KeyUnsigned)
		}
	}

	return append(notes, KeySelfReported, KeyHowToVerify)
}
