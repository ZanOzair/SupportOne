package explain

// The shared steps. A step that says the same thing in two places is one
// string in one place: "back up anything you would hate to lose" is the same
// sentence whether a drive is failing or a backup has gone stale.
const (
	stepBackupNow      = "explain.step.backup_now"
	stepSetUpBackup    = "explain.step.set_up_backup"
	stepCheckBackupNow = "explain.step.check_backup_now"
	stepReconnectDrive = "explain.step.reconnect_drive"

	stepStopUsing   = "explain.step.stop_using"
	stepReplaceDisk = "explain.step.replace_disk"
	stepWatchDisk   = "explain.step.watch_disk"

	stepFreeSpace       = "explain.step.free_space"
	stepEmptyBin        = "explain.step.empty_bin"
	stepMoveFilesOff    = "explain.step.move_files_off"
	stepUninstallUnused = "explain.step.uninstall_unused"

	stepInstallUpdates = "explain.step.install_updates"
	stepRestart        = "explain.step.restart"
	stepClosePrograms  = "explain.step.close_programs"
	stepReduceStartup  = "explain.step.reduce_startup"
	stepAddMemory      = "explain.step.add_memory"

	stepCheckCable    = "explain.step.check_cable"
	stepRestartRouter = "explain.step.restart_router"

	stepTurnOnEncryption = "explain.step.turn_on_encryption"
	stepTurnOnFirewall   = "explain.step.turn_on_firewall"
	stepTurnOnAntivirus  = "explain.step.turn_on_antivirus"

	stepReplaceBattery = "explain.step.replace_battery"
	stepKeepCharger    = "explain.step.keep_charger"

	stepUpdateDrivers = "explain.step.update_drivers"
	stepNoteWhen      = "explain.step.note_when"

	stepRunAsAdmin  = "explain.step.run_as_admin"
	stepInstallTool = "explain.step.install_tool"
	stepCheckAgain  = "explain.step.check_again"
	stepCheckClock  = "explain.step.check_clock"

	stepNothing = "explain.step.nothing"
)

// The fixes and walkthroughs a rule may point at. They are names here and
// nothing more: the explainer resolves each against the registry before it
// reaches the user, so a build without one simply does not offer it.
const (
	fixTempClear = "temp.clear"
	fixFlushDNS  = "net.flush-dns"

	wizardConnection = "wizard.connection"
)

// rule is what the table holds for one verdict. The explanation itself is not
// here: it is derived from the verdict key by CauseKey, so the two cannot
// drift apart.
type rule struct {
	// Steps are message keys in the order they are worth trying.
	Steps []string

	// Fixes and Wizards are candidate IDs, resolved before use.
	Fixes   []string
	Wizards []string

	// Escalate marks a finding past what someone should work through alone.
	Escalate bool
}

