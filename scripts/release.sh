#!/usr/bin/env bash
#
# Build every release artifact, reproducibly.
#
# The same script runs in CI and on a person's machine, so that "I rebuilt it
# and got the same bytes" is something anyone can check rather than something
# this project asserts. Given the same commit and the same Go toolchain, every
# file this writes is byte-for-byte identical, including the archives.
#
# Usage:
#   scripts/release.sh [output-directory]
#
# Environment:
#   VERSION             what to stamp into the binaries (default: git describe)
#   SOURCE_DATE_EPOCH   the build timestamp (default: the commit's own time)
#
set -euo pipefail

# Sorting, and anything else locale-sensitive, must not depend on who is
# running this.
export LC_ALL=C

repo_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_root"

out=${1:-dist}

# --- what to stamp -----------------------------------------------------------

# The commit's own timestamp, not the clock. Two builds of one commit an hour
# apart must not differ, or "reproducible" means nothing.
: "${SOURCE_DATE_EPOCH:=$(git log -1 --pretty=%ct)}"
export SOURCE_DATE_EPOCH

: "${VERSION:=$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
commit=$(git rev-parse HEAD)
build_date=$(date -u -d "@${SOURCE_DATE_EPOCH}" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null \
  || date -u -r "${SOURCE_DATE_EPOCH}" +%Y-%m-%dT%H:%M:%SZ)

if [ -n "$(git status --porcelain)" ]; then
  # Not fatal — a person may well be testing an unreleased change — but the
  # result is not something anyone else can reproduce, and saying so is the
  # difference between a warning and a lie.
  echo "warning: the working tree has uncommitted changes; this build is not reproducible by anyone else" >&2
fi

# --- tools -------------------------------------------------------------------

# Deterministic archives need GNU tar's --sort and --mtime. BSD tar has
# neither, so on macOS this asks for gtar rather than quietly producing an
# archive whose bytes depend on the order the filesystem happened to return.
tar_bin=tar
if ! tar --version 2>/dev/null | head -1 | grep -q "GNU tar"; then
  if command -v gtar >/dev/null 2>&1; then
    tar_bin=gtar
  else
    echo "error: GNU tar is required for reproducible archives (on macOS: brew install gnu-tar)" >&2
    exit 1
  fi
fi

for tool in zip go git; do
  command -v "$tool" >/dev/null 2>&1 || { echo "error: $tool is required" >&2; exit 1; }
done

# The Windows artifacts carry an icon, version metadata and an installer, and
# all three are part of what gets hashed. Building them only when the tools
# happen to be present would mean two people building one commit get different
# files, so these are required rather than optional.
goversioninfo=${GOVERSIONINFO:-goversioninfo}
if ! command -v "$goversioninfo" >/dev/null 2>&1; then
  if [ -x "$(go env GOPATH)/bin/goversioninfo" ]; then
    goversioninfo="$(go env GOPATH)/bin/goversioninfo"
  else
    echo "error: goversioninfo is required for the Windows icon and version metadata" >&2
    echo "       go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.4.1" >&2
    exit 1
  fi
fi

# The Windows agent draws its own window through WebView2, and the Microsoft
# loader that starts the runtime ships beside the executable. It is taken from
# the pinned go-webview2 module rather than committed here: go.sum already fixes
# those bytes, so the build has one source of truth for them and no Microsoft
# binary lives in this repository.
#
# Shipping it as a file is not packaging taste. Without it the library maps an
# embedded copy into the process without the operating system's loader, and that
# is a technique this project will not use -- see internal/platform.
webview2_module="github.com/jchv/go-webview2"
webview2_version=$(go list -m -f '{{.Version}}' "$webview2_module")
webview2_sdk="$(go env GOMODCACHE)/${webview2_module}@${webview2_version}/webviewloader/sdk"
if [ ! -d "$webview2_sdk" ]; then
  echo "error: the WebView2 loader is missing from the module cache" >&2
  echo "       run: go mod download ${webview2_module}" >&2
  exit 1
fi

command -v makensis >/dev/null 2>&1 || {
  echo "error: makensis (NSIS) is required to build the Windows installer" >&2
  echo "       Debian/Ubuntu: apt-get install nsis" >&2
  exit 1
}

