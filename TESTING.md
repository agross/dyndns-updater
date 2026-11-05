# Testing

Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and `gofmt -l .`.
The default suite needs no credentials and never updates real DNS or submits real
mail. Local HTTP fake providers, temporary configuration/state, synthetic
interface inputs, controlled timestamps, and fake sendmail executables exercise
observable CLI and provider boundaries.

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
- `TestCLILegacyStateAndPendingMail` reuses a pre-mapping version-1 state fixture
  without provider calls and preserves pending content/order across mail failure.
- `TestCLIUnsafeStatePreserved` rejects unsafe state names and pending content,
  asserts no provider/mail activity, and compares original state bytes afterward.
- `TestBuiltCLIMappingNameAndFakeMail` builds and runs the real binary against a
  local HTTP service and executable fake sendmail, asserting exact output, full
  mail content, persisted names, no-op reuse, and rejection without side effects.
- Existing process-kill, concurrency, checkpoint, notification retry, provider
  contract, diagnostic, and journey tests now use mapping configuration and quoted
  output. They continue to exercise the same durability/delivery boundaries.

## Provider retry acceptance (#16)

- `TestCLIDeSECRetryDeadline` checks missing/invalid/overflowing/valid delta/date
  Retry-After, failure and suppressed exits, deadline persistence across CLI runs,
  retry at the boundary, IPv6 preservation, queued-mail retry and recovery mail.
- `TestCLIInterventionRecovery` independently tests HTTP 400/401/404 after a known
  confirmation, retention of the previous address, suppression despite elapsed
  refresh, no repeated failure mail, token/target/interface/endpoint corrections,
  state removal, recovery, and exclusion of secrets/untrusted text from state/mail.
- `TestCLISharedDeSECRateLimit` checks a longer preferred deadline, distinct-token
  sharing, no request for a second deSEC profile, independent HE success, source
  profile removal, configuration/credential changes not bypassing a surviving wait,
  unchanged-failure mail deduplication, and deadline recovery.
- `TestCLIHEAbuseAcceptedButFailing` checks the owner-requested address advancement
  while failing, same-address suppression across CLI runs and address-change recovery.
  `TestCLIHEAbuseResumeConditions` checks exact refresh and credential/identity
  correction boundaries. `TestCLIUnverifiedHEErrorsAreNotAssumedAccepted` rejects
  unknown tokens, misleading abuse/success, oversized bodies and address mismatch
  without inventing HE cooldowns or accepting unconfirmed addresses.
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
  the CLI, names both absent interfaces, and checks single profile diagnostics
  without mail. `TestCLIProfileAndMailFailuresReportedOnce` checks provider
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
  errors, recovery, missing-interface suppression, and all-profile summaries.
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


The subprocess interface tests use Linux `lo` and skip when that interface is not
available (for example macOS); the synthetic-address journeys run on both release
platforms. Tests use brief bounded waits only to assert that a blocked invocation
has not reached its provider, never to simulate the refresh interval.

[Provider fixtures are based on authoritative documentation](docs/provider-contracts.md),
not live-provider experiments. Real-provider verification, actual gateway network
manager integration, MTA delivery, timer installation, and production migration
remain separate authorized operator checks. Public IP discovery and fallback DNS
are out of scope for #2 (issues #3/#4).
