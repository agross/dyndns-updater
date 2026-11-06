# Codex-led development workflow

This repository maintains its own development policy, originally derived from `agross/dev`. It is a policy and onboarding reference, not an installed automation system. No workflow automation, preview hosting, or production integration is configured by these documents.

For setup and the supervised pilot, see [Project onboarding](docs/onboarding.md).

## Confirmed decisions

- GitHub is the authoritative home for work items and documentation. Intake includes new feature ideas, defects, support requests, and investigations.
- The human owner shapes feature priorities. An issue labeled `ready-for-agent` authorizes implementation of its described scope; no separate explicit scope approval is required. The label does not authorize a merge, production release, financial commitment, or account change.
- Record intended behavior, acceptance criteria, and exclusions in the issue. Escalate material scope changes and unclear requirements to the owner. Codex chooses implementation details within the issue scope.
- Codex may create small, isolated, clearly marked prototypes to help evaluate an idea. Prototypes must not change production or be merged, and there is no commitment to retain their code. Limit prototype work to one at a time and 30 minutes, with no production data or new paid infrastructure. At the limit, present findings and questions to the human.
- Prototype review should use hosted previews where possible and downloadable CI artifacts such as application packages where possible. Visual changes must include screenshots or short videos.
- Codex may automatically publish nonproduction previews using approved hosting with restricted access, synthetic data, no production credentials, and no new financial commitments. Previews are removed when their work item closes. If these conditions cannot be met, Codex provides CI artifacts and visual evidence instead. This is an explicit exception to the earlier blanket deployment prohibition, not permission to deploy to production. No preview has been deployed or configured.
- Initially, at most one implementation and one independent read-only investigation may be active. Implementation is capped at two hours per labeled work item, and investigation at one hour, before stopping for the human's decision. There are no automatic budget resets and no new paid infrastructure without approval.
- The human ranks work priorities. Codex selects the highest-ranked eligible work item with the `ready-for-agent` label. Codex may propose reprioritization but may not replace the human's priorities or interrupt active implementation without approval.
- The human owner performs merges initially. Codex supplies acceptance evidence, unresolved risks, and independent reviewer findings. Deployment side effects of merging must remain under human control.
- A defect issue records observed behavior, expected behavior, reproduction, the proposed change boundary, and the regression test. The `ready-for-agent` label authorizes implementation. Changes to intended behavior are treated as feature proposals.
- Codex may perform up to three repair and independent review rounds within the issue scope. Unresolved disagreements, failing checks after the allotted rounds, and material scope changes are escalated. The handoff before a human merge includes acceptance evidence, test results, review findings against the current commit, and remaining risks. Checks must not be weakened merely to obtain a passing result.
- Codex may send support responses autonomously rather than preparing drafts for routine human approval. Codex may explain published prices and existing invoices. Refunds, credits, discounts, purchases, subscription changes, and promises of compensation require the human's approval. This supersedes the earlier draft-only recommendation.
- Risky support behavior is not autonomous. Account changes are permitted only through explicitly approved procedures with identity verification. Permission grants, destructive changes, sensitive disclosures, and other risky actions require the human's approval. Access controls must never be bypassed. Permission to reply is not blanket authorization to change accounts or production data.
- Actionable requests and blockers are recorded in GitHub, with a mention when the human's decision is needed. A daily digest and other notification schedules have not been approved.
- Suspected outages or data exposure trigger an immediate GitHub mention with impact, evidence, uncertainty, and a suggested action. Codex may continue safe, read-only investigation. It must not deploy, release, change production, or automatically interrupt active implementation.
- Codex must not deploy to production or automate production releases. This overrides the earlier proposal for an automated release executor and preauthorized automated incident mitigation. Nonproduction previews are the limited exception described above.
- No backup approver has been designated.

## Changes

This fork is maintained in this repository. Policy changes use pull requests and human-controlled merges. Complete a supervised pilot before enabling unattended work; the [onboarding guide](docs/onboarding.md) describes the remaining checks.

## Implementation work still required

- Reconcile and verify Hermes automation with the label-based authorization rule.
- Configure scoped identities and permissions that enforce the agreed authority boundaries.
- Connect the selected application repositories, telemetry, support channel, CI, and approved preview hosting.
- Implement bounded work selection, time tracking, review retries, notification deduplication, and preview cleanup.
- Choose an application and work item for a pilot, then test normal and failure paths before enabling unattended operation.

These are future implementation tasks, not configured capabilities or permission to enable services.

## Documents

- [Project onboarding](docs/onboarding.md)
- [Glossary](CONTEXT.md)
- [GitHub as the system of record](docs/adr/0001-github-system-of-record.md)

No automation should be activated solely because these documents exist. Activation requires its own authorization.
