# Testing

Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and `gofmt -l .`.
The default suite needs no credentials and never updates real DNS or submits real
mail. Local HTTP fake providers, temporary configuration/state, synthetic
interface inputs, controlled timestamps, and fake sendmail executables exercise
observable CLI and provider boundaries.

## Release selection acceptance (#39)

`go test -run TestRelease ./...` reads the real release workflow and executes
its validation, resolution and tag shell steps against temporary local Git
repositories with a default branch named `trunk` and a divergent dispatch HEAD.
It checks omitted/explicit HEAD, older reachable commits, unreachable/unknown/
option-like/multiline selections, strict version syntax, creation of a missing
tag, matching lightweight/annotated tags, conflicting-tag preservation and
inaccessible-remote failure. Git and Bash are required; no GitHub credentials,
network repositories or release publication are used. The workflow-gate test
also binds the dispatch defaults, tested checkout SHA and build/tag/publish
ordering to the reusable CI dependency. The publication step is deliberately
not executed locally. Run `actionlint .github/workflows/*.yml` as well.

## Source-independent confirmation acceptance (#37)

This supersedes #3's original source-dependent confirmation contract.

- `TestCLISourceOnlyConfirmation` confirms through local web discovery, switches to
  a synthetic interface, reverses the change, adds/removes/replaces/reorders sources
  and checks request/mail counts, unchanged output, retained address/time, discovery
  on every invocation, and refresh at the original exact deadline.
- `TestCLISourceEditChangedAddressStrictAcceptance` exercises a changed address,
  HTTP-200 malformed success, retained confirmation/failure deduplication, and
  provider-confirmed recovery after further source edits. Destination-change tests
  cover provider, target, endpoint and missing confirmation.
- `TestCLILegacySourceConfirmation` uses independently constructed historical hashes
  to test exact reuse, unverifiable source edits requiring one fresh confirmation,
  durable transition and unchanged legacy queued bodies. The interface-era journey
  also reuses an exactly verifiable legacy identity. `TestCLILegacyUnverifiedRejected`
  rejects trust in an unknown destination and advances only on strict acceptance.
- Legacy intervention tests cover all deSEC holds, source edits before migration,
  repeated runs, detection failure, credential correction and subsequent destination
  correction. An unverifiable simultaneous destination correction remains
  conservatively restricted; see README. Legacy deadline tests cover deSEC shared
  wait propagation/removal and HE pauses through source edits and exact expiry.
- `TestBuiltCLISourceEditQueuedMail` runs a real binary and fake sendmail across
  source replacement and restart, retries queued bytes without new provider updates,
  retains confirmation time/address and detects again each time. Corrupt retry-key
  versions are rejected without changing state or attempting provider/mail activity.
- Existing ordered detection, validation, independent-profile, cleanup/rename,
  checkpoint, lock, notification durability, deSEC IPv6 preservation and provider
  restrictions remain covered by the full suite. No live services are used.

Focused: `go test -run 'TestCLISource|TestCLILegacy|TestCLICorruptRetryVersion|TestBuiltCLISourceEdit' ./...`.
Final gates: full tests, full race suite, vet, build, formatting and diff checks.

## Ordered public IPv4 discovery acceptance (#3)

All behavior tests exercise the CLI, real HTTP transports, temporary configuration
and state, synthetic interface inputs and local discovery/provider/mail fixtures.
Public-looking addresses are synthetic response data only, never dial targets.

- `TestCLIDiscoverySourceObjects` and `TestCLIRejectSourceConfigurations` cover
  required object lists, scalar rejection, exactly-one selectors, duplicate/unknown
  keys, non-string/empty values, named-service validation, URL safety, legacy
  `interface` rejection and invalid later entries before locking or other effects.
- `TestCLIDiscoveryOrderedSources` exercises interface-first, private/missing
  interface fallback, web-first and stop-on-first-success, including no leaked
  provider authentication and deSEC `myipv6=preserve`.
- `TestCLIDiscoveryNamedServices` uses generated, trusted local TLS certificates,
  local system DNS and fail-closed local dial routing for the actual named URLs.
  It checks Host/SNI/path, IPv4 dialing, plain-text whitespace, the verified HE HTML
  shape, surrounding markup, unrelated addresses, absent and ambiguous markers.
  `TestCLIDiscoveryMixedNamedAndCustom` checks ordered named fallback and proves a
  custom URL at the HE hostname still rejects HTML as non-plain-text.
