package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCLISourceEditChangedAddressStrictAcceptance(t *testing.T) {
	var calls atomic.Int32
	var reject atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if reject.Load() {
			fmt.Fprint(w, "good 8.0.2.20")
			return
		} // HTTP 200 is not deSEC acceptance.
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	j := newJourney(t)
	j.write(sourceProfile("dns", srv.URL, "if:wan"))
	j.run(0)
	before := j.state().Profiles["dns"]
	j.now = j.now.Add(time.Minute)
	j.ip = "8.0.2.20"
	j.write(sourceProfile("dns", srv.URL, "if:new"))
	reject.Store(true)
	j.run(1)
	got := j.state().Profiles["dns"]
	if calls.Load() != 2 || !got.Failing || got.Address != before.Address || !got.Updated.Equal(before.Updated) {
		t.Fatal("rejection advanced confirmation")
	}
	j.write(sourceProfile("dns", srv.URL, "if:third"))
	j.run(1)
	if calls.Load() != 3 || len(j.mails) != 2 {
		t.Fatal("failure lost or duplicate notification")
	}
	reject.Store(false)
	j.run(0)
	if got := j.state().Profiles["dns"]; got.Failing || got.Address != j.ip || !got.Updated.Equal(j.now) || len(j.mails) != 3 {
		t.Fatal("strict recovery missing")
	}
}

func TestCLISourceIndependentDestinationChanges(t *testing.T) {
	for _, change := range []string{"provider", "target", "endpoint", "missing"} {
		t.Run(change, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if ip := r.URL.Query().Get("myip"); ip != "" {
					fmt.Fprint(w, "good "+ip)
				} else {
					fmt.Fprint(w, "good")
				}
			}))
			defer srv.Close()
			j := newJourney(t)
			base := sourceProfile("dns", srv.URL, "if:wan")
			j.write(base)
			j.run(0)
			switch change {
			case "provider":
				base = strings.Replace(base, "provider: desec", "provider: tunnelbroker", 1)
			case "target":
				base = strings.ReplaceAll(base, "example.invalid", "other.invalid")
			case "endpoint":
				base = strings.ReplaceAll(base, srv.URL, srv.URL+"/other")
			case "missing":
				if err := os.Remove(filepath.Join(j.dir, "state.json")); err != nil {
					t.Fatal(err)
				}
			}
			j.write(base)
			j.run(0)
			if calls.Load() != 2 || j.state().Profiles["dns"].Address != j.ip {
				t.Fatal("destination/missing confirmation did not update")
			}
		})
	}
}