command -v dpkg-deb >/dev/null 2>&1 || {
  echo "error: dpkg-deb is required to build the Linux packages" >&2
  echo "       Debian/Ubuntu: it ships with dpkg; elsewhere: apt-get install dpkg" >&2
  exit 1
}

# The checksum tool is resolved once, as a command rather than a shell
# function, so the pipeline below can hand it a list of files.
if command -v sha256sum >/dev/null 2>&1; then
  sha_cmd="sha256sum"
else
  # macOS ships shasum instead. Its output format matches sha256sum's, so
  # SHA256SUMS reads the same wherever it was produced.
  sha_cmd="shasum -a 256"
fi

# --- targets -----------------------------------------------------------------

# The agent is what a person downloads, so it is built for everything this
# toolchain can reach — including the 32-bit machines that are exactly the ones
# most likely to be short of disk space and in need of a look.
#
# What this does not reach is documented rather than glossed over: Go 1.24 sets
# the floor at Windows 10, macOS 12 and Linux with kernel 3.2. A Windows 7
# machine cannot run this build at any architecture, and no amount of extra
# targets changes that.
agent_targets="windows/amd64 windows/arm64 windows/386
               darwin/amd64 darwin/arm64
               linux/amd64 linux/arm64 linux/386 linux/arm"

# The fleet server is optional, self-hosted, and its supported path is the
# container image. These two archives exist for someone who would rather run
# the binary directly; a Windows or macOS build of it would be a download
# nobody has asked for.
server_targets="linux/amd64 linux/arm64"

rm -rf "$out"
mkdir -p "$out"

echo "SupportOne ${VERSION} (${commit})"
echo "build date ${build_date} (SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH})"
echo

# numeric_version reduces the version to the four-number form Windows resource
# metadata insists on. A tag like v1.2.3 becomes 1.2.3; anything else — a
# commit hash from a build off a branch — becomes 0.0.0, which is honest about
# not being a release rather than inventing a number.
numeric_version() {
  case "${1#v}" in
    [0-9]*.[0-9]*.[0-9]*)
      printf '%s' "${1#v}" | sed -E 's/^([0-9]+\.[0-9]+\.[0-9]+).*$/\1/'
      ;;
    *)
      printf '0.0.0'
      ;;
  esac
}

# start_here writes the first thing a non-technical person should read.
#
# The operating system is going to warn them about this file, because it is not
# signed by a certificate authority. That warning is correct, and telling
# someone to click past it without saying why would be teaching them the exact
# habit that gets people compromised. So this explains what the warning means,
# and gives them a check that is stronger than the signature would have been.
start_here() {
  cmd=$1 goos=$2

  cat <<HEADER
SupportOne ${VERSION}
=====================

What this is: one program that looks at this computer and explains, in plain
language, what it finds. It changes nothing unless you ask it to, and it sends
nothing anywhere unless you click to send it.

HEADER

  if [ "$cmd" = "supportone-server" ]; then
    cat <<'SERVER'
This is the OPTIONAL fleet server, for a technician who wants a dashboard of
machines that chose to report to them. It is not the program an ordinary user
runs -- that is supportone-agent.

  1. Set a secret of at least 24 characters:

       export SUPPORTONE_FLEET_TOKEN='...'

  2. Run it:

       ./supportone-server

It refuses to start without that token, because a fleet server without one is
a list of other people's machines served to anyone who asks.
SERVER
    return
  fi

  case "$goos" in
    windows)
      cat <<'WINDOWS'
THE EASIER WAY
--------------

If you downloaded this .zip, there is also an installer next to it on the
releases page, called SupportOne-Setup-...exe. It puts SupportOne in your
Start Menu, offers a desktop icon, and adds an entry to Settings > Apps so
you can remove it like any other program. It installs for you only and does
not ask for administrator rights.

This .zip is for anyone who would rather unpack it themselves. Both contain
exactly the same program.

