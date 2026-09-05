# Security

## Reporting a vulnerability

Use GitHub's private reporting: **[Report a vulnerability](https://github.com/ZanOzair/SupportOne/security/advisories/new)**
(the Security tab → Advisories → Report a vulnerability).

Please do not open a public issue for a vulnerability. A public issue is a
working description of how to attack every installed copy, published before
anyone can update.

What helps most, in rough order:

- What an attacker gets. "Reads a file they should not" is a different report
  from "runs a command as the user."
- How to reproduce it, ideally with the commands you ran.
- The version (`supportone-agent --version`) and the operating system.
- Whether it needs the attacker to be on the machine already, on the same
  network, or neither.

You do not need a proof-of-concept exploit. A clear description of the flaw is
enough to act on.

### What to expect

This is a small project, so the honest answer about timing is that a report
gets looked at when there is someone to look at it — not within a guaranteed
window. What is promised:

- An acknowledgement that the report arrived.
- A plain answer about whether it is a real issue and what happens next,
  including "this is a known limitation, here is where it is documented."
- Credit in the advisory when it is fixed, unless you would rather not be
  named.

## What counts as a vulnerability here

SupportOne's design is documented in [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md),
which names what it defends against and — just as importantly — what it does
not. Some things that look like findings are documented non-goals, and saying
so is not a brush-off; it is the model working as written.

**These are vulnerabilities. Please report them.**

- Anything that changes the machine without passing the consent gate: a fix
  that applies without its plan token and acknowledgement, a wizard step that
  acts before the user answers, a profile that repairs something.
- Anything that sends data off the machine without an explicit confirmation of
  that specific payload.
- A way to make the agent run a command that is not compiled in, or to inject
  into one that is.
- A way to reach the local interface from outside the machine, to bypass its
  session token, or to get a page on another site to talk to it.
- A fix whose rollback does not restore what was there.
- A credential, token or file content written to the audit log.
- A way to make a report or ticket contain identifying detail that redaction
  was asked to remove.
- A way to make the agent report a machine as healthier than it is.
- Anything that makes a release's signature, checksums or provenance say
  something untrue.

**These are documented limitations, not vulnerabilities.**

- Downloads are not Authenticode-signed or Apple-notarized, so the operating
  system warns about them. See [docs/RELEASE.md](docs/RELEASE.md); it needs a
  paid, identity-verified certificate.
- The agent cannot verify its own integrity. No binary can; `--version` says so.
- An administrator can do anything the agent can, without it.
- A compromised operating system can lie to every check, and the report will
  faithfully repeat the lie.
- Another process running as the same user can reach the local interface. It
  could also read the same files directly.
- SupportOne cannot see or restrict a remote-help session once one starts. It
  records the consent, not the session.
- A signature says these bytes came from this workflow at this commit. It is
  not a review of the code.

If you think one of these is worse than the documentation admits, that is worth
reporting too — a limitation described too gently is its own kind of bug.

## Supported versions

The latest release. This project does not backport fixes to older tags.

## How the code defends itself

Not claims to trust — things you can check, and where to check them.

| | |
|---|---|
| Every OS command is compiled in, passed as separate arguments, no shell | `internal/platform/oscmd.go` |
| Every change goes through one consent gate | `internal/remediate` |
| Every fix has a rollback with a test proving it restores | `internal/fixes/*/` |
| Two places can open a connection, both behind the same rule | `internal/egress` |
| Parsers are fuzzed; every input that ever crashed one is a permanent test | `scripts/fuzz.sh`, `*/testdata/fuzz/` |
| Static analysis, vulnerability scanning and an SBOM run in CI | `.github/workflows/ci.yml` |
| Releases are reproducible, and every release rebuilds itself to prove it | `.github/workflows/release.yml` |

There is no telemetry, no analytics and no crash reporting: not disabled,
absent. Nothing about your machine reaches this project unless you send it.
