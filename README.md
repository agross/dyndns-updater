# dyndns-updater

A single-binary, one-shot Go client that updates deSEC DNS A records and Hurricane
Electric Tunnelbroker IPv4 endpoints from ordered public IPv4 address sources. deSEC
AAAA records are always preserved. No Perl or Python runtime is required.

## Configuration and operation

Copy [the example YAML](examples/dyndns-updater.yaml) to a private local file
(typically `/etc/dyndns-updater.yaml`, root-owned mode `0600`). Configure one or
more named profiles and create the state directory with mode `0700` before running:

```sh
go build -trimpath -o dyndns-updater .
./dyndns-updater --help
./dyndns-updater --config /path/to/config.yaml
```

The default configuration path is `/etc/dyndns-updater.yaml`. Unknown YAML keys,
duplicate mapping keys, multiple documents, unsupported providers, missing
credentials, and invalid durations/targets fail before any updates or mail.
`refresh` defaults to `600h` (25 days); use Go duration syntax, not `25d`.
`timeout` defaults to `30s` per web source, provider request, and sendmail invocation.
`state` and `sendmail` paths must be absolute. Mail defaults to local `root` via
`/usr/sbin/sendmail -oi -- root`; submission is bounded by the configured timeout.

### Address sources

Every profile requires a nonempty, ordered list of source objects:

```yaml
sources:
  - if: telekom
  - web: ifconfig.me
  - web: he
  - web: ipify
  - url: https://discovery.example.invalid/ip
```

Each object has exactly one selector with a nonempty string value. Unknown or
additional properties, duplicate keys, scalar/prefix entries and unknown named
services are rejected. The former `interface` setting is rejected; there is no
compatibility alias or automatic configuration migration. All sources in all
profiles are validated before networking, mail, state locking or writes, even
entries that would not be reached after an earlier success.

- `if` selects the first usable IPv4 returned by the OS for that interface.
  Its selected address must be public; private/unavailable interfaces fall
  through to the next source rather than searching later interface addresses.
- `web: ifconfig.me` requests `https://ifconfig.me/ip` and `web: ipify` requests
  `https://api.ipify.org/`; both require plain-text IPv4 responses.
- `web: he` requests `https://checkip.dns.he.net/` and extracts the IPv4 immediately
  associated with the unique `Your IP address is :` HTML text marker. Missing or
  ambiguous markers fail; unrelated addresses in the document are not selected.
- `url` accepts a custom HTTPS endpoint returning exactly one IPv4 literal, apart
  from surrounding whitespace. Prose, multiple addresses, HTML, JSON and IPv6 fail,
  even when the hostname belongs to a known service. URLs cannot contain credentials
  or fragments. Literal-loopback HTTP is allowed only for local testing.

All selected addresses must be public IPv4: private, shared/CGNAT, loopback,
link-local, unspecified, multicast, broadcast, documentation, benchmarking and
reserved ranges are rejected. Sources are tried sequentially; the first valid
address stops detection. DNS/connect/TLS/read failures, timeouts, non-200 responses,
redirects (never followed), oversized bodies (over 64 KiB), malformed content and
non-public addresses advance to the next source. Earlier failures do not trigger
failure mail if a later source succeeds. Provider credentials never go to discovery.

Web requests use IPv4 connections, normal system routing, verified TLS and the
fallback DNS policy below. An `if` source does not bind later traffic to that
interface. Each web source gets a fresh `timeout` budget shared by its system DNS,
fallback DNS, connection, TLS and body; provider requests get a separate budget.
Total discovery duration can grow with the number of web entries. Detection runs
anew on every invocation, even for otherwise unchanged or retry-restricted profiles.
A provider rejection does not restart discovery or try another source.

Exhaustion fails that profile and returns a nonzero overall exit without sending
its provider update. Other profiles continue. The last confirmation and provider
retry restrictions are retained; detection is retried next invocation without a
new cooldown. Failure state and notification intent are checkpointed durably and
repeated failures are deduplicated until strict provider acceptance clears them.
Detection alone never means provider confirmation or recovery.

### Fallback DNS

System DNS is always tried first. If `fallback_dns` is omitted, failed lookups
fall back to Cloudflare (`1.1.1.1`), Google (`8.8.8.8`), then Quad9 (`9.9.9.9`),
all on port 53. Existing configurations need no migration but gain this default
fallback behavior. A nonempty list replaces these defaults with your chosen
ordered literal DNS server IP addresses:

