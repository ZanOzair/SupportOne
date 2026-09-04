# SupportOne

One small binary that reads a computer, explains what it finds in plain language, and changes nothing without being asked.

SupportOne consolidates ten IT-support functions — diagnostics, safe fixes, guided wizards, provisioning, reporting — into a single codebase with a plugin architecture, serving two audiences: the person with the problem, and the technician helping them.

## Status

**Phase 3 of 6 — the snapshot, repairs that can be undone, and every finding explained.** Fourteen read-only checks, a local web interface, HTML and JSON reports, redaction before anything is saved, three fixes and two guided walkthroughs behind a consent gate — and now a plain-language explanation of every verdict, from a table built into the binary, plus an optional model you can point at your own endpoint.

The fleet server and remote help are later phases and are **not** in this build. The agent says so rather than implying otherwise.

| Phase | Contents | State |
|---|---|---|
| 0 | Registries, platform layer, audit log, i18n, CI | Complete |
| 1 | Snapshot: 12 checks, local web UI, HTML + JSON report, redaction | Complete |
| 2 | Fixes and guided wizards, restore points, rollback | Complete |
| 3 | Offline explainer, optional LLM assistant, performance and backup analysis | Complete |
| 4 | Patch reporter, screenshot-to-ticket, optional fleet server | Next |
| 5 | Remote-help consent wrapper, provisioning, scheduled reports | Planned |

## What it checks

Fifteen checks, each read-only, each documented with its thresholds in [docs/CHECKS.md](docs/CHECKS.md):

`os.info` · `hardware.inventory` · `hardware.ram` · `disk.volumes` · `disk.smart` · `drivers.problem` (Windows) · `eventlog.errors` · `network.config` · `updates.os` · `startup.items` · `security.posture` · `battery.health` · `performance.load` · `backup.status` · `updates.installed`

## What it can repair

Three fixes and two guided walkthroughs, each documented with its exact behaviour in [docs/FIXES.md](docs/FIXES.md):

`temp.clear` · `print.clear-spooler` (Windows) · `net.flush-dns`

`wizard.connection` — "I can't get online" · `wizard.printing` (Windows) — "it won't print"

Nothing is deleted: the two fixes that clear files move them into a quarantine directory on the same volume, and the undo moves them back. `net.flush-dns` reports that it **cannot** be undone, because the previous cache cannot be put back — the explanation says why that costs nothing. Resetting the TCP/IP stack is deliberately not offered: it needs a reboot and cannot be reversed, so it does not meet the bar the other fixes meet.

## What it explains

Every verdict any check can report is explained in plain language, with an ordered list of things to try, from a table compiled into the binary. It works with the network unplugged and needs no API key. A guard test fails the build if any verdict has no explanation — that is the gate, not a claim.

An optional second tier can send the report to a model endpoint you choose. It is **off**: nothing is contacted unless you enable it, name an endpoint, read the exact bytes that would leave, and confirm that send. Whatever the model replies, the only actionable thing it can return is a repair ID, and those are checked against the repairs this build actually carries before you are offered anything. Details and reasoning in [docs/EXPLAINER.md](docs/EXPLAINER.md).

## What runs where

This is the honest scope. It is not "works everywhere".

- **Windows 10/11** — the primary target. Every check runs here.
- **macOS and Linux** — the same codebase and the same interfaces, with platform-specific implementations. A check that cannot exist on a platform is not offered there and the report says so: `drivers.problem` is Windows-only, because neither macOS nor Linux has an equivalent notion of a device in an error state.
- **Phones, tablets, Chromebooks, any browser** — can reach the web interface if the user explicitly enables access beyond loopback. **The agent itself requires a desktop OS**: Windows, macOS, or Linux. That distinction is what "works everywhere" honestly means here.
- **Firmware** — firmware and BIOS details are read where the operating system exposes them. Nothing is ever flashed or modified.

Some answers need administrator rights on some platforms — SMART attributes on Linux, BitLocker status on Windows. The agent does not require elevation to start; a check that could not read something reports that it could not, and names why.

## What the design actually guarantees

Verifiable properties, not marketing claims. The limits of each are documented in [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md).

