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

### AI assistant *(Phase 3)*

The model is untrusted input, not a control path.

- Tier 1 is a deterministic rule engine: offline, no network, no model. It is the default and it alone must make the product useful.
- Tier 2 is off by default, uses the user's own key or local endpoint, and is opt-in after showing exactly what would be sent, redacted.
- The model cannot execute anything. It may only return fix IDs, which are validated against the registry before anything is offered. An unknown ID is discarded silently at the boundary.
- Prompt injection via a machine's own data (a malicious filename, a crafted event log entry) is assumed possible. It is contained by the same rule: the worst a manipulated model can do is name a fix ID, and the whitelist decides whether that ID is real.

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

### Fleet server *(Phase 4)*

Planned: Argon2id password hashing, per-tenant data isolation, TLS required, rate limiting, no telemetry, and self-hosting documentation so data can stay in one jurisdiction for PDPA/GDPR purposes. This section will be rewritten with specifics when the server exists.

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
