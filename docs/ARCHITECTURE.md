# Architecture

## What this is

SupportOne is one codebase serving two audiences:

- **The person with the problem** runs a single binary, gets a plain-language health report, and can apply safe, reversible fixes.
- **The technician** receives that report and — optionally, from Phase 4 — sees a fleet of them in a self-hosted dashboard.

Ten support functions live behind one plugin architecture, so adding the eleventh means adding a file to a registry, not editing the core.

## Runtime choices

| Component | Choice | Why |
|---|---|---|
| Agent core | Go | One statically linked binary per target, no runtime for the user to install, cross-compiles to all six targets from any host |
| Agent UI | Local web UI served by the binary | Same UI on every OS with no Electron and no GUI toolkit; the binary stays a few MB |
| Server (optional) | Go + SQLite, Postgres optional | Self-hostable as one binary or one Compose file, so client data can stay wherever the client needs it |
| Frontend | TypeScript + Vite + React + Tailwind, embedded via `go:embed` | No CDN at runtime, no assets to ship alongside the executable |

The agent works with no network access at all. Nothing in the diagnostic path requires a server, an account, or an internet connection.

## Repository layout

```
cmd/
  supportone-agent/     end-user binary
  supportone-server/    optional fleet server (Phase 4)
internal/
  checks/               diagnostic plugin contract + registry
    all/                the one file that decides which checks are compiled in
    cim/                shared decoder for Windows CIM/WMI JSON
    system/ storage/ network/ events/
    updates/ startup/ security/ drivers/
  fixes/                remediation plugin contract + registry
  consent/              audit log, consent flow, restore points
  localui/              the loopback server that hosts the interface
  redact/               removing identifying detail before anything is saved
  report/               HTML and JSON rendering
  platform/             OS abstraction layer
  i18n/                 embedded message catalogs
web/agent-ui/           the interface: TypeScript, React, Tailwind, Vite
docs/
.github/workflows/      build, test, lint, vulnerability scan, SBOM
```

Directories arrive with the phase that needs them. There are no empty placeholders for features that do not exist yet.

## The plugin contract

Everything the product does to or with a machine is one of two things.

### Check — reads, never writes

```go
type Check interface {
	ID() string                              // stable, dotted: "disk.smart"
	Platforms() []platform.OS
	RequiresAdmin() bool
	Run(ctx context.Context) (Result, error) // read-only, cancellable, time-limited
}
```

A check registers itself at `init()` time, and `internal/checks/all` is the single file that decides which packages are compiled in. The registry filters by platform at run time, so a snapshot on macOS is never offered a check that only Windows can answer.

Within a check package the split matters: **collectors** carry build constraints and are thin, doing little more than reading a file or running one compiled-in command; **parsers** carry no build constraints at all. That is what lets Windows CIM output and macOS `system_profiler` output be tested from recorded fixtures on a Linux CI runner.

Three rules hold for every check:

1. **It never modifies the machine.** Anything that writes is a Fix.
2. **It reports `unknown`, never `ok`, when it could not determine an answer.** A check that was skipped, timed out, panicked, or lacked rights is named in the output with the reason. A check that did not run is not a check that passed.
3. **It is offered only where it can answer honestly.** `drivers.problem` has no meaningful macOS equivalent, so it does not register on macOS, and the report says the check is unavailable on this platform rather than showing an invented result.

### Fix — writes, only with consent

```go
type Fix interface {
	ID() string                                // stable, dotted: "net.flush-dns"
	Describe() Explanation                     // exactly what will change, before it runs
	Platforms() []platform.OS
	RequiresAdmin() bool
	Reversible() bool
	Preflight(ctx context.Context) error       // refuse if unsafe; changes nothing
	Apply(ctx context.Context) (Outcome, error)
	Rollback(ctx context.Context) error
}
```

The registry refuses to accept a fix that cannot describe what it changes, because the user's confirmation is given against that description.

`Registry.Resolve` is the single gate every suggestion passes through, including suggestions from the optional AI assistant: candidate IDs are matched against the registry and the current platform, and anything unrecognised is discarded before the user ever sees it. There is no path from a string to an action except a compiled-in fix whose ID matches exactly.

## The local interface

The agent serves its own interface rather than shipping a GUI toolkit, so the same screens render on every OS and the binary stays a few megabytes.

`internal/localui` binds `127.0.0.1:0` — the operating system picks a free port, which is what makes the address unguessable — mints a 32-byte token from the OS random source, and opens the browser at a URL carrying it. The page keeps the token in memory and removes it from the address bar, so it does not reach browser history or a screenshot.

Every `/api/` request must present that token, compared in constant time. Every response carries a Content-Security-Policy with no inline script and no remote origin, and every request is checked against the listening address: a page that rebinds a DNS name to `127.0.0.1` still cannot forge the `Origin` and `Host` headers the browser sends. The server shuts down when idle.

Static files are served without a token, because they carry no data: the snapshot only ever arrives through the API.

`internal/redact` sits between the snapshot and anything leaving the process. It walks the JSON shape of a result's evidence, so a new check needs no changes there, and the interface can show the user exactly what a policy would leave before they save it.

## Message keys, not prose

Check results and fix explanations carry i18n message keys rather than English sentences. The same result renders in English or Bahasa Melayu from one code path, and adding a language is a new JSON file, not a code change. Catalogs are embedded in the binary, so translation works offline. A key missing from every catalog renders as the key itself — visible, rather than silently blank.

## The audit log

`internal/consent` appends one line per action to a plain-text file in the user's config directory: agent start and stop, every check run with its verdict, every consent decision, every fix applied or rolled back, and every byte sent off the machine.

It is append-only, created `0600`, and values are escaped so a field can never forge a second entry. It records what happened, never credentials or file contents.

## Phase map

| Phase | Adds | Ships when |
|---|---|---|
| 0 | Registries, platform layer, audit log, i18n, CI | Six targets build, CI green |
| 1 | Snapshot: 12 checks, local web UI, HTML+JSON report, redaction | A report worth sending a client |
| 2 | Fixes and guided wizards, restore points, consent flow | Every fix has a passing rollback test |
| 3 | Tier-1 offline explainer, optional Tier-2 LLM, performance and backup analysis | Every check result explained offline, no API key |
| 4 | Patch reporter, screenshot-to-ticket, optional server and dashboard | Compose up, technician sees a real fleet |
| 5 | Remote-help consent wrapper, provisioning, scheduled reports | A monthly client report generates end to end |