TO RUN IT FROM HERE
-------------------

  1. Double-click supportone-agent.exe

  2. Windows will say "Windows protected your PC" and
     "Microsoft Defender SmartScreen prevented an unrecognized app from
     starting."

     This is EXPECTED, and the warning is telling you the truth: this program
     is not signed by a certificate authority, so Windows does not know who
     wrote it. Please do not get in the habit of clicking past these.

     Check it instead. Open PowerShell and run:

       Get-FileHash .\supportone-agent.exe -Algorithm SHA256

     Compare what it prints against the SHA256SUMS file published with this
     download at:

       https://github.com/ZanOzair/SupportOne/releases

     If it matches, this is exactly the file the project's build produced, and
     you have verified that yourself rather than trusting anybody's word for
     it. That is a stronger check than a signature.

  3. Then click "More info" and "Run anyway". Windows only asks once.

  4. Nothing seems to happen for about half a minute -- that is the checks
     running -- and then the SupportOne window opens showing the results.
     It is SupportOne's own window: its icon, its own button on the taskbar,
     no terminal and no browser.

If no window opens at all, SupportOne shows you the address in a small message
box. It starts with http://127.0.0.1 and works on this computer only; nothing
on the internet can reach it.

About the extra file: WebView2Loader.dll sits next to supportone-agent.exe and
is what lets SupportOne draw its own window. It is Microsoft's file, shipped
unmodified, and WEBVIEW2-NOTICE.txt is its licence. Keep the two together --
on its own, supportone-agent.exe falls back to opening a browser instead. If
you used the installer, this is already handled.

To stop it: close the window, or click Close in the page. Either one stops the
program. If you would rather watch what it is doing, open a Command Prompt and
run it from there -- it prints to the prompt when started that way.
WINDOWS
      ;;
    darwin)
      cat <<'MACOS'
TO RUN IT
---------

  1. Drag SupportOne.app into your Applications folder, then double-click it.

  2. macOS will refuse, saying the developer cannot be verified. This is
     EXPECTED: this program is not notarized by Apple, so Gatekeeper does not
     know who wrote it. The warning is correct, and clicking past warnings is
     a bad habit to build.

     Check it instead. In Terminal, in this folder:

       shasum -a 256 SupportOne.app/Contents/MacOS/supportone-agent

     Compare the .tar.gz you downloaded against the SHA256SUMS file published
     with it at:

       https://github.com/ZanOzair/SupportOne/releases

     If it matches, this is exactly the file the project's build produced, and
     you verified it yourself rather than trusting anyone.

  3. Then: System Settings -> Privacy & Security, scroll down, and click
     "Open Anyway" next to the message about SupportOne. macOS only asks once.

  4. The checks run for about half a minute and a SupportOne window opens by
     itself. If you have Chrome, Edge or Brave installed it is a window of its
     own, with its own icon in the Dock. With only Safari it opens as a tab
     instead -- Safari has no way to do the former, and the page is the same.

If nothing opens, Terminal prints an address starting with http://127.0.0.1 --
copy that into your browser. That address is on this computer only; nothing on
the internet can reach it.

To stop it: close the window, or press Ctrl+C in Terminal.
MACOS
      ;;
    *)
      cat <<'LINUX'
THE EASIER WAY
--------------

On Debian, Ubuntu, Mint or anything else using .deb packages, there is a
supportone_..._amd64.deb next to this download on the releases page:

    sudo apt install ./supportone_1.0.0_amd64.deb

That puts SupportOne in your applications menu with an icon, and
"sudo apt remove supportone" takes it away again.

TO RUN IT FROM HERE
-------------------

  1. Open a terminal in this folder and run:

       chmod +x supportone-agent
       ./supportone-agent

     To get a menu entry without installing the package, copy the two things
     in desktop-integration/ into place:

       mkdir -p ~/.local/share/applications ~/.local/share/icons/hicolor/256x256/apps
       cp desktop-integration/supportone.desktop ~/.local/share/applications/
       cp desktop-integration/icons/256.png \
          ~/.local/share/icons/hicolor/256x256/apps/supportone.png

  2. Before that, if you want to check the file is what the project published:

       sha256sum supportone-agent

     and compare it against the SHA256SUMS file published with this download
     at https://github.com/ZanOzair/SupportOne/releases

  3. The checks run for about half a minute and a SupportOne window opens by
     itself -- a window of its own where Chromium, Chrome or Edge is
     installed, and an ordinary browser tab otherwise.

