# Operations

Phase 5 adds three things a technician runs and one thing a user agrees to:
scheduled monthly reports, provisioning profiles, and the consent wrapper in
front of a remote-help session.

None of them is a new way for SupportOne to reach the network, change a
machine, or watch anybody. Each is documented below with what it does and,
more importantly, what it deliberately does not.

---

## Monthly reports

```sh
supportone-agent --monthly ~/Documents/SupportOne
```

Runs the checks, redacts the result, and writes two files into that folder:

```
supportone-2026-09.html
supportone-2026-09.json
```

The period comes from the snapshot's own timestamp, so a report written on the
first of the month is filed under the month it describes. Running it twice in
one month **replaces** that month's pair rather than adding to it — a scheduler
that runs for three years leaves 72 files, not 1,100.

The HTML is the same report the interface saves: self-contained, no scripts, no
remote references, and it carries the plain-language explanation of every
finding so it still says something useful when it is read in December.

### It always redacts, and it sends nothing

This is the one command most likely to run with nobody watching. Two decisions
follow from that:

- **It redacts everything, without asking.** Computer name, username, serial
  numbers and network addresses are removed. Every other path in SupportOne
  lets you choose what to strip, because you are there to choose. Here nobody
  is, and the protective answer is the one to take by default.
- **It sends nothing, anywhere.** A monthly report that mailed itself would be
  an outbound connection nobody agreed to at the moment it happened. The files
  are on the disk; moving them is a person's job.

If you want the report to reach a technician, `--report` with a fleet server is
the path, and it is interactive by design: it shows you the exact bytes and
waits for you to type `send`. See [FLEET.md](FLEET.md).

## Scheduling it

```sh
supportone-agent --schedule ~/Documents/SupportOne
```

This **prints** the scheduler entry for the platform you are on. It installs
nothing.

| Platform | Mechanism | Where it goes |
|---|---|---|
| Windows | Task Scheduler | a `schtasks /Create` line to paste into a Command Prompt |
| macOS | launchd | a property list for `~/Library/LaunchAgents`, plus the `launchctl load` line |
| Linux | cron | a crontab line, added with `crontab -e` |

All three run at 07:00 on the first of the month, so the report exists before
the person who asked for it goes looking.

Two reasons it prints rather than installs. First, adding a scheduled task is a
change to a machine, and every change in SupportOne goes through the consent
gate as a fix with a working rollback — a scheduled task installed by a
diagnostic tool, silently, is exactly the pattern this project exists to not be.
Second, the printed entry is the version that survives: a year from now,
somebody asking what this thing on their computer is can read the command, see
that it writes files to a folder, and read the undo line printed directly
beneath it.

The entry runs `--monthly`, which is unattended, redacts fully, and contacts
nothing. There is nothing in the scheduled path that can change the machine.

---

## Provisioning profiles

A profile is a technician's written standard: which checks a machine must pass,
how bad each may get, why, and what to offer when it does not.

```sh
supportone-agent --profile front-desk.json
supportone-agent --profile front-desk.json --json    # for scripts
```

```json
{
  "schema": 1,
  "name": "Front desk laptops",
  "expectations": [
    {
      "check": "disk.volumes",
      "worst": "attention",
      "why": "Below 10% these machines stop taking card payments.",
      "offer": ["temp.clear"]
    },
    { "check": "security.posture", "worst": "ok" },
    { "check": "backup.status", "worst": "attention" }
  ]
}
```

`worst` is the worst verdict that still counts as met: `ok` means the check must
be clean, `attention` tolerates a warning, `urgent` tolerates anything the check
can report.

The exit status is non-zero when the machine does not conform, so this works in
a loop over a fleet. The report is printed either way — the exit code summarises
what was already said, it does not replace it.

### What a profile cannot do

- **It cannot change anything.** `offer` names repairs to *suggest*. Nothing in
  a profile applies a fix; applying one still requires `--fix`, its own
  description, and its own confirmation. A file that could silently repair
  machines would be dynamic code with extra steps.
- **It cannot name a repair this build does not carry.** Every ID in `offer` is
  resolved against the compiled-in registry for this platform before it is
  shown. What is not there is not offered.
- **It cannot smuggle a rule past you.** An unknown field is a parse error, not
  a shrug: a misspelled `"expectatiosn"` would be a rule that silently does not
  apply, which in a compliance document is worse than a refusal to load.
