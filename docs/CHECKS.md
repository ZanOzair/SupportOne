# Checks

Every check in this document is read-only. None of them modifies the machine, and none of them makes a network connection.

Three rules hold throughout:

- **A check that cannot get an answer reports `unknown`, never `ok`.** A check that was skipped, timed out, or lacked rights is named in the report with the reason.
- **A check is offered only where it can answer honestly.** Where a platform has no equivalent, the check is not registered there and the report says it is unavailable, rather than showing an invented result.
- **Thresholds are written down here.** Nothing decides "urgent" by a number this document does not state.

## Severity

| Severity | Means |
|---|---|
| OK | Looked, found nothing wrong. A valid and useful answer. |
| Needs attention | Worth acting on, but not today. |
| Urgent | Risks data or downtime now. |
| Could not check | No answer was available. Never a stand-in for OK. |

## The twelve

### `os.info` — Windows, macOS, Linux

Operating system name, version, build, kernel and uptime; install date where the platform records one.

Reads `/etc/os-release` and `/proc/uptime` on Linux, `Win32_OperatingSystem` on Windows, `sw_vers` and `kern.boottime` on macOS. Always OK: it is inventory, not a verdict. Linux keeps no install date, so that field is reported as absent rather than guessed.

### `hardware.inventory` — Windows, macOS, Linux

Make, model, CPU and core count.

A virtual machine or a board with no DMI data reports no model; the check says the machine does not report one instead of printing an empty name.

### `hardware.ram` — Windows, macOS, Linux

Installed memory, and on Windows the slot count and speed.

| Verdict | When |
|---|---|
| Needs attention | Under 4 GiB installed |
| OK | 4 GiB or more |

Apple silicon has no slots to report, and Linux slot detail needs root, so those fields are absent rather than filled with a guess.

### `disk.volumes` — Windows, macOS, Linux

Every fixed drive, its filesystem, and how much room is left. Pseudo-filesystems (tmpfs, proc, sysfs and the like) are excluded: they are kernel bookkeeping, not drives a person has.

Judged on the volume with the least free space:

| Verdict | When |
|---|---|
| Urgent | Under 5% free |
| Needs attention | Under 10% free |
| OK | 10% or more free |

### `disk.smart` — Windows, macOS, Linux

Each physical drive's own health verdict, and the count of sectors it has retired because they went bad.

| Verdict | When |
|---|---|
| Urgent | A drive reports that it is failing |
| Needs attention | A drive has retired one or more bad sectors |
| Could not check | No drive would report a verdict |
| OK | Every drive reports itself healthy |

**Administrator rights:** required on Linux only, where reading SMART attributes through `smartctl` needs root. Windows (`Get-PhysicalDisk`) and macOS (`diskutil`) expose a verdict to any user. Windows' own failure prediction (`MSStorageDriver_FailurePredictStatus`) is read as well when rights allow, and its absence does not fail the check.

USB enclosures and some NVMe bridges pass no SMART data through at all. That is recorded as unknown, and the report says it is normal rather than implying a fault.

### `drivers.problem` — Windows only

Devices Windows has flagged with a Configuration Manager error, which is what Device Manager shows with a warning triangle.

Error code 45 — "not currently connected" — is skipped: a dock that went home with its owner is not a driver fault. Codes without a known meaning are reported by number rather than described inaccurately.

macOS and Linux have no equivalent notion, so the check is not registered there.

### `eventlog.errors` — Windows, macOS, Linux

Errors the system logged about itself: the Windows System log, journald, or the macOS unified log. The Security log on Windows is deliberately not read; it needs elevation and is not this check's business.

**A raw error count is not a problem count.** Every machine logs some errors, and presenting that number as a list of problems is the pattern that made the PC-cleaner category untrustworthy. This check looks for the thing that actually indicates a fault: the same source failing over and over.

| Verdict | When |
|---|---|
| Needs attention | Any event the platform itself marked critical |
| Needs attention | One source and event ID appearing 10 or more times |
| OK | Errors present, none repeating that often |
| OK | No errors at all |

