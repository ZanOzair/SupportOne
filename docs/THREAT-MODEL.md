# Threat model

This document is written to be falsifiable. It says what SupportOne protects, who it protects against, where it can be attacked, and — importantly — what it does not defend against. No claim here is "total", "complete", or "100%".

Scope as of Phase 0: the agent binary, its registries, the audit log, and the build pipeline. Sections marked *(Phase N)* describe surfaces that do not exist yet; they are listed so the design is fixed before the code is written, and they will be revised when built.

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

### Local web UI *(Phase 1)*

The agent serves its UI on loopback and opens the user's browser. Planned controls:

- Bind `127.0.0.1` only, on a random high port.
- A random per-session bearer token, required on every request.
- `Origin` and `Host` header validation on every request, which is what actually blocks DNS rebinding — a rebound attacker resolves a hostname to `127.0.0.1` but cannot forge these.
- A strict Content-Security-Policy with no inline script and no remote origins.
- Automatic shutdown when idle.

Residual risk: any process on the machine running as the user can read the token from the process's own environment and connect. This is not a boundary the agent can enforce; a local attacker with the user's rights has already won.

### Fix execution

- Fixes are compiled in. They are reachable only by exact ID lookup against the registry.
- Nothing downloads and executes code at runtime. There is no `eval`, no plugin loading from disk, no script fetched from a server.
- No shell command is assembled from user input or model output. The commands a fix runs are fixed at compile time.
- A mutating action requires confirmation of that specific action, against an explicit list of what will change.
- Elevation is requested for the one action that needs it, at the moment it runs, and dropped after.

Residual risk: a fix is still code that changes a machine. A bug in a fix can break something. This is mitigated by restore points, rollback implementations, and a rollback test per fix — not eliminated.

### AI assistant *(Phase 3)*

The model is untrusted input, not a control path.

- Tier 1 is a deterministic rule engine: offline, no network, no model. It is the default and it alone must make the product useful.
- Tier 2 is off by default, uses the user's own key or local endpoint, and is opt-in after showing exactly what would be sent, redacted.
- The model cannot execute anything. It may only return fix IDs, which are validated against the registry before anything is offered. An unknown ID is discarded silently at the boundary.
- Prompt injection via a machine's own data (a malicious filename, a crafted event log entry) is assumed possible. It is contained by the same rule: the worst a manipulated model can do is name a fix ID, and the whitelist decides whether that ID is real.

### Data leaving the machine

- The agent makes no outbound connection unless the user acts.
- Before sending, the exact payload is shown, serials, hostnames and usernames can be redacted, and the send is confirmed.
- Every send is recorded in the audit log with its destination and byte count.
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