- `TestCLIDiscoveryInvalidResponsesFallback` rejects non-public IPv4 categories,
  malformed/multiple addresses, prose, IPv6, HTML/JSON and oversized bodies.
  `TestCLIDiscoveryHTTPFailuresAndBudgets` covers status, redirect, truncated read,
  slow headers/body, untrusted TLS, connection failures and a fresh next-source
  budget after the first times out. Local provider requests still succeed.
- `TestCLIDiscoveryDNSFallback` uses local UDP/TCP fixtures for system/first/later
  success, negative/empty/unusable/IPv6-only answers, SERVFAIL, truncation, timeout,
  exhaustion, original Host/SNI and no resolver fallback after HTTP/redirect errors.
  `TestCLIFallbackDefaultsAndOverrides` now runs both provider and discovery
  journeys, redirecting all default/public resolver destinations to local fixtures.
  `TestCLIDiscoveryLiteralSkipsDNS` proves literal-IP discovery bypasses DNS;
  `TestCLIDiscoverySharedDNSAndHTTPBudget` shares one deadline across DNS and HTTP.
  Repeat deadline tests under the race detector to catch detached DNS workers.
- `TestCLIDiscoveryExhaustionDurability` preserves the prior address/time, persists
  failure intent, deduplicates repeated failures, retries identical queued mail,
  continues independent profiles and clears failure only after provider acceptance.
  `TestBuiltCLIDiscoveryFailureMailRestart` repeats the journey with a built binary,
  separate processes and executable fake sendmail, including failed mail retries.
- `TestCLIDiscoveryIdentityAndFailureDeduplication` requires provider recovery for
  outstanding failure after source-list changes without duplicate failure mail; refresh, no-op,
  provider intervention, shared deSEC deadlines and HE abuse-pause journeys now use
  source-object configurations and retain their durable restriction assertions.
- Existing cleanup, checkpoint, process-kill, locking, name, notification, retry,
  DNS and provider suites remain active. Expected diagnostics and detection-failure
  mail changed deliberately; no tests update real DNS, send real mail or change
  system/gateway settings. Built journeys use local web sources instead of accepting
  loopback as public. No compatibility alias or configuration migration is tested.

Focused command: `go test -run 'TestCLIDiscovery|TestCLIRejectSource|TestBuiltCLIDiscovery|TestCLIFallbackDefaults' ./...`.
Final gates remain the full suite, full race suite, vet and clean `gofmt -l .`.

## Profile and DNS logging acceptance (#32)

- `TestCLIFallbackResolutionTrace` checks exact ordered attempt/failure/success
  output with local DNS and HTTPS fixtures, first/later success, unusable/empty
  answers, exhaustion, disabled fallback and timeout without later attempts.
  Names with spaces, Unicode, quotes, literal backslashes and percent text are
  rendered literally; credentials, URLs and raw errors stay out of diagnostics.
- System-success and overlapping DNS/HTTP deadline tests distinguish DNS recovery
  from provider confirmation. Deadline diagnostics retain the active resolver and
  never invent DNS success. DNS traces stay out of notification summaries.
- Existing ordinary/space-name, built-CLI, unusual-name, invalid-name/state,
  credential-safety and independent-profile journeys retain their assertions,
  with only CLI prefix expectations changed. State and mail bytes are unchanged.

## Fallback DNS acceptance (#4)

`go test -run 'TestCLI.*Fallback' ./...` exercises the CLI entry point with
synthetic interface addresses, temporary configuration/state, fake mail, local
UDP/TCP DNS servers and a locally trusted HTTPS provider. The system resolver is
redirected to a local fixture in-process; no system DNS settings are modified.
These tests are intentionally nonparallel while replacing process-global default
resolver/transport values, and restore them afterward.

- `TestCLIFallbackDNSOrderAndBoundaries` asserts system-first/first/later fallback
  requests, negative/empty/SERVFAIL/unusable answers, UDP-to-TCP truncation,
  original HTTP Host/TLS SNI, deSEC authentication/IPv6 preservation, no duplicate
  provider request and no fallback for HTTP/provider failures or redirects.
- `TestCLIFallbackFailureRecovery` retains confirmed addresses/times on DNS failure,
  allows another profile to confirm, retries across invocations without a new
  provider cooldown, and checks failure-mail deduplication and recovery mail.
