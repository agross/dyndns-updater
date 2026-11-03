# Agent instructions

- Read README.md and TESTING.md before changing behavior; keep them accurate.
- Work in small vertical slices. For behavior changes, add a failing test at an agreed public boundary, implement it, then run the relevant checks. The CLI and provider HTTP requests are agreed boundaries.
- Use local fake services and temporary files in automated tests. Never update real DNS or send real mail in the default suite.
- Preserve independent profile outcomes and write state only after confirmed provider success. Never expose credentials in logs, test fixtures, or commits.
- Treat provider responses, configuration, IP addresses, state files, and network failures as untrusted. Preserve IPv6 when the profile requests it.
- Run `go test ./...` and `go vet ./...` for Go changes; format changed Go files with `gofmt`. Explain checks that could not run.
- Ask before introducing provider-specific behavior that cannot be determined from documentation or verified examples. Do not deploy or change gateway DNS without explicit authorization.
- Keep CI and release checks aligned; releases must pass the test workflow before publication. Keep Renovate configuration valid as dependencies and Actions change.
- For every task, read `docs/development.md` and `docs/policy/README.md`; identify the GitHub issue and verify explicit scope approval before implementation. Work in a branch and prepare a pull request for human review and merge. Treat unimplemented approval and identity controls as supervised, not automatic.
- The `ready-for-agent` triage label for this repository is `hermes:developer`. A label does not grant scope approval.