- **Read-only by default.** Every check only reads. The only code that changes anything is a fix, and a fix runs only through the consent gate: it is described in full, and it is applied only against a confirmation that repeats that exact description back. A plan is good for one change. Silence is not consent.
- **Always reversible, or it says otherwise.** Every fix implements a rollback, and every fix has a test proving the rollback restores what was there. A fix that cannot honestly claim that reports it cannot, rather than implying an undo that does not exist.
- **No outbound connection, unless you ask for one.** The agent works with the network unplugged, and nothing in the checking, explaining or repairing path contacts anything. The one exception is the optional assistant, which is off, needs an endpoint you supply, and shows you the exact bytes before it sends them. `updates.os` deliberately reads local records rather than asking Windows Update, Apple, or a package mirror what is new — that would be an outbound connection the user did not ask for. There is no telemetry, no analytics, no crash reporting: not disabled, absent.
- **You choose what leaves.** Reports are saved to your own computer. Before saving, you pick what to strip — computer name, username and home folder, serial numbers, network addresses — and can see the exact file that would be written.
- **No dynamic code.** Nothing is downloaded and executed at runtime. Every OS command a check runs is compiled in, passed as separate arguments with no shell involved, and never assembled from user input or model output.
- **A log you can read.** Every check, consent decision, applied fix, rollback and saved file is appended to a plain-text audit log in your own config directory.
- **The interface is local.** It binds loopback on a random port, requires a token minted for that run, validates `Origin` and `Host` on every request, sends a strict Content-Security-Policy, and shuts itself down when idle.

## Running it

Requires Go 1.24 or newer to build. There are no other build dependencies — the interface is already built and embedded.

```sh
go build -o supportone-agent ./cmd/supportone-agent
./supportone-agent
```

With no flags it runs the checks and opens its interface in your browser, served from `127.0.0.1` on a random port.

| Flag | Effect |
|---|---|
| `--text` | Write the snapshot to the terminal and exit |
| `--json` | Write the snapshot to the terminal as JSON and exit |
| `--list-checks` | List the checks available on this computer |
| `--list-fixes` | List the repairs available on this computer, and what each changes |
| `--list-wizards` | List the guided walkthroughs available on this computer |
| `--fix <id>` | Describe one repair and, only if you type its ID back, apply it |
| `--wizard <id>` | Walk through one problem, one question at a time |
| `--no-explain` | Print verdicts without the plain-language explanation |
| `--assist` | Offer to send the report to a model endpoint (off unless given) |
| `--assist-endpoint` | An OpenAI-shaped chat completions URL; HTTPS, or http only on this computer |
| `--assist-model` | The model to ask for at that endpoint |
| `--no-browser` | Print the interface address instead of opening a browser |
| `--lang` | Language tag, `en` or `ms` (defaults to the system language) |
| `--dry-run` | Report what would change without changing anything |
| `--audit-log` | Path to the audit log (defaults to your config directory) |
| `--timeout` | Time limit for a single check |
| `--idle-timeout` | Close the interface after this long unused |
| `--version` | Build information |

`--fix` prints exactly what will change and then asks you to type the repair's ID. Anything else — `y`, `yes`, a blank line, or nothing at all because the command was run from a script — leaves the computer untouched. Where no restore point can be made, that is a second question with its own answer. If the repair can be undone, the offer to undo it comes straight after, because the record of what was applied lives in the process that applied it.

Builds you make yourself are unsigned development builds, and the agent says so in its own output. Signed, notarized releases are a later deliverable.

## Working on it

```sh
go test ./...                     # Go tests, including golden-file report rendering
go test ./internal/report -update # rewrite the golden report after an intended change

cd web/agent-ui
npm ci && npm run lint && npm run build   # the interface; dist/ is committed
```

The built interface in `web/agent-ui/dist` is committed so `go build` works without Node. CI rebuilds it and fails if the committed copy is stale.

Adding a thirteenth check means a new package under `internal/checks/` and one import line in `internal/checks/all` — never a change to the registry or the runner. [docs/CHECKS.md](docs/CHECKS.md) has the details.

Adding a fix is the same shape, with one extra requirement that is a gate rather than a suggestion: **a rollback test that proves the machine is as it was.** [docs/FIXES.md](docs/FIXES.md) has the details.

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — layout, plugin contracts, phase map
- [docs/CHECKS.md](docs/CHECKS.md) — every check, what it reads, and its thresholds
- [docs/FIXES.md](docs/FIXES.md) — the consent gate, every fix, every walkthrough, and what is deliberately not offered
- [docs/EXPLAINER.md](docs/EXPLAINER.md) — the offline explainer, the optional assistant, and the egress gate
- [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md) — assets, adversaries, controls, residual risks, non-goals

## License

Apache License 2.0. See [LICENSE](LICENSE).