- Deadline tests cover unresponsive DNS, cancellation, no attempts beyond expiry,
  and one shared budget for successful DNS followed by a slow HTTP response.
- TLS tests reject untrusted certificates and wrong hostnames; connection failures
  do not trigger fallback. Proxy tests retain HTTPS CONNECT policy and never
  bypass a failed proxy or perform provider DNS locally when a proxy owns it.
- IPv6 loopback tests contact a bracketed IPv6 DNS endpoint (skipping only when
  IPv6 loopback is unavailable); literal provider endpoints do not query DNS.
- `TestCLIFallbackDefaultsAndOverrides` verifies omitted configuration uses
  `1.1.1.1:53`, `8.8.8.8:53`, then `9.9.9.9:53`; explicit `fallback_dns: []`
  disables fallback; a nonempty custom list replaces the defaults. Default
  resolver dialing is redirected to local fixtures, with unknown destinations
  rejected, so these tests never contact public DNS. CLI outcomes, provider
  requests and persisted confirmations are checked as well as ordered endpoints.
- Unavailable/disabled-list tests cover a closed local resolver, later success and
  system-only failure behavior when an explicit empty list disables fallback.
- CLI diagnostics tests cover failed system/fallback lookups, empty/unusable
  answers, disabled fallback, quiet successful lookup, and credential-safe logs
  excluded from notification summaries.
- Configuration tests accept IPv4/IPv6 literals with default/explicit ports and
  reject invalid later entries before interface/provider/mail/state side effects.

The full existing suite continues to exercise locking, atomic checkpoints, retry
restrictions, independent profiles and built-CLI/fake-sendmail journeys. No live
provider, resolver, mail or gateway configuration is used for fallback verification.
An unreachable resolver may consume the entire shared request budget, so later
fallbacks are not guaranteed to run; this is documented rather than hidden by a
per-resolver timeout reset.

## CI and artifact verification

For workflow changes, run `actionlint .github/workflows/*.yml` and `git diff --check` in addition to formatting, vet, and race tests. Verify that CI's `push` trigger includes only `main`, `pull_request` remains enabled, and `workflow_call` still accepts the release commit ref. The release workflow must continue to pass `needs.resolve.outputs.commit` to CI and require both resolution and tests before publication. Do not trigger a release to test this configuration.

For PR artifacts, exercise the workflow's build commands for Linux and macOS on amd64 and arm64, verify `checksums.txt`, extract the Linux amd64 archive on a matching host, and run its `--help`. On GitHub, confirm exactly one `pull_request` CI run for the current head, no feature-branch `push` CI run, and exactly four separately downloadable artifacts clearly named for Linux amd64, Linux arm64, macOS amd64, and macOS arm64. Download each artifact and confirm it contains only the matching archive and a `checksums.txt` that verifies that archive independently. Verify every archive's executable mode and run the extracted Linux amd64 `--help` on a matching host. Confirm the artifact names and archive filenames use Git's short abbreviation of the run's checked-out merge SHA (`git rev-parse --short HEAD`, distinct from the PR head; length is not fixed), matching the build step's `short_sha` output, and retain the 14-day expiry. A `main` push cannot be exercised until a human merges; check its trigger configuration without pushing to `main`. These checks do not prove runtime compatibility on every target. See the [artifact instructions](README.md#try-a-pull-request-build).

## Removed-profile state acceptance (#15)

- `TestCLIProfileCleanup` verifies cleanup on no-op and unavailable-interface
  runs, retained confirmations, independent provider failure/success, no cleanup
  notification, and a fresh update when the same name/identity/address is restored.
- `TestCLIRenamedProfileRequiresSuccess` verifies old-name removal, an attempted
  update under the new name, no confirmation on failure, and successful retry.
- `TestCLICleanupPreservesQueuedMailAndIntent` preserves queued bodies and order
  (including removed names) through mail failure, and recovers existing dirty
  intent into a current-configuration summary without redundant provider calls.
- `TestCLICleanupRejectsInvalidInputsWithoutWrites` compares state bytes after
  invalid configuration and corrupt state, with no extra provider/mail activity.
- `TestBuiltCLICleanupRetriesRemovedProfileMail` uses the built binary, local HTTP
  provider, and executable fake sendmail: removal persists during a no-op, queued
  mail is retried byte-for-byte once, and a subsequent invocation adds no mail.
