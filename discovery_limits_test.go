package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCLIDiscoveryHTTPFailuresAndBudgets(t *testing.T) {
	for _, mode := range []string{"status", "redirect", "read failure", "slow headers", "slow body", "TLS", "connection", "fresh budget"} {
		t.Run(mode, func(t *testing.T) {
			var first, later, providers atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/first":
					first.Add(1)
					switch mode {
					case "status":
						w.WriteHeader(503)
						fmt.Fprint(w, "untrusted-body secret-query")
						return
					case "redirect":
						http.Redirect(w, r, "/later?secret-query", 302)
						return
					case "read failure":
						w.Header().Set("Content-Length", "100")
						fmt.Fprint(w, "8.8.4.4")
						return
					case "slow body":
						w.(http.Flusher).Flush()
						<-r.Context().Done()
						return
					default:
						<-r.Context().Done()
						return
					}
				case "/later":
					later.Add(1)
					if mode == "fresh budget" {
						time.Sleep(60 * time.Millisecond)
					}
					fmt.Fprint(w, "9.9.9.9")
				default:
					providers.Add(1)
					if r.URL.Query().Get("myipv4") != "9.9.9.9" {
						t.Error("wrong candidate")
					}
					fmt.Fprint(w, "good")
				}
			}))
			defer srv.Close()
			endpoint := srv.URL + "/first?secret-query"
			if mode == "TLS" {
				tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted TLS accepted") }))
				defer tlsSrv.Close()
				endpoint = tlsSrv.URL
			}
			if mode == "connection" {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				endpoint = "http://" + l.Addr().String()
				l.Close()
			}
			j := newJourney(t)
			j.write(sourceProfile("dns", srv.URL+"/provider", "url:"+endpoint, "url:"+srv.URL+"/later"))
			j.fallback()
			out := j.run(0)
			if later.Load() != 1 || providers.Load() != 1 || j.state().Profiles["dns"].Address != "9.9.9.9" || len(j.mails) != 1 {
				t.Fatal("fallback/notification contract failed")
			}
			if strings.Contains(out, "secret-query") || strings.Contains(out, "untrusted-body") || strings.Contains(j.mails[0], "source 1") {
				t.Fatal("unsafe diagnostics")
			}
		})
	}
}

func TestCLIDiscoveryIdentityAndFailureDeduplication(t *testing.T) {
	var calls atomic.Int32
	reject := atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if reject.Load() {
			w.WriteHeader(500)
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	j := newJourney(t)
	j.write(sourceProfile("dns", srv.URL, "if:wan"))
	j.run(0)
	confirmed := j.state().Profiles["dns"]
	j.missing = true
	j.run(1)
	j.write(sourceProfile("dns", srv.URL, "if:wan", "web:ipify"))
	// Success of the first source skips ipify. Changing source configuration is
	// still a new identity requiring provider acceptance, not recovery by detection.
	j.missing = false
	reject.Store(true)
	j.run(1)
	if len(j.mails) != 2 || !j.state().Profiles["dns"].Failing {
		t.Fatal("source change duplicated an uncleared failure")
	}
	reject.Store(false)
	j.run(0)
	if calls.Load() != 3 || len(j.mails) != 3 || j.state().Profiles["dns"].Identity == confirmed.Identity {
		t.Fatal("source identity/recovery missing")
	}
	// Reordering sources also invalidates confirmation, even if address is equal.
	j.write(sourceProfile("dns", srv.URL, "if:wan", "if:other"))
	j.run(0)
	j.write(sourceProfile("dns", srv.URL, "if:other", "if:wan"))
	j.run(0)
	if calls.Load() != 5 {
		t.Fatal("source identity change skipped provider confirmation")
	}
}

func TestCLIDiscoveryExhaustionRetainsProviderPauses(t *testing.T) {
	for _, provider := range []string{"desec", "tunnelbroker"} {
		t.Run(provider, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if provider == "desec" {
					w.Header().Set("Retry-After", "120")
					w.WriteHeader(429)
				} else {
					fmt.Fprint(w, "abuse")
				}
			}))
			defer srv.Close()
			j := newJourney(t)
			base := profile("dns", provider, srv.URL)
			j.write(base)
			j.run(1)
			restriction := *j.state().Profiles["dns"].Retry
			j.missing = true
			j.run(1)
			j.run(1)
			if got := j.state().Profiles["dns"].Retry; got == nil || *got != restriction || calls.Load() != 1 || len(j.mails) != 1 {
				t.Fatal("exhaustion lost provider pause or duplicated failure mail")
			}
			j.missing = false
			j.write(strings.Replace(base, "sources: [{if: wan}]", "sources: [{if: other}, {if: wan}]", 1))
			j.run(1)
			if calls.Load() != 1 || len(j.mails) != 1 || j.state().Profiles["dns"].Retry.Until != restriction.Until {
				t.Fatal("source change bypassed surviving provider pause")
			}
		})
	}
}
