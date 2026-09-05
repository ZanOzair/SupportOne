# SupportOne

One small binary that reads a computer, explains what it finds in plain language, and changes nothing without being asked.

SupportOne consolidates ten IT-support functions — diagnostics, safe fixes, guided wizards, provisioning, reporting — into a single codebase with a plugin architecture, serving two audiences: the person with the problem, and the technician helping them.

## Status

**Phase 6 of 6 — everything above, and a release you can check rather than trust.** Fifteen read-only checks, a local web interface, HTML and JSON reports, redaction before anything is saved, three fixes and two guided walkthroughs behind a consent gate, a plain-language explanation of every verdict, an optional model you can point at your own endpoint, a patch statement, a support bundle, an optional fleet server, scheduled monthly reports, provisioning profiles, a consent wrapper in front of remote help — and now a reproducible, signed release with build provenance.

**SupportOne implements no remote desktop protocol and will not.** What Phase 5 added is the sentence before a session and the record after it.

Releases are reproducible, signed with Sigstore, and carry SLSA build provenance. They are **not** Authenticode-signed or Apple-notarized, so **Windows SmartScreen and macOS Gatekeeper will warn about them** — see [docs/RELEASE.md](docs/RELEASE.md) for why, and for how to verify a download instead.

| Phase | Contents | State |
|---|---|---|
| 0 | Registries, platform layer, audit log, i18n, CI | Complete |
| 1 | Snapshot: 12 checks, local web UI, HTML + JSON report, redaction | Complete |
| 2 | Fixes and guided wizards, restore points, rollback | Complete |
| 3 | Offline explainer, optional LLM assistant, performance and backup analysis | Complete |
| 4 | Patch reporter, screenshot-to-ticket, optional fleet server | Complete |
| 5 | Remote-help consent wrapper, provisioning profiles, scheduled reports | Complete |
| 6 | Reproducible release, Sigstore signing, provenance, digest-pinned CI | Complete |

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

## Handing it to a technician

`--ticket` writes a support bundle: your description of the problem, the redacted report, the explanations, and an image you chose. It is saved to your computer and sent nowhere — attaching it to an email is your move, not the tool's.

**SupportOne does not take the screenshot.** A screenshot cannot be redacted by field; it captures whatever happened to be visible. You pick the file, so you have already decided what is in it.

There is also an optional fleet server you can host yourself: `docker compose up`, and a technician sees the machines that chose to report to them. It can only receive — there is no route that asks a machine anything, runs anything on one, or makes one report again. Details and residual risks in [docs/FLEET.md](docs/FLEET.md).

## Letting someone connect

SupportOne implements **no remote desktop protocol**. Writing one means writing screen capture, input injection and transport encryption — three security-critical things this project would do worse than the people who already do them.

What it adds is the part those tools mostly leave out: a moment where you are told, in words, what you are about to let someone do.

`--remote "Aisyah from IT"` prints what a session allows — they can see your screen, type on your keyboard, read any file you can open, and act as you on anything you are signed in to — and waits for you to type `allow`. Then it starts a remote-help program **you already have**, from a short compiled-in list, with no arguments. It never installs one, never configures one, and never connects one to anybody.

And it states its own limit rather than implying more: **once a session starts, SupportOne can see nothing.** It cannot watch the session, restrict it, or end it. The audit log records that you agreed at one time and said it was over at another — an account of a decision, not surveillance of a session. Details in [docs/OPERATIONS.md](docs/OPERATIONS.md).

## Standards and schedules

`--profile front-desk.json` measures a machine against a standard a technician wrote: which checks must pass, how bad each may get, why, and what to offer when one does not. It exits non-zero when the machine does not conform, so it works in a loop over a fleet.

A profile **measures and never changes**. Its `offer` field names repairs to suggest; applying one still needs `--fix` and its own confirmation. A check that could not answer, or that this build does not carry, counts *against* the profile — certifying a machine on the strength of checks that did not run is not a certification.

`--monthly <folder>` writes that month's client report locally, fully redacted, and sends it nowhere. `--schedule <folder>` **prints** the scheduler entry that would run it — with the line that removes it again directly beneath — and installs nothing.

## What runs where

This is the honest scope. It is not "works everywhere".

- **Windows 10/11** — the primary target. Every check runs here. Built for 64-bit, ARM64 and 32-bit.
- **Windows 8.1 and earlier, including Windows 7** — **not supported.** Go 1.24 sets the floor at Windows 10, and no build of this release will start on an older one. That is a hard limit, not a gap to work around.
- **macOS and Linux** — the same codebase and the same interfaces, with platform-specific implementations. A check that cannot exist on a platform is not offered there and the report says so: `drivers.problem` is Windows-only, because neither macOS nor Linux has an equivalent notion of a device in an error state.
- **Phones, tablets, Chromebooks, any browser** — can reach the web interface if the user explicitly enables access beyond loopback. **The agent itself requires a desktop OS**: Windows, macOS, or Linux. That distinction is what "works everywhere" honestly means here.

Nine builds of the agent ship with each release, so most desktop hardware still in use is covered:

| | |
|---|---|
| Windows | `amd64`, `arm64`, `386` — Windows 10 or newer |
| macOS | `amd64` (Intel), `arm64` (Apple silicon) — macOS 12 or newer |
| Linux | `amd64`, `arm64`, `386`, `arm` (ARMv6+, so a Raspberry Pi Zero upward) — kernel 3.2 or newer |

The floors in that table come from the Go toolchain, not from choices this project made, and they are stated rather than left for someone to discover when nothing happens.
- **Firmware** — firmware and BIOS details are read where the operating system exposes them. Nothing is ever flashed or modified.

