// Command supportone-agent runs SupportOne's read-only diagnostics on the
// machine it is started on.
//
// Started with no flags it opens its interface in the user's browser, served
// from loopback. It makes no outbound network connection: nothing leaves this
// computer unless the user saves or sends it themselves.
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

	"github.com/ZanOzair/SupportOne/internal/assist"
	"github.com/ZanOzair/SupportOne/internal/checks"
	_ "github.com/ZanOzair/SupportOne/internal/checks/all"
	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/egress"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	_ "github.com/ZanOzair/SupportOne/internal/fixes/all"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/localui"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"
	"github.com/ZanOzair/SupportOne/internal/remediate"
	"github.com/ZanOzair/SupportOne/internal/restore"
	"github.com/ZanOzair/SupportOne/internal/wizard"
	_ "github.com/ZanOzair/SupportOne/internal/wizard/all"
	agentui "github.com/ZanOzair/SupportOne/web/agent-ui"
)

// Build metadata, set via -ldflags at release time. An unset version means an
// unsigned local build, and the agent says so rather than implying a release.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

type options struct {
	json        bool
	text        bool
	dryRun      bool
	lang        string
	listChecks  bool
	listFixes   bool
	listWizards bool
	fix         string
	wizard      string
	noExplain   bool

	// The support bundle. It is written to disk and sent nowhere.
	ticket   string
	describe string
	attach   string

	// The fleet report. Off unless every one of these is given.
	report      bool
	fleetServer string
	fleetName   string

	// The assistant is off unless every one of these is given. Nothing
	// reaches the network on a default invocation.
	assist         bool
	assistEndpoint string
	assistModel    string

	auditPath   string
	timeout     time.Duration
	idleTimeout time.Duration
	noBrowser   bool
	showVer     bool
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "supportone-agent: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
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

	switch {
	case opts.listChecks:
		return listChecks(stdout, bundle, host)
	case opts.listFixes:
		return listFixes(stdout, bundle, host)
	case opts.listWizards:
		return listWizards(stdout, bundle, host)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := audit.Append(consent.Event{
		Type: consent.EventAgentStart,
		Fields: map[string]string{
			"version": version,
			"os":      string(host.OS),
			"arch":    host.Arch,
			"dry_run": fmt.Sprint(opts.dryRun),
		},
	}); err != nil {
		return err
	}
	defer func() {
		_ = audit.Append(consent.Event{Type: consent.EventAgentStop})
	}()

	// The terminal modes exist for technicians and scripts. Anyone who just
	// runs the file gets the interface.
	switch {
	case opts.ticket != "":
		return writeTicket(ctx, stdout, bundle, audit, host, opts)
	case opts.fix != "":
		return applyFix(ctx, stdin, stdout, bundle, audit, host, opts)
	case opts.wizard != "":
		return runWizard(ctx, stdin, stdout, bundle, audit, host, opts)
	case opts.json:
		snap := takeSnapshot(ctx, audit, host, opts)
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(snap)
	case opts.text:
		snap := takeSnapshot(ctx, audit, host, opts)
		writeText(stdout, bundle, snap, host, opts, audit.Path())
		if opts.assist {
			if err := askAssistant(ctx, stdin, stdout, bundle, audit, host, snap, opts); err != nil {
				return err
			}
		}
		if opts.report {
			return sendReport(ctx, stdin, stdout, bundle, audit, snap, opts)
		}
		return nil
	default:
		return serveUI(ctx, stdout, audit, host, opts)
	}
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var opts options
	fs := flag.NewFlagSet("supportone-agent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.BoolVar(&opts.json, "json", false, "write the snapshot to standard output as JSON and exit")
	fs.BoolVar(&opts.text, "text", false, "write the snapshot to standard output as text and exit")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "report what would change without changing anything")
	fs.StringVar(&opts.lang, "lang", "", "language tag, e.g. en or ms (default: system language)")
	fs.BoolVar(&opts.listChecks, "list-checks", false, "list the checks available on this computer and exit")
	fs.BoolVar(&opts.listFixes, "list-fixes", false, "list the repairs available on this computer and exit")
	fs.BoolVar(&opts.listWizards, "list-wizards", false, "list the guided walkthroughs available on this computer and exit")
	fs.StringVar(&opts.fix, "fix", "", "describe one repair and, after you confirm it, apply it")
	fs.StringVar(&opts.wizard, "wizard", "", "walk through one problem step by step")
	fs.BoolVar(&opts.noExplain, "no-explain", false, "print verdicts without the plain-language explanation")
	fs.StringVar(&opts.ticket, "ticket", "", "write a support bundle to this file or folder and exit")
	fs.StringVar(&opts.describe, "describe", "", "your own description of the problem, to go in the bundle")
	fs.StringVar(&opts.attach, "attach", "", "image files to include in the bundle, comma separated")
	fs.BoolVar(&opts.report, "report", false, "offer to send this report to the fleet server named below")
	fs.StringVar(&opts.fleetServer, "fleet-server", "", "the fleet server's address; HTTPS, or http only on this computer")
	fs.StringVar(&opts.fleetName, "fleet-name", "", "what this machine should be called in that dashboard")
	fs.BoolVar(&opts.assist, "assist", false, "offer to send the report to the model endpoint configured below")
	fs.StringVar(&opts.assistEndpoint, "assist-endpoint", "", "an OpenAI-shaped chat completions URL; HTTPS, or http only on this computer")
	fs.StringVar(&opts.assistModel, "assist-model", "", "the model to ask for at that endpoint")
	fs.StringVar(&opts.auditPath, "audit-log", "", "path to the audit log (default: per-user config directory)")
	fs.DurationVar(&opts.timeout, "timeout", checks.DefaultTimeout, "time limit for a single check")
	fs.DurationVar(&opts.idleTimeout, "idle-timeout", localui.DefaultIdleTimeout, "close the interface after this long with nobody using it")
	fs.BoolVar(&opts.noBrowser, "no-browser", false, "print the interface address instead of opening a browser")
	fs.BoolVar(&opts.showVer, "version", false, "print build information and exit")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if opts.json && opts.text {
		return options{}, fmt.Errorf("--json and --text ask for two different outputs; choose one")
	}
	if opts.attach != "" && opts.ticket == "" {
		return options{}, fmt.Errorf("--attach only means something with --ticket, which is what an attachment goes into")
	}
	if opts.fix != "" && opts.wizard != "" {
		return options{}, fmt.Errorf("--fix and --wizard ask for two different things; choose one")
	}
	if opts.assist {
		if opts.assistEndpoint == "" {
			return options{}, fmt.Errorf("--assist needs --assist-endpoint: there is no default, and no endpoint is contacted without one")
		}
		if err := assist.CheckEndpoint(opts.assistEndpoint); err != nil {
			return options{}, err
		}
	}
	if opts.report {
		if opts.fleetServer == "" {
			return options{}, fmt.Errorf("--report needs --fleet-server: there is no default, and no server is contacted without one")
		}
		if opts.fleetName == "" {
			return options{}, fmt.Errorf("--report needs --fleet-name: what this machine is called in someone else's dashboard is your decision, not something to take from the hostname")
		}
		if err := egress.CheckURL(opts.fleetServer); err != nil {
			return options{}, err
		}
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

// takeSnapshot runs the checks available on this machine and records each one
// in the audit log.
func takeSnapshot(ctx context.Context, audit *consent.Log, host platform.Host, opts options) checks.Snapshot {
	elevated, err := platform.IsElevated()
	if err != nil {
		elevated = false
	}

	snap := checks.RunAll(ctx, checks.Default, host, elevated, opts.timeout)
	snap.AgentVersion = version

	for _, res := range snap.Results {
		_ = audit.Append(consent.Event{
			Type:    consent.EventCheckRun,
			Subject: res.CheckID,
			Fields: map[string]string{
				"severity": string(res.Severity),
				"duration": res.Duration.String(),
			},
		})
	}
	return snap
}

// newExplainer builds the offline explainer over the compiled-in registries,
// so an explanation can only ever offer a repair this binary carries.
func newExplainer(host platform.Host) *explain.Explainer {
	return explain.New(fixes.Default, wizard.Default, host.OS)
}

// newAssistant builds the optional second tier. It is off unless the user
// asked for it and named an endpoint, and it contacts nothing until a specific
// send is confirmed.
func newAssistant(audit *consent.Log, host platform.Host, opts options) *assist.Assistant {
	return assist.New(assist.Config{
		Enabled:  opts.assist,
		Endpoint: opts.assistEndpoint,
		Model:    opts.assistModel,
	}, fixes.Default, host.OS, audit)
}

// newApplier builds the one path through which anything on this machine can
// change: the registry of compiled-in fixes, the audit log, and the platform's
// restore mechanism.
func newApplier(audit *consent.Log, host platform.Host, opts options) *remediate.Applier {
	applier := remediate.New(fixes.Default, audit, restore.New(), host.OS)
	applier.DryRun = opts.dryRun
	return applier
}

func serveUI(ctx context.Context, w io.Writer, audit *consent.Log, host platform.Host, opts options) error {
	server, err := localui.New(localui.Config{
		Assets: agentui.Assets,
		Snapshot: func(ctx context.Context) checks.Snapshot {
			return takeSnapshot(ctx, audit, host, opts)
		},
		Audit:        audit,
		Version:      version,
		Host:         host,
		Identity:     redact.CurrentIdentity(),
		Lang:         opts.lang,
		IdleTimeout:  opts.idleTimeout,
		Fixes:        fixes.Default,
		Applier:      newApplier(audit, host, opts),
		Wizards:      wizard.Default,
		CheckTimeout: opts.timeout,
		Explainer:    newExplainer(host),
		Assistant:    newAssistant(audit, host, opts),
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "SupportOne is running at %s\n", server.URL())
	fmt.Fprintln(w, "This address is on this computer only. Press Ctrl+C to stop.")

	if !opts.noBrowser {
		if err := platform.OpenBrowser(ctx, server.URL()); err != nil {
			// A machine with no desktop session still gets a usable agent:
			// the address is already printed above.
			fmt.Fprintf(w, "Could not open a browser (%v). Open the address above yourself.\n", err)
		}
	}
	return server.Serve(ctx)
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

		explainer := newExplainer(host)
		for _, res := range snap.Results {
			fmt.Fprintf(w, "  [%s] %s — %s\n",
				bundle.T("severity."+string(res.Severity)), res.CheckID, bundle.T(res.Summary, res.Args...))
			if !opts.noExplain {
				writeAdvice(w, bundle, explainer, res)
			}
		}
	}

	for _, id := range snap.SkippedAdmin {
		fmt.Fprintf(w, "  [%s] %s — %s\n",
			bundle.T("severity.unknown"), id, bundle.T("agent.checks.requires_admin"))
	}

	fmt.Fprintf(w, "\n%s\n", bundle.T("agent.audit.location", auditPath))
}
