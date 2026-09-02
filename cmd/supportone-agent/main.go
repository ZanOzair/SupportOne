// Command supportone-agent runs SupportOne's read-only diagnostics on the
// machine it is started on.
//
// It makes no outbound network connection. Nothing leaves this computer unless
// the user explicitly sends it.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/ZanOzair/supportone/internal/checks"
	"github.com/ZanOzair/supportone/internal/consent"
	"github.com/ZanOzair/supportone/internal/i18n"
	"github.com/ZanOzair/supportone/internal/platform"
)

// Build metadata, set via -ldflags at release time. An unset version means an
// unsigned local build, and the agent says so rather than implying a release.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

type options struct {
	json       bool
	dryRun     bool
	lang       string
	listChecks bool
	auditPath  string
	timeout    time.Duration
	showVer    bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "supportone-agent: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}

	if opts.showVer {
		fmt.Fprintf(stdout, "supportone-agent %s (commit %s, built %s)\n", version, commit, buildDate)
		return nil
	}

	bundle, err := i18n.Load(opts.lang)
	if err != nil {
		return err
	}

	host := platform.CurrentHost()
	if !host.OS.Valid() {
		return fmt.Errorf("%s", bundle.T("agent.platform.unsupported"))
	}

	auditPath := opts.auditPath
	if auditPath == "" {
		if auditPath, err = consent.DefaultPath(); err != nil {
			return err
		}
	}
	audit, err := consent.Open(auditPath)
	if err != nil {
		return err
	}
	defer func() { _ = audit.Close() }()

	if opts.listChecks {
		return listChecks(stdout, bundle, host)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return snapshot(ctx, stdout, bundle, audit, host, opts)
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var opts options
	fs := flag.NewFlagSet("supportone-agent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.BoolVar(&opts.json, "json", false, "write the snapshot as JSON instead of text")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "report what would change without changing anything")
	fs.StringVar(&opts.lang, "lang", "", "language tag, e.g. en or ms (default: system language)")
	fs.BoolVar(&opts.listChecks, "list-checks", false, "list the checks available on this computer and exit")
	fs.StringVar(&opts.auditPath, "audit-log", "", "path to the audit log (default: per-user config directory)")
	fs.DurationVar(&opts.timeout, "timeout", checks.DefaultTimeout, "time limit for a single check")
	fs.BoolVar(&opts.showVer, "version", false, "print build information and exit")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return opts, nil
}

func listChecks(w io.Writer, bundle *i18n.Bundle, host platform.Host) error {
	available := checks.Default.ForPlatform(host.OS)
	if len(available) == 0 {
		fmt.Fprintln(w, bundle.T("agent.checks.none"))
		return nil
	}

	fmt.Fprintln(w, bundle.T("agent.checks.available", len(available), host.OS.Display()))
	for _, c := range available {
		if c.RequiresAdmin() {
			fmt.Fprintf(w, "  %s (%s)\n", c.ID(), bundle.T("agent.checks.requires_admin"))
			continue
		}
		fmt.Fprintf(w, "  %s\n", c.ID())
	}
	return nil
}

func snapshot(ctx context.Context, w io.Writer, bundle *i18n.Bundle, audit *consent.Log, host platform.Host, opts options) error {
	elevated, err := platform.IsElevated()
	if err != nil {
		return err
	}

	if err := audit.Append(consent.Event{
		Type: consent.EventAgentStart,
		Fields: map[string]string{
			"version":  version,
			"os":       string(host.OS),
			"arch":     host.Arch,
			"elevated": fmt.Sprint(elevated),
			"dry_run":  fmt.Sprint(opts.dryRun),
		},
	}); err != nil {
		return err
	}

	snap := checks.RunAll(ctx, checks.Default, host, elevated, opts.timeout)
	snap.AgentVersion = version

	for _, res := range snap.Results {
		if err := audit.Append(consent.Event{
			Type:    consent.EventCheckRun,
			Subject: res.CheckID,
			Fields: map[string]string{
				"severity": string(res.Severity),
				"duration": res.Duration.String(),
			},
		}); err != nil {
			return err
		}
	}

	if opts.json {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(snap); err != nil {
			return err
		}
	} else {
		writeText(w, bundle, snap, host, opts, audit.Path())
	}

	return audit.Append(consent.Event{
		Type:   consent.EventAgentStop,
		Fields: map[string]string{"checks_run": fmt.Sprint(len(snap.Results))},
	})
}

func writeText(w io.Writer, bundle *i18n.Bundle, snap checks.Snapshot, host platform.Host, opts options, auditPath string) {
	fmt.Fprintf(w, "%s %s — %s\n\n", bundle.T("app.name"), version, bundle.T("app.tagline"))
	if version == "dev" {
		fmt.Fprintf(w, "%s\n", bundle.T("agent.build.unsigned"))
	}
	if opts.dryRun {
		fmt.Fprintf(w, "%s\n", bundle.T("agent.dry_run.active"))
	}

	if len(snap.Results) == 0 {
		fmt.Fprintf(w, "\n%s\n", bundle.T("agent.checks.none"))
	} else {
		fmt.Fprintf(w, "\n%s\n\n", bundle.T("agent.checks.available", len(snap.Results), host.OS.Display()))
		for _, res := range snap.Results {
			fmt.Fprintf(w, "  [%s] %s — %s\n",
				bundle.T("severity."+string(res.Severity)), res.CheckID, bundle.T(res.Summary))
		}
	}

	for _, id := range snap.SkippedAdmin {
		fmt.Fprintf(w, "  [%s] %s — %s\n",
			bundle.T("severity.unknown"), id, bundle.T("agent.checks.requires_admin"))
	}

	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.audit.location", auditPath))
}