Some answers need administrator rights on some platforms — SMART attributes on Linux, BitLocker status on Windows. The agent does not require elevation to start; a check that could not read something reports that it could not, and names why.

## What the design actually guarantees

Verifiable properties, not marketing claims. The limits of each are documented in [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md).

- **Read-only by default.** Every check only reads. The only code that changes anything is a fix, and a fix runs only through the consent gate: it is described in full, and it is applied only against a confirmation that repeats that exact description back. A plan is good for one change. Silence is not consent.
- **Always reversible, or it says otherwise.** Every fix implements a rollback, and every fix has a test proving the rollback restores what was there. A fix that cannot honestly claim that reports it cannot, rather than implying an undo that does not exist.
- **No outbound connection, unless you ask for one.** The agent works with the network unplugged, and nothing in the checking, explaining or repairing path contacts anything. The one exception is the optional assistant, which is off, needs an endpoint you supply, and shows you the exact bytes before it sends them. `updates.os` deliberately reads local records rather than asking Windows Update, Apple, or a package mirror what is new — that would be an outbound connection the user did not ask for. There is no telemetry, no analytics, no crash reporting: not disabled, absent.
- **You choose what leaves.** Reports are saved to your own computer. Before saving, you pick what to strip — computer name, username and home folder, serial numbers, network addresses — and can see the exact file that would be written.
- **No dynamic code.** Nothing is downloaded and executed at runtime. Every OS command a check runs is compiled in, passed as separate arguments with no shell involved, and never assembled from user input or model output.
- **Parsers that do not fall over.** Every parser that reads operating-system output is fuzzed, and every input that has ever crashed one is committed as a permanent test case that runs on each build. Two bugs were found this way and fixed.
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
| `--report` | Offer to send this report to the fleet server named below |
| `--fleet-server` | The fleet server's address; HTTPS, or http only on this computer |
| `--fleet-name` | What this machine should be called in that dashboard |
| `--ticket <path>` | Write a support bundle to a file or folder |
| `--profile <path>` | Measure this computer against a profile; exits non-zero if it does not conform |
| `--monthly <dir>` | Write this month's client report into a folder, fully redacted, and exit |
| `--schedule <dir>` | Print the scheduler entry that would do that monthly, and exit |
| `--list-remote-tools` | List the remote-help programs already installed here |
| `--remote <name>` | Agree to a remote-help session with the person named |
| `--remote-tool <id>` | Which installed remote-help program to start |
| `--describe` | Your own description of the problem, to go in the bundle |
| `--attach` | Image files to include in the bundle, comma separated |
| `--no-browser` | Print the interface address instead of opening a browser |
| `--lang` | Language tag, `en` or `ms` (defaults to the system language) |
| `--dry-run` | Report what would change without changing anything |
| `--audit-log` | Path to the audit log (defaults to your config directory) |
| `--timeout` | Time limit for a single check |
| `--idle-timeout` | Close the interface after this long unused |
| `--version` | Build information |

`--fix` prints exactly what will change and then asks you to type the repair's ID. Anything else — `y`, `yes`, a blank line, or nothing at all because the command was run from a script — leaves the computer untouched. Where no restore point can be made, that is a second question with its own answer. If the repair can be undone, the offer to undo it comes straight after, because the record of what was applied lives in the process that applied it.

`--version` prints what the build says it is, and then says what that is worth: a version a program prints about itself is not evidence, because a changed copy prints whatever it was changed to print. It points at the check that does mean something.

Builds you make yourself say they are development builds, and a build made from a tree with uncommitted changes says that too rather than quoting a commit hash it does not match.

## Downloading it

Releases carry every artifact, a `SHA256SUMS`, a Sigstore signature over it, and a `BUILD-INFO.txt` naming the commit and the exact Go toolchain that built them.

```sh
sha256sum -c SHA256SUMS --ignore-missing

cosign verify-blob SHA256SUMS \
  --signature SHA256SUMS.sig --certificate SHA256SUMS.pem \
  --certificate-identity-regexp 'https://github.com/ZanOzair/SupportOne/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Or skip trusting this project entirely and rebuild it: `scripts/release.sh` produces byte-identical files from the same commit and toolchain, in any directory, at any time. Every release runs that comparison against itself — one job builds, a second rebuilds from a fresh checkout without being told the first job's answers, and the release fails if they differ.

**These downloads are not Authenticode-signed or Apple-notarized, so your operating system will warn you about them, and it is right to.** Both need a certificate tied to a verified legal identity, which is an annual purchase this project has not made. [docs/RELEASE.md](docs/RELEASE.md) says what that would take, and what to check in the meantime.

## Working on it

```sh
go test ./...                     # Go tests, including golden-file rendering and every fuzz seed
go test ./internal/report -update # rewrite the golden report after an intended change
scripts/fuzz.sh                   # hunt for new parser crashes (30s per target)
scripts/release.sh                # build every release artifact, reproducibly

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
- [docs/FLEET.md](docs/FLEET.md) — the optional server, why it can only receive, and what it does not protect against
- [docs/OPERATIONS.md](docs/OPERATIONS.md) — monthly reports, profiles, remote help, and what each deliberately does not do
- [docs/RELEASE.md](docs/RELEASE.md) — verifying a download, rebuilding it yourself, and exactly what is and is not signed
- [SECURITY.md](SECURITY.md) — how to report a vulnerability, and what counts as one here
- [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md) — assets, adversaries, controls, residual risks, non-goals

## License

Apache License 2.0. See [LICENSE](LICENSE).
