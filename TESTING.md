# Testing

The default suite is deterministic, offline, and safe to run without credentials. Test observable behavior through the CLI and provider HTTP requests; use temporary config and state directories and local fake HTTP/DNS services and a fake sendmail executable. Exercise complete user journeys rather than asserting internal implementation details. Add one failing test per behavior slice before implementing it.

Run `go test ./...` locally. CI also runs `go vet ./...`, checks formatting, and runs tests with the race detector. The first test builds the executable and checks its help output; it is only a scaffold, not evidence that DNS updates work.

## Acceptance cases to implement

- Initial update sends the detected address; unchanged address skips an update; changed address updates; elapsed refresh interval forces an update.
- Failed, malformed, or rejected provider responses never mark an update successful; later runs retry. A successful profile stays independent of a failing profile.
- deSEC receives its documented IPv6 preservation option; Tunnelbroker gets the expected DynDNS2 request. Verify actual provider contracts against documentation before locking fixtures.
- Invalid YAML, invalid addresses, missing interface, unavailable detection services, corrupt state, and unwritable state produce clear failures without leaking credentials.
- Concurrent invocations cannot corrupt state or lose a successful profile update. Test interruption and restart around state persistence.
- DNS fallback works when the primary resolver fails; resolver and provider timeouts remain bounded. Test IPv4 and IPv6 address selection separately when supported.
- Notification success and failure are observable; mail errors cannot turn failed DNS updates into successes or silently erase them.
- Timer and interface-hook invocations use the same one-shot update rules. Exercise scheduling with a controlled clock or explicit inputs rather than waiting 25 days.

Before production use, separately validate real provider behavior and a gateway migration with explicit authorization. Live-provider tests are not part of the default suite.
