# Development Workflow

This glossary defines the work and authority concepts used in the development workflow.

## Language

**Work item**:
A tracked feature idea, defect, support request, or investigation. Its existence does not by itself authorize implementation.
_Avoid_: Complaint as an umbrella term for all work.

**Feature idea**:
A proposed new capability or improvement that may be evaluated before a commitment is made to implement it.
_Avoid_: Approved feature when the proposal has not been approved.

**Human owner**:
The person responsible for feature scope and shape and the sole authority for approvals in this workflow.
_Avoid_: Approver team, backup approver.

**Approval record**:
A durable record of an explicit decision made by the human owner. Approval of implementation is bound to a fixed version of the applicable brief; a decision originally confirmed during a local conversation is explicitly identified as recorded on the human's behalf.
_Avoid_: Agent approval when referring to human authorization.

**Feature brief**:
The description of a feature's problem, intended users, behavior examples, non-goals, relevant user experience or API changes, and major tradeoffs that the human approves as its scope and shape.
_Avoid_: Implementation plan when referring only to the approved product commitments.

**Prototype**:
A small, isolated code experiment intended to help the human evaluate an idea before approving implementation. It does not imply a commitment to merge, release to production, or retain the code.
_Avoid_: Approved implementation, release candidate.

**Hosted preview**:
A non-production instance intended for the human to try a proposed change. Its existence does not imply that the feature is approved for implementation or release.
_Avoid_: Production release.

**Agreed-scope summary**:
The concise statement of the proposed scope, acceptance criteria, and exclusions resulting from a work item's discussion. It becomes the authorized implementation scope only when the human explicitly approves its exact content.
_Avoid_: Consensus when referring to approval inferred without an explicit human decision.

**Work priority**:
The order assigned by the human owner for selecting among eligible work items. An agent's suggestion to change that order is not itself a new priority.
_Avoid_: Agent preference when referring to the authoritative order.

**Defect brief**:
The description of observed and expected behavior, reproduction, the proposed change boundary, and a regression test that the human approves before a bug fix is implemented.
_Avoid_: Feature brief when the work only restores intended behavior.

**Support response**:
An answer sent to a user about a question or problem. Permission to send a response does not by itself define permission to change the user's account or production data.
_Avoid_: Account action when referring only to communication.

**Financial commitment**:
An action or promise that changes a financial obligation or benefit, including a refund, credit, discount, purchase, subscription change, or promise of compensation. Explaining published prices or an existing invoice is not itself a financial commitment.
_Avoid_: Pricing explanation when referring to a change or promise.

**Suspected incident**:
An observed condition that may indicate an outage or data exposure, with impact and cause not necessarily confirmed. Reporting a suspected incident does not authorize production changes.
_Avoid_: Confirmed incident before the evidence supports that conclusion.
