# Codex-led development workflow

This repository records the development policy agreed with its human owner. It is a policy and onboarding reference, not an installed automation system. No workflow automation, preview hosting, or production integration is configured by these documents.

To adopt this policy in an application repository, follow [Onboard a project](docs/onboarding.md).

## Confirmed decisions

- GitHub is the authoritative home for work items and documentation. Intake includes new feature ideas, defects, support requests, and investigations.
- The dedicated private repository `agross/dev` will hold the shared workflow documentation. Application issues, pull requests, and application-specific documentation remain with their application repositories.
- The human owner is the sole approver and is responsible for the scope and shape of each feature.
- Human approval is required before implementation is authorized. Routine implementation choices within the approved scope do not require repeated approval; material changes require renewed approval.
- Approval covers the problem, intended users, concrete behavior examples, non-goals, relevant user experience or API changes, and major tradeoffs. Codex chooses internal implementation details unless they alter these commitments.
- GitHub holds the approval record. Discussion and consensus happen in issues; separate committed brief files are not required. Codex summarizes the agreed scope, acceptance criteria, and exclusions in the issue for explicit human approval. Approval must bind to that exact text. An explicit local confirmation in Codex must also be recorded in GitHub, marked as recorded by Codex on the human's behalf and including the relevant confirmation text. Changing the approved scope requires renewed approval. Tooling must preserve the approved text and detect changes without requiring the human to manage hashes or commit IDs. The technical implementation of this recording and checking is still to be built.
- Before scope approval, Codex may create small, isolated, clearly marked prototypes to help the human evaluate an idea. Prototypes must not change production or be merged, and there is no commitment to retain their code. Full implementation still requires approval. Prototype work is limited to one prototype at a time and up to 30 minutes of agent work, with no production data or new paid infrastructure. Existing approved preview hosting may be used under the preview policy below. At the limit, Codex stops and presents findings, a runnable demonstration where practical, and questions for the human.
- Prototype review should use hosted previews where possible and downloadable CI artifacts such as application packages where possible. Visual changes must include screenshots or short videos.
- Codex may automatically publish nonproduction previews using approved hosting with restricted access, synthetic data, no production credentials, and no new financial commitments. Previews are removed when their work item closes. If these conditions cannot be met, Codex provides CI artifacts and visual evidence instead. This is an explicit exception to the earlier blanket deployment prohibition, not permission to deploy to production. No preview has been deployed or configured.
- Initially, at most one implementation and one independent read-only investigation may be active. Implementation is capped at two hours per approved work item, and investigation at one hour, before stopping for the human's decision. There are no automatic budget resets and no new paid infrastructure without approval.
- The human ranks work priorities. Codex selects the highest-ranked eligible work item with an approved brief. Codex may propose reprioritization but may not replace the human's priorities or interrupt active implementation without approval.
- The human owner performs merges initially. Codex supplies acceptance evidence, unresolved risks, and independent reviewer findings. Deployment side effects of merging must remain under human control.
- Bug fixes require human scope approval initially. A defect brief records observed behavior, expected behavior, reproduction, the proposed change boundary, and the regression test. Investigation may proceed before approval. Changes to intended behavior are treated as feature proposals.
- Codex may perform up to three repair and independent review rounds within approved scope. Unresolved disagreements, failing checks after the allotted rounds, and material scope changes are escalated. The handoff before a human merge includes acceptance evidence, test results, review findings against the current commit, and remaining risks. Checks must not be weakened merely to obtain a passing result.
- Codex may send support responses autonomously rather than preparing drafts for routine human approval. Codex may explain published prices and existing invoices. Refunds, credits, discounts, purchases, subscription changes, and promises of compensation require the human's approval. This supersedes the earlier draft-only recommendation.
- Risky support behavior is not autonomous. Account changes are permitted only through explicitly approved procedures with identity verification. Permission grants, destructive changes, sensitive disclosures, and other risky actions require the human's approval. Access controls must never be bypassed. Permission to reply is not blanket authorization to change accounts or production data.
- Actionable requests and blockers are recorded in GitHub, with a mention when the human's decision is needed. A daily digest and other notification schedules have not been approved.
- Suspected outages or data exposure trigger an immediate GitHub mention with impact, evidence, uncertainty, and a suggested action. Codex may continue safe, read-only investigation. It must not deploy, release, change production, or automatically interrupt approved implementation.
- Codex must not deploy to production or automate production releases. This overrides the earlier proposal for an automated release executor and preauthorized automated incident mitigation. Nonproduction previews are the limited exception described above.
- No backup approver has been designated.

## Adoption and changes

The human owner approved initial publication with the explicit local instruction, "Commit and push." That authorizes this initial documentation publication directly; it does not grant standing permission for future direct pushes, agent merges, or production releases. The instruction is [recorded by Codex on the human's behalf in issue #1](https://github.com/agross/dev/issues/1). Future policy changes and project onboarding require an approved scope and human-controlled merge.

Project adoption begins with an issue in the target application repository. Pin the adopted policy revision, configure project-specific commands and access boundaries, and complete a supervised pilot before enabling unattended work. The [onboarding guide](docs/onboarding.md) describes the recommended setup; it is not a claim that those controls already exist.

## Implementation work still required

- Build and verify issue approval capture, approved-text preservation, and change detection, including local confirmations and revoked approvals.
- Configure scoped identities and permissions that enforce the agreed authority boundaries.
- Connect the selected application repositories, telemetry, support channel, CI, and approved preview hosting.
- Implement bounded work selection, time tracking, review retries, notification deduplication, and preview cleanup.
- Choose an application and work item for a pilot, then test normal and failure paths before enabling unattended operation.

These are future implementation tasks, not configured capabilities or permission to enable services.

## Documents

- [Onboard a project](docs/onboarding.md)
- [Glossary](CONTEXT.md)
- [GitHub as the system of record](docs/adr/0001-github-system-of-record.md)

No automation should be activated solely because these documents exist. Implementation and activation require their own approved scope.
