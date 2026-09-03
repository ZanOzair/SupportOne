# Fixes and guided walkthroughs

Everything in this document changes a machine. Nothing in it changes a machine without being asked, in words, about that specific change.

Four rules hold throughout:

- **Nothing happens until it has been described.** A fix cannot be applied unless its exact list of changes was shown and confirmed back. There is no path from an ID to an action that skips the description.
- **A fix that cannot honestly claim to be reversible says so.** `Reversible()` reports whether `Rollback` restores the prior state. One fix here reports `false`, and this document says exactly why that costs the user nothing.
- **Nothing is deleted.** The two fixes that clear files move them into a quarantine directory on the same volume. Reclaiming the space for good is a separate decision the user makes later, with the files still in front of them.
- **A repair that ran is not a problem that went away.** A walkthrough asks its question again after a change, and records a fix that did not help as a fix that did not help.

## The consent gate

Every change goes through `internal/remediate`, in the same three steps.

**Plan** describes the fix, runs its read-only preflight, and asks the platform whether a restore point can be made. It changes nothing, which is why it is the only step the interface can take on its own.

**Apply** refuses unless all of the following hold:

| Requirement | Why |
|---|---|
| A valid, unused plan token | Proves this exact plan was shown. Tokens are single-use, so one agreement covers one change. |
| An acknowledgement repeating the plan's change list, in order | A caller that cannot reproduce the list did not display it. |
| The preflight did not refuse | A fix's own judgement that the change is unsafe or pointless now. |
| Administrator rights, where the fix declares it needs them | Better to say so than to offer a button that fails. |
| A restore point, or an explicit acceptance of going without one | The absence of a safety net is a thing to be told, not to discover. |

**Rollback** undoes what this session applied. A rollback that fails leaves the fix on the list of things still applied, because forgetting it would leave a change on the machine that nothing records.

Every one of those decisions, including every refusal, is written to the audit log.

In the browser, each change in the list carries its own checkbox. That is not a formality the interface invented: the server compares what comes back against what it described, so ticking the boxes is the only way to produce a confirmation the gate accepts. In the terminal, confirming means typing the fix ID back — printing a list of changes and then acting on `y` would be a confirmation of having read nothing. Silence, from a pipe or a script with nothing on standard input, changes nothing.

## Restore points

A fix's own rollback undoes that fix. A restore point is the wider net, for when something else goes wrong in the same session.

| Platform | Mechanism | When it is unavailable |
|---|---|---|
| Windows | System Restore checkpoint (`Checkpoint-Computer`, `MODIFY_SETTINGS`) | System Restore is switched off, or the agent is not elevated. SupportOne does not switch it on for you as a side effect of a repair. |
| macOS | APFS local snapshot (`tmutil localsnapshot`) | The volume does not support it, or the agent is not elevated. |
| Linux | None | There is no mechanism every distribution has. Btrfs and ZFS snapshots, Timeshift and LVM all depend on how the machine was set up, and a tool that guessed wrong would either fail or quietly do nothing while claiming otherwise. |

A macOS local snapshot lives on the same disk, so it is not a backup. It is what macOS offers as a point to return to, and it is described that way rather than as one.

Where no restore point can be made, the user is told which of these is the reason, and going ahead is a second question with its own answer.

## The three fixes

### `temp.clear` — Windows, macOS, Linux

Moves temporary files that have gone untouched for more than **seven days** into a quarantine directory inside the temporary directory itself, which guarantees a rename on the same volume rather than a copy.

- **Needs administrator rights:** no. It touches the current user's own temporary directory and nothing else. Asking for elevation to tidy your own files would be asking for more than the job needs.
- **Reversible:** yes. `Rollback` moves every file back to where it came from, with its original permissions.
- **Refuses when:** nothing in the directory is old enough to move. A fix that ran anyway and reported success for moving zero files would teach the user that confirming a change is a formality.
- **Leaves alone:** any file another program currently has open. In a temporary directory that is the normal case, not a failure, and the outcome says how many were left.

### `print.clear-spooler` — Windows only

Stops the print service, moves every queued job into quarantine, and starts the service again.

- **Needs administrator rights:** yes, for both.
- **Reversible:** yes. The queued documents — including the one that was jamming the queue, in case it mattered — are moved, not deleted, and `Rollback` puts them back.
- **Refuses when:** the queue is already empty. Stopping and starting the print service to move nothing is a change with no benefit.
- **Always:** starts the service again, including when the work in between fails. A machine left unable to print at all would be a worse outcome than the jam it was called about. If any job cannot be moved, the ones already moved are put back, so a half-cleared queue is never left behind.
- **Windows only, deliberately.** macOS and Linux queue through CUPS, whose spool directory mixes job data with the control files CUPS needs to stay consistent. Clearing that the way this fix clears the Windows queue is not the same operation, and offering it under the same name would be worse than not offering it.