- `TestInterruptedLaterProfileKeepsEarlierSuccess` now also kills the real process
  after a confirmation checkpoint, removes that confirmed profile, and restarts.
  It verifies cleanup, OS lock release, provider counts, and recovery of notification
  intent into a summary containing only the current configuration.
- Exact-name identity tests retain their names in configuration while testing
  distinct identities; absent names are no longer expected to accumulate in state.

## Notification sender display name (#30)

- `TestBuiltCLISenderName` builds and runs the CLI with temporary configuration,
  a local HTTP provider and an executable fake sendmail. NUL-delimited argument
  capture verifies the exact omitted/configured arguments, a single literal
  `-F` value (spaces, punctuation, quotes, Unicode, percent/template/shell-like
  text), unchanged recipient handling and no envelope override. Complete message
  bytes prove no explicit `From` header and unchanged summary construction.
- The same journey captures failed submission (without leaking MTA stderr), then
  retries two queued summaries in order with a changed configured name, unchanged
  bodies/confirmations, no redundant provider update and no new no-op mail.
- `TestCLIRejectUnsafeSenderName` rejects empty/blank/option-like values, CR, LF,
  NUL, tab, DEL, Unicode controls, format controls, line/paragraph separators and
  non-scalar values through CLI configuration validation. It asserts no provider
  or mail attempts, no new state/lock entries and byte-identical existing state.
- Run `go test -run 'TestBuiltCLISenderName|TestCLIRejectUnsafeSenderName' ./...`.
  The full suite retains independent-profile, notification trigger and durability
  regressions. Tests use no real DNS updates or mail; real MTA compatibility and
  remote delivery/authentication remain operator checks.

## Notification generation timestamps (#24)

- `TestCLINotificationGenerationTime` uses the CLI with controlled non-UTC time,
  local providers and fake mail to assert the exact UTC body, persisted queue,
  failed submission attempts, retry bodies and ordered distinct timestamps. It
  checks reconstructed intent uses restart time, independently timed failure and
  recovery transitions, no-op suppression, and provider/mail call counts.
- `TestCLINotificationPendingCompatibility` retries mixed timestamp-free legacy,
  quoted/current and timestamped bodies byte-for-byte, including a legacy profile
  named `Summary generated at`, without changing the version-1 state format.
- `TestCLIRejectMalformedNotificationNames` also rejects invalid/noncanonical time,
  duplicate/misplaced headers, missing results and injected content before provider
  or mail activity, comparing original state bytes after rejection.
- `TestBuiltCLIMappingNameAndFakeMail` checks a real binary timestamp against its
  execution window and the complete fake-sendmail message.
  `TestKilledAfterLocalMailAcceptanceRetries` verifies both submissions contain
  exactly the same persisted timestamped body through the duplicate-mail crash
  window. Existing trigger, CLI-output, independent-outcome and credential-safety
  tests remain active; no real DNS or mail is used.

## Notification name presentation (#22)

- `TestCLINotificationNamesAndRestart` asserts exact unquoted all-profile mail,
  literal unquoted CLI results, sorted spaces/punctuation/quotes/backslashes/
  colons/Unicode, distinct quoted and unquoted names, and durable retry after
  failed submission without repeating providers.
- `TestCLINotificationPendingCompatibility` delivers old quoted, legacy and new
  summaries byte-for-byte in queue order across failed mail and restart.
- `TestCLIRejectMalformedNotificationNames` checks invalid status/address,
  delimiter-like text, injected lines, controls, empty/duplicate names and state
  preservation before provider/mail activity.
- Existing fake-sendmail subprocess, independent failure/recovery, no-op,
  crash/checkpoint and cleanup journeys retain CLI assertions while expecting
  literal names only in newly generated mail. Literal backslash escape text is
  a valid name; the unsafe pending fixture uses an actual control character.

## Mapping profile-name acceptance (#17)

- `TestCLIMappingNames` checks exact CLI output, provider call counts, decoded
  names in state/mail, and unchanged identity across quoted/unquoted YAML keys.
- `TestCLIRejectUnsafeMappingNames` rejects non-string/empty/control/format/line
  separator keys, duplicate decoded names, unknown fields, a separate `name`, and
  legacy lists before provider/mail activity or state creation.
