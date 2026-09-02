# SupportOne

One small binary that reads a computer, explains what it finds in plain language, and changes nothing without being asked.

SupportOne consolidates ten IT-support functions — diagnostics, safe fixes, guided wizards, provisioning, reporting — into a single codebase with a plugin architecture, serving two audiences: the person with the problem, and the technician helping them.

## Status

**Phase 0 of 6 — foundation.** The scaffolding is in place: plugin registries, platform abstraction, audit log, i18n, CLI, and CI that builds all six targets.

**There are no diagnostic checks in this build yet.** Running the agent today tells you so rather than showing an empty report that looks like a clean bill of health. The first twelve checks arrive in Phase 1.

| Phase | Contents | State |
|---|---|---|
| 0 | Registries, platform layer, audit log, i18n, CI | Complete |
| 1 | Snapshot: 12 checks, local web UI, HTML + JSON report, redaction | Next |
| 2 | Fixes and guided wizards, restore points, rollback | Planned |
| 3 | Offline explainer, optional LLM assistant, performance and backup analysis | Planned |
| 4 | Patch reporter, screenshot-to-ticket, optional fleet server | Planned |
| 5 | Remote-help consent wrapper, provisioning, scheduled reports | Planned |

## What runs where

This is the honest scope. It is not "works everywhere".

- **Windows 10/11** — the primary target. Every module is built for Windows first.
- **macOS and Linux** — the same codebase and the same interfaces, with platform-specific implementations. A module that cannot exist on a platform is not offered there, and the report says so. It never fails silently and never fabricates a result.
- **Phones, tablets, Chromebooks, any browser** — can reach the web UI and (from Phase 4) the technician dashboard, if the user explicitly enables access beyond loopback. **The agent itself requires a desktop OS**: Windows, macOS, or Linux. That distinction is what "works everywhere" honestly means here.
- **Firmware** — firmware and BIOS details are read where the operating system exposes them. Nothing is ever flashed or modified.

## What the design actually guarantees

Verifiable properties, not marketing claims. The limits of each are documented in [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md).

- **Read-only by default.** Diagnostics never modify the machine. Anything that writes is a Fix, and a Fix runs only after the user confirms that specific action against an explicit list of what will change.
- **No outbound connection unless you send one.** The agent works fully offline. There is no telemetry, no analytics, no crash reporting, no usage statistics — not disabled by default, but absent.
- **No dynamic code.** Nothing is downloaded and executed at runtime. Fixes are compiled in and reachable only by exact ID from a whitelist, which is also the boundary the optional AI assistant is confined to: it can name a fix ID, and an ID that is not in the registry is discarded before you see it.
- **Least privilege.** The agent does not need administrator rights to start. Elevation is requested for the one action that needs it, when it runs.
- **A log you can read.** Every check, consent decision, fix, and byte sent is appended to a plain-text audit log in your own config directory.

## Build and run

Requires Go 1.24 or newer. There are no other build dependencies.

```sh
go build -o supportone-agent ./cmd/supportone-agent
./supportone-agent --list-checks
./supportone-agent --json
```

| Flag | Effect |
|---|---|
| `--json` | Write the snapshot as JSON instead of text |
| `--dry-run` | Report what would change without changing anything |
| `--lang` | Language tag, e.g. `en` or `ms` (defaults to the system language) |
| `--list-checks` | List the checks available on this computer |
| `--audit-log` | Path to the audit log (defaults to your config directory) |
| `--timeout` | Time limit for a single check |
| `--version` | Build information |

Builds produced locally are unsigned development builds, and the agent says so in its own output. Signed, notarized releases are a Phase 1 deliverable.

Run the tests:

```sh
go test ./...
```

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — layout, plugin contracts, phase map
- [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md) — assets, adversaries, controls, residual risks, non-goals

## License

Apache License 2.0. See [LICENSE](LICENSE).