### `net.flush-dns` — Windows, macOS, Linux

Clears the machine's cached record of which names point where.

| Platform | Commands |
|---|---|
| Windows | `ipconfig /flushdns` |
| macOS | `dscacheutil -flushcache`, then `killall -HUP mDNSResponder` |
| Linux | `resolvectl flush-caches`, when systemd-resolved is present |

- **Needs administrator rights:** yes. The cache belongs to the system resolver, not to the user's session.
- **Reversible: no**, and this is the one fix here that says so. The previous cache cannot be put back, and rebuilding it by looking the same names up again would be a different action wearing an undo's name. What is lost is a few milliseconds on the next lookup, not information — the machine refills the cache itself, on demand. "Lost nothing" is still not "put back", so the flag reports `false` and the explanation says why that is fine.
- **Refuses when:** the tool is not installed, or (on Linux) nothing is caching, so there is nothing to clear.

## What is deliberately not here

**Resetting the TCP/IP stack** (`netsh int ip reset` and its equivalents) is the obvious fourth fix, and it is not shipped. It requires a reboot to take effect and cannot be undone: there is no operation that restores the previous stack configuration. That fails the bar every other fix here meets, so rather than shipping it with a rollback that does nothing and a `Reversible()` that lies, it is left out. This is a flagged omission, not an oversight.

**Starting a stopped print service** is not a fix either, though the printing walkthrough detects one. Starting a system service is a change with a wide blast radius and a rollback (stopping it again) that would leave the machine worse than it found it if the service was stopped deliberately. The walkthrough says what to do instead.

## Guided walkthroughs

A walkthrough is a sequence of read-only questions, each with at most one thing to try when the answer is wrong. After anything is changed, the same question is asked again — that re-check is what decides whether the step is recorded as fixed.

A question that could not be answered is never treated as one that passed.

### Outcomes

| Outcome | Means |
|---|---|
| Fixed | Something was wrong, it was repaired, and asking again confirmed it. |
| Unresolved | Something was found and is still outstanding: declined, unavailable, or the repair did not help. |
| Unverified | A change was made whose effect nothing can check. A cache that refills itself looks identical after a useful repair and after a useless one, so the step is recorded as changed, never as fixed, and the user is asked to try the thing again and see. |
| No fault found | Every question came back clean. The complaint is real and this tool cannot see the cause. That is a result, and a more useful one to hand a technician than a confident guess. |
| Stopped | The user ended it part-way. |

### `wizard.connection` — Windows, macOS, Linux

For "I can't get online".

1. **Is this computer connected to anything, and did it get an address?** Reads the interface list the OS already holds. An interface that is up but holds only a link-local address is the everyday "connected to the router, no lease" case and is named separately from having no connection at all. Advice only.
2. **Does it know where to send traffic?** Answered by the `network.config` check, so the walkthrough inherits that check's platform work rather than growing a shallower second copy of it. Advice only.
3. **Is the saved list of website addresses worth clearing?** Offers `net.flush-dns`. Marked unverifiable, for the reason in the outcome table above.

Nothing here sends a packet. A tool that "tested your connection" by contacting a server would be making the outbound connection this agent promises not to make, and would be reporting on that server as much as on the machine.

### `wizard.printing` — Windows only

For "it won't print".

1. **Is the print service running?** Reads `sc query spooler`. First, because a stopped service explains every other answer and the queue cannot be cleared while it is down. Advice only.
2. **Is anything stuck in the queue?** Counts the spool directory. Offers `print.clear-spooler`.
3. **Is a printer set as the default?** Reads `Win32_Printer`. Advice only.

### Escalation

However a walkthrough ends, it hands over the complaint, every question asked, and what came of each — in message keys and step IDs, with no machine identifiers. What to send, and whether to send it, stays the user's decision.

## Adding a fix

1. A new package under `internal/fixes/` implementing the `Fix` interface, with its message keys as exported constants.
2. Entries in every catalog under `internal/i18n/locales/`.
3. One import line in `internal/fixes/all`. That list is the whitelist: a fix not named there cannot be planned, confirmed or applied, whatever asks for it.
4. **A rollback test that proves the machine is as it was.** This is the gate, not a suggestion. A fix whose rollback is a no-op reports `Reversible() == false` and explains, in its `Undo` message, exactly where that leaves the user.
