# Development workflow

The human owner is `agross`. Shared policy is copied from `agross/dev` into [policy/README.md](policy/README.md), with its linked references alongside it. Project-specific decisions and commands are recorded here. The copy is local policy, not evidence that automation or authorization controls are installed.

Work starts in a GitHub issue. An issue labeled `hermes:developer` is ready for implementation; the label is sufficient authorization for its scope. Separate explicit scope approval is not required. Use a branch and pull request, run CI and independent review, and provide evidence and outstanding risks. The human merges; releases require a human-triggered workflow. Operate under supervision until separate identities and permissions have been implemented and verified. This project rule overrides the copied policy’s separate scope-approval requirement. Existing Hermes automation may still enforce that older requirement until updated.

Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and check formatting with `gofmt -l .`. The CLI currently provides help only. Go must be installed separately. No production deployment is authorized by onboarding.

The shared onboarding guide recommends recording a policy revision; the owner chose not to record one locally. Future adjustments to the project and its policy copy require review. No separate preview host or support integration is configured; Hermes automation has not yet been updated or verified against the label-based rule.
