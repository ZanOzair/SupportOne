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

# The agent is what a person downloads, so it is built for everything.
agent_targets="windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64"

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

# build_one <command> <goos> <goarch>
build_one() {
  cmd=$1 goos=$2 goarch=$3

  suffix=""
  [ "$goos" = "windows" ] && suffix=".exe"

  stage="${out}/stage/${cmd}-${VERSION}-${goos}-${goarch}"
  mkdir -p "$stage"

  # -trimpath removes the building machine's paths from the binary; without it
  # the same source built in two directories produces two different files.
  # CGO off keeps it one static file with no library to be missing later.
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOTOOLCHAIN=local \
    go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${commit} -X main.buildDate=${build_date}" \
      -o "${stage}/${cmd}${suffix}" "./cmd/${cmd}"

  # What a person needs beside the program: the licence, and the document that
  # says what the thing does.
  cp LICENSE README.md "$stage/"

  # Every timestamp in the archive comes from the commit, not from when this
  # ran. Directories too, or their mtimes leak the build time.
  find "$stage" -exec touch -h -d "@${SOURCE_DATE_EPOCH}" {} +

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
  else
    "$tar_bin" --sort=name --format=gnu \
      --owner=0 --group=0 --numeric-owner \
      --mtime="@${SOURCE_DATE_EPOCH}" \
      -C "${out}/stage" -cf - "$archive_base" \
      | gzip -n -9 > "${out}/${archive_base}.tar.gz"
    echo "  ${archive_base}.tar.gz"
  fi
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
  find . -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' -o -name 'BUILD-INFO.txt' \) \
    | sed 's|^\./||' | sort | xargs $sha_cmd > SHA256SUMS
)

echo
echo "Wrote $(find "$out" -maxdepth 1 -type f | wc -l | tr -d ' ') files to ${out}/"
echo
cat "${out}/SHA256SUMS"
