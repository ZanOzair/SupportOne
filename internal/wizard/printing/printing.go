// Package printing walks a user through "it won't print".
//
// The three questions are the ones that account for most of it, in the order a
// technician asks them: is the print service running, is something jammed in
// the queue, and is the computer even pointed at a printer. Each is answered by
// reading Windows' own state, and only the middle one has a repair the agent
// can make.
package printing

import (
	"context"
	"os"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/fixes/spooler"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/wizard"
)

// ID is the stable identifier this wizard is offered and audited under.
const ID = "wizard.printing"

// Message keys this wizard resolves through internal/i18n.
const (
	KeyTitle     = "wizard.printing.title"
	KeyComplaint = "wizard.printing.complaint"

	KeyStepService       = "wizard.printing.step.service"
	KeyServiceRunning    = "wizard.printing.service.running"
	KeyServiceStopped    = "wizard.printing.service.stopped"
	KeyServiceUnreadable = "wizard.printing.service.unreadable"
	KeyServiceAdvice     = "wizard.printing.service.advice"

	KeyStepQueue       = "wizard.printing.step.queue"
	KeyQueueEmpty      = "wizard.printing.queue.empty"
	KeyQueueStuck      = "wizard.printing.queue.stuck"
	KeyQueueStuckOne   = "wizard.printing.queue.stuck.one"
	KeyQueueUnreadable = "wizard.printing.queue.unreadable"
	KeyQueueAdvice     = "wizard.printing.queue.advice"

	KeyStepPrinter       = "wizard.printing.step.printer"
	KeyPrinterSet        = "wizard.printing.printer.set"
	KeyPrinterNone       = "wizard.printing.printer.none"
	KeyPrinterUnreadable = "wizard.printing.printer.unreadable"
	KeyPrinterAdvice     = "wizard.printing.printer.advice"
)

// Compiled-in read-only queries. Neither is assembled from anything the user
// or a model provided.
const (
	scExe = "sc"
	psExe = "powershell"

	queryDefaultPrinter = `Get-CimInstance -ClassName Win32_Printer -ErrorAction SilentlyContinue | ` +
		`Where-Object { $_.Default } | Select-Object -First 1 Name | ConvertTo-Json -Compress`
)

// serviceProbe reads whether the Windows print service is running.
func serviceProbe(run platform.Runner) wizard.Probe {
	return func(ctx context.Context) (wizard.Finding, error) {
		raw, err := run(ctx, scExe, "query", "spooler")
		if err != nil {
			return wizard.Finding{Unknown: true, Summary: KeyServiceUnreadable}, nil
		}
		if serviceRunning(raw) {
			return wizard.Finding{OK: true, Summary: KeyServiceRunning}, nil
		}
		return wizard.Finding{Summary: KeyServiceStopped}, nil
	}
}

// queueProbe counts what is waiting in the print queue.
func queueProbe(dir string) wizard.Probe {
	return func(context.Context) (wizard.Finding, error) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return wizard.Finding{Unknown: true, Summary: KeyQueueUnreadable}, nil
		}

		queued := 0
		for _, e := range entries {
			// The agent's own holding folder is not a print job.
			if strings.HasPrefix(e.Name(), "SupportOne-quarantine") {
				continue
			}
			queued++
		}

		if queued == 0 {
			return wizard.Finding{OK: true, Summary: KeyQueueEmpty}, nil
		}
		summary := KeyQueueStuck
		if queued == 1 {
			summary = KeyQueueStuckOne
		}
		return wizard.Finding{Summary: summary, Args: []any{queued}}, nil
	}
}

// printerProbe reads whether a default printer is set at all.
func printerProbe(run platform.Runner) wizard.Probe {
	return func(ctx context.Context) (wizard.Finding, error) {
		raw, err := run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", queryDefaultPrinter)
		if err != nil {
			return wizard.Finding{Unknown: true, Summary: KeyPrinterUnreadable}, nil
		}

		name := defaultPrinterName(raw)
		if name == "" {
			return wizard.Finding{Summary: KeyPrinterNone}, nil
		}
		return wizard.Finding{OK: true, Summary: KeyPrinterSet, Args: []any{name}}, nil
	}
}

// New builds the wizard. The runner and spool directory are the seams tests
// substitute; production uses the compiled-in defaults.
func New(run platform.Runner, spoolDir string) *wizard.Wizard {
	return &wizard.Wizard{
		ID:        ID,
		Title:     KeyTitle,
		Complaint: KeyComplaint,

		// Windows only, for the same reason print.clear-spooler is: this
		// wizard reads the Windows print service and the Windows spool
		// directory, and understands neither's CUPS equivalent.
		Platforms: []platform.OS{platform.Windows},

		Steps: []wizard.Step{
			{
				// First, because a stopped service explains everything else
				// and nothing can be cleared while it is down.
				ID:     "printing.service",
				Title:  KeyStepService,
				Ask:    serviceProbe(run),
				Advice: KeyServiceAdvice,
			},
			{
				ID:     "printing.queue",
				Title:  KeyStepQueue,
				Ask:    queueProbe(spoolDir),
				FixID:  spooler.ID,
				Advice: KeyQueueAdvice,
			},
			{
				ID:     "printing.printer",
				Title:  KeyStepPrinter,
				Ask:    printerProbe(run),
				Advice: KeyPrinterAdvice,
			},
		},
	}
}

// DefaultSpoolDir is where Windows keeps queued print jobs.
const DefaultSpoolDir = `C:\Windows\System32\spool\PRINTERS`

func init() {
	wizard.MustRegister(New(platform.RunRead, DefaultSpoolDir))
}
