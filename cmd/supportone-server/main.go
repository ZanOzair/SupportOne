// Command supportone-server is the optional, self-hostable fleet server.
//
// It receives reports that agents chose to send and shows a technician what
// those machines reported. It cannot ask a machine anything, run anything on
// one, or make one report again: the arrow points one way, and there is no
// code here that points it the other.
//
// The agent works fully without this. Nothing in the diagnostic, explaining or
// repairing path needs a server, and a person who never runs one loses nothing
// but the fleet view.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/fleet"
	"github.com/ZanOzair/SupportOne/internal/provenance"
)

// Build metadata, set via -ldflags at release time.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// TokenEnv is where the server reads its token. It is an environment variable
// and never a flag: a flag is visible in the process list to every other user
// on the machine.
const TokenEnv = "SUPPORTONE_FLEET_TOKEN"

type options struct {
	addr    string
	dataDir string
	lang    string
	showVer bool
}

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "supportone-server: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	opts, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}

	if opts.showVer {
		fmt.Fprintf(stdout, "%s\n", provenance.Current("supportone-server", version, commit, buildDate).Line())
		return nil
	}

	token := strings.TrimSpace(getenv(TokenEnv))
	if token == "" {
		// Refusing to start is the only safe default. A fleet server with no
		// token is a list of other people's machines, served to anyone.
		return fmt.Errorf("set %s to a secret of at least %d characters; the server will not serve a fleet without one",
			TokenEnv, fleet.MinTokenLength)
	}

	store, err := fleet.OpenStore(opts.dataDir)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	server, err := fleet.New(fleet.Config{
		Store:  store,
		Token:  token,
		Lang:   opts.lang,
		Logger: logger,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Fprintf(stdout, "SupportOne fleet server %s listening on %s\n", version, opts.addr)
	fmt.Fprintf(stdout, "Data directory: %s\n", store.Dir())
	fmt.Fprintln(stdout, "The dashboard asks for a password: it is the token, with any username.")
	fmt.Fprintln(stdout, "Put this behind a reverse proxy that terminates TLS before exposing it.")

	return server.Serve(ctx, opts.addr)
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var opts options

	fs := flag.NewFlagSet("supportone-server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.addr, "addr", ":8080", "address to listen on")
	fs.StringVar(&opts.dataDir, "data", "/var/lib/supportone", "directory to keep machine records in")
	fs.StringVar(&opts.lang, "lang", "", "dashboard language, e.g. en or ms (default: system language)")
	fs.BoolVar(&opts.showVer, "version", false, "print build information and exit")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return opts, nil
}