```yaml
fallback_dns:
  - "192.0.2.10"           # default port 53
  - "2001:db8::10"        # bare IPv6, default port 53
  - "192.0.2.20:5353"     # explicit port
  - "[2001:db8::20]:5353" # IPv6 with a port requires brackets
```

These documentation addresses are examples, not working public resolvers. Set
`fallback_dns: []` to disable fallback and use only system DNS. Hostnames,
malformed literals, zone identifiers, and ports outside 1–65535 are rejected. The entire list is
validated before state locking/writes, provider requests, or mail submission.
Changing this list does not change profile identity or invalidate confirmed state.

System DNS is tried first. Only a failed/negative lookup or a lookup with no usable
addresses causes fallback; servers are tried in list order until one succeeds or
the request deadline expires. DNS uses UDP and TCP when needed, directly to the
configured IP. Literal-IP provider and discovery endpoints skip DNS. No DNS cache or system DNS
changes are introduced. The existing `timeout` is one shared budget for system
DNS, all fallback attempts, connection, TLS, and the HTTP response, **not** a fresh
budget per server. An unresponsive resolver can consume the whole budget, leaving
no time for later entries. Cancellation stops outstanding fallback resolution.

Connection, TLS, HTTP, and provider-response failures do not trigger another
resolver or repeat an update. The original HTTP hostname, TLS SNI and certificate
verification, HTTPS restrictions and redirect policy are retained. Environment
proxy policy is unchanged: when an HTTP(S) proxy resolves the provider, fallback
does not replace that proxy's DNS or bypass a failing proxy. Proxy-host resolution
itself continues to use normal system DNS.

Resolver exhaustion is the existing credential-safe provider transport failure:
other profiles continue, last confirmed addresses remain intact, and later
invocations retry with the existing failure/recovery notification rules. Choose
trusted resolvers and permit their UDP/TCP traffic: a fallback receives provider and discovery
hostname queries and can return different addresses. TLS verification remains
essential. Failed system and fallback lookups (including empty/unusable answers)
are logged to stderr with the profile and resolver. Each attempted fallback is
logged in order, followed by its failure or successful DNS resolution. DNS success
means usable addresses were obtained, not that the provider update succeeded.
System resolver success adds no DNS trace. Diagnostics omit raw errors, queries,
URLs and credentials and do not enter mail summaries. No live DNS or gateway changes are
required to test this feature.

Unchanged successful profiles are skipped until refresh is due. Failed updates
are retried on later invocations unless a durable provider retry restriction applies
(see below). Other profiles continue independently, except deSEC profiles share
a conservative rate-limit pause. HTTP 200
alone is not success: provider-specific response validation is required. See
[provider contracts and limits](docs/provider-contracts.md) for verified request
and response details, endpoint overrides, and the HE refresh caveat.

### State, concurrency, and recovery

The state file records confirmed addresses, timestamps, hashed profile identity,
provider failure state, and pending notification content, never credentials.
Provider, target, or endpoint changes invalidate a profile's old confirmation.
Source selectors, values, and ordering are discovery configuration, not provider
confirmation identity. Source-only edits retain the same-address confirmation and
its original refresh deadline, cause no new notification, and never unlock provider
retry restrictions. Detection still runs on every invocation; a changed address,
missing confirmation, due refresh, or outstanding failure follows the normal update
flow. Credential-only changes do not invalidate confirmed success, but release
intervention restrictions. A failed update, including HE `abuse`, retains its last confirmed
address and timestamp. An active HE pause survives all configuration changes,
although identity changes still invalidate the old confirmation.
After loading a valid configuration and state, each run removes confirmations for
names absent from the configuration while holding the state lock. Cleanup uses
the existing atomic writes even when all retained profiles skip or report errors;
configured names retain their state, including during interface/provider failures.
A renamed profile, or a removed name restored after a run persisted its absence,
requires a fresh successful provider update even with the same identity/address
before refresh is due. Removing and restoring a name between invocations records
no absence. Cleanup does not delete provider-side records or itself trigger mail:
queued summaries mentioning old names remain intact and retry in order, and
interrupted-run notification intent generates a summary of the current configuration.
Invalid configuration or state prevents cleanup and leaves the state untouched.
Removing the state file forces fresh updates and removes all retry restrictions;
do not use this to defeat provider throttling. Removing a profile removes its retry
state too; shared deSEC deadlines already copied to other profiles remain.
Corrupt or unreadable state causes
an error and is preserved, not silently reset. Keep the directory private and use
a local filesystem supporting advisory locks, atomic rename, and fsync.

