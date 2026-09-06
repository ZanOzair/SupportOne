# Contributing

## Building it

You need Go 1.24 or newer. Nothing else — the interface is already built and
embedded in the repository.

```sh
git clone https://github.com/ZanOzair/SupportOne.git
cd SupportOne
go build -o supportone-agent ./cmd/supportone-agent
./supportone-agent
```

To work on the interface itself you also need Node 22:

```sh
cd web/agent-ui
npm ci
npm run build     # writes dist/, which is committed
```

`dist/` is committed deliberately, and CI fails if it does not match the sources.
A binary that embeds the interface should not need a JavaScript toolchain to
build.

## The checks a change has to pass

CI runs all of these, so run them before pushing.

```sh
go build ./...
go test ./...                       # 80% coverage floor on internal/
go vet ./...
GOOS=windows go vet ./...           # and 386, arm64, darwin, linux/arm
golangci-lint run                   # includes gosec
govulncheck ./...
```

**Use the exact linter version CI uses.** It is in `.github/workflows/ci.yml` as
`GOLANGCI_LINT_VERSION`. An older one reports zero issues on code CI rejects; this
has cost real time on this project more than once.

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@<that version>
```

Tests run on Windows as well as Linux in CI. If you assert POSIX permission bits,
put the test behind `//go:build !windows`.

### Fuzzing

Parsers are fuzzed. `scripts/fuzz.sh` finds every target and runs it. Crashing
inputs are committed under `testdata/fuzz/` and replay on every `go test`, so a
fixed bug stays fixed.

### Reproducibility

`scripts/release.sh` must produce byte-identical output on any machine. The
release workflow builds everything twice on separate runners and refuses to sign
anything if they differ.

If you verify this by hand, **make the two checkouts differ in more than their
path — deliberately age one of them.** Two clones made in the same second share
their file timestamps and will agree for a reason that has nothing to do with the
build. That has hidden a real bug here before.

## Rules that are not negotiable

These are the point of the project, not style preferences.

- **Read-only by default.** A diagnostic never changes the machine. Anything that
  does requires a specific, per-action confirmation stating exactly what will
  change.
- **Least privilege.** Never require administrator rights to start. Ask for
  elevation only for the action that needs it, at the moment it runs.
- **Zero egress by default.** No outbound connection unless the user clicks to
  send, having first seen the exact payload with the chance to redact it.
- **No dynamic code.** Nothing downloaded and executed at runtime. No `eval`. No
  shell command built from user input or model output. Repairs are compiled in
  and reachable only by exact ID.
- **Always reversible.** Every repair implements a working rollback, with a test
  proving it.
- **No telemetry.** None. Not anonymous, not aggregated, not optional.
- **Honest language.** No fake urgency, no invented problem counts. Report zero
  problems plainly when there are zero. Never claim anything is "100% secure" or
  "100% compatible" — see [SECURITY.md](SECURITY.md).
- **No unnecessary files.** No speculative abstractions for features that are not
  in scope yet.

A change that reaches the network, runs a new OS command, or writes outside the
user's own directories needs a note in [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md)
saying what it does and what it does not protect against.

## Commits

[Conventional Commits](https://www.conventionalcommits.org): `feat:`, `fix:`,
`docs:`, `test:`, `chore:`, with an optional scope — `fix(windows):`.

Write the body for somebody reading it in a year with no memory of the
conversation. Say what changed and why that was the right trade, including what
it costs. If something is unverified, say so; a commit that overstates what was
checked is worse than one that admits a gap.

## Releases

[docs/RELEASE.md](docs/RELEASE.md) has the process. In short: push a `v*` tag and
the workflow builds, rebuilds, compares, signs and publishes. It publishes nothing
if the two builds differ.

Old releases are never deleted. A version that shipped stays available with its
checksums and signature, so anyone can go back to it.

## Where things live

| | |
|---|---|
| `cmd/` | The two programs: the agent, and the optional fleet server |
| `internal/checks/` | One package per check |
| `internal/fixes/` | Repairs, each with its rollback |
| `internal/platform/` | Everything OS-specific, including process spawning |
| `internal/localui/` | The loopback HTTP server |
| `web/agent-ui/` | The interface (React), built into `dist/` |
| `scripts/release.sh` | The whole release build |
| `docs/` | Architecture, threat model, checks, operations, release |

[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) explains why the pieces are shaped
the way they are, which is more useful than the list above.
