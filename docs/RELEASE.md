# Releases

How a release is made, how to check one you downloaded, and — the part most
projects leave out — exactly which of those checks exist here and which do not.

---

## Verifying a download

Every release publishes `SHA256SUMS`, a signature over it, and the certificate
that signature was made with.

```sh
# 1. The files are what the checksums say.
sha256sum -c SHA256SUMS --ignore-missing

# 2. The checksums are what this project published.
cosign verify-blob SHA256SUMS \
  --signature SHA256SUMS.sig \
  --certificate SHA256SUMS.pem \
  --certificate-identity-regexp 'https://github.com/ZanOzair/SupportOne/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The second command is the one that matters. Checksums alone prove only that
your download is intact, not that it came from here — anyone can publish a file
and a hash of it. The signature ties `SHA256SUMS` to the workflow run that
produced it, and that signature is recorded in a public transparency log, so a
signature made in secret is not a thing that can happen.

There is also a SLSA provenance attestation, which says which workflow at which
commit produced each archive:

```sh
gh attestation verify supportone-agent-*.tar.gz --repo ZanOzair/SupportOne
```

### Rebuilding it yourself

The strongest check does not need trusting this project at all: build it and
see if you get the same bytes.

```sh
git clone https://github.com/ZanOzair/SupportOne.git
cd SupportOne
git checkout <the commit in BUILD-INFO.txt>
VERSION=<version> SOURCE_DATE_EPOCH=<epoch from BUILD-INFO.txt> scripts/release.sh
diff dist/SHA256SUMS <the published SHA256SUMS>
```

`BUILD-INFO.txt` ships in every release with the commit, the epoch and the exact
Go toolchain, because "reproducible" means *given the same inputs* and the
toolchain is one of them: a different Go patch release produces different bytes
from identical source.

You need GNU tar for this. On macOS, `brew install gnu-tar` — the script checks
and says so rather than producing an archive whose bytes depend on the order the
filesystem happened to return.

Every release already does this check on itself: one job builds the artifacts, a
second job builds them again from a fresh checkout without being told the first
job's answers, and the release fails if the two differ. **If your rebuild does
not match, please say so publicly.** That is the entire point of the file.

---

## What is signed, and what is not

| | |
|---|---|
| Sigstore keyless signature over `SHA256SUMS` | **yes** |
| SLSA build provenance | **yes** |
| Reproducible from source | **yes**, and checked every release |
| Windows Authenticode | **no** |
| Apple notarization | **no** |

**Windows SmartScreen and macOS Gatekeeper will warn you about these
downloads.** They are right to. An unsigned binary from the internet is exactly
what those warnings exist for, and this project is not going to tell you to
click through a security warning as though it were a formality.

The two missing signatures are not an oversight and not a to-do that got
dropped. Both require a certificate issued against a verified legal identity:

- **Windows**: an OV or EV code-signing certificate from a CA. An OV
  certificate is signed but still accumulates SmartScreen reputation from zero;
  an EV certificate carries reputation immediately and needs a hardware token.
- **macOS**: an Apple Developer Program membership, a Developer ID Application
  certificate, and notarization — uploading each build to Apple, which scans it
  and issues a ticket that gets stapled to the file.

Both are annual costs tied to a named person or company. Until someone makes
that commitment, the honest position is the one above: verifiable, reproducible,
and unsigned in the way the operating system cares about.

### If you are that someone

The release workflow does **not** contain Authenticode or notarization steps.
That is deliberate: an untested signing command in the release path is worse
than an honest gap, and neither could be tested without the certificate.

If you have one, here is what to add. Treat these as a starting point to verify,
not as commands that have ever run in this repository.

**Windows**, on the Linux runner, using `osslsigncode` so no Windows runner is
needed:

```sh
osslsigncode sign \
  -pkcs12 cert.p12 -pass "$WINDOWS_SIGNING_PASSWORD" \
  -n "SupportOne" -i https://github.com/ZanOzair/SupportOne \
  -t http://timestamp.digicert.com \
  -in supportone-agent.exe -out supportone-agent-signed.exe
