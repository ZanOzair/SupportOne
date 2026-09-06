# Changelog

Every released version, what changed in it, and whether it is worth installing.

Old releases are never removed. If a newer version breaks something for you, the
one before it is still on the [releases page](https://github.com/ZanOzair/SupportOne/releases)
with its checksums and signature intact, and you can go back to it.

Versions follow [semantic versioning](https://semver.org): the middle number
changes when something is added, the last when something is fixed.

---

## v1.3.2 — the one to use

**Released 5 September 2026 · commit `75b3187`**

Fixed: on Windows, the system tools each check runs were opening a console
window of their own, so a black window flickered on and off for the whole time
the checks ran.

This was a consequence of v1.2.0 making the agent a windowed program. A process
with no console that starts a console program does not get to skip the console —
Windows creates a new one for the child. The tools are now started with both
`CREATE_NO_WINDOW` and `SW_HIDE`: the first says do not make a console, the
second says that if one is made anyway, do not show it.

v1.3.1 set only the first flag, on the strength of Microsoft's documentation
saying it is sufficient. On a real machine it was not.

**If you are installing SupportOne for the first time, install this one.**

---

## v1.3.1

**Released 5 September 2026 · commit `eb27a66`**

First attempt at the flashing console windows above. Sets `CREATE_NO_WINDOW`
only. Superseded by v1.3.2; there is no reason to choose this one.

---

## v1.3.0 — SupportOne draws its own window

**Released 5 September 2026 · commit `a6f4a41`**

Added: on Windows, the interface now opens in a window the program creates and
owns, using the WebView2 runtime that is part of Windows. No browser process, no
browser profile, its own icon in the taskbar and Alt-Tab. Closing the window ends
the program.

`WebView2Loader.dll` ships beside the executable. It is Microsoft's
redistributable, unmodified, and it has to be a file on disk: without it the
WebView2 library maps an embedded copy into the process behind the operating
system's loader, which is a technique this project does not use. An executable
separated from its loader falls back to a browser window rather than breaking.

macOS and Linux open an app window from an installed Chromium-family browser
instead — no address bar, no tabs, its own dock entry. Where no such browser
exists, an ordinary browser tab. `--in-browser` asks for the tab deliberately.

Also: closing the window now stops the program, rather than leaving it running
until the idle timeout.

Known issue, fixed in v1.3.2: flashing console windows during the checks.

---

## v1.2.0 — no terminal window

**Released 5 September 2026 · commit `59c7cdc`**

Fixed: double-clicking the Windows agent no longer opens a black terminal window
behind the interface. The agent is linked as a windowed program.

Run from a Command Prompt it borrows that prompt's console, so `--text`,
`--json`, `--version` and `--list-checks` still print where you can read them.

Known issue, fixed in v1.3.2: this is the change that introduced the flashing
console windows.

---

## v1.1.1

**Released 5 September 2026 · commit `a8aaae8`**

Fixed: the Windows installer was not reproducible. NSIS stores each packed
file's modification time, and git sets those to whenever the checkout happened,
so two machines cloning minutes apart produced different installers from
identical content. `SetDateSave off` removes the dependency.

The v1.1.0 release failed its own reproducibility gate because of this and
published nothing, which is the order that gate was built in for.

---

## v1.1.0 — packaging for macOS and Linux

**Commit `9d097ef` · tagged, but no release was published**

The reproducibility gate refused it. The tag is kept rather than deleted, because
a tag that produced no release is part of the record. Use v1.1.1, which is this
change plus the fix.

Added: macOS archives contain a real `SupportOne.app` bundle with its icon.
Debian-family Linux gets `.deb` packages for amd64, arm64, i386 and armhf, with a
desktop entry and the full icon set.

---

## v1.0.0 — first release

**Released 5 September 2026 · commit `6439c21`**

Nine builds of the agent — Windows amd64/arm64/386, macOS amd64/arm64, Linux
amd64/arm64/386/arm — and two of the optional fleet server.

Reproducible: the release workflow builds everything twice on separate machines
and compares byte for byte before it will sign anything. Signed with Sigstore
keyless signing, with SLSA build provenance and a CycloneDX SBOM.

Everything the program does was already in place at this version: fifteen
read-only checks, three reversible repairs behind a per-action consent gate,
two guided walkthroughs, the offline explainer, the optional model assistant
behind an egress gate, support bundles, the optional fleet server, monthly
reports, provisioning profiles, and the remote-help consent wrapper.

Not signed with an Authenticode certificate or notarized by Apple, then or now.
Windows SmartScreen and macOS Gatekeeper warn about every release, and they are
right to — see [docs/INSTALL.md](docs/INSTALL.md) for how to verify a download
instead of clicking past the warning.
