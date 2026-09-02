// Command supportone-server is the optional, self-hostable fleet server.
//
// It is not implemented yet: the fleet server arrives in Phase 4. The command
// exists so the build, packaging and CI pipeline cover it from the start, and
// it refuses to start rather than pretending to serve anything.
package main

import (
	"fmt"
	"os"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("supportone-server %s (commit %s, built %s)\n", version, commit, buildDate)
		return
	}

	fmt.Fprintln(os.Stderr, "supportone-server: the fleet server is not implemented yet (planned for Phase 4).")
	fmt.Fprintln(os.Stderr, "The agent works fully offline without it: run supportone-agent.")
	os.Exit(1)
}
