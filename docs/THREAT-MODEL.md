# Threat model

This document is written to be falsifiable. It says what SupportOne protects, who it protects against, where it can be attacked, and — importantly — what it does not defend against. No claim here is "total", "complete", or "100%".

Scope as of Phase 1: the agent binary, its twelve read-only checks, the local interface, the redaction and report path, the audit log, and the build pipeline. Sections marked *(Phase N)* describe surfaces that do not exist yet; they are listed so the design is fixed before the code is written, and they will be revised when built.

## Assets

| Asset | Why it matters |
|---|---|
| Diagnostic data about the user's machine | Hostnames, serial numbers, usernames, installed software, network config. Identifying, and useful to an attacker profiling a target |
| The user's machine state | Fixes change real system settings. A fix triggered without consent is an attack, not a feature |
| The audit log | The user's record of what was done to their machine. If it can be edited or forged, no other guarantee is checkable |
| The agent binary and its release artifacts | A tampered build is an attacker running as the user, invited |
| Technician credentials *(Phase 4)* | Access to a fleet of machines |

## Adversaries

1. **A malicious website in the user's browser.** The agent serves a local UI; any page the user has open can attempt requests to it.
2. **Another local user or process on the same machine.** Reads the audit log, connects to the local UI port, or tampers with files the agent writes.
3. **A network attacker between the user and anything the agent talks to.** Only relevant once the user chooses to send something.
4. **A compromised dependency or build machine.** Supply-chain compromise of the binary the user is told to trust.
5. **A hostile or manipulated AI response *(Phase 3)*.** The optional Tier-2 model returning content designed to cause a harmful action.
6. **A person with physical access.** Named for completeness; see non-goals.

## Attack surface and controls

### Local web UI

The agent serves its UI on loopback and opens the user's browser. Implemented controls, each covered by a test in `internal/localui`:

- Binds `127.0.0.1` only, on a port the OS assigns from the ephemeral range.
- A 32-byte token from the OS random source, minted per run, required on every `/api/` request and compared in constant time.
- `Origin` and `Host` validated against the listening address on every request, which is what actually blocks DNS rebinding — a rebound attacker resolves a hostname to `127.0.0.1` but cannot forge these headers.
- `Content-Security-Policy: default-src 'none'` with `script-src 'self'`, no inline script, no remote origin, and `frame-ancestors 'none'`.
- Shuts down after 15 minutes unused, or when the user closes the session from the page.

Static files are served without a token. They carry no data — the snapshot reaches the page only through the token-protected API — and requiring one would mean putting the token in every asset request.

Residual risk: any process on the machine running as the user can read the token from the process's own environment and connect. This is not a boundary the agent can enforce; a local attacker with the user's rights has already won.

### Fix execution

- Fixes are compiled in. They are reachable only by exact ID lookup against the registry.
- Nothing downloads and executes code at runtime. There is no `eval`, no plugin loading from disk, no script fetched from a server.
- No shell command is assembled from user input or model output. The commands a fix runs are fixed at compile time.
- A mutating action requires confirmation of that specific action, against an explicit list of what will change. `internal/remediate` enforces this: a plan token is single-use, and an acknowledgement that does not repeat the plan's change list, in order, is refused and logged as a denial.
- Elevation is requested for the one action that needs it, at the moment it runs, and dropped after. A fix that declares it needs rights the agent does not hold is described but not offered.
- A fix that clears files moves them into a quarantine directory rather than deleting them, so the rollback is a rename rather than a restore from nothing.
- Where a change cannot be undone, the fix reports `Reversible() == false` and the interface says so. Nothing claims an undo it does not have.

Residual risks, none of them eliminated:

- A fix is still code that changes a machine. A bug in a fix can break something. Restore points, rollback implementations and a rollback test per fix reduce this; they do not remove it.
- A rollback can itself fail — a file put back where something else now sits, a service that will not restart. The quarantine reports a partial restore rather than claiming success, and the fix stays on the list of things still applied, but the machine is left in a state the user has to be told about.
- On Linux no system restore point exists. The user is told, and the decision to proceed without one is theirs and is recorded.
- A confirmation is only as meaningful as the interface presenting it. The gate can prove the change list was reproduced; it cannot prove a human read it.

### AI assistant

The model is untrusted input, not a control path.

- Tier 1 is a deterministic table compiled into the binary: offline, no network, no model, no key. It is the default and it alone makes the product useful. A guard test fails the build if any check verdict has no explanation.
- Tier 2 is off. It contacts nothing unless a person enabled it, supplied an endpoint, saw the exact bytes that would leave the machine, and confirmed that specific send. Prepare builds the payload and contacts nobody; Ask will not send without the token Prepare issued, and a token is good for one send.
- The model cannot execute anything. It may only return fix IDs, which are resolved against the registry before anything is offered. An unknown ID, a shell string, or a repair that does not run on this platform is discarded at the boundary.
- Its prose is contained rather than trusted: control characters stripped, length capped on a rune boundary, and displayed as the model's words beside the Tier-1 explanation rather than in place of it.
- Prompt injection via a machine's own data (a malicious filename, a crafted event log entry) is assumed possible. It is contained by the same rule: the worst a manipulated model can do is name a fix ID, and the whitelist decides whether that ID is real.
- No credential is stored. The key is read from `SUPPORTONE_ASSIST_KEY` for the life of the process and never written to a config file, the OS keychain, or the audit log. A tool that keeps your API key becomes a place it can leak from.

Residual risks:

