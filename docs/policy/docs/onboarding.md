# Onboard a project

Adopt this policy through a scoped onboarding issue and a configuration PR in the target application repository. Start with supervised operation; enable unattended work only after its controls have been built and tested. This guide recommends a setup, but does not install it.

The [shared policy](../README.md) remains authoritative. Application-specific setup, runbooks, issues, PRs, and evidence live with the application.

## 1. Open an onboarding issue

Name the target repository and confirm its visibility. For an existing repository, inspect its instructions, build, CI, deployment triggers, and permissions before changing anything. Preserve existing conventions and controls.

Capture these project-specific facts and decisions in the issue:

- Human owner: `agross`; no alternate approver unless explicitly appointed.
- Application purpose, intended users, stack, and services.
- Existing setup, build, test, local-run, and packaging commands, or the gaps that need to be filled.
- Support entry point and telemetry sources, if available.
- Approved preview host and access restrictions, or the artifact-only fallback.
- Existing operating-cost allowances and any new expenditure requiring approval.
- Current deployment triggers, production credential locations, and which identities can merge or change settings. Record references, never secret values.
- Scope of onboarding, acceptance criteria, exclusions, and the first pilot work item.

Codex can gather repository facts and propose the setup. The human approves its scope in the issue, or locally with the confirmation recorded in GitHub. Creating an issue is not itself approval. Do not start building product features as an unannounced part of onboarding.

If the application repository does not exist, request authorization for its exact name and visibility before creating it. A missing preview host or support integration does not block a code-only pilot; document unavailable capabilities instead of inventing access.

## 2. Make the policy available locally

Choose an explicit commit of `agross/dev` to adopt. A recommended target-repository layout is:

```text
AGENTS.md
docs/
  development.md
  policy/
    upstream.md
  runbooks/
.github/
  ISSUE_TEMPLATE/
  pull_request_template.md
  workflows/
```

Copy the policy from the selected revision into `docs/policy/upstream.md`; keep it as an identifiable, unmodified reference copy rather than independently maintained policy. In `docs/development.md`, record its source repository, exact source commit, copied file, human owner, approved local exceptions, commands, and available integrations. Copy linked policy dependencies if they are needed, and preserve their source identity. Review upstream changes through a PR; do not silently follow the latest branch at runtime.

The root `AGENTS.md` should direct Codex to read the local policy and project setup, identify the relevant issue, verify explicit scope approval before implementation, and use the documented commands. Extend existing instructions rather than overwriting them. Resolve conflicting instructions during onboarding.

