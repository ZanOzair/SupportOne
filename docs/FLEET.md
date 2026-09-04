# The fleet server

Optional, self-hostable, and off unless someone runs it. The agent works fully without it: nothing in the diagnostic, explaining or repairing path needs a server, and a person who never runs one loses nothing but the fleet view.

## What it is, and what it deliberately is not

It receives reports that machines chose to send, and shows a technician what those machines reported.

**The arrow points one way.** There is no route that asks a machine anything, runs anything on one, or makes one report again. Not "disabled", not "planned" — absent. A test asserts it by trying `/api/run`, `/api/fix`, `/api/command` and `/api/collect` and requiring every one to be missing.

That shapes what a dashboard here means. It is a record of what people told you, not a feed from machines you are watching. A machine that stops reporting is shown as out of touch, and the server cannot do anything about that, because making a machine report is exactly the capability it does not have.

**Nothing is installed as a service.** No daemon, no scheduled task, no agent running in the background. Every report exists because a person at that machine ran the agent, read what it found, saw the exact bytes, and typed `send`.

## Running it

```sh
export SUPPORTONE_FLEET_TOKEN="$(openssl rand -base64 32)"
docker compose up --build
```

Then open http://localhost:8080. It asks for a password: that is the token, with any username.

There is no default token, and compose fails to start without one. A default would quietly become the token every deployment that skipped this step is using.

The image is built from source in two stages and the runtime stage is `scratch`: the static binary, a CA bundle, and nothing else. There is no shell in it to run if someone gets a command into it. It runs as UID 65532, read-only except for its data volume, with all capabilities dropped and `no-new-privileges`.

### TLS

The server speaks plain HTTP and does not terminate TLS. Put it behind a reverse proxy that does — Caddy, nginx, Traefik, whatever you already run — before exposing it beyond a trusted network.

Until you have, compose binds it to loopback. `SUPPORTONE_FLEET_BIND=0.0.0.0:8080` opens it up, and is a decision to make deliberately rather than a default to inherit.

## Storage

Files, not rows. Someone who wants to run this should not also have to run a database.

One JSON file per machine, under the data directory, written to a temporary name and renamed into place at `0600`. A reader never sees a half-written record, a crash mid-write cannot destroy the previous one, and other accounts on the host cannot read the fleet.

Each machine keeps its **20 most recent reports**. A fleet view is about what is true now and what changed recently; an archive is a different product with different retention questions.

## Identity

A machine is filed under a name **the person at that machine chose** — `--fleet-name "Reception PC"`. There is no fallback to the hostname. What a machine is called in someone else's dashboard is a decision, not something to harvest.

The record is keyed by a digest of that name, so:

- the same machine updates rather than duplicating;
- capitalisation and stray spaces do not split one machine into two;
- the name never reaches a URL, a log line, or a file listing on the server.

The server's log records the identifier and never the name. A fleet server's log should not become a directory of whose computer is whose.

## Authentication

| Surface | Credential | Why |
|---|---|---|
| `POST /api/report` | Bearer token | What an agent presents. |
| The dashboard | HTTP Basic, any username, the token as password | The browser already knows how to prompt for this. Nothing here invents a session. |
| `GET /healthz` | None | A load balancer should not have to be handed the fleet's credential, and learns nothing from asking. |

Both are compared in constant time. The server refuses to start with a token shorter than 24 characters.

This is one shared secret, not per-machine credentials. That is the honest limit of it: anyone holding the token can submit a report under any name, and anyone holding it can read the fleet. It suits the shape this is built for — a technician and the machines they look after — and it is not what you would want for tenants who do not trust each other.

## Sending a report

Reporting is the second and last thing the agent does that leaves the machine, and it is gated exactly like the assistant.

```sh
export SUPPORTONE_FLEET_TOKEN="the same token the server has"
./supportone-agent --text --report \
  --fleet-server https://fleet.example.com \
  --fleet-name "Reception PC"
```

| Rule | Behaviour |
|---|---|
| Off by default | `--report` is required, and there is no default server. Without `--fleet-server` and `--fleet-name` it is an error, not a fallback. |
| HTTPS except here | Plain HTTP is allowed only on loopback, for trying a server out on the same box. |
| Show, then send | The exact bytes are printed before anything leaves. Not a summary — the payload. |
| One agreement, one send | The confirmation is bound to that specific payload and is good once. |
| Silence is not consent | Anything but typing `send` — a blank line, a pipe, a script — sends nothing. |
| Always redacted | The terminal path strips hostname, username, serial numbers and addresses. Someone reporting into a fleet is not weighing each field, and the protective choice is the one to make when they are not asked. |
| Recorded | `DATA_SENT` in the audit log with the host, byte count, whether it was redacted, and whether it was delivered — including the sends that failed. |

The credential is read from `SUPPORTONE_FLEET_TOKEN` and never written anywhere: not a config file, not the OS keychain, not the audit log. A missing token is reported *before* a payload is built and a decision taken on it, rather than after.

## What a technician sees

Machines worst-first — most urgent findings, then most needing attention, then most recently seen. Each one links to its latest report, rendered with the same offline explanation the person at the machine read, so both ends of a support conversation are looking at the same words.

A report the sender redacted is marked as redacted, so the blanks read as deliberate rather than as missing data.

An empty dashboard says why it is empty and how a report arrives, rather than looking broken.

## Residual risks

- **One shared token.** Anyone holding it can submit under any name or read the whole fleet. There are no per-machine credentials and no way to tell two senders apart.
- **A report is a claim.** The server stores what it was sent. A machine name is chosen by the sender, so nothing stops a holder of the token filing a report under someone else's name.
- **Reports are only as fresh as the last send.** Nothing polls. "Out of touch" is a real state the server can display and not one it can resolve.
- **The data directory holds other people's machine reports.** They are `0600` and the container is read-only apart from that volume, but anyone with the volume has them.
- **TLS is not this server's job.** Run it behind a proxy that terminates TLS. Until then, keep it on loopback.
