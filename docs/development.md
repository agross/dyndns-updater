# Development workflow

The human owner is `agross`. The [development policy](policy/README.md) is maintained in this repository. It originated in `agross/dev` but is now independent. The policy is not evidence that automation or authorization controls are installed.

Work starts in a GitHub issue. An issue labeled `hermes:developer` is ready for implementation; the label is sufficient authorization for its scope. Separate explicit scope approval is not required. Use a branch and pull request, run CI and independent review, and provide evidence and outstanding risks. The human merges; releases require a human-triggered workflow. Operate under supervision until separate identities and permissions have been implemented and verified. Existing Hermes automation may still enforce that older requirement until updated.

Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and check formatting with `gofmt -l .`. The CLI currently provides help only. Go must be installed separately. No production deployment is authorized by onboarding.

Future policy changes require review. No separate preview host or support integration is configured; Hermes automation has not yet been updated or verified against the label-based rule.
