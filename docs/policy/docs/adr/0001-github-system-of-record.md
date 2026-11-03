---
status: accepted
---

# Use GitHub as the workflow system of record

Use GitHub for work items, documentation, and human approval records, with the dedicated private repository `agross/dev` for shared workflow documentation. This replaces the proposed split between GitHub and Nextcloud so that work and decisions have one authoritative home; feature ideas are part of intake alongside defects and support requests. Discussion and approval happen in issues without requiring separate committed brief files; explicit local approvals must also be recorded in GitHub against the exact agreed-scope summary and marked as recorded on the human's behalf.

## Consequences

The workflow must distinguish a human decision from an agent's report of that decision. Because issue comments are editable, tooling must preserve the exact approved scope and detect changes; an issue comment URL alone does not fix its content. Application issues, pull requests, and application-specific documentation remain with their application repositories, linked to the shared policy. The human owner authorized the initial documentation commit and push; this is not standing authorization for future direct pushes or agent merges.
