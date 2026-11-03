# Project onboarding

This guide describes the supervised setup for this repository. The [project policy](../README.md) is maintained here; these documents do not install automation or prove that access controls are enforced.

## Work and authorization

Use GitHub issues for features, defects, investigations, and support. Describe intended behavior, acceptance criteria, and exclusions in each issue. The `hermes:developer` label authorizes implementation; separate explicit scope approval is not required. Recheck the label when starting or resuming work. Stop implementation if the label is removed or the issue scope materially changes, and escalate scope changes or unclear requirements to the owner. A label does not authorize merging, production changes, financial commitments, or account changes.

Work on branches and open pull requests. Run CI and independent review against the current head; give the human acceptance evidence, checks, findings, and remaining risks. Limit repair and review to three rounds. The human merges and controls production releases.

## Access and integrations

Use separate scoped developer and reviewer identities. Require pull requests and CI, prevent force pushes, disable auto-merge, and enforce the human's merge and release authority outside agent instructions. Protect policy and workflow changes. Never give an unattended worker the human owner’s personal administrator token. Actions performed with the human’s credentials are not technically distinguishable from the human’s actions. Verify actual repository settings and test negative cases on a safe test branch or through a dry run; never test a boundary with a live production mutation; if identity or permission boundaries cannot be enforced, keep operation supervised and withhold broad credentials. Do not purchase upgrades without approval.

Verify webhook delivery, signature validation, event routing, and access scope before enabling automation. Reconcile Hermes with the label-based rule; older Hermes skills may still require explicit approval. Do not treat the label as evidence that these controls exist.

Never check out untrusted pull-request code in a privileged job exposing write tokens or secrets. Keep production credentials out of untrusted CI and preview execution. Use synthetic data and disposable infrastructure. Grant telemetry read-only, bounded, and redacted access; enable support and previews only within their separately approved access and hosting boundaries. Clean up previews when work closes. Never use previews to deploy to production.

## Supervised pilot

Run one labeled work item through implementation, CI, independent review, human merge, and, where applicable, human-controlled release. Verify failure and revocation cases, budget and concurrency limits, review escalation, permissions, and preview cleanup where applicable. Limit prototypes to 30 minutes, implementations to two hours per work item, and independent investigations to one hour; stopping does not reset the budget. After three unsuccessful review rounds, escalate.

Do not enable unattended operation until its scope is separately authorized and durable work selection, deduplication, concurrency, budgets, cancellation, failure reporting, and identity boundaries are implemented and tested.
