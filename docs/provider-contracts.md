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
  responses fail and are retried on later invocations.

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
