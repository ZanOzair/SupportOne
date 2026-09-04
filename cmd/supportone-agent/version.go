package main

import (
	"fmt"
	"io"

	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/provenance"
)

// thisBuild describes the running binary from the values stamped at link time.
func thisBuild() provenance.Build {
	return provenance.Current("supportone-agent", version, commit, buildDate)
}

// writeProvenance prints what this build is, and says what that is worth.
//
// The second part is the point. A version number a program prints about itself
// is not evidence of anything — a tampered copy prints whatever it was changed
// to print — so the number comes with the sentence saying so and with where to
// go for a check that means something.
func writeProvenance(w io.Writer, bundle *i18n.Bundle) {
	build := thisBuild()

	fmt.Fprintf(w, "%s\n", build.Line())
	for _, key := range build.Notes() {
		fmt.Fprintf(w, "\n%s\n", bundle.T(key))
	}
}
