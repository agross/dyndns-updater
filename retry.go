package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Only controlled codes are persisted. No provider text or credential is stored.
type retryState struct {
	Kind  string    `json:"kind"`
	Until time.Time `json:"until,omitempty"`
	Salt  string    `json:"salt,omitempty"`
	Key   string    `json:"key,omitempty"`
}

type providerFailure struct {
	kind  string
	until time.Time
}

func (e providerFailure) Error() string { return retryMessage(e.kind, e.until) }

func retryMessage(kind string, until time.Time) string {
	switch kind {
	case "desec-auth":
		return "deSEC authentication failed; correct token/configuration or remove state to retry"
	case "desec-hostname":
		return "deSEC hostname rejected (HTTP 400); correct target/token/configuration or remove state to retry"
	case "desec-target":
		return "deSEC domain not found (HTTP 404); correct target/token/configuration or remove state to retry"
	case "desec-rate":
		return "deSEC rate limited; retry at " + until.UTC().Format(time.RFC3339)
	case "he-abuse":
		return "HE abuse response; update unconfirmed; retry at " + until.UTC().Format(time.RFC3339)
	}
	return "invalid provider response"
}

func retryKey(p Profile, salt string) string {
	credentials := []string{p.Token}
	if p.Provider == "tunnelbroker" {
		credentials = []string{p.Username, p.Password}
	}
	b, _ := json.Marshal(append([]string{salt, identity(p)}, credentials...))
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func newRetry(p Profile, failure providerFailure) (*retryState, error) {
	r := &retryState{Kind: failure.kind, Until: failure.until}
	if r.Kind == "desec-rate" {
		return r, nil
	}
	var salt [32]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return nil, fmt.Errorf("cannot initialize retry state")
	}
	r.Salt = hex.EncodeToString(salt[:])
	r.Key = retryKey(p, r.Salt)
	return r, nil
}
func hexDigest(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}
func validRetry(r *retryState, p profileState) bool {
	if r == nil {
		return true
	}
	if !p.Failing {
		return false
	}
	if r.Kind == "desec-rate" {
		return !r.Until.IsZero() && r.Until.Year() >= 1 && r.Until.Year() <= 9999 && r.Salt == "" && r.Key == ""
	}
	switch r.Kind {
	case "desec-auth", "desec-hostname", "desec-target":
	case "he-abuse":
		if !hexDigest(r.Salt) || !hexDigest(r.Key) {
			return false
		}
		if r.Until.IsZero() {
			// Legacy records stored an assumed address and response timestamp.
			return p.Address != "" && !p.Updated.IsZero() && p.Updated.Add(15*time.Minute).Year() <= 9999
		}
		return r.Until.Year() >= 1 && r.Until.Year() <= 9999
	default:
		return false
	}
	return r.Until.IsZero() && hexDigest(r.Salt) && hexDigest(r.Key)
}

// A rate limit cannot be bypassed by rotating credentials or changing a target.
// Account ownership cannot be derived from tokens/targets, so conservatively
// share a 429 across all deSEC profiles in this state file (even distinct tokens
// and endpoints). Copy the latest restriction to other affected profiles before their
// request; it then survives removal/reconfiguration of the originating profile.
func blockedRetry(s diskState, p Profile, old profileState, now time.Time) *retryState {
	r := old.Retry
	if r != nil && r.Kind == "he-abuse" && now.Before(r.Until) {
		return r
	}
	if r != nil && r.Kind != "desec-rate" && r.Kind != "he-abuse" && retryKey(p, r.Salt) == r.Key {
		return r
	}
	if p.Provider == "desec" {
		var latest *retryState
		for _, state := range s.Profiles {
			r := state.Retry
			if r != nil && r.Kind == "desec-rate" && now.Before(r.Until) && (latest == nil || r.Until.After(latest.Until)) {
				latest = r
			}
		}
		if latest != nil {
			return latest
		}
	}
	return nil
}

func retryDeadline(headers http.Header, now time.Time) time.Time {
	fallback := now.Add(2 * time.Minute)
	values := headers.Values("Retry-After")
	if len(values) != 1 {
		return fallback
	}
	value := strings.TrimSpace(values[0])
	// HTTP delta-seconds accepts digits only, not signs, fractions, or overflow.
	digits := value != ""
	for _, r := range value {
		if r < '0' || r > '9' {
			digits = false
		}
	}
	if digits {
		seconds, err := strconv.ParseUint(value, 10, 64)
		if err == nil && seconds <= uint64((time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC).Unix()-now.Unix())) {
			// Construct by Unix seconds rather than time.Duration to avoid overflow.
			return time.Unix(now.Unix()+int64(seconds), int64(now.Nanosecond())).UTC()
		}
	}
	if date, err := http.ParseTime(value); err == nil && date.Year() <= 9999 {
		if date.Before(now) {
			return now.UTC()
		}
		return date.UTC()
	}
	return fallback
}
