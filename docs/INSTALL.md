# Installing SupportOne

Written for somebody who has never opened a terminal. If you have, the
[README](../README.md) is faster.

Follow the section for your computer. Every step says what you should see, so
you can tell whether it worked before moving on.

- [Windows](#windows)
- [macOS](#macos)
- [Linux](#linux)
- [Checking the download is genuine](#checking-the-download-is-genuine)
- [Removing it again](#removing-it-again)

---

## Windows

### 1. Which file do you need?

Press **Windows key + I** → **System** → scroll down → **About**, and read
**System type**:

| It says | Download the file ending |
|---|---|
| 64-bit operating system, x64-based processor | `-amd64.exe` — nearly everyone |
| ARM-based processor | `-arm64.exe` |
| 32-bit operating system | `-386.exe` |

### 2. Download

Go to the [releases page](https://github.com/ZanOzair/SupportOne/releases) and
open the newest version. Under **Assets**, download **two** files:

- `SupportOne-Setup-<version>-amd64.exe` — the installer
- `SHA256SUMS` — the list of fingerprints, for step 3

Your browser may say the file "isn't commonly downloaded". Choose **Keep**. That
warning is about how many people have downloaded it, not about what is in it.

### 3. Check it is genuine

**Do not skip this.** It takes thirty seconds and it is the step that means you
do not have to take anybody's word for anything. See
[Checking the download is genuine](#checking-the-download-is-genuine) below, then
come back here.

### 4. Install

Double-click the installer.

**Windows will say "Windows protected your PC".** This is expected:

1. Click **More info**
2. Click **Run anyway**

That warning appears because the installer is not signed with a paid certificate
tied to a verified company identity. It is not a judgement about the file — and
you have just checked the file yourself, which is a stronger check than a
signature. [docs/RELEASE.md](RELEASE.md) explains what signing would take.

Then click through: **Next → I Agree → Next → Install → Finish**.

**No administrator password is needed.** SupportOne installs for your account
only, under `%LOCALAPPDATA%\Programs\SupportOne`. A program that only reads your
computer has no business demanding administrator rights to install.

### 5. Check it worked

Open **SupportOne** from the Start Menu. Under "Your computer's health" you
should see the version you installed:

```
Computer: Windows (amd64) · v1.3.2
```

If it shows an older version, the old one was still running when you installed.
Close SupportOne, open Task Manager (**Ctrl+Shift+Esc**), end any
`supportone-agent.exe`, and install again.

### What you should see when it runs

- A **SupportOne window** — its own icon, its own taskbar button, no address bar
- **No black windows** flashing while the checks run
- Results after about half a minute
- Closing the window **ends the program** — it does not keep running

If any of that is wrong, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

### The two files

The installed folder contains `supportone-agent.exe` and `WebView2Loader.dll`.
Both are needed: the DLL is what lets SupportOne draw its own window. If you
unpack the `.zip` by hand rather than using the installer, keep them together —
on its own, the executable falls back to opening a browser window instead.

---

## macOS

**Which file:** Apple Silicon (M1 and later) needs `darwin-arm64`. Intel Macs
need `darwin-amd64`. Apple menu → About This Mac tells you which you have.

1. Download `supportone-agent-<version>-darwin-arm64.tar.gz` and `SHA256SUMS`
   from the [releases page](https://github.com/ZanOzair/SupportOne/releases).
2. Check the download — see [below](#checking-the-download-is-genuine).
3. Double-click the `.tar.gz` to unpack it. Inside is **SupportOne.app**. Drag it
   to your Applications folder.
4. The first time you open it, macOS will say the developer cannot be verified.
   Go to **System Settings → Privacy & Security**, scroll down, and click
   **Open Anyway** next to the message about SupportOne. macOS asks once.

**What you should see:** a window with no address bar and its own Dock icon, if
you have Chrome, Edge, Brave or Chromium installed. With only Safari it opens as
a browser tab instead — Safari has no equivalent, and the page is the same.

---

## Linux

### Debian, Ubuntu, Mint and relatives

```sh
sudo apt install ./supportone_<version>_amd64.deb
```

That installs the program, an applications-menu entry and its icons. Remove it
with `sudo apt remove supportone`.

Packages are built for `amd64`, `arm64`, `i386` and `armhf`.

### Anything else

```sh
tar xzf supportone-agent-<version>-linux-amd64.tar.gz
cd supportone-agent-<version>-linux-amd64
./supportone-agent
```

The archive includes a `desktop-integration/` folder with a `.desktop` entry and
the icon set, if you want a menu entry without a package.

### On a machine with no desktop

```sh
./supportone-agent --text
```

prints the whole report to the terminal instead of opening anything.

---

## Checking the download is genuine

Every release publishes `SHA256SUMS`: a fingerprint of each file, calculated from
every byte in it. Change one byte and the fingerprint changes completely.

You calculate the fingerprint of what you downloaded and compare. If they match,
your copy is exactly what the build produced.

### Windows

Open **PowerShell** (Windows key, type `powershell`, press Enter), and paste this
whole block. Change the filename on the second line if yours differs.

```powershell
cd $env:USERPROFILE\Downloads
$file = "SupportOne-Setup-v1.3.2-amd64.exe"
$mine = (Get-FileHash $file -Algorithm SHA256).Hash.ToLower()
$published = ((Select-String -Path SHA256SUMS -Pattern $file -SimpleMatch).Line -split '\s+')[0].ToLower()
if ($mine -eq $published) { Write-Host "MATCH - safe to install" -ForegroundColor Green }
else { Write-Host "DIFFERENT - do not install" -ForegroundColor Red }
```

- **MATCH** — install it.
- **DIFFERENT** — delete the file, download it again, and check again. If it is
  still different, [open an issue](https://github.com/ZanOzair/SupportOne/issues)
  and say so. Do not install it.

### macOS and Linux

```sh
sha256sum -c SHA256SUMS --ignore-missing     # Linux
shasum -a 256 -c SHA256SUMS --ignore-missing # macOS
```

### Going further: the signature

The checksums file is itself signed, so you can check it was produced by this
project's build and not substituted along with the files it describes. This needs
[cosign](https://github.com/sigstore/cosign) installed:

```sh
cosign verify-blob SHA256SUMS \
  --signature SHA256SUMS.sig \
  --certificate SHA256SUMS.pem \
  --certificate-identity-regexp 'https://github.com/ZanOzair/SupportOne/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

### Going further still: build it yourself

`BUILD-INFO.txt` in each release names the commit and the exact Go toolchain that
produced it. The build is reproducible, so building that commit yourself should
give you byte-identical files. [docs/RELEASE.md](RELEASE.md) has the steps.

If yours differ, please say so publicly. That is the check that does not depend
on trusting anyone, including the person who published the release.

---

## Removing it again

| | |
|---|---|
| **Windows** | Settings → Apps → Installed apps → SupportOne → ⋯ → Uninstall |
| **macOS** | Drag SupportOne.app to the Bin |
| **Debian-family Linux** | `sudo apt remove supportone` |
| **Unpacked archive** | Delete the folder |

Uninstalling deliberately leaves your audit log and any saved reports alone.
Removing the program should not remove the record of what it did. They live in
your user configuration directory:

| | |
|---|---|
| Windows | `%APPDATA%\SupportOne` |
| macOS | `~/Library/Application Support/SupportOne` |
| Linux | `~/.config/SupportOne` |

Delete that folder yourself if you want them gone.
