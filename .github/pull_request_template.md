## What this changes

<!-- What it does, and why that was the right trade. If it costs something,
     say so here rather than leaving it to be discovered. -->

## How it was checked

<!-- What you actually ran, not what you believe to be true. If something could
     not be verified — no Windows machine to hand, say — write that down. An
     honest gap is more useful than an assumed pass. -->

- [ ] `go build ./...`
- [ ] `go test ./...`
- [ ] `go vet ./...`, including the cross-compiled targets
- [ ] `golangci-lint run` with the version pinned in `ci.yml`
- [ ] Tried it on a real machine

## Rules

- [ ] Nothing changes the machine without a specific per-action confirmation
- [ ] Nothing is downloaded and executed at runtime
- [ ] No new outbound connection that is not behind the egress gate
- [ ] Any new repair has a rollback, and a test proving the rollback works
- [ ] No telemetry
- [ ] `docs/THREAT-MODEL.md` updated, if this reaches the network, runs a new OS
      command, or writes outside the user's own directories