```

Sign the `.exe` **before** it goes into the zip, and re-checksum afterwards —
signing changes the file, so a signature applied after `SHA256SUMS` is written
invalidates it.

**macOS** needs a `macos-latest` runner:

```sh
codesign --timestamp --options runtime \
  --sign "Developer ID Application: Your Name (TEAMID)" supportone-agent
ditto -c -k --keepParent supportone-agent supportone-agent.zip
xcrun notarytool submit supportone-agent.zip \
  --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" \
  --password "$APPLE_APP_PASSWORD" --wait
```

A bare binary cannot have a notarization ticket stapled to it — only an app
bundle, disk image or installer package can. A notarized loose binary is
verified online at first run instead, which means a first run offline still
warns. Producing a `.app` or a `.pkg` is more work than this project has done.

Note the timestamp server in both: without one, every signature expires when the
certificate does, and old releases stop validating.

Adding a signing secret without adding these steps does not sign anything. The
workflow watches for exactly that and warns in the job log, because a
certificate configured and unused is worse than none — it is a false sense of
being signed.

---

## Cutting a release

```sh
git tag -a v1.2.3 -m "SupportOne 1.2.3"
git push origin v1.2.3
```

The tag triggers the workflow. In order it builds every artifact, generates the
SBOM, rebuilds everything independently and fails if the two differ, signs the
checksums, attaches provenance, and publishes the release with notes stating
what is and is not signed.

To exercise the whole pipeline without publishing, run the workflow manually
from the Actions tab and give it a version. It builds, rebuilds, compares and
signs, and publishes nothing.

### What ends up in a release

| File | What it is |
|---|---|
| `supportone-agent-<version>-<os>-<arch>.{tar.gz,zip}` | The agent, for all six targets |
| `supportone-server-<version>-linux-<arch>.tar.gz` | The optional fleet server |
| `SHA256SUMS` | Every file above, hashed |
| `SHA256SUMS.sig`, `SHA256SUMS.pem` | The Sigstore signature and its certificate |
| `BUILD-INFO.txt` | Commit, epoch and toolchain: what you need to rebuild it |
| `supportone-<version>.cdx.json` | CycloneDX SBOM |

The fleet server is built for Linux only. It is a thing you host, its supported
path is the container image in `compose.yaml`, and a Windows or macOS build of
it would be a download nobody has asked for. The agent is built for everything,
because the agent is what a person downloads.

---

## What the build guarantees

- **No path or clock leaks in.** `-trimpath` removes the building machine's
  directories; every timestamp comes from the commit, not from when the build
  ran. Two builds an hour apart, in different directories, produce identical
  files — and the release workflow proves that on every release rather than
  asserting it.
- **One static file.** `CGO_ENABLED=0`, so there is no library to be missing on
  the machine that runs it.
- **Pinned everything.** The Go toolchain is pinned to an exact patch, not a
  range. Every GitHub Action is pinned to a commit digest rather than a tag,
  because a tag can be moved and an action runs with the job's own access.
- **No key to lose.** Signing is keyless: the certificate is issued to the
  workflow's identity for the moment it is used and expires immediately. This
  project stores no signing key, which means it has none to leak.

## What it does not

- **A signature is not a safety review.** It says these bytes came from this
  workflow at this commit. It says nothing about whether the code is any good.
- **Provenance covers the build, not the source.** It proves what was built. If
  a bad commit were merged, the release would faithfully attest to having built
  it.
- **Reproducibility is not a guarantee against a compromised toolchain.** If the
  Go distribution itself were backdoored, every rebuild would agree with every
  other one, and all of them would be wrong.
- **The agent cannot verify itself.** `--version` prints what was compiled into
  it, and says as much: a changed copy would print whatever it was changed to
  print. Verification has to come from outside the file.