On a machine with no desktop, use:

       ./supportone-agent --text

which prints the whole report to the terminal instead.

The web address it prints starts with http://127.0.0.1 and is on this computer
only; nothing on the internet can reach it. Press Ctrl+C to stop.
LINUX
      ;;
  esac

  cat <<'FOOTER'

WHAT IT WILL NOT DO
-------------------

  * It does not change anything without describing the change first and
    waiting for you to confirm that exact change.
  * It does not send anything anywhere unless you click to send it, and it
    shows you the exact contents first.
  * It has no telemetry, no analytics and no crash reporting. Not switched
    off -- not present.
  * It keeps a plain-text log of everything it did, which you can open in any
    text editor. The program tells you where.

Full documentation: https://github.com/ZanOzair/SupportOne
Reporting a security problem: see SECURITY.md in that repository.
FOOTER
}

# build_one <command> <goos> <goarch>
build_one() {
  cmd=$1 goos=$2 goarch=$3

  suffix=""
  [ "$goos" = "windows" ] && suffix=".exe"

  stage="${out}/stage/${cmd}-${VERSION}-${goos}-${goarch}"
  mkdir -p "$stage"

  # GOARM=6 rather than the default 7: it runs on every ARMv6 board as well as
  # newer ones, and a diagnostic tool is worth more running everywhere than
  # running slightly faster somewhere.
  goarm=""
  [ "$goarch" = "arm" ] && goarm=6

  # -trimpath removes the building machine's paths from the binary; without it
  # the same source built in two directories produces two different files.
  # CGO off keeps it one static file with no library to be missing later.
  #
  # -buildvcs=false because Go's own VCS stamp is read from the working tree at
  # the moment each binary is linked, and this script writes into that tree as
  # it goes: the first target would be stamped clean and every one after it
  # dirty, from one invocation. The commit is stamped explicitly below, from
  # git, once — so the ambient stamp is redundant here and only ever a way for
  # the output to depend on when it ran.
  # A Windows binary with no icon and no version metadata shows as a blank
  # page in Explorer and tells Properties nothing about itself. Both come from
  # a resource object compiled here and linked by the Go toolchain, which picks
  # it up from the command's own directory by its GOOS/GOARCH suffix.
  # The Windows agent is linked as a GUI program, so double-clicking it opens
  # the interface without a black terminal window behind it. It borrows the
  # console of whatever started it at run time, so --text and --version still
  # print when a technician runs it from a prompt. The fleet server keeps its
  # console: it is a daemon somebody watches the output of.
  subsystem=""
  if [ "$goos" = "windows" ] && [ "$cmd" = "supportone-agent" ]; then
    subsystem=" -H windowsgui"
  fi

  syso=""
  if [ "$goos" = "windows" ]; then
    syso="cmd/${cmd}/resource_windows_${goarch}.syso"
    bits="-64"
    case "$goarch" in
      386) bits="" ;;
      arm64) bits="-64 -arm" ;;
    esac
    # shellcheck disable=SC2086
    "$goversioninfo" $bits \
      -icon build/windows/supportone.ico \
      -product-version "$(numeric_version "$VERSION")" \
      -file-version "$(numeric_version "$VERSION")" \
      -o "$syso" build/windows/versioninfo.json
  fi

  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOARM="$goarm" GOTOOLCHAIN=local \
    go build -trimpath -buildvcs=false \
      -ldflags "-s -w${subsystem} -X main.version=${VERSION} -X main.commit=${commit} -X main.buildDate=${build_date}" \
      -o "${stage}/${cmd}${suffix}" "./cmd/${cmd}"

  # What a person needs beside the program: the licence, the document that says
  # what the thing does, and — for anyone who has never opened a terminal — one
  # page telling them what to click.
  cp LICENSE README.md "$stage/"
  start_here "$cmd" "$goos" > "${stage}/START-HERE.txt"

  # The window the agent draws needs Microsoft's loader beside it. The agent
  # checks for this file and falls back to a browser window when it is absent,
  # so an archive missing it degrades rather than breaks -- but no archive
  # should be missing it.
  if [ "$goos" = "windows" ] && [ "$cmd" = "supportone-agent" ]; then
    case "$goarch" in
      amd64) webview2_arch=x64 ;;
      386)   webview2_arch=x86 ;;
      arm64) webview2_arch=arm64 ;;
      *)     echo "error: no WebView2 loader for windows/${goarch}" >&2; exit 1 ;;
    esac
    cp "${webview2_sdk}/${webview2_arch}/WebView2Loader.dll" "${stage}/WebView2Loader.dll"
    cp build/windows/WEBVIEW2-NOTICE.txt "${stage}/"
  fi

  # macOS shows a bare binary as a blank document that opens a terminal. An
  # application bundle is what makes it a double-clickable program with an
  # icon in Finder and Launchpad — and a bundle is only a directory with a
  # plist in it, so this needs no Apple tooling.
  if [ "$goos" = "darwin" ] && [ "$cmd" = "supportone-agent" ]; then
    app="${stage}/SupportOne.app"
    mkdir -p "${app}/Contents/MacOS" "${app}/Contents/Resources"
    mv "${stage}/${cmd}" "${app}/Contents/MacOS/supportone-agent"
    cp build/macos/supportone.icns "${app}/Contents/Resources/"
    sed "s|__VERSION__|$(numeric_version "$VERSION")|g" \
      build/macos/Info.plist.template > "${app}/Contents/Info.plist"
    printf 'APPL????' > "${app}/Contents/PkgInfo"
  fi

  # A Linux desktop finds programs through a .desktop file and an icon in the
  # hicolor theme. Shipping both in the archive means someone who unpacks it
  # by hand can still get a menu entry, and the .deb below installs the same
  # two files to the same places.
  if [ "$goos" = "linux" ] && [ "$cmd" = "supportone-agent" ]; then
    mkdir -p "${stage}/desktop-integration"
    cp build/linux/supportone.desktop "${stage}/desktop-integration/"
    cp -r build/linux/icons "${stage}/desktop-integration/"
  fi

  # Every timestamp in the archive comes from the commit, not from when this
  # ran. Directories too, or their mtimes leak the build time.
  find "$stage" -exec touch -h -d "@${SOURCE_DATE_EPOCH}" {} +

  [ -n "$syso" ] && rm -f "$syso"

  archive_base="${cmd}-${VERSION}-${goos}-${goarch}"
  if [ "$goos" = "windows" ]; then
    # -X drops the extra attributes (uid, gid, high-precision times) that
    # would otherwise differ between machines. Entries are added in sorted
    # order because zip stores them in the order it is given them.
    (
      cd "${out}/stage"
      find "$archive_base" -type f | sort | zip -q -X -9 "../${archive_base}.zip" -@
    )
    echo "  ${archive_base}.zip"

    if [ "$cmd" = "supportone-agent" ]; then
      # The installer is what makes this look like a program somebody
      # installed rather than a file they found: a Start Menu entry, an
      # optional desktop icon, and an entry in Settings > Apps that removes
      # it again. It installs per-user and asks for no administrator rights,
      # because the program itself does not need them.
      installer="SupportOne-Setup-${VERSION}-${goarch}.exe"
      makensis -V1 \
        -DVERSION="$(numeric_version "$VERSION")" \
        -DARCH="$goarch" \
        -DSOURCE="$(cd "$stage" && pwd)" \
        -DOUTFILE="$(cd "$out" && pwd)/${installer}" \
        build/windows/installer.nsi > /dev/null
      touch -d "@${SOURCE_DATE_EPOCH}" "${out}/${installer}"
      echo "  ${installer}"
    fi
  else
    "$tar_bin" --sort=name --format=gnu \
      --owner=0 --group=0 --numeric-owner \
      --mtime="@${SOURCE_DATE_EPOCH}" \
      -C "${out}/stage" -cf - "$archive_base" \
      | gzip -n -9 > "${out}/${archive_base}.tar.gz"
    echo "  ${archive_base}.tar.gz"

    if [ "$goos" = "linux" ] && [ "$cmd" = "supportone-agent" ]; then
      build_deb "$goarch" "$stage"
    fi
  fi
}