- A send is irreversible. Once the payload reaches the endpoint, what the operator of that endpoint does with it is outside this tool's control. That is why it is off, why the bytes are shown rather than summarised, and why the terminal path redacts fully without asking.
- Redaction removes what it is told to remove — hostnames, usernames and home paths, serial numbers, network addresses. A check's evidence could still carry something identifying that none of those categories names, and the preview is the defence: the user reads the actual payload, not a description of it.
- Tier 1's advice is a fixed table written by the authors. It can be wrong for an unusual machine, and it does not learn. It is auditable and identical for everyone, which is the trade being made.

### Data leaving the machine

- The agent makes no outbound connection unless the user acts. This shapes the checks themselves: `updates.os` reads local records rather than asking Windows Update, Apple or a package mirror what is available, because that query would be an outbound connection the user did not request.
- `internal/redact` strips hostnames, usernames and home paths, serial numbers, and IP and MAC addresses, by field name and by value shape. The interface previews the exact payload before anything is written, and the report marks itself as redacted so a reader knows the blanks are deliberate.
- Every saved or sent file is recorded in the audit log with its destination and whether it was redacted.
- There is no telemetry, no analytics, no crash reporting, no "anonymous usage statistics". Not off-by-default: absent.

### Audit log

- Append-only writes, `0600`, in the user's own config directory.
- Field values are escaped so no logged string can forge an entry.
- No credentials or file contents are written.

Residual risk: a process running as the user can delete or truncate the file. Detecting that requires an append-only store outside the user's control, which is out of scope for a tool the user owns and runs. The log is a record for the user, not evidence against them.

### Supply chain

- Dependencies pinned; `go.sum` committed.
- CI runs `govulncheck` and `gosec`, and fails the build on high-severity findings.
- A CycloneDX SBOM is generated per build.
- Releases publish SHA-256 checksums for every artifact.
- Signing (Windows Authenticode, macOS notarization) is documented; unsigned development builds identify themselves as such in their own output.

Residual risk: pinning and scanning reduce exposure to known-vulnerable and typosquatted dependencies. Neither detects a novel backdoor in a legitimate dependency, and reproducible builds prove a binary matches a source tree — not that the source tree is benign.

### Fleet server

Optional, self-hostable, and off unless someone runs it. The agent works fully without it.

- **The arrow points one way.** The server receives reports and has no route that asks a machine anything, runs anything on one, or makes one report again. Absent, not disabled: a test asserts it by trying /api/run, /api/fix, /api/command and /api/collect and requiring every one to be missing.
- Nothing is installed as a service on a reported machine. Every report exists because a person at that machine ran the agent, saw the exact bytes, and typed `send`.
- It refuses to start without a token of at least 24 characters, and compose declares the variable with no default. A default becomes the token every deployment that skipped the step is using.
- The submission API takes a bearer token; the dashboard uses the browser's own credential prompt rather than a session mechanism of our own. Both compare in constant time.
- A machine is filed under a digest of the name its user chose, so the name never reaches a URL, a log line, or a file listing. The server's log records identifiers, never names: a fleet log should not become a directory of whose computer is whose.
- Records are written to a temporary file and renamed into place at 0600. Request bodies are bounded; the dashboard is server-rendered and its Content-Security-Policy permits no script at all.
- The runtime image is `scratch`: a static binary, a CA bundle, and no shell to run if someone gets a command into it. Read-only, all capabilities dropped, no-new-privileges, non-root.

Residual risks:

- **One shared token.** Anyone holding it can submit a report under any name and can read the whole fleet. There are no per-machine credentials and no way to tell two senders apart. That suits a technician and the machines they look after; it is not what tenants who distrust each other would need.
- **A report is a claim, not an attestation.** The server stores what it was sent, and the sender chooses the name it is filed under.
- **Reports are only as fresh as the last send.** Nothing polls, so "out of touch" is a state the server can display and not one it can resolve.
- **TLS is not this server's job.** It speaks plain HTTP and must sit behind a proxy that terminates TLS; compose binds it to loopback until someone decides otherwise.
- The data directory holds other people's machine reports. They are 0600, but anyone with the volume has them.

### Support bundles

A ticket carries a description, the redacted snapshot, the offline advice, and an image the user chose.

- **SupportOne has no screen-capture code.** A screenshot cannot be redacted by field: it captures whatever happened to be visible. The user picks the file, so they have already decided what is in it, and this binary has no capability to abuse.
- Attachments are judged by sniffed content, not by extension, and only images are accepted — a support bundle must not become a way to move anything off a machine under a support label.
- Names are reduced to one safe file name inside `attachments/`, with backslashes treated as separators whatever platform built the bundle, and an attachment cannot take the name of the bundle's own files.
- Writing a bundle is not sending one. It is saved locally and the output says so.

## Non-goals

Naming these honestly is part of the model.

- **Physical access.** A machine an attacker can boot from their own media is not defended by an application.
- **An already-compromised OS.** SupportOne reads what the OS tells it. A kernel-level rootkit can lie to it, and the report will faithfully repeat the lie.
- **Protecting the user from themselves as administrator.** A user who is already an administrator can do anything SupportOne can, without it.
- **Firmware.** Firmware and BIOS information is read where the OS exposes it. Nothing is ever flashed or modified.
- **Anti-malware.** SupportOne is not a scanner, does not detect malware, and does not remove it.
- **Defending the local UI against processes running as the same user.** See above.

## Review

This document is revised at every phase boundary, and any change to the local UI, the fix registry, the assistant boundary, or the send path is expected to update it in the same change.