func TestCLILegacyUnverifiedRejected(t *testing.T) {
	var calls atomic.Int32
	var accept atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if accept.Load() {
			fmt.Fprint(w, "good")
		} else {
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	j := newJourney(t)
	legacySourceState(j, srv.URL+"/unknown-destination", "")
	j.write(sourceProfile("dns", srv.URL, "if:wan"))
	j.run(1)
	got := j.state().Profiles["dns"]
	if calls.Load() != 1 || got.Address != "" || !got.Updated.IsZero() || !got.Failing {
		t.Fatal("unverified legacy destination trusted")
	}
	accept.Store(true)
	j.run(0)
	if calls.Load() != 2 || j.state().Profiles["dns"].Address != j.ip {
		t.Fatal("fresh confirmation missing")
	}
}

func TestCLILegacyInterventionDetectionFailure(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") }))
	defer srv.Close()
	j := newJourney(t)
	legacySourceState(j, srv.URL, "desec-auth")
	j.write(sourceProfile("dns", srv.URL, "if:other"))
	j.missing = true
	j.run(1)
	j.run(1)
	j.missing = false
	j.run(1)
	if calls.Load() != 0 || !j.state().Profiles["dns"].Failing {
		t.Fatal("migration/detection outage lost intervention")
	}
	// Once a conservative legacy block is rebound, destination correction works.
	j.write(sourceProfile("dns", srv.URL+"/corrected", "if:other"))
	j.run(0)
	if calls.Load() != 1 {
		t.Fatal("destination correction failed")
	}
}

func TestCLILegacySharedDeadline(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") }))
	defer srv.Close()
	j := newJourney(t)
	legacySourceState(j, srv.URL, "desec-auth")
	s := j.state()
	p := s.Profiles["dns"]
	p.Retry = &retryState{Kind: "desec-rate", Until: j.now.Add(2 * time.Minute)}
	s.Profiles["dns"] = p
	if err := saveState(filepath.Join(j.dir, "state.json"), s); err != nil {
		t.Fatal(err)
	}
	second := strings.ReplaceAll(sourceProfile("other", srv.URL+"/other", "if:new"), "synthetic-secret", "other-secret")
	j.write(sourceProfile("dns", srv.URL, "if:changed") + second)
	j.run(1)
	if calls.Load() != 0 || j.state().Profiles["other"].Retry == nil || !j.state().Profiles["other"].Retry.Until.Equal(p.Retry.Until) {
		t.Fatal("legacy shared deadline was not copied")
	}
	j.write(second)
	j.run(1)
	if calls.Load() != 0 {
		t.Fatal("legacy deadline lost on origin removal")
	}
	j.now = j.now.Add(2 * time.Minute)
	j.run(0)
	if calls.Load() != 1 {
		t.Fatal("shared legacy deadline expiry did not retry")
	}
}

func TestCLICorruptRetryVersionPreserved(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(401) }))
	defer srv.Close()
	j := newJourney(t)
	j.write(sourceProfile("dns", srv.URL, "if:wan"))
	j.run(1)
	path := filepath.Join(j.dir, "state.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := strings.Replace(string(original), `"key_version": 1`, `"key_version": 9`, 1)
	if corrupt == string(original) {
		t.Fatal("fixture did not change")
	}
	if err := os.WriteFile(path, []byte(corrupt), 0600); err != nil {
		t.Fatal(err)
	}
	attempts := len(j.mailAttempts)
	j.run(1)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != corrupt || calls.Load() != 1 || len(j.mailAttempts) != attempts {
		t.Fatal("corrupt retry version caused side effects")
	}
}

func TestCLILegacyProviderDeadlines(t *testing.T) {
	for _, provider := range []string{"desec", "tunnelbroker"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if provider == "desec" {
					fmt.Fprint(w, "good")
				} else {
					fmt.Fprint(w, "good "+r.URL.Query().Get("myip"))
				}
			}))
			defer srv.Close()
			j := newJourney(t)
			legacySourceState(j, srv.URL, "desec-auth")
			s := j.state()
			p := s.Profiles["dns"]
			wait := 2 * time.Minute
			if provider == "desec" {
				p.Retry = &retryState{Kind: "desec-rate", Until: j.now.Add(wait)}
			} else {
				wait = 15 * time.Minute
				// Deadline is profile-wide; the historical key is never used to unlock it.
				p.Retry.Kind = "he-abuse"
				p.Retry.Until = j.now.Add(wait)
			}
			s.Profiles["dns"] = p
			if err := saveState(filepath.Join(j.dir, "state.json"), s); err != nil {
				t.Fatal(err)
			}
			base := strings.ReplaceAll(profile("dns", provider, srv.URL), "sources: [{if: wan}]", "sources: [{if: changed}]")
			j.write(base)
			j.now = j.now.Add(wait - time.Second)
			j.run(1)
			if calls.Load() != 0 || !j.state().Profiles["dns"].Retry.Until.Equal(p.Retry.Until) {
				t.Fatal("legacy pause bypassed")
			}
			j.now = j.now.Add(time.Second)
			j.run(0)
			if calls.Load() != 1 || j.state().Profiles["dns"].Failing {
				t.Fatal("legacy pause expiry did not retry")
			}
		})
	}
}