**Window:** seven days on Windows and Linux; **24 hours on macOS**, because querying the unified log over a week routinely takes longer than a snapshot should. The window is reported in the result, so a quiet day is never read as a quiet week.

### `network.config` — Windows, macOS, Linux

Interfaces and addresses (from the Go standard library, so one implementation serves every platform), the default gateway, and the configured DNS servers.

| Verdict | When |
|---|---|
| Urgent | No interface holds a routable address |
| Needs attention | Connected, but no default gateway |
| Needs attention | Connected, but no DNS server configured |
| OK | Connected, with a gateway |

An interface holding only a link-local address is not counted as connected: that means the machine did not get a lease.

The check reads local configuration. It sends no packets and probes nothing.

### `updates.os` — Windows, macOS, Linux

When the operating system last installed updates, and how many the machine already knows are waiting.

**This check never contacts an update server.** Asking Windows Update, Apple, or a distribution mirror what is new is an outbound connection, and the agent makes none the user did not ask for. So:

- Windows reads the Windows Update agent's own record of its last successful install, falling back to the newest installed hotfix. Pending counts are not available without going online, and are reported as unknown.
- Linux simulates an upgrade against the **local** package cache (`apt-get -s upgrade`, or `dnf -C check-update`), which reflects the last time the machine itself refreshed that cache.
- macOS reads `LastFullSuccessfulDate` from the software update preferences.

| Verdict | When |
|---|---|
| Urgent | Last update over 180 days ago |
| Needs attention | Last update over 60 days ago |
| Needs attention | Updates are known to be waiting |
| Could not check | The machine keeps no record |
| OK | Updated within 60 days, nothing waiting |

### `startup.items` — Windows, macOS, Linux

What the machine launches on its own: `Win32_StartupCommand`, freedesktop autostart entries, or launch agents and daemons. Entries disabled with `Hidden=true` or `X-GNOME-Autostart-enabled=false` are not counted, because they do not run.

Always OK. This is an inventory. Whether a long list is slowing the machine down is a question for the performance analyser in a later phase; calling a list of programs a problem because it is long would be manufacturing one.

### `security.posture` — Windows, macOS, Linux

Disk encryption, firewall, and registered antivirus.

- Windows: BitLocker on the system drive, every firewall profile, and the products registered with Security Center. Firewall reports on only when **every** profile is on — a machine with the public profile disabled is unprotected on the network it is most exposed to.
- macOS: FileVault via `fdesetup`, and the application firewall's global state.
- Linux: LUKS containers via `lsblk`. Reading the packet filter needs root on every mainstream distribution, and the agent does not ask for elevation just to look, so the firewall is reported as unknown with that reason attached.

Antivirus is reported as **not applicable** on macOS and Linux rather than "off": neither exposes a registered-antivirus service, and reporting "off" would imply something is missing that was never there.

| Verdict | When |
|---|---|
| Needs attention | Two or more of the three are switched off |
| Needs attention | Disk encryption off |
| Needs attention | Firewall off |
| Needs attention | Antivirus off |
| Could not check | Nothing could be read from this account |
| OK | Nothing readable is switched off |

BitLocker status without administrator rights is **unknown**, not "unencrypted".

### `battery.health` — Windows, macOS, Linux

Full-charge capacity as a percentage of design capacity, and cycle count where the platform reports it.

| Verdict | When |
|---|---|
| Urgent | Under 50% of original capacity |
| Needs attention | Under 80% of original capacity |
| Could not check | A battery is present but reports too little to judge |
| OK | 80% or more, or no battery at all |

A desktop with no battery is a fact, not a fault: it reports OK.

## Adding a thirteenth

Write a package under `internal/checks/`, implement the `Check` interface, register it from `init()`, and add one import line to `internal/checks/all`. Keep the collector thin and the parsing separate — parser files carry no build constraints, which is how Windows and macOS output is tested from recorded fixtures on any machine.

Then add this document's row, and its message keys to every catalog in `internal/i18n/locales`. A test fails if a key is missing from one.