# build_deb turns one staged Linux build into an installable package.
#
# It is the same shape as the Windows installer and for the same reason: on a
# desktop Linux machine, "download a tarball and put the binary somewhere on
# your PATH" is a set of instructions, and a package is a double-click.
build_deb() {
  goarch=$1 stage=$2

  # Debian's architecture names are its own, not Go's.
  case "$goarch" in
    amd64) debarch=amd64 ;;
    arm64) debarch=arm64 ;;
    386)   debarch=i386 ;;
    arm)   debarch=armhf ;;
    *)     return 0 ;;
  esac

  root="${out}/stage/deb-${debarch}"
  rm -rf "$root"
  mkdir -p "${root}/DEBIAN" \
           "${root}/usr/bin" \
           "${root}/usr/share/applications" \
           "${root}/usr/share/doc/supportone"

  install -m 0755 "${stage}/supportone-agent" "${root}/usr/bin/supportone-agent"
  install -m 0644 build/linux/supportone.desktop "${root}/usr/share/applications/supportone.desktop"
  install -m 0644 LICENSE "${root}/usr/share/doc/supportone/copyright"
  install -m 0644 README.md "${stage}/START-HERE.txt" "${root}/usr/share/doc/supportone/"

  for icon in build/linux/icons/*.png; do
    size=$(basename "$icon" .png)
    mkdir -p "${root}/usr/share/icons/hicolor/${size}x${size}/apps"
    install -m 0644 "$icon" "${root}/usr/share/icons/hicolor/${size}x${size}/apps/supportone.png"
  done

  sed -e "s|__VERSION__|${VERSION#v}|" -e "s|__ARCH__|${debarch}|" \
    build/linux/control.template > "${root}/DEBIAN/control"

  find "$root" -exec touch -h -d "@${SOURCE_DATE_EPOCH}" {} +

  package="supportone_${VERSION#v}_${debarch}.deb"
  dpkg-deb --root-owner-group --build "$root" "${out}/${package}" > /dev/null
  touch -d "@${SOURCE_DATE_EPOCH}" "${out}/${package}"
  echo "  ${package}"
}

echo "Building the agent:"
for target in $agent_targets; do
  build_one supportone-agent "${target%/*}" "${target#*/}"
