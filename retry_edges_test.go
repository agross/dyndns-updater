package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCLIRetryAfterEdges(t *testing.T) {
	for _, tc := range []struct {
		values []string
		wait   time.Duration
	}{
		{[]string{"0"}, 0}, {[]string{"1"}, time.Second}, {[]string{"+1"}, 2 * time.Minute},
		{[]string{"1.5"}, 2 * time.Minute}, {[]string{"1", "300"}, 2 * time.Minute},
		{[]string{"Wed, 31 Dec 2025 23:59:59 GMT"}, 0},
	} {
		t.Run(fmt.Sprint(tc.values), func(t *testing.T) {
			j := newJourney(t)
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					for _, v := range tc.values {
						w.Header().Add("Retry-After", v)
					}
					w.WriteHeader(429)
					return
				}
				fmt.Fprint(w, "good")
			}))
			defer srv.Close()
			j.write(profile("dns", "desec", srv.URL))
			j.run(1)
			if tc.wait > 0 {
				j.now = j.now.Add(tc.wait - time.Nanosecond)
				j.run(1)
				if calls != 1 {
					t.Fatal("retried before deadline")
				}
				j.now = j.now.Add(time.Nanosecond)
			}
			j.run(0)
			if calls != 2 {
				t.Fatal("did not retry at deadline")
			}
		})
	}
}

func TestCLIHEAbuseResumeConditions(t *testing.T) {
	for _, change := range []string{"refresh", "password", "username", "target", "endpoint", "interface", "provider"} {
		t.Run(change, func(t *testing.T) {
			j := newJourney(t)
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					fmt.Fprint(w, "abuse\n")
					return
				}
				if r.URL.Query().Get("myipv4") != "" {
					fmt.Fprint(w, "good")
					return
				}
				fmt.Fprint(w, "nochg "+r.URL.Query().Get("myip"))
			}))
			defer srv.Close()
			base := profile("tunnel", "tunnelbroker", srv.URL)
			j.write(base)
			j.run(1)
			if change == "refresh" {
				data, err := os.ReadFile(j.config)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(j.config, []byte(strings.ReplaceAll(string(data), "refresh: 600h", "refresh: 1s")), 0600); err != nil {
					t.Fatal(err)
				}
				j.now = j.now.Add(15*time.Minute - time.Nanosecond)
				j.run(1)
				j.now = j.now.Add(time.Nanosecond)
			} else {
				from, to := "synthetic-password", "new-password"
				switch change {
				case "provider":
					from, to = "provider: tunnelbroker", "provider: desec"
				case "username":
					from, to = "synthetic-user", "new-user"
				case "target":
					from, to = "example.invalid", "new.invalid"
				case "endpoint":
					from, to = srv.URL, srv.URL+"/new"
				case "interface":
					from, to = "interface: wan", "interface: changed"
				}
				j.write(strings.ReplaceAll(base, from, to))
				j.now = j.now.Add(15*time.Minute - time.Nanosecond)
				j.run(1)
				if calls != 1 || len(j.mails) != 1 {
					t.Fatal("configuration bypassed active HE deadline")
				}
				j.now = j.now.Add(time.Nanosecond)
			}
			j.run(0)
			if calls != 2 || len(j.mails) != 2 {
				t.Fatal("HE recovery missing")
			}
		})
	}
}

func TestCLIUnverifiedHEErrorsAreNotAssumedAccepted(t *testing.T) {
	for _, body := range []string{"badauth", "911", "dnserr", "abuse extra", "abuse\ngood 192.0.2.10", strings.Repeat(" ", 4096) + "abuse", "good 192.0.2.99", "abuse synthetic-secret"} {
		t.Run(fmt.Sprint(len(body), body[:min(len(body), 16)]), func(t *testing.T) {
			j := newJourney(t)
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, body) }))
			defer srv.Close()
			j.write(profile("he", "tunnelbroker", srv.URL))
			j.run(1)
			j.run(1)
			if calls != 2 || j.state().Profiles["he"].Address != "" || len(j.mails) != 1 {
				t.Fatal("unverified HE response accepted or invented retry rule")
			}
		})
	}
}

func TestBuiltCLIRetryRestrictionsSurviveRestart(t *testing.T) {
	if _, err := interfaceIPv4("lo"); err != nil {
		t.Skip("Linux loopback lo unavailable")
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") == "Token synthetic-secret" {
			w.WriteHeader(401)
			fmt.Fprint(w, "untrusted synthetic-secret")
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	bin := buildCLI(t)
	cfg, state, mail := cliConfig(t, srv.URL)
	for range 2 {
		out, err := exec.Command(bin, "--config", cfg).CombinedOutput()
		if err == nil || !strings.Contains(string(out), "authentication failed") {
			t.Fatalf("expected failure: %v %s", err, out)
		}
	}
	content, err := os.ReadFile(mail)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || strings.Count(string(content), "Subject:") != 1 {
		t.Fatal("restart bypassed restriction or duplicated mail")
	}
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(strings.ReplaceAll(string(data), "synthetic-secret", "rotated-secret")), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "--config", cfg).CombinedOutput()
	if err != nil {
		t.Fatalf("rotation: %v %s", err, out)
	}
	stored, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(mail)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || strings.Count(string(content), "Subject:") != 2 || strings.Contains(string(stored)+string(content), "synthetic-secret") || strings.Contains(string(stored), "rotated-secret") {
		t.Fatal("recovery/security contract broken")
	}
}