// rules is the whole of Tier 1: every verdict any compiled-in check can
// report, and what to do about it. A guard test walks the message catalog and
// fails if a verdict has no entry here, or an entry has no explanation.
var rules = map[string]rule{
	// Shared verdicts, reported by any check that could not finish. These
	// three are why a missing answer is never a passing answer.
	"check.unknown.failed":       {Steps: []string{stepCheckAgain}},
	"check.unknown.needs_admin":  {Steps: []string{stepRunAsAdmin}},
	"check.unknown.tool_missing": {Steps: []string{stepInstallTool}},

	// Inventory. Nothing to act on: these are facts, not verdicts.
	"check.os.info.ok":                    {Steps: []string{stepNothing}},
	"check.hardware.inventory.ok":         {Steps: []string{stepNothing}},
	"check.hardware.inventory.unreported": {Steps: []string{stepNothing}},

	// Memory installed.
	"check.hardware.ram.ok":  {Steps: []string{stepNothing}},
	"check.hardware.ram.low": {Steps: []string{stepClosePrograms, stepReduceStartup, stepAddMemory}},

	// Battery.
	"check.battery.health.ok":         {Steps: []string{stepNothing}},
	"check.battery.health.absent":     {Steps: []string{stepNothing}},
	"check.battery.health.worn":       {Steps: []string{stepKeepCharger, stepReplaceBattery}},
	"check.battery.health.failing":    {Steps: []string{stepKeepCharger, stepReplaceBattery}, Escalate: true},
	"check.battery.health.unreadable": {Steps: []string{stepCheckAgain}},

	// Free space.
	"check.disk.volumes.ok":   {Steps: []string{stepNothing}},
	"check.disk.volumes.none": {Steps: []string{stepCheckAgain}},
	"check.disk.volumes.low": {
		Steps: []string{stepEmptyBin, stepMoveFilesOff, stepUninstallUnused},
		Fixes: []string{fixTempClear},
	},
	"check.disk.volumes.critical": {
		Steps: []string{stepEmptyBin, stepMoveFilesOff, stepUninstallUnused, stepFreeSpace},
		Fixes: []string{fixTempClear},
	},

	// Drive health. This is the one place the advice leads with "back up",
	// because a drive that reports these does not get better.
	"check.disk.smart.ok":       {Steps: []string{stepNothing}},
	"check.disk.smart.no_disks": {Steps: []string{stepCheckAgain}},
	"check.disk.smart.unknown":  {Steps: []string{stepRunAsAdmin, stepInstallTool}},
	"check.disk.smart.bad_spots": {
		Steps: []string{stepBackupNow, stepWatchDisk},
	},
	"check.disk.smart.failing": {
		Steps:    []string{stepBackupNow, stepStopUsing, stepReplaceDisk},
		Escalate: true,
	},

	// Network configuration.
	"check.network.config.ok": {Steps: []string{stepNothing}},
	"check.network.config.no_address": {
		Steps:   []string{stepCheckCable, stepRestartRouter},
		Wizards: []string{wizardConnection},
	},
	"check.network.config.no_gateway": {
		Steps:   []string{stepRestartRouter},
		Wizards: []string{wizardConnection},
	},
	"check.network.config.no_dns": {
		Steps:   []string{stepRestartRouter},
		Fixes:   []string{fixFlushDNS},
		Wizards: []string{wizardConnection},
	},
	"check.network.config.interfaces_unreadable": {Steps: []string{stepCheckAgain}},

	// Updates.
	"check.updates.os.ok":         {Steps: []string{stepNothing}},
	"check.updates.os.pending":    {Steps: []string{stepInstallUpdates, stepRestart}},
	"check.updates.os.stale":      {Steps: []string{stepInstallUpdates, stepRestart}},
	"check.updates.os.very_stale": {Steps: []string{stepInstallUpdates, stepRestart}, Escalate: true},
	"check.updates.os.unknown":    {Steps: []string{stepCheckAgain}},

	// What starts with the machine.
	"check.startup.items.ok":   {Steps: []string{stepNothing}},
	"check.startup.items.none": {Steps: []string{stepNothing}},

	// Security posture. Each of these is one switch the user can turn on.
	"check.security.posture.ok":            {Steps: []string{stepNothing}},
	"check.security.posture.no_encryption": {Steps: []string{stepTurnOnEncryption}},
	"check.security.posture.no_firewall":   {Steps: []string{stepTurnOnFirewall}},
	"check.security.posture.no_antivirus":  {Steps: []string{stepTurnOnAntivirus}},
	"check.security.posture.several_off": {
		Steps: []string{stepTurnOnEncryption, stepTurnOnFirewall, stepTurnOnAntivirus},
	},
	"check.security.posture.unreadable": {Steps: []string{stepRunAsAdmin}},

	// Windows device problems.
	"check.drivers.problem.ok":             {Steps: []string{stepNothing}},
	"check.drivers.problem.not_applicable": {Steps: []string{stepNothing}},
	"check.drivers.problem.found":          {Steps: []string{stepUpdateDrivers, stepRestart}},

	// The system log.
	"check.eventlog.errors.none":  {Steps: []string{stepNothing}},
	"check.eventlog.errors.quiet": {Steps: []string{stepNothing}},
	"check.eventlog.errors.repeated": {
		Steps: []string{stepNoteWhen, stepInstallUpdates, stepRestart},
	},
	"check.eventlog.errors.critical": {
		Steps:    []string{stepNoteWhen, stepBackupNow},
		Escalate: true,
	},

	// Load and memory pressure right now.
	"check.performance.load.ok":         {Steps: []string{stepNothing}},
	"check.performance.load.busy":       {Steps: []string{stepClosePrograms, stepReduceStartup}},
	"check.performance.load.busy_now":   {Steps: []string{stepCheckAgain}},
	"check.performance.load.memory_low": {Steps: []string{stepClosePrograms, stepReduceStartup, stepAddMemory}},
	"check.performance.load.memory_critical": {
		Steps: []string{stepClosePrograms, stepRestart, stepReduceStartup, stepAddMemory},
	},
	"check.performance.load.swapping":   {Steps: []string{stepClosePrograms, stepRestart, stepAddMemory}},
	"check.performance.load.unreadable": {Steps: []string{stepCheckAgain}},

	// Backups.
	"check.backup.status.ok":             {Steps: []string{stepNothing}},
	"check.backup.status.not_applicable": {Steps: []string{stepCheckBackupNow}},
	"check.backup.status.none":           {Steps: []string{stepCheckBackupNow, stepSetUpBackup}},
	"check.backup.status.never_run":      {Steps: []string{stepReconnectDrive, stepCheckBackupNow}},
	"check.backup.status.stale":          {Steps: []string{stepReconnectDrive, stepBackupNow}},
	"check.backup.status.very_stale":     {Steps: []string{stepReconnectDrive, stepBackupNow, stepSetUpBackup}},
	"check.backup.status.unreadable":     {Steps: []string{stepCheckClock, stepCheckBackupNow}},
}

// Verdicts returns every verdict key the table answers, sorted. The guard test
// and the documentation both read it, so neither can quietly fall behind.
func Verdicts() []string {
	out := make([]string, 0, len(rules))
	for key := range rules {
		out = append(out, key)
	}
	sortStrings(out)
	return out
}