- `TestCLISafeNameRenderingAndOrder` asserts sorted provider requests, exact safe
  summary rendering of punctuation/escapes, independent outcomes, and retry.
- `TestCLIExactDecodedNameIdentity` covers spaces, numeric/boolean-looking string
  names, case/whitespace/Unicode distinctions, identity changes, refresh, and address
  changes without live providers.
- `TestCLILegacyStateAndPendingMail` reads an interface-era version-1 state fixture,
  reuses verifiable confirmation and preserves
  legacy pending content/order across mail failure.
- `TestCLIUnsafeStatePreserved` rejects unsafe state names and pending content,
  asserts no provider/mail activity, and compares original state bytes afterward.
- `TestBuiltCLIMappingNameAndFakeMail` builds and runs the real binary against a
  local HTTP service and executable fake sendmail, asserting exact output, full
  mail content, persisted names, no-op reuse, and rejection without side effects.
- Existing process-kill, concurrency, checkpoint, notification retry, provider
  contract, diagnostic, and journey tests now use mapping configuration and unquoted
  output. They continue to exercise the same durability/delivery boundaries.

## Provider retry acceptance (#16)

- `TestCLIDeSECRetryDeadline` checks missing/invalid/overflowing/valid delta/date
  Retry-After, failure and suppressed exits, deadline persistence across CLI runs,
  retry at the boundary, IPv6 preservation, queued-mail retry and recovery mail.
- `TestCLIInterventionRecovery` independently tests HTTP 400/401/404 after a known
  confirmation, retention of the previous address, suppression despite elapsed
  refresh, no repeated failure mail, source edits retaining intervention, token/target/endpoint corrections,
  state removal, recovery, and exclusion of secrets/untrusted text from state/mail.
- `TestCLISharedDeSECRateLimit` checks a longer preferred deadline, distinct-token
  sharing, no request for a second deSEC profile, independent HE success, source
  profile removal, configuration/credential changes not bypassing a surviving wait,
  unchanged-failure mail deduplication, and deadline recovery.
- #34 supersedes #16's original HE assumed-acceptance behavior:
  `TestCLIHEAbuseUnconfirmedAndFailing` verifies an initial abuse leaves no confirmation,
  including changed-address suppression and recovery after 15 minutes.
  `TestCLIHEAbuseResumeConditions` checks exact deadline and credential/identity/
  refresh changes cannot bypass the pause. `TestCLIHEAbuseDeadlineJourney` covers
  response-time UTC deadlines with subsecond/non-UTC clocks, preserved prior
  confirmation, independent profiles, durable failed mail, repeated abuse renewing
  the deadline, exact-boundary recovery and quiet subsequent runs.
  `TestBuiltCLIHEAbuseSurvivesRestart` executes separate processes with loopback
  HTTP and fake sendmail, checking the receipt-time deadline and one failure mail.
  `TestCLIHEAbuseMalformedStatePreserved` rejects invalid deadlines, fingerprints,
  failure flags and queued statuses without provider/mail activity or file changes,
  including zero-deadline records with an address and historical HE assumed-status
  messages. Legacy HE migration is intentionally unsupported per PR #35 review.
  `TestCLIUnverifiedHEErrorsAreNotAssumedAccepted` rejects
  unknown tokens, misleading abuse/success, oversized bodies and address mismatch
  without inventing retry rules for unknown responses or accepting unconfirmed addresses.
- `TestCLIRetryAfterEdges` checks zero, short valid delays, past HTTP dates, signed/
  fractional/duplicate headers, no early retry, and exact-boundary recovery.
- `TestCLIInvalidRetryStatePreserved` rejects invalid retry codes/fields and compares
  original bytes with no provider request. Existing legacy-state, corrupt/unreadable
  state, process-kill, atomic checkpoint, concurrency and mail tests still run.
- `TestBuiltCLIRetryRestrictionsSurviveRestart` builds the actual executable,
  exercises separate invocations with OS loopback and executable fake sendmail,
  proves one auth request and one failure notification across restart, then rotates
  credentials and verifies strict confirmation/recovery without credential leaks.

The generic retry trace test now uses HTTP 500; deSEC 401 has its own intervention
assertions rather than expecting an unconditional retry. No checks were weakened.
All tests are offline; provider response fixtures follow the source matrix, with
HE abuse and deSEC fallback explicitly attributed to the owner's issue decisions.