- **It cannot pass a machine nobody looked at.** A check that could not answer,
  or that this build does not carry, counts *against* the profile — reported as
  "no answer" or "not here", never folded into the met column. Certifying a
  machine on the strength of checks that did not run is the failure mode this
  rule exists to prevent.

Two expectations for one check are refused, because a profile that contradicts
itself would report both answers.

---

## Remote help

SupportOne implements **no remote desktop protocol**, and will not. Doing so
means writing screen capture, input injection and transport encryption — three
security-critical things this project would do worse than the people who
already do them.

What it adds is the part those tools mostly leave out.

```sh
supportone-agent --list-remote-tools
supportone-agent --remote "Aisyah from IT" --remote-tool rustdesk
```

The second command prints, before anything starts:

```
Aisyah from IT is asking to connect to this computer. Here is what that lets them do:

  - They can see your screen: everything on it, including anything you open by accident.
  - They can move your mouse and type on your keyboard as if they were sitting here.
  - They can open and read any file you can open, including personal ones.
  - Anything they do is done as you: websites you are signed in to are signed in for them too.
  - SupportOne cannot watch this session or limit what they do. It only records
    that you agreed to it and when you said it was over.
  - You can end it at any time by closing the remote-help program.
```

Then it waits for the word `allow`. `y`, `yes`, a blank line, or a script piping
nothing all leave the machine untouched and nobody let in — and the refusal is
written to the audit log as `CONSENT_DENIED`, because a record that shows a
question asked and then nothing is a worse record than one that says the answer
was no.

The wording is blunt on purpose. "The technician can see your screen" is a
sentence people skim. "They can read any file you can open" is one they stop at,
and stopping at it is the entire point of the panel.

### The whitelist

`--list-remote-tools` reports which of a small compiled-in list is already
installed: RustDesk, AnyDesk, TeamViewer, plus Quick Assist on Windows and
Screen Sharing on macOS. Each is looked for on `PATH` and in the fixed places
its installer uses.

- **SupportOne never installs one.** Not a download, not a package-manager
  invocation, not a link. Downloading and running code at runtime is the one
  thing this project does not do, and an installer is that with a friendly name.
- **It never configures one.** It does not create an account, register an
  unattended-access password, set a relay server, or connect the program to
  anybody. It opens the program with no arguments; the program shows its own
  code, and giving that code to the technician is the user's act.
- **It can start nothing else.** The path it launches is the one it resolved for
  a compiled-in entry. No part of it comes from user input, no arguments are
  passed, and no shell is involved.
- **Being on the list is not an endorsement.** These are programs common enough
  that finding one already installed is worth reporting.

If the tool you want is not on the list — or is not installed — the session can
still be recorded. Run `--remote` with no `--remote-tool`, and SupportOne takes
the consent and writes the record while you open the program yourself. Refusing
to make a record would only mean the session happened without one.

### The honest limit

**Once a session starts, SupportOne can see nothing.**

It cannot watch what the technician does, cannot restrict it, cannot log it, and
cannot end it. Closing the remote-help program is what ends a session; the
button in the interface, and the Enter key in the terminal, close the *record*.

The audit log says exactly that much and no more:

```
2026-09-04T09:28:24Z	CONSENT_ASKED	remote session	technician=Aisyah from IT	tool=rustdesk
2026-09-04T09:28:31Z	CONSENT_GIVEN	remote session	technician=Aisyah from IT	tool=rustdesk
2026-09-04T09:28:31Z	REMOTE_STARTED	remote session	launched=true	technician=Aisyah from IT	tool=rustdesk
2026-09-04T09:51:02Z	REMOTE_ENDED	remote session	duration=22m31s	technician=Aisyah from IT
```

That is an account of a decision, not surveillance of a session, and it is
written in those terms rather than in ones that imply more. A session left open
because the agent was closed stays open in the record — nobody told SupportOne
it was over, and inventing an end time would be a worse record than an honest
gap.

One session at a time. A second confirmation while one is open is refused, and
the refusal names who is already in.

---

## What this does not do

Stated plainly, because a support tool is judged by what it declines to build:

- No unattended access. There is no way to configure a session that starts
  without somebody present agreeing to it. A remote-help tool that can be
  triggered remotely is a backdoor, whoever installs it.
- No session recording, keystroke capture, or screen capture. Not of the
  technician, not of the user.
- No credential storage of any kind. SupportOne holds no password, token or
  key for any remote-help tool, and no configuration file it writes contains
  one.
- No profile-driven changes. A profile measures; it never applies.
- No scheduled changes. The only scheduled path is `--monthly`, which reads.