done

echo
echo "Building the fleet server:"
for target in $server_targets; do
  build_one supportone-server "${target%/*}" "${target#*/}"
done

rm -rf "${out}/stage"

# --- what it takes to reproduce this ----------------------------------------

# Reproducible means "given the same inputs", and the Go toolchain is one of
# them: a different patch release produces different bytes from identical
# source. Anyone checking these hashes needs to know which one to install, so
# the answer ships beside them rather than being something to guess.
cat > "${out}/BUILD-INFO.txt" <<INFO
SupportOne ${VERSION}

commit             ${commit}
SOURCE_DATE_EPOCH  ${SOURCE_DATE_EPOCH}
build date         ${build_date}
toolchain          $(go version | awk '{print $3}')

To rebuild these exact files:

  git clone https://github.com/ZanOzair/SupportOne.git
  cd SupportOne
  git checkout ${commit}
  VERSION=${VERSION} SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH} scripts/release.sh

Then compare your SHA256SUMS against the published one. They should match
line for line. If they do not, say so publicly — that is the point of this
file existing.
INFO

# --- checksums ---------------------------------------------------------------

# One file listing every artifact, so a person verifies one hash and the rest
# follow from it. Sorted, so this file is reproducible too.
(
  cd "$out"
  find . -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' -o -name '*.exe' -o -name '*.deb' -o -name 'BUILD-INFO.txt' \) \
    | sed 's|^\./||' | sort | xargs $sha_cmd > SHA256SUMS
)

echo
echo "Wrote $(find "$out" -maxdepth 1 -type f | wc -l | tr -d ' ') files to ${out}/"
echo
cat "${out}/SHA256SUMS"
