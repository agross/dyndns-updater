# Development workflow

The human owner is `agross`. Shared policy is copied from `agross/dev` into [policy/upstream.md](policy/upstream.md), with its linked references alongside it. Project-specific decisions and commands are recorded here. The copy is local policy, not evidence that automation or authorization controls are installed.

Work starts in a GitHub issue. Discuss a concrete scope and obtain the owner's explicit approval before implementation. Record local approvals in the issue on the owner's behalf. Use a branch and pull request, run CI and independent review, and provide evidence and outstanding risks. The human merges; releases require a human-triggered workflow. Operate under supervision until approval recording and separate identities and permissions have been implemented and verified. Do not claim labels or agent assertions constitute approval.

Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and check formatting with `gofmt -l .`. The CLI currently provides help only. Go must be installed separately. No production deployment is authorized by onboarding.

The shared onboarding guide recommends recording a policy revision; the owner chose not to record one locally. Future adjustments to the project and its policy copy require review. No separate preview host, support integration, or automated approval gate is configured.