Runs block on a stable separate `<state>.lock` file for their entire operation.
The file intentionally remains after both successful and failed invocations; its
existence is **not** a busy/stale-lock indicator. Mutual exclusion comes from the
kernel's advisory `flock` on the open file, not from creating or deleting it. Closing
the descriptor on exit (including SIGKILL) releases the lock, so the next invocation
can reuse the same file. Do **not** delete it while a process might hold it: a new
file at the same path would have a different inode and could permit concurrent
holders, bypassing serialization.
State is atomically replaced with mode `0600`,
synced along with its parent directory, and checkpointed after each attempted
provider update so an independent success survives interruption before a later
profile finishes. A crash after provider acceptance but before state persistence
can cause another safe update on restart. A new address appearing during an update
is detected by the next run; only the requested address is recorded on strict
confirmation.

### Legacy confirmation transition (#37)

New confirmation identities are version-tagged and exclude discovery sources. The
updater verifies a legacy source-dependent hash against the current configuration
(or the historical interface identity for a single matching `if` source). An exact
match reuses the confirmed address and timestamp without a provider request before
refresh. An opaque mismatch cannot prove an unchanged destination: one fresh strict
provider acceptance is required, subject to all surviving retry restrictions. This
can happen when sources are edited at the same time as upgrading. Subsequent
source-only edits no longer require reconfirmation. No credentials are persisted.

Legacy intervention fingerprints are explicitly transitioned using their persisted
identity and the current credentials. Unchanged credentials retain the block even
when the legacy destination cannot be verified; the block is rebound to the current
destination. Thus a destination-only correction made simultaneously with an
unverifiable legacy transition may conservatively remain blocked. Correcting the
credentials or subsequently correcting the destination retains normal retry behavior.
Shared deSEC deadlines and active HE abuse pauses retain their original deadlines.
Confirmation reuse or detection alone never clears an outstanding failure. Queued
mail is retried unchanged; corrupt state is rejected and preserved before any
provider request or mail submission. Older binaries do not understand the new
retry-key version field; do not downgrade against transitioned retry state.

### Provider retry restrictions

- deSEC HTTP 401, 400, and 404 stop requests until the token or profile identity
  (provider, target, endpoint) changes, or that profile's state is
  explicitly removed. Diagnostics identify authentication, hostname, or domain
  correction respectively. Source edits, an address change, or elapsed refresh do not
  release these intervention restrictions. Fixes made only on the provider side
  require explicit state removal to retry.
- deSEC HTTP 429 pauses pending deSEC updates until the persisted UTC deadline.
  Prefer one valid `Retry-After` (nonnegative integer seconds or HTTP date);
  missing, duplicate, malformed, or overflowing values use the owner-selected
  two-minute fallback. Zero/past deadlines permit the next invocation to retry.
  Deadlines are measured on receipt, not process start. Because accounts cannot
  be inferred reliably from tokens or domains, **all deSEC profiles sharing a state
  file share the latest active deadline**, even across tokens/endpoints. This can
  delay unrelated accounts conservatively. Already-successful no-op profiles
  remain successful. Token/target changes do not bypass a surviving shared pause.
