# Provider contracts

Verified against authoritative documentation during implementation of #2:

- [deSEC IP Update API](https://desec.readthedocs.io/en/latest/dyndns/update-api.html)
  (including its RST source): HTTPS GET to `https://update.dedyn.io/`,
  `Authorization: Token TOKEN`, `hostname=TARGET`, `myipv4=ADDRESS`, and
  **`myipv6=preserve` on every request**. The documented success is HTTP 200
  with body `good`; other bodies are not accepted as confirmed success.
- [HE Tunnelbroker API documentation](https://forums.he.net/index.php?topic=3153.0)
  and [HE's DynDNS2 response clarification](https://forums.he.net/index.php?topic=2012.0):
  HTTPS GET to `https://ipv4.tunnelbroker.net/nic/update`, HTTP Basic auth
  with username and tunnel-specific Update Key, `hostname=TUNNEL_ID`, and
  `myip=ADDRESS`. Accept HTTP 200 with `good ADDRESS` or `nochg ADDRESS`
  only when ADDRESS matches the requested IPv4 address. Other or malformed
  responses fail and are retried on later invocations, except the explicit
  owner-approved `abuse` handling below.

Credentials never enter query parameters. Redirects are not followed; HTTP
status, generic transport errors, and generic rejection errors are reported
without reflecting provider bodies or credential-bearing request headers.
Requests have bounded timeouts and response bodies.

HE asks clients to update only when the address changes. Issue #2 explicitly
requires configurable periodic refresh; the example uses the requested 25 days,
not the one-minute polling interval. Confirm the gateway's intended refresh
policy with HE before production migration. No live provider requests have been
made; local fixtures establish client behavior, not a live-provider verification.

An optional profile `endpoint` overrides the documented URL for testing or a
trusted proxy. HTTPS is mandatory except for HTTP to a literal loopback IP.
Do not point it at an untrusted service: it receives that profile's credentials.
Public address discovery and custom DNS resolvers are not implemented (#3/#4).

## Response and retry matrix (#16)

Sources re-read during #16:

1. [deSEC IP Update API](https://desec.readthedocs.io/en/latest/dyndns/update-api.html):
   HTTP 200 `good`, authentication 401, undetermined hostname 400, domain not found 404.
2. [deSEC rate limits](https://desec.readthedocs.io/en/latest/rate-limits.html):
   HTTP 429, `Retry-After` seconds; per-account limits, per-domain dynDNS 2/2min,
   and broader user limits. The header does not expose a safe account identifier.
3. [HE administrator API documentation](https://forums.he.net/index.php?topic=3153.0):
   Dyn API compatibility and update-only-on-address-change guidance, not an
   enumerated error or retry contract.
4. [Owner's verified HE observation and deSEC fallback decision](https://github.com/agross/dyndns-updater/issues/16#issuecomment-6667593783),
   and [explicit HE address-state override](https://github.com/agross/dyndns-updater/issues/16#issuecomment-6669641469).
   These intentionally revise #16's original strict-confirmation requirement for
   HE `abuse`; they are project policy, not a claim of documented HE success.
5. [HTTP Retry-After syntax](https://www.rfc-editor.org/rfc/rfc9110#section-10.2.3)
   permits delta-seconds or an HTTP date. deSEC documents seconds; accepting an
   HTTP date is defensive HTTP compatibility, not a claim about observed headers.

| Provider / status / body | Controlled result | Durable retry disposition | Source |
| --- | --- | --- | --- |
| deSEC 200, exactly `good` | confirmed requested IPv4 | clear restriction, record address/time and recovery | 1 |
| deSEC 401, any body | authentication failed; correct token/configuration | intervention | 1; client intervention policy |
| deSEC 400, any body | hostname rejected; correct target/token/configuration | intervention | 1; client intervention policy |
| deSEC 404, any body | domain not found; correct target/token/configuration | intervention | 1; client intervention policy |
| deSEC 429, any body | rate limited; UTC retry deadline | wait until deadline; share latest active wait across pending deSEC updates in the same state file | 2, 4, 5 |
| HE 200, `good ADDRESS` or `nochg ADDRESS`, matching requested IPv4 | confirmed requested IPv4 | clear restriction, record address/time and recovery | existing #2 contract / 3 |
| HE 200, exactly `abuse` | failure; address assumed accepted | advance address/time but remain failing; suppress same address until refresh, credential or identity change | 4 |
| Either provider, other non-200 status (including redirect) | HTTP failure with numeric status only | retry next invocation | no verified provider-specific retry guidance |
| Either provider, malformed/oversized/unexpected 200 body, including contradictory success/error text | invalid response or generic rejection | retry next invocation; never advance address | strict client validation |
| Either provider, DNS/connect/TLS/timeout/read failure | generic transport/invalid-response diagnostic | retry next invocation; never advance address | client policy, not provider-specific timing |

Success/error tokens allow surrounding whitespace, but not extra fields or
contradictory text. Responses are bounded to 4096 bytes when body classification
is needed. deSEC's error classification uses HTTP status alone; error bodies are
not consumed or reflected. All unknown HE error tokens (including `badauth`,
`nohost`, `dnserr`, `911`) remain generic errors: Dyn's 30-minute rule is **not**
assigned to HE without evidence. A non-200 `abuse` is not assumed accepted.

### Exact recovery rules

Intervention holds for deSEC 400/401/404 survive address changes, refresh and
restarts. Token or identity (provider/target/interface/endpoint) corrections
release them. Removing that profile's state also releases them; provider-side-only
corrections cannot be detected without another request and need an explicit reset.
HE `abuse` holds only for the stored address and current credentials/identity,
until `updated + refresh` using the current configuration; change any of these to
retry. Both initial and suppressed abuse are failures/nonzero exits; recovery is
notified only after a strict successful response. Repeated unchanged suppression
alone does not create new mail. Credential-only rotation never invalidates an
already-successful confirmation.

A single valid Retry-After delta or date is preferred over the two-minute fallback,
even if shorter. Missing, duplicate, negative, fractional, nonnumeric, or out-of-range
values use two minutes. Delta-seconds preserve receipt-time subsecond precision;
dates are absolute UTC; past dates/zero allow retry on the next run. A 429 never
confirms an update, even when its body says `good`. No raw body/header content is
persisted, logged, or mailed. All deadlines use wall-clock UTC: an operator clock
jump can change the effective wait.

Because token equality does not prove account identity (one account can have many
tokens), this client conservatively shares 429 deadlines across **all deSEC
profiles in one state file**, including different targets, tokens, and endpoints.
This may pause unrelated accounts; use independent state files only for genuinely
independent accounts, not to bypass an account limit. Already-successful no-op
profiles and HE updates continue. Copies of a shared deadline persist per blocked
profile under the lock; removing a source profile does not remove copies already
saved elsewhere. No rate coordination exists across different state files or hosts.
Deleting state/all relevant profiles explicitly clears their restrictions, just as
it forces fresh updates. Existing intervention blocks are retained rather than
replaced by a shared wait; after correction, any surviving shared wait still applies.

Retry state adds optional validated fields to version-1 profile records. Unknown
codes, inconsistent fields, missing fingerprints/deadlines, duplicate JSON keys,
or other invalid state stop the run without overwriting the file or contacting
providers/mail. Previously queued summaries remain valid and retry in order,
including legacy statuses. Checkpoints and atomic replacement retain the existing
lock, private modes, interruption recovery and at-least-once mail semantics.
Fingerprints use a per-restriction random 256-bit salt and SHA-256 over structured
identity and only the credentials used by that provider. They are not reversible,
but private state and strong credentials remain necessary to resist offline guesses.
No live DNS update or real mail was used to verify these contracts.
