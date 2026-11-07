package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCLIHEAbuseDeadlineJourney(t *testing.T) {
	j := newJourney(t)
	j.now = time.Date(2026, 1, 1, 3, 0, 0, 123000000, time.FixedZone("east", 3*3600))
	calls := map[string]int{}
	abuse := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		if r.URL.Path == "/dns" {
			fmt.Fprint(w, "good")
			return
		}
		// Advance the clock while receiving the response, proving the deadline
		// is measured from the response rather than invocation start.
		j.now = j.now.Add(7 * time.Second)
		if abuse {
			fmt.Fprint(w, "abuse")
			return
		}
		fmt.Fprint(w, "good "+r.URL.Query().Get("myip"))
	}))
	defer srv.Close()
	j.write(profile("he", "tunnelbroker", srv.URL+"/he") + profile("dns", "desec", srv.URL+"/dns"))
	j.run(0)
	confirmed := j.state().Profiles["he"]
	abuse = true
	j.ip = "192.0.2.20"
	j.mailError = true
	j.run(1)
	first := j.state().Profiles["he"]
	until := j.now.UTC().Add(15 * time.Minute)
	if first.Address != confirmed.Address || !first.Updated.Equal(confirmed.Updated) || !first.Failing || !first.Retry.Until.Equal(until) || first.Retry.Until.Location() != time.UTC {
		t.Fatalf("invalid unconfirmed state: %+v", first)
	}
	pending := j.state().Pending[0]
	j.now = until.Add(-time.Nanosecond)
	j.mailError = false
	out := j.run(1)
	if calls["/he"] != 2 || calls["/dns"] != 2 || len(j.mails) != 2 || !strings.HasSuffix(j.mails[1], pending) || !j.state().Profiles["he"].Retry.Until.Equal(until) || !strings.Contains(out, "retry at "+until.Format(time.RFC3339)) {
		t.Fatalf("suppression/durable mail/independence failed: %s", out)
	}
	j.now = until
	j.run(1)
	next := j.state().Profiles["he"]
	if calls["/he"] != 3 || !next.Retry.Until.Equal(j.now.UTC().Add(15*time.Minute)) || !next.Retry.Until.After(until) || len(j.mails) != 2 || next.Address != confirmed.Address || !next.Updated.Equal(confirmed.Updated) {
		t.Fatal("repeated abuse did not renew wait without confirmation or mail")
	}
	j.now = next.Retry.Until.Add(-time.Nanosecond)
	j.run(1)
	if calls["/he"] != 3 {
		t.Fatal("early repeated retry")
	}
	abuse = false
	j.now = next.Retry.Until
	j.run(0)
	recovered := j.state().Profiles["he"]
	if calls["/he"] != 4 || recovered.Address != j.ip || recovered.Failing || recovered.Retry != nil || len(j.mails) != 3 {
		t.Fatal("confirmed recovery missing")
	}
	j.run(0)
	if calls["/he"] != 4 || len(j.mails) != 3 {
		t.Fatal("recovered profile not quiet")
	}
}

func TestBuiltCLIHEAbuseSurvivesRestart(t *testing.T) {
	if _, err := interfaceIPv4("lo"); err != nil {
		t.Skip("Linux loopback lo unavailable")
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "abuse") }))
	defer srv.Close()
	bin := buildCLI(t)
	cfg, state, mail := cliConfig(t, srv.URL)
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(strings.ReplaceAll(string(data), "provider: desec", "provider: tunnelbroker")), 0600); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC()
	for range 2 {
		out, err := exec.Command(bin, "--config", cfg).CombinedOutput()
		if err == nil || !strings.Contains(string(out), "update unconfirmed; retry at") {
			t.Fatalf("expected restricted failure: %v %s", err, out)
		}
	}
	after := time.Now().UTC()
	data, err = os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	var s diskState
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	p := s.Profiles["dns"]
	content, err := os.ReadFile(mail)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || strings.Count(string(content), "Subject:") != 1 || p.Address != "" || !p.Updated.IsZero() || !p.Failing || p.Retry.Until.Before(before.Add(15*time.Minute)) || p.Retry.Until.After(after.Add(15*time.Minute)) {
		t.Fatal("built CLI restart lost deadline, failure, or notification semantics")
	}
}

func TestCLIHEAbuseMalformedStatePreserved(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "abuse") }))
	defer srv.Close()
	j.write(profile("he", "tunnelbroker", srv.URL))
	j.run(1)
	path := filepath.Join(j.dir, "state.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"missing-deadline", "bad-fingerprint", "not-failing", "malformed-deadline", "invalid-pending"} {
		var s diskState
		if err := json.Unmarshal(original, &s); err != nil {
			t.Fatal(err)
		}
		p := s.Profiles["he"]
		switch change {
		case "missing-deadline":
			p.Retry.Until = time.Time{}
		case "bad-fingerprint":
			p.Retry.Key = "bad"
		case "not-failing":
			p.Failing = false
		case "invalid-pending":
			s.Pending = []string{"he: HE abuse response; update unconfirmed; retry at not-a-time\n"}
		}
		s.Profiles["he"] = p
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if change == "malformed-deadline" {
			data = []byte(strings.ReplaceAll(string(data), p.Retry.Until.Format(time.RFC3339Nano), "not-a-time"))
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		j.run(1)
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(data) || calls != 1 || len(j.mailAttempts) != 1 {
			t.Fatalf("malformed state not preserved: %s", change)
		}
	}
}

func TestCLIHEAbuseLegacyMigration(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprint(expired), func(t *testing.T) {
			j := newJourney(t)
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				fmt.Fprint(w, "nochg "+r.URL.Query().Get("myip"))
			}))
			defer srv.Close()
			j.write(profile("he", "tunnelbroker", srv.URL))
			j.run(0)
			path := filepath.Join(j.dir, "state.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var s diskState
			if err := json.Unmarshal(data, &s); err != nil {
				t.Fatal(err)
			}
			p := s.Profiles["he"]
			p.Failing = true
			// Historical retry fields are valid fixed-width digests; migration does not
			// depend on their matching the current credentials.
			p.Retry = &retryState{Kind: "he-abuse", Salt: strings.Repeat("a", 64), Key: strings.Repeat("b", 64)}
			s.Profiles["he"] = p
			legacy := "he: HE abuse response; address assumed accepted; retry on address/configuration change or when refresh is due\n"
			s.Pending = []string{legacy}
			data, err = json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			deadline := p.Updated.Add(15 * time.Minute)
			j.now = deadline.Add(-time.Nanosecond)
			if expired {
				j.now = deadline
			}
			if !expired {
				j.run(1)
				migrated := j.state().Profiles["he"]
				if calls != 1 || migrated.Address != "" || !migrated.Updated.IsZero() || !migrated.Failing || !migrated.Retry.Until.Equal(deadline) || !strings.HasSuffix(j.mails[1], legacy) {
					t.Fatal("legacy assumed state claimed confirmation or lost deadline/mail")
				}
				j.run(1)
				if calls != 1 || len(j.mails) != 2 {
					t.Fatal("migration suppression not durable/quiet")
				}
				j.now = deadline
			}
			j.run(0)
			recovered := j.state().Profiles["he"]
			if calls != 2 || recovered.Failing || recovered.Retry != nil || recovered.Address != j.ip || len(j.mails) != 3 {
				t.Fatal("legacy state did not recover at deadline")
			}
		})
	}
}
