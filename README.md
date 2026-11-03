# dyndns-updater

A planned command-line dynamic DNS client, distributed as a single Go binary. This repository is an early development scaffold; it cannot update DNS yet.

## Goals

- YAML configuration with independent profiles for deSEC and Hurricane Electric Tunnelbroker.
- IPv4 discovery from an interface or a public service; preserve deSEC IPv6 records when configured.
- Persistent state to avoid unnecessary updates, with periodic refresh and retry after failures.
- One-shot operation scheduled by systemd timers, including a refresh after a configurable interval (the intended gateway uses 25 days).
- Configurable fallback DNS resolvers and email notifications through the local mail submission program.

The existing gateway uses interface `telekom`, DynDNS2 for both providers, and deSEC's `myipv6=preserve` parameter. Credentials and hostnames belong in local configuration, not this repository. Provider-specific refresh limits and responses need verification before implementation.

## Development

Install Go 1.27.3 or newer. Run `go test ./...` and `go vet ./...`. See [TESTING.md](TESTING.md) for test boundaries and planned coverage. `go run . --help` shows the current scaffold; running without arguments exits with an error because updating is not implemented.

## Initial slices

1. Specify configuration and validation, including secret handling.
2. Add one-shot interface detection and a DynDNS2 update against local fake servers.
3. Add reliable state, independent profiles, retries, and timer-based refresh.
4. Add local email notifications for the first release; add public IP detection and fallback DNS in later slices.

Order and details may change as provider behavior is verified. No deployment configuration or production migration has been validated yet.

## CI, releases, and dependencies

GitHub Actions checks formatting, runs `go vet`, and runs the test suite with the race detector on pushes and pull requests. A human starts the release workflow for an existing tag in the form `v1.2.3`; it runs those checks before publishing a GitHub release with Linux and macOS binaries for amd64 and arm64, plus SHA-256 checksums. Releases are not yet configured or tested on GitHub; they require a GitHub remote and Actions permissions to create releases.

Renovate is configured to propose dependency and GitHub Actions updates. Enable the Renovate GitHub App for the repository after it exists on GitHub. The Go module path is provisional until the repository URL is known.

## Agreed project direction

The intended GitHub repository is public under `agross`. Work will use branches, pull requests, passing CI, and human merges. The first usable release must support the gateway’s deSEC and Tunnelbroker profiles and email notifications through the local mail submission program. A systemd timer will schedule the one-shot command every minute. Concurrent invocations wait for an operating-system lock on a separate stable lock file; it is released when the holder exits or is killed, while the state file can be replaced atomically. Send one email listing every configured profile when any profile changes meaningful state: its address changes and the update succeeds, it enters a DNS, HTTP, or provider API failure, or it recovers from such a failure. Repeated unchanged failures, an unchanged successful refresh, and other no-ops do not send mail. Persist profile and notification state across invocations; retry a notification if local sendmail submission fails. All errors yield a nonzero exit status. Interface and configuration errors do not trigger mail; DNS, HTTP, and provider API errors may trigger mail when their failure state changes. Submit mail using the local sendmail interface. Detailed scope and the onboarding workflow are still being discussed.

Removing the state file forces updates on the next run. A corrupt or unreadable state file is an error and must be preserved for diagnosis. Notification delivery is at least once: if the process dies after the local MTA accepts mail but before acceptance is recorded, a retry may duplicate it. Each run detects the current address independently; a change during an update is handled by a subsequent run.
