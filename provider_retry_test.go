package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIInterventionRecovery(t *testing.T) {
	for _, code := range []int{400, 401, 404} {
		for _, change := range []string{"token", "target", "interface", "endpoint", "remove-state"} {
			t.Run(fmt.Sprintf("%d-%s", code, change), func(t *testing.T) {
				j := newJourney(t)
				calls := 0
				status := 200
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.WriteHeader(status)
					if status == 200 {
						fmt.Fprint(w, "good")
					} else {
						fmt.Fprint(w, "synthetic-secret untrusted-body")
					}
				}))
				defer srv.Close()
				base := profile("dns", "desec", srv.URL)
				j.write(base)
				j.run(0)
				j.ip = "8.0.2.20"
				status = code
				j.run(1)
				j.now = j.now.Add(1000 * time.Hour)
				out := j.run(1)
				if calls != 2 || len(j.mails) != 2 || j.state().Profiles["dns"].Address != "8.0.2.10" || !strings.Contains(out, "correct") {
					t.Fatal("intervention/confirmation/notification contract broken")
				}
				status = 200
				switch change {
				case "token":
					j.write(strings.ReplaceAll(base, "synthetic-secret", "changed-secret"))
				case "target":
					j.write(strings.ReplaceAll(base, "example.invalid", "new.invalid"))
				case "interface":
					j.write(strings.ReplaceAll(base, "sources: [{if: wan}]", "sources: [{if: newwan}]"))
				case "endpoint":
					j.write(strings.ReplaceAll(base, srv.URL, srv.URL+"/new"))
				case "remove-state":
					if err := os.Remove(filepath.Join(j.dir, "state.json")); err != nil {
						t.Fatal(err)
					}
				}
				j.run(0)
				if calls != 3 || len(j.mails) != 3 || j.state().Profiles["dns"].Address != j.ip {
					t.Fatal("correction did not resume")
				}
				b, err := os.ReadFile(filepath.Join(j.dir, "state.json"))
				if err != nil {
					t.Fatal(err)
				}
				for _, text := range []string{"synthetic-secret", "untrusted-body", "changed-secret"} {
					if strings.Contains(string(b)+out+strings.Join(j.mails, ""), text) {
						t.Fatal("unsafe output/state/mail")
					}
				}
			})
		}
	}
}

func TestCLISharedDeSECRateLimit(t *testing.T) {
	j := newJourney(t)
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		if r.URL.Path == "/a" && calls["/a"] == 1 {
			w.Header().Set("Retry-After", "300")
			w.WriteHeader(429)
			return
		}
		if r.URL.Path == "/he" {
			fmt.Fprint(w, "good "+r.URL.Query().Get("myip"))
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	a := profile("a", "desec", srv.URL+"/a")
	b := strings.ReplaceAll(profile("b", "desec", srv.URL+"/b"), "synthetic-secret", "another-secret")
	he := profile("c", "tunnelbroker", srv.URL+"/he")
	j.write(a + b + he)
	j.run(1)
	if calls["/a"] != 1 || calls["/b"] != 0 || calls["/he"] != 1 || len(j.mails) != 1 {
		t.Fatal("shared wait blocked wrong profiles")
	}
	// Restrictions survive restart and removal of the originating profile, and
	// changing the second token/target must not defeat the wait.
	j.write(strings.ReplaceAll(strings.ReplaceAll(b, "another-secret", "rotated-secret"), "example.invalid", "other.invalid") + he)
	j.now = j.now.Add(299 * time.Second)
	j.run(1)
	if calls["/b"] != 0 {
		t.Fatal("rate limit bypassed by configuration/credential change")
	}
	mailCount := len(j.mails)
	j.run(1)
	if len(j.mails) != mailCount {
		t.Fatal("unchanged suppression notified")
	}
	j.now = j.now.Add(time.Second)
	j.run(0)
	if calls["/b"] != 1 || calls["/he"] != 1 || len(j.mails) != mailCount+1 {
		t.Fatal("shared deadline not recovered")
	}
}

func TestCLIInvalidRetryStatePreserved(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(401) }))
	defer srv.Close()
	j.write(profile("dns", "desec", srv.URL))
	j.run(1)
	path := filepath.Join(j.dir, "state.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"kind":"injected"}`, `{"kind":"desec-rate"}`, `{"kind":"desec-auth","salt":"bad","key":"bad"}`, `{"kind":"he-abuse","salt":"bad","key":"bad"}`} {
		var value map[string]any
		if err := json.Unmarshal(original, &value); err != nil {
			t.Fatal(err)
		}
		var retry any
		if err := json.Unmarshal([]byte(raw), &retry); err != nil {
			t.Fatal(err)
		}
		value["profiles"].(map[string]any)["dns"].(map[string]any)["retry"] = retry
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		j.run(1)
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(data) || calls != 1 {
			t.Fatal("invalid retry state modified or provider contacted")
		}
	}
}

func TestCLIDeSECRetryDeadline(t *testing.T) {
	for _, header := range []string{"", "invalid synthetic-secret", "-1", "999999999999999999999999", "120", "Thu, 01 Jan 2026 00:02:00 GMT"} {
		t.Run(header, func(t *testing.T) {
			j := newJourney(t)
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Query().Get("myipv6") != "preserve" {
					t.Error("IPv6 not preserved")
				}
				if calls == 1 {
					if header != "" {
						w.Header().Set("Retry-After", header)
					}
					w.WriteHeader(429)
					fmt.Fprint(w, "synthetic-secret untrusted")
					return
				}
				fmt.Fprint(w, "good")
			}))
			defer srv.Close()
			j.write(profile("dns", "desec", srv.URL))
			j.mailError = true
			j.run(1)
			j.now = j.now.Add(119 * time.Second)
			j.mailError = false
			out := j.run(1)
			if calls != 1 || len(j.mails) != 1 || !strings.Contains(out, "2026-01-01T00:02:00Z") {
				t.Fatalf("deadline/mail not retained: calls=%d mail=%v out=%s", calls, j.mails, out)
			}
			j.now = j.now.Add(time.Second)
			j.run(0)
			if calls != 2 || len(j.mails) != 2 {
				t.Fatal("deadline recovery failed")
			}
		})
	}
}

func TestCLIHEAbuseUnconfirmedAndFailing(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, "abuse")
			return
		}
		fmt.Fprint(w, "good "+r.URL.Query().Get("myip"))
	}))
	defer srv.Close()
	j.write(profile("tunnel", "tunnelbroker", srv.URL))
	j.run(1)
	j.run(1)
	if calls != 1 || j.state().Profiles["tunnel"].Address != "" || len(j.mails) != 1 {
		t.Fatal("abuse must remain unconfirmed, suppress repeat and remain failing")
	}
	j.ip = "8.0.2.20"
	j.run(1)
	j.now = j.now.Add(15 * time.Minute)
	j.run(0)
	if calls != 2 || len(j.mails) != 2 {
		t.Fatal("changed address did not recover")
	}
}
