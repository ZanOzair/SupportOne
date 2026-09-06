# When something is wrong

Every problem in this list has actually happened. If yours is not here, please
[open an issue](https://github.com/ZanOzair/SupportOne/issues) — the answer being
missing is itself a bug in the documentation.

Before anything else: **which version are you running?** Open SupportOne and read
the line under "Your computer's health", or run

```
"%LOCALAPPDATA%\Programs\SupportOne\supportone-agent.exe" --version
```

More than one problem below is really "an older version is still installed".

---

## Installing

### "Windows protected your PC" / "unrecognized app"

Expected. Click **More info** → **Run anyway**.

The installer is not signed with an Authenticode certificate, which needs an
annual purchase tied to a verified legal identity. The warning is about the
absence of that certificate, not about the contents of the file. Verify the
download yourself instead — [INSTALL.md](INSTALL.md#checking-the-download-is-genuine)
takes thirty seconds and proves more than a signature does.

### macOS: "cannot be opened because the developer cannot be verified"

Same situation. **System Settings → Privacy & Security**, scroll down, click
**Open Anyway**. macOS asks once.

### I installed a new version but it still shows the old one

The old one was running, so its files could not be replaced.

1. Close SupportOne.
2. **Ctrl+Shift+Esc** → find `supportone-agent.exe` → **End task**.
3. Install again.

To be certain: uninstall first (Settings → Apps → SupportOne → Uninstall), check
that `%LOCALAPPDATA%\Programs` has no SupportOne folder, then install.

### The checksum does not match

Delete the file and download it again — a truncated download is the usual cause.
If a fresh download still does not match, **do not install it**, and
[open an issue](https://github.com/ZanOzair/SupportOne/issues) saying which file
and which version. That is worth knowing about publicly.

---

## Running

### It opens a browser tab instead of its own window

**On Windows**, SupportOne draws its own window, and falls back to a browser only
when it cannot. Two reasons:

1. **`WebView2Loader.dll` is not next to `supportone-agent.exe`.** If you
   unpacked the `.zip` by hand, both files must stay together. The installer
   handles this for you.
2. **The WebView2 runtime is not installed.** It ships with Windows 11 and
   reaches Windows 10 through Edge, so this is rare — Windows Server and some
   LTSC builds are the usual exceptions. Microsoft distributes the runtime
   separately if you want it.

**On macOS and Linux**, a window without browser furniture needs an installed
Chromium-family browser (Chrome, Edge, Brave, Chromium, Vivaldi). With only
Safari, or a minimal Linux desktop, you get an ordinary tab. This is a deliberate
trade — [ARCHITECTURE.md](ARCHITECTURE.md) explains what closing it would cost.

The page and everything on it are identical either way.

### Black windows flash on and off while the checks run

Fixed in **v1.3.2**. Older versions started their system tools in a way that let
Windows give each one a console window.

If you see this on v1.3.2 or later, please open an issue — say which Windows
version you are on, and whether Settings → Privacy & security → For developers →
Terminal is set to Windows Terminal, Windows Console Host, or Let Windows decide.

### A black terminal window opens behind the interface

Fixed in **v1.2.0**. Install a newer version.

### The program keeps running after I close the window

Fixed in **v1.3.0** — closing the window now stops the program.

On older versions it shut down after fifteen minutes idle. You can end it from
Task Manager, or click **Close** inside the page, which always worked.

### Nothing happens when I double-click it

Give it half a minute — the checks run before anything appears.

If nothing appears after a minute, run it from a Command Prompt:

```
"%LOCALAPPDATA%\Programs\SupportOne\supportone-agent.exe"
```

It borrows the prompt's console, so any error is printed where you can read it.
Include that text in an issue.

### The window opens but stays empty, or no results appear

Run it with `--text`, which does the same checks and prints to the terminal:

```
"%LOCALAPPDATA%\Programs\SupportOne\supportone-agent.exe" --text
```

- **Results appear** — the checks are fine and the interface is the problem.
- **No results** — the checks themselves are failing, and the output will say
  which one and why.

Either way, that output is the useful thing to attach to an issue.

### A check says "unknown" instead of an answer

That is the intended behaviour, not a failure. A check reports "unknown" when the
tool it needs is missing or refused, and names it. A diagnostic that guesses
rather than admitting a gap is worse than useless. [CHECKS.md](CHECKS.md) lists
what each check needs.

### It says a repair needs administrator rights

Correct. SupportOne starts without administrator rights on purpose, and asks for
elevation only for the specific action that needs it, at the moment it runs.
Diagnostics never need it — only some repairs do.

### My antivirus flagged it

Report it to your antivirus vendor as a false positive, and please open an issue
here naming the product and the detection.

Unsigned executables from the internet attract heuristic detections; that is what
heuristics are for. What you can check independently: the file matches the
published checksum, the checksum is signed by this project's build, and the build
is reproducible from the commit named in `BUILD-INFO.txt`. Nothing here downloads
or executes code at runtime — see [THREAT-MODEL.md](THREAT-MODEL.md).

---

## Privacy and network

### Does it send anything anywhere?

No, unless you click to send it.

The agent makes no outbound connection on its own. There is no telemetry, no
analytics, no crash reporting, no update check. The interface is served from
`127.0.0.1` on a random port, reachable only from your own machine.

Two optional features can reach outside, and both refuse to move until you have
seen the exact payload and confirmed it: sending a report to a fleet server, and
asking a model endpoint you configure. Both are off unless you turn them on.

### Something is listening on a port — is that it?

Yes, while it is open. `127.0.0.1` only, on a port the operating system picks, with
a random token required on every request. Nothing outside your computer can reach
it. It stops when you close the window. [THREAT-MODEL.md](THREAT-MODEL.md) has
the detail.

---

## Reporting a problem

Please include:

1. The version (`--version`)
2. Your operating system and version
3. What you expected, and what happened
4. The output of `--text` if it is about a check

The audit log is a plain text file you can read first — everything SupportOne did
is in it, in order, and it never records secrets.