- HE HTTP 200 with exactly `abuse` (surrounding whitespace allowed) reports failure
  without confirming the requested address. Persist a UTC deadline 15 minutes
  after receipt; suppress this profile until that deadline, even across restarts,
  address, credential, identity, or refresh changes. At or after expiry, retry the
  current address; another `abuse` starts a fresh 15-minute wait. Only confirmed
  success clears failure and notifies recovery. Other profiles continue normally.
  This delay is a project decision (#34), not a documented HE guarantee. Legacy
  zero-deadline abuse records and historical assumed-acceptance notification text
  are unsupported and rejected without modifying state; no migration is performed.
  The configurable refresh policy remains; review the HE production caveat in
  the provider contracts.
- Other responses and transport failures remain credential-safe failures, retried
  on the next invocation without an invented provider-specific delay.

Suppression returns nonzero with an actionable reason (and deadline for 429 and HE abuse),
without sending an update. Restriction state survives restarts under the existing
lock and atomic checkpoints. Intervention/HE restrictions store a random salt and
one-way SHA-256 fingerprint of relevant configuration and credentials, never the
credentials or a reversible encoding. Keep state private: the fingerprint is not
an encrypted credential vault and weak credentials remain susceptible to offline
guessing. Existing version-1 state remains readable without new fields; older
binaries do not understand the added retry fields and safely reject that state.
See [the response/retry matrix and sources](docs/provider-contracts.md).

### Notification sender display name

Optionally set a literal display name for a compatible local sendmail:

```yaml
sender_name: 'dyndns-updater on gateway-home'
```

With this setting, submission uses `sendmail -oi -F NAME -- RECIPIENT`, with
`NAME` passed as one separate argument, never through a shell. The updater does
not add a `From` header or specify a sender address or envelope sender (`-f`);
the local mail system remains responsible for those. The recipient and message
construction are unchanged. Omit `sender_name` (or use YAML null) to retain the
existing `sendmail -oi -- RECIPIENT` behavior. An explicitly empty or
whitespace-only name is invalid, as are names beginning with `-` after leading
whitespace, Unicode controls/format controls, and line/paragraph separators.
Validation occurs before provider requests, state locking/writes, or mail.

Spaces, punctuation, quotes, backslashes, percent signs and ordinary Unicode are
passed literally, without trimming, hostname lookup, substitutions or templates.
Ansible or another configuration manager can render a host-specific literal into
this value; the application does not interpret `%s`, `%(hostname)s` or `{{ ... }}`.
Changing the name does not affect profile identity, confirmations, notification
triggers or persisted summary bodies. Pending mail uses the current configured
name on every submission attempt; bodies and queue order remain unchanged.

This requires a sendmail-compatible MTA accepting a separate `-F` full-name
argument and generating the sender header when no `From` header is provided.
The observed Postfix setup supports this behavior; other MTAs/wrappers may ignore
or reject the option, so verify compatibility locally before enabling it. Failed
submission retains the existing durable retry behavior and credential-safe errors.
Local acceptance does not guarantee remote delivery, SPF, DKIM or DMARC success.
No MTA, gateway or DNS configuration is changed by the updater.

### Notifications and exits

One summary containing every configured profile is queued when a successful
update changes its recorded address (including initial confirmation), or a profile
enters/exits detection or provider failure (DNS, HTTP, transport, or API). No
notification is triggered by a no-op, successful same-address refresh, or repeated/changed
errors while still failing. Detection exhaustion now triggers failure mail, including
missing/private-interface-only profiles; recovery requires provider success. A previously
queued notification is still retried during detection outages. Invalid
configuration does not submit mail.

New notification lines use the exact configured profile name without added quotes,
for example `Tunnel Broker: confirmed 1.2.3.4`. CLI diagnostics and results also
use the exact validated name, without added quotes or escapes.

Each newly queued summary begins with `Summary generated at: 2026-01-01T00:00:00Z`
(using its actual generation time in UTC, RFC 3339). This is the summary creation
time, **not** the provider event time or eventual mail delivery time. The timestamp
is persisted with the body and never refreshed during retries or recovery of an
already queued message. If an interrupted run left only notification intent, the
summary is timestamped when reconstructed on restart; the original event time is
unknown. Existing timestamp-free pending messages are delivered unchanged.

Summaries and retry intent are persisted before local sendmail submission. Failed
submission returns nonzero and is retried later without repeating successful
provider updates. Pending summaries are delivered in order; after an interrupted
multi-profile run a durable change flag produces an all-profile summary on restart.
Delivery is **at least once**: a crash after local MTA acceptance but before writing
its acknowledgement may duplicate mail. Local acceptance does not guarantee final
delivery. No SMTP client or mail server is included.

Each invocation logs source attempts by index and selector, failures, and the
detected IPv4 address (with the quoted interface for an `if` source),
then the decision **before** contacting the provider. Update reasons distinguish
missing confirmed state, changed profile identity, retry after failure, changed
address, and due refresh. A no-op explicitly says the address is unchanged and
refresh is not due. For example:

```text
gateway-dns: source 1 (if) attempting IPv4 detection
gateway-dns: interface "en0" detected IPv4 8.8.4.4
gateway-dns: updating provider: no confirmed state
gateway-dns: confirmed 8.8.4.4
```

These operational trace lines go to stderr with the final all-profile results;
they do not enter persisted mail summaries or change notification triggers. An
unavailable interface produces a failed source attempt; later sources may still
succeed. No failed source fabricates an address or sends a provider update.

Every error returns a nonzero exit status, even when other profiles succeeded.
Each final profile result is printed once as `<profile name>: <result>`; unavailable
source diagnostics identify the ordered index and selector, without URL/query contents. Run-level errors use
`[error] <reason>` (without the profile-prefix colon). Profile names are rendered
literally after validation rejects controls and line separators. Configuration errors identify the quoted
attempted filename (including the default path when `--config` is omitted), without
including configuration contents or raw parser errors.
State-lock open errors identify the state directory that must exist and be writable.
Diagnostics omit credentials, raw provider bodies, and sendmail output. Secure
both configuration and state: state includes operational addresses and mail content.
Configuration is limited to 1 MiB and state to 8 MiB. A persistent mail outage
with many changes can fill the pending-summary backlog; an oversized state write
fails without replacing the previous state and requires operator attention.

### Profile names and migration

`profiles` is a YAML mapping keyed by each profile's name, not a list. Keys must
be nonempty strings. Names may contain spaces, punctuation, quotes, backslashes,
and ordinary Unicode text. Quote YAML keys that would otherwise be numbers,
booleans, null, or YAML syntax (for example `"17"`, `"true"`, or `"Home: DNS"`).
Unicode control characters, format controls (including bidi overrides), and
line/paragraph separators are rejected. Unknown settings and a separate `name`
field are rejected, as are duplicate decoded names, even with different quoting.

The exact decoded string identifies state: names are not trimmed, case-folded,
or Unicode-normalized. `Home DNS` and `"Home DNS"` identify the same profile;
`" Home DNS"` and `"home DNS"` do not. Profiles and newly generated summaries use
ascending decoded-name order (case-sensitive Unicode/code-point order), replacing
list order. Names are displayed literally without added quotes in CLI logs/results
and newly generated notification summaries, never as mail headers. Credentials
and raw parser diagnostics are not echoed.

Convert each former list entry's `name` into its mapping key and remove the
`name` field and list dash. For example, replace:

```yaml
profiles:
  - name: gateway-dns
    provider: desec
    # remaining settings...
```

with:

```yaml
profiles:
  gateway-dns:
    provider: desec
    # same remaining settings...
```

The former list syntax now fails with a mapping/migration diagnostic; no automatic
conversion is performed. Existing version-1 state needs **no conversion**: the same
decoded name retains state lookup. See [legacy confirmation transition](#legacy-confirmation-transition-37);
existing pending summaries remain readable and are delivered byte-for-byte.
Changing provider destination identity still forces an update; credential-only rotation does not.
Already pending notifications retain their original order and content (including
older quoted names and legacy unquoted names). State names and pending summaries are validated before any
provider or mail action. Pending bodies must match the current or legacy generated
summary status grammar with valid visible names, including names containing `: `
and literal quotes/backslashes. Escapes in a pending name are not decoded: literal
`\n` text is safe name text, while actual control characters are rejected. The
unchanged state format cannot distinguish an old quoted name from a new name
containing literal quotes; validation does not rewrite either. Arbitrary status
text, unsafe names, or malformed content fail safely
without replacing the state file. Preserve that file as evidence and investigate
rather than clearing the queue blindly. State is not authenticated: keep it private,
as validation cannot detect a tampered but otherwise well-formed result.

## Scheduling (examples, not an automatic deployment)

Install the binary at `/usr/local/bin/dyndns-updater` and review the provided
[systemd service](examples/systemd/dyndns-updater.service) and
[timer](examples/systemd/dyndns-updater.timer). The service creates the private
state directory; the timer invokes the one-shot service once per minute. It runs
as root to read private configuration and use the gateway's local MTA. The
[interface-up hook](examples/interface-up.sh) invokes the same command; install
it in the appropriate hook location for the gateway's network manager, with
executable permissions. Manual invocations must have access to the state directory.
No example is installed or enabled by the build or tests. Real-provider validation
and gateway migration require separate authorization.

## Development

Install Go 1.27.3 or newer. Run:

```sh
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
```

See [TESTING.md](TESTING.md), [agent guidance](AGENTS.md), and the
[development workflow](docs/development.md). Work uses issues, branches, PRs,
independent review, and human merges. The default suite uses only local fake
providers, temporary state, synthetic addresses, and fake sendmail; it never
updates real DNS or submits real mail.

## CI, releases, and dependencies

GitHub Actions checks formatting, runs `go vet`, and runs the test suite with the race detector once per pull request update and on pushes to `main`. Pushes to other branches and tags do not start a separate CI run. The same checks are reusable by the manually triggered release workflow, which publishes Linux and macOS binaries for amd64 and arm64, plus SHA-256 checksums.

### Manually release a tested commit

A human runs **Actions → Release → Run workflow**, supplying:

- `version`: a required version such as `v1.2.3` (`vMAJOR.MINOR.PATCH`, with no
  leading zeroes, prerelease suffix or build metadata).
- `commit`: optional, default `HEAD`. Empty or `HEAD` selects the repository's
  **default branch tip**, not the branch selected in the dispatch UI. To release
  an older commit, supply its commit SHA (or an available Git commit-ish).

The workflow fetches the default branch, resolves the selection once to a full
commit SHA, and rejects commits not reachable from that branch. Reusable CI
checks that exact commit; only after it passes does the release job build that
same commit and create the version tag if missing. Existing lightweight or
annotated tags are accepted only when they resolve to the tested commit. Tags
pointing elsewhere are rejected, never force-moved. A final tag check before
publication rejects tags moved after testing. If a concurrent tag creation
conflicts or the remote cannot be inspected, the run fails safely; inspect the
failure before rerunning. An already published release is not overwritten.

No tag needs to be created in advance. Workflow validation and the offline
regression suite do not dispatch a release, publish anything, or change gateway
configuration. Release publication remains a separate human-triggered operation.

### Try a pull request build

After the pull request's **CI** workflow succeeds, open its run from the PR's **Checks** tab (or the repository's **Actions** tab). Sign in to GitHub and choose the download for your machine from the run's **Artifacts** section:

| Platform | Artifact name |
| --- | --- |
| Linux amd64 | `dyndns-updater-pr-<PR number>-linux-amd64-<short merge SHA>` |
| Linux arm64 | `dyndns-updater-pr-<PR number>-linux-arm64-<short merge SHA>` |
| macOS amd64 (Intel) | `dyndns-updater-pr-<PR number>-macos-amd64-<short merge SHA>` |
| macOS arm64 (Apple Silicon) | `dyndns-updater-pr-<PR number>-macos-arm64-<short merge SHA>` |

Artifacts expire after 14 days; rerun CI if needed. Each download is a ZIP containing only that target's `.tar.gz` archive and its own `checksums.txt`. The archive contains an executable named `dyndns-updater`; archive filenames use `linux` for Linux and `darwin` for macOS, with `amd64`/`arm64` selecting the CPU architecture (Apple Silicon uses `darwin_arm64`).

Extract the ZIP, then verify and unpack the archive for your machine, for example on Linux amd64:

```sh
sha256sum -c checksums.txt
mkdir -p pr-build
tar -xzf dyndns-updater_<merge-SHA>_linux_amd64.tar.gz -C pr-build
./pr-build/dyndns-updater --help
```

Replace `<merge-SHA>` with the short SHA in the downloaded filenames. Both artifact names and archive filenames use Git's abbreviation of the checked-out merge commit (`git rev-parse --short HEAD`), computed once by the build step. Git chooses a unique prefix (normally at least seven characters), so its length is not fixed; the full SHA remains available in the workflow run. On macOS use `shasum -a 256 -c checksums.txt` to verify checksums. The tar archive preserves the executable bit; if your extraction tool removes it, run `chmod +x pr-build/dyndns-updater`.

These are untrusted, unsigned test builds of GitHub's PR **merge commit**, not just the branch head and not a published release. Review the changes before running them and use an isolated test environment without production credentials. Checksums detect damaged downloads, not a malicious build. PR CI only builds and uploads artifacts after the checks pass; it neither publishes releases nor deploys to a gateway. Push CI and release CI do not upload PR artifacts. Cross-compilation does not establish runtime compatibility on macOS or arm64.

This implementation does not publish a release, enable a timer, or deploy anything.
Renovate proposes Go, module, and GitHub Actions updates.
