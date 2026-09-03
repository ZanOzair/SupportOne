# SupportOne

One small binary that reads a computer, explains what it finds in plain language, and changes nothing without being asked.

SupportOne consolidates ten IT-support functions — diagnostics, safe fixes, guided wizards, provisioning, reporting — into a single codebase with a plugin architecture, serving two audiences: the person with the problem, and the technician helping them.

## Status

**Phase 1 of 6 — the diagnostic snapshot.** Twelve read-only checks, a local web interface, HTML and JSON reports, and redaction before anything is saved.

Fixes, the offline explainer, the optional AI assistant, the fleet server and remote help are later phases and are **not** in this build. The agent says so rather than implying otherwise.

| Phase | Contents | State |
|---|---|---|
| 0 | Registries, platform layer, audit log, i18n, CI | Complete |
| 1 | Snapshot: 12 checks, local web UI, HTML + JSON report, redaction | Complete |
| 2 | Fixes and guided wizards, restore points, rollback | Next |
| 3 | Offline explainer, optional LLM assistant, performance and backup analysis | Planned |
| 4 | Patch reporter, screenshot-to-ticket, optional fleet server | Planned |
| 5 | Remote-help consent wrapper, provisioning, scheduled reports | Planned |

## What it checks

Twelve checks, each read-only, each documented with its thresholds in [docs/CHECKS.md](docs/CHECKS.md):

`os.info` · `hardware.inventory` · `hardware.ram` · `disk.volumes` · `disk.smart` · `drivers.problem` (Windows) · `eventlog.errors` · `network.config` · `updates.os` · `startup.items` · `security.posture` · `battery.health`

## What runs where

This is the honest scope. It is not "works everywhere".

- **Windows 10/11** — the primary target. Every check runs here.
- **macOS and Linux** — the same codebase and the same interfaces, with platform-specific implementations. A check that cannot exist on a platform is not offered there and the report says so: `drivers.problem` is Windows-only, because neither macOS nor Linux has an equivalent notion of a device in an error state.
- **Phones, tablets, Chromebooks, any browser** — can reach the web interface if the user explicitly enables access beyond loopback. **The agent itself requires a desktop OS**: Windows, macOS, or Linux. That distinction is what "works everywhere" honestly means here.
- **Firmware** — firmware and BIOS details are read where the operating system exposes them. Nothing is ever flashed or modified.

Some answers need administrator rights on some platforms — SMART attributes on Linux, BitLocker status on Windows. The agent does not require elevation to start; a check that could not read something reports that it could not, and names why.

## What the design actually guarantees

Verifiable properties, not marketing claims. The limits of each are documented in [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md).

- **Read-only.** Every check in this build only reads. Nothing writes to the machine. Fixes arrive in Phase 2 behind per-action consent.
- **No outbound connection.** The agent works with the network unplugged, and nothing in the checking path contacts anything. `updates.os` deliberately reads local records rather than asking Windows Update, Apple, or a package mirror what is new — that would be an outbound connection the user did not ask for. There is no telemetry, no analytics, no crash reporting: not disabled, absent.
- **You choose what leaves.** Reports are saved to your own computer. Before saving, you pick what to strip — computer name, username and home folder, serial numbers, network addresses — and can see the exact file that would be written.
- **No dynamic code.** Nothing is downloaded and executed at runtime. Every OS command a check runs is compiled in, passed as separate arguments with no shell involved, and never assembled from user input or model output.
- **A log you can read.** Every check, consent decision and saved file is appended to a plain-text audit log in your own config directory.
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
| `--no-browser` | Print the interface address instead of opening a browser |
| `--lang` | Language tag, `en` or `ms` (defaults to the system language) |
| `--dry-run` | Report what would change without changing anything |
| `--audit-log` | Path to the audit log (defaults to your config directory) |
| `--timeout` | Time limit for a single check |
| `--idle-timeout` | Close the interface after this long unused |
| `--version` | Build information |

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

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — layout, plugin contracts, phase map
- [docs/CHECKS.md](docs/CHECKS.md) — every check, what it reads, and its thresholds
- [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md) — assets, adversaries, controls, residual risks, non-goals

## License

Apache License 2.0. See [LICENSE](LICENSE).