## Acceptance evidence
- `TestCLIHelp` builds and runs CLI help.
- `TestCLIConfigurationFileDiagnostics` checks exact output and nonzero exits for
  missing files, unreadable directory paths, and invalid YAML, including quoted
  filenames and credential-safe errors. Run-level errors use `[error]`, distinct
  from profile names; mixed profile/mail failures retain both independent errors.
- `TestCLIMissingStateDirectory` identifies the required state directory.
- `TestCLIMissingInterfacesReportedOnce` exercises OS interface lookup through
  the CLI, checks ordered source-failure diagnostics and one all-profile
  detection-failure notification without provider requests. `TestCLIProfileAndMailFailuresReportedOnce` checks provider
  failures are not duplicated and run-level mail errors remain visible.
- `TestCLIUpdateDecisionTrace` asserts exact CLI trace output, exit status, and
  provider-call counts for initial update, no-op, address change, refresh at the
  configured boundary, failure/retry/recovery, and changed profile identity. It
  covers quoted source interfaces, credential-safe logs, and separation from mail
  summaries. Existing multi-profile missing-interface and mixed provider/mail
  failure tests retain their exact-output assertions with trace lines where relevant.
- `TestUpdateJourney` covers first confirmation, no-op, changed address,
  controlled 25-day refresh, state removal, and deSEC IPv6 preservation/auth.
- `TestFailureTransitionsAndIndependentProfiles` covers Tunnelbroker request/auth,
  `nochg` confirmation, independent results, failure entry, persistent/changed
  errors, recovery, durable missing-interface failure, and all-profile summaries.
- `TestIdentityAndCorruptState` covers credential-only rotation, provider/target/
  interface changes, malformed/invalid state preservation, and unreadable state.
- `TestRejectedResponsesAndTimeout` covers HTTP failures, rejected/malformed/
  oversized responses, response-address mismatch, retry, and bounded timeout.
- `TestInvalidConfigDoesNotLeakOrMail` covers malformed/multiple-document YAML,
  unknown keys, empty profiles, insecure/credential-bearing endpoint rejection,
  nonzero exits, and secret-free diagnostics. `TestStateContainsNoCredentials`
  checks persisted state.
- `TestPendingMailAndCrashBoundary` covers pending submission retries independent
  of provider updates and durable notification intent after interruption.
- `TestAddressChangeDuringUpdate` records only the requested address and catches
  the later address on the next invocation.
- `TestBuiltCLIWithLocalSendmail` executes the actual built CLI, OS loopback
  detection, local HTTP provider and executable fake MTA; verifies exact detection/
  decision/result output for first update and no-op, persistent lock-file existence
  after success and failure with successful subsequent invocation, private state
  permissions, sendmail failure exit, and later mail retry.
- `TestConcurrentCLIAndKilledLockHolder` executes overlapping CLI processes,
  proves the second waits, kills the lock holder, and verifies OS lock release,
  retry, and intact state. It also tests normal serialized no-op behavior.
- `TestInterruptedLaterProfileKeepsEarlierSuccess` kills a process while its
  second provider is blocked and verifies the earlier confirmed result and
  notification intent survive restart without a redundant first-provider request.
- `TestConfigurationValidationAtCLI` rejects invalid durations, recipient header
  injection, relative paths, duplicates, invalid credentials/targets, and oversized YAML.
- `TestKilledAfterLocalMailAcceptanceRetries` uses an executable fake MTA that
  accepts mail then SIGKILLs its parent updater. Restart retries the durable
  pending message, demonstrating the documented duplicate-mail crash window.


Some historical subprocess tests still check Linux `lo` availability and skip when it is not
available (for example macOS); subprocess addresses now come from loopback web
fixtures because loopback interface addresses are not public. The synthetic-address
journeys run on both release platforms. Tests use brief bounded waits only to assert that a blocked invocation
has not reached its provider, never to simulate the refresh interval.

[Provider fixtures are based on authoritative documentation](docs/provider-contracts.md),
not live-provider experiments. Real-provider verification, actual gateway network
manager integration, MTA delivery, timer installation, and production migration
remain separate authorized operator checks. Public discovery and fallback DNS are covered by the local fixtures for #3/#4;
no test requires external discovery or public resolver traffic.
