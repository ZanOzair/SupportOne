package all

import (
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// wantIDs is the full set of Phase 1 checks. The list is written out so that
// removing or renaming a check is a deliberate edit here, not a silent loss
// from a report someone is relying on.
var wantIDs = []string{
	"battery.health",
	"disk.smart",
	"disk.volumes",
	"drivers.problem",
	"eventlog.errors",
	"hardware.inventory",
	"hardware.ram",
	"network.config",
	"os.info",
	"security.posture",
	"startup.items",
	"updates.os",
}

func TestEveryCheckIsRegistered(t *testing.T) {
	registered := checks.Default.All()
	if len(registered) != len(wantIDs) {
		t.Fatalf("registered %d checks, want %d: %v", len(registered), len(wantIDs), ids(registered))
	}
	for i, want := range wantIDs {
		if registered[i].ID() != want {
			t.Errorf("check %d = %q, want %q", i, registered[i].ID(), want)
		}
	}
}

func TestEveryCheckDeclaresAPlatformItCanAnswerOn(t *testing.T) {
	for _, c := range checks.Default.All() {
		if len(c.Platforms()) == 0 {
			t.Errorf("%s declares no platforms", c.ID())
		}
		for _, p := range c.Platforms() {
			if !p.Valid() {
				t.Errorf("%s declares unknown platform %q", c.ID(), p)
			}
		}
	}
}

func TestPlatformSpecificChecksAreOfferedOnlyWhereTheyAnswer(t *testing.T) {
	// drivers.problem has no honest equivalent outside Windows, so it must not
	// appear in a macOS or Linux snapshot at all.
	for _, os := range []platform.OS{platform.Darwin, platform.Linux} {
		for _, c := range checks.Default.ForPlatform(os) {
			if c.ID() == "drivers.problem" {
				t.Errorf("drivers.problem is offered on %s, where it cannot give an answer", os)
			}
		}
	}

	var found bool
	for _, c := range checks.Default.ForPlatform(platform.Windows) {
		if c.ID() == "drivers.problem" {
			found = true
		}
	}
	if !found {
		t.Error("drivers.problem is not offered on Windows, where it does answer")
	}
}

// TestEveryMessageKeyIsTranslated is the guard against a check reporting a
// verdict that renders as a bare key in the user interface.
func TestEveryMessageKeyIsTranslated(t *testing.T) {
	bundle, err := i18n.Load(i18n.Base)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	for _, key := range append(append([]string{}, messageKeys...), interfaceKeys...) {
		if got := bundle.T(key); got == key {
			t.Errorf("message key %q has no translation", key)
		}
	}
}

// messageKeys lists every key a Phase 1 check can put in a result. The checks
// keep their keys unexported, so this list is the seam that keeps the catalogs
// honest.
var messageKeys = []string{
	checks.KeyToolMissing, checks.KeyCheckFailed, checks.KeyNeedsAdmin,

	"check.os.info.ok",
	"check.hardware.inventory.ok", "check.hardware.inventory.unreported",
	"check.hardware.ram.ok", "check.hardware.ram.low",
	"check.battery.health.absent", "check.battery.health.ok", "check.battery.health.worn",
	"check.battery.health.failing", "check.battery.health.unreadable",

	"check.disk.volumes.ok", "check.disk.volumes.ok.one", "check.disk.volumes.low",
	"check.disk.volumes.critical", "check.disk.volumes.none",
	"check.disk.smart.ok", "check.disk.smart.ok.one", "check.disk.smart.failing", "check.disk.smart.bad_spots",
	"check.disk.smart.unknown", "check.disk.smart.no_disks",

	"check.network.config.ok", "check.network.config.no_address",
	"check.network.config.no_gateway", "check.network.config.no_dns",
	"check.network.config.interfaces_unreadable",

	"check.drivers.problem.ok", "check.drivers.problem.found", "check.drivers.problem.found.one",
	"check.startup.items.ok", "check.startup.items.ok.one", "check.startup.items.none",

	"check.updates.os.ok", "check.updates.os.pending", "check.updates.os.pending.one", "check.updates.os.stale",
	"check.updates.os.very_stale", "check.updates.os.unknown",

	"check.security.posture.ok", "check.security.posture.no_encryption",
	"check.security.posture.no_firewall", "check.security.posture.no_antivirus",
	"check.security.posture.several_off", "check.security.posture.unreadable",

	"check.eventlog.errors.none", "check.eventlog.errors.quiet", "check.eventlog.errors.quiet.one",
	"check.eventlog.errors.repeated", "check.eventlog.errors.critical", "check.eventlog.errors.critical.one",
}

// interfaceKeys are the labels the local interface and the saved report use.
// They live in the same catalogs, so the same guard covers them.
var interfaceKeys = []string{
	"ui.heading", "ui.subheading", "ui.machine", "ui.checked_at", "ui.recheck", "ui.rechecking",
	"ui.running", "ui.error", "ui.language", "ui.evidence", "ui.nothing_wrong",
	"ui.skipped", "ui.skipped_note", "ui.save", "ui.save_html", "ui.save_json",
	"ui.redaction", "ui.redaction_note", "ui.redact_hostnames", "ui.redact_usernames",
	"ui.redact_serials", "ui.redact_addresses", "ui.preview", "ui.preview_note",
	"ui.hide_preview", "ui.audit", "ui.close", "ui.closed", "ui.offline_note",

	"report.title", "report.subtitle", "report.machine", "report.generated", "report.agent",
	"report.unsigned", "report.summary", "report.no_findings", "report.checked",
	"report.evidence", "report.skipped", "report.skipped_note", "report.audit",
	"report.about", "report.about_body", "report.redacted",
}

func ids(cs []checks.Check) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID()
	}
	return out
}