Codex discovers repository instructions through `AGENTS.md`; a link in an ordinary README is not an automatic cross-repository policy installation. Confirm that a fresh session can identify the policy, limits, commands, and approval requirement. See [official OpenAI documentation](https://learn.chatgpt.com/docs/agent-configuration/agents-md).

The pinned copy is a runtime reference. `agross/dev` remains the source of shared policy. Do not copy private policy content into a public application repository without explicit permission; decide how that runner will obtain an authorized copy instead.

## 3. Establish intake and approval

Provide issue templates for feature ideas, defects, investigations, and support escalations. Keep early feature ideas lightweight: problem, desired outcome, and examples are enough to begin discussion. Do not require an implementation plan to capture an idea.

A practical initial board uses Inbox, Shaping, Awaiting scope approval, Ready, Implementing, Awaiting human merge, Merged / awaiting release, and Verified. Track blockers separately. These names are suggested configuration, not additional authority grants. Priorities are ranked by the human; the worker selects only eligible approved work.

The approval flow stays in the issue:

1. Discuss the problem, alternatives, and evidence.
2. Post an agreed-scope summary with acceptance criteria and exclusions.
3. Obtain explicit approval from the human owner.
4. Record the exact approved text, its source, and the approval evidence. Mark local confirmations as recorded by Codex on the human's behalf.
5. Recheck approval before starting or resuming implementation. Changed, revoked, deleted, or ambiguous approval blocks further implementation until resolved.

Preserve the scope text in a trusted approval record with change-detection metadata. A hash is useful for detecting changes, but a hash next to editable text is not tamper-proof authorization. A worker-editable status label, an agent's assertion of consensus, or a customer message cannot grant approval. The approval recorder must distinguish direct human input from untrusted issue, code, or log content.

The recorder and automatic gate are not supplied by this repository. During the supervised pilot, explicitly verify the record before dispatching each task. Do not claim automatic enforcement until that component is implemented and tested.

## 4. Enforce authority outside the agent

Use scoped identities and isolated execution environments. Do not give an unattended implementation worker the human owner's personal administrator token.

| Capability | Intended boundary |
| --- | --- |
| Issue discussion and evidence | Read and write only in approved repositories |
| Code changes | Work branches and PRs; no default-branch writes or merges |
| Review | Code, requirements, and sanitized evidence; no production credentials |
| Telemetry | Read-only, service- and tenant-scoped, redacted, with query limits |
| Support | Safe replies through the approved channel; no financial or risky actions |
| Previews | Approved nonproduction host, restricted access, synthetic data, no production credentials or new financial commitments |
| Production | No agent deployment or release credentials |

Require PRs and existing CI checks, prevent force pushes, disable auto-merge, and keep the human as the merge actor. Protect workflow and policy changes as well as application code. Rules, identity separation, or a constrained publishing service must enforce the worker's inability to merge; telling it not to merge is not sufficient.

Inspect the actual repository plan and available controls before promising enforcement. Private-repository branch protection has plan requirements, and some actor restrictions depend on repository ownership. Do not purchase an upgrade without approval. If the required boundary cannot be enforced, keep operation supervised and withhold the broad credential. See [GitHub protected branches](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches).

Use separate identities for agent PR creation and human review. If actions are performed with the human's credentials, they are not technically distinguishable from the human's actions merely because a comment says "Codex."

Test negative cases using a safe test branch or dry-run mechanism: the worker cannot change protection settings, push to the default branch, merge, obtain production credentials, or execute an unauthorized account action. Do not test a boundary by attempting a live production mutation.

## 5. Establish build, tests, and review evidence

Document working commands for setup, build, fast tests, integration tests, local execution, and packaging. Reuse existing tooling. Where relevant, add unit, integration, contract, and a small number of critical end-to-end tests. Use disposable infrastructure and synthetic fixtures.

Run CI on proposed changes and require its results before the human merges. Upload application packages, test reports, and other useful outputs as CI artifacts. Visual changes require screenshots or short videos tied to the tested commit. Artifacts are distinct from publishing a release or a package to a public registry. See [GitHub workflow artifacts](https://docs.github.com/en/actions/concepts/workflows-and-actions/workflow-artifacts).

Keep PR builds unprivileged and separate from deployment. Never check out untrusted PR code in a privileged job that exposes write tokens or secrets. Pin third-party actions to reviewed commit SHAs, grant minimal job permissions, and isolate reusable runners from production credentials. See [GitHub secure use guidance](https://docs.github.com/en/actions/reference/security/secure-use).

Configure independent review against the current PR head and the approved issue scope. The human receives acceptance evidence, test results, reviewer findings, and risks. Limit repair/review iterations to three; do not weaken checks or acceptance criteria to make them pass.

## 6. Add optional integrations one at a time

### Telemetry

Expose bounded log, metric, trace, and deployment-version queries. Redact secrets and unnecessary customer information before model access. Keep service, environment, timestamp, trace ID, and source links so evidence can be checked. No production SSH or database write access is implied.

Keep telemetry credentials outside the environment running untrusted application code. Verify both a permitted read and a denied out-of-scope query. Do not enable an integration until its scope has been verified.

### Support

Identify the exact channel, authorized users or tenants, knowledge sources, and escalation path. Exercise safe replies, financial requests, and risky account requests against test cases before allowing autonomous sending. A financial or risky request must stop for human approval. Do not expose internal evidence in a customer response.

### Previews

Use an already approved host and verify access control before sharing the URL. A private source repository does not itself establish preview privacy. Never pass production credentials or customer data to a preview. No new hosting commitment is authorized by the policy.

Provide a package and visual evidence when a safe hosted preview is unavailable. Automate removal when the work item closes, account for abandoned previews, and verify teardown. Preview automation must not invoke the production release path.

## 7. Run a supervised pilot before unattended work

Choose one small approved feature or defect. Exercise the complete loop: issue discussion, scope approval, optional prototype, implementation, CI, review, evidence, human merge, and human-controlled production release if applicable. Keep "merged" distinct from "verified in production."

Validate failure paths as well as success:

- An unapproved idea does not trigger implementation.
- Approval that changes, is revoked, or cannot be recorded blocks work.
- A prototype stops at 30 minutes, an implementation at two hours, and an investigation at one hour. Restarting a session does not reset the budget.
- At most one prototype, one implementation, and one independent read-only investigation are active within their respective limits; the project must not use multiple workers to evade them.
- Repeated review failures escalate after three rounds.
- Financial and risky support requests stop for human approval.
- Worker permissions prevent merges and production deployment.
- Previews are private to intended reviewers and are removed as required.
- One actionable GitHub mention requests a decision; repeated wakeups do not repeatedly mention unchanged state. Suspected incidents are surfaced promptly.

Only then approve the scope of unattended operation. The eventual dispatcher needs durable job state, deduplication, concurrency and budget enforcement, cancellation, and failure reporting. These are implementation requirements, not features installed by adding `AGENTS.md`.

## Completion checklist

- [ ] Onboarding scope approved in the application issue.
- [ ] Policy revision recorded and available to a fresh Codex session.
- [ ] Existing repository guidance preserved; conflicts resolved.
- [ ] Project commands work locally and in CI.
- [ ] Issue intake includes new ideas, defects, investigations, and support.
- [ ] Approval recording and change detection verified, or operation explicitly kept supervised.
- [ ] Worker identity and permissions tested; human retains merges and production release.
- [ ] Tests, packages, and visual evidence available as applicable.
- [ ] Optional integrations verified or explicitly marked unavailable.
- [ ] Preview cleanup and fallback tested, if previews are enabled.
- [ ] Budget, review-loop, and escalation failure cases exercised.
- [ ] Human reviews and merges the onboarding PR.
- [ ] Any later unattended activation receives separate approval.
