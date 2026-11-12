package main

import (
	"bytes"
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

func TestCLIDiscoveryExhaustionDurability(t *testing.T) {
	j := newJourney(t)
	var discovery, updates atomic.Int32
	body := "8.8.4.4"
	reject := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/discover" {
			discovery.Add(1)
			fmt.Fprint(w, body)
			return
		}
		updates.Add(1)
		if reject {
			w.WriteHeader(500)
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	j.write(sourceProfile("a", srv.URL+"/provider", "web:"+srv.URL+"/discover"))
	j.run(0)
	confirmed := j.state().Profiles["a"]
	body = "10.0.0.1 secret-response"
	j.mailError = true
	j.run(1)
	failed := j.state()
	if !failed.Profiles["a"].Failing || failed.Profiles["a"].Address != confirmed.Address || !failed.Profiles["a"].Updated.Equal(confirmed.Updated) || len(failed.Pending) != 1 {
		t.Fatalf("failure not durable: %+v", failed)
	}
	queued := failed.Pending[0]
	j.run(1)
	if len(j.state().Pending) != 1 || j.state().Pending[0] != queued || len(j.mailAttempts) != 3 {
		t.Fatal("failure duplicated or pending mail not retried")
	}
	j.mailError = false
	j.run(1)
	if len(j.mails) != 2 || j.mails[1] != queued || len(j.state().Pending) != 0 {
		t.Fatal("mail intent not delivered unchanged")
	}
	// An independent profile still succeeds during exhaustion.
	j.write(sourceProfile("a", srv.URL+"/provider", "web:"+srv.URL+"/discover") + sourceProfile("b", srv.URL+"/provider", "if:wan"))
	j.ip = "9.9.9.9"
	j.run(1)
	if j.state().Profiles["b"].Address != "9.9.9.9" {
		t.Fatal("independent outcome lost")
	}
	// Detection success is not provider confirmation or recovery; no source replay.
	body = "8.8.4.4"
	reject = true
	before := discovery.Load()
	mail := len(j.mails)
	j.run(1)
	if !j.state().Profiles["a"].Failing || len(j.mails) != mail || discovery.Load() != before+1 {
		t.Fatal("discovery claimed recovery or provider failure restarted it")
	}
	reject = false
	j.run(0)
	if j.state().Profiles["a"].Failing || len(j.mails) != mail+1 {
		t.Fatal("provider-confirmed recovery missing")
	}
}

func TestCLIRejectSourceConfigurations(t *testing.T) {
	for _, setting := range []string{
		"sources: []", "sources: null", "sources: if:wan", "sources: [if:wan]", "sources: [17]", "sources: [true]",
		"sources: [{}]", "sources: [{if: wan, web: ipify}]", "sources: [{if: wan, if: other}]",
		"sources: [{if: wan, extra: value}]", "sources: [{other: wan}]", "sources: [{17: wan}]",
		"sources: [{if: ''}]", "sources: [{if: '   '}]", "sources: [{web: ''}]", "sources: [{url: ''}]",
		"sources: [{if: 17}]", "sources: [{web: true}]", "sources: [{url: []}]", "sources: [{if: null}]",
		"sources: [{if: wan}, {web: unknown}]", "sources: [{web: 'https://api.ipify.org/'}]",
		"sources: [{url: 'http://example.invalid/'}]", "sources: [{url: 'https://user:secret@example.invalid/'}]",
		"sources: [{url: 'https://example.invalid/#frag'}]", "sources: [{url: 'https://example.invalid/#'}]",
		"sources: [{url: 'https://example.invalid:0/'}]", "sources: [{url: 'https://example.invalid:99999/'}]",
		"sources: [{url: 'https://example.invalid:/'}]", "sources: [{url: '/relative'}]", "sources: [{url: 'https:///'}]",
		"interface: wan", "sources: [{if: wan}]\n    interface: wan",
	} {
		t.Run(setting, func(t *testing.T) {
			j := newJourney(t)
			j.write(strings.Replace(sourceProfile("dns", "http://127.0.0.1:1", "if:wan"), `sources: [{if: "wan"}]`, setting, 1))
			state := filepath.Join(j.dir, "state.json")
			original := []byte("preserve existing bytes")
			if err := os.WriteFile(state, original, 0600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			d := dependencies{func(string) (string, error) { t.Fatal("interface attempted"); return "", nil }, time.Now, func(Config, string) error { t.Fatal("mail attempted"); return nil }}
			if runCLI([]string{"--config", j.config}, &out, d) != 1 || !strings.Contains(out.String(), "configuration") {
				t.Fatalf("not rejected: %s", out.String())
			}
			got, err := os.ReadFile(state)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("state changed")
			}
			if _, err := os.Stat(state + ".lock"); !os.IsNotExist(err) {
				t.Fatal("lock created")
			}
		})
	}
}

func TestCLIDiscoveryInvalidResponsesFallback(t *testing.T) {
	for _, body := range []string{"10.0.0.1", "100.64.0.1", "127.0.0.1", "169.254.1.1", "172.16.0.1", "192.168.0.1", "0.1.2.3", "224.0.0.1", "255.255.255.255", "192.0.2.1", "198.51.100.1", "203.0.113.1", "198.18.0.1", "240.1.1.1", "192.0.0.1", "192.88.99.1", "::1", "::ffff:8.8.4.4", "8.8.4.4 9.9.9.9", "address 8.8.4.4", "<b>8.8.4.4</b>", `{"ip":"8.8.4.4"}`, "8.8.4.999", strings.Repeat("x", 65537)} {
		t.Run(fmt.Sprintf("body-%d", len(body))+"-"+strings.ReplaceAll(body[:min(len(body), 25)], "/", "_"), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/bad" {
					fmt.Fprint(w, body)
					return
				}
				calls.Add(1)
				fmt.Fprint(w, "good")
			}))
			defer srv.Close()
			j := newJourney(t)
			j.ip = "9.9.9.9"
			j.write(sourceProfile("dns", srv.URL+"/provider", "web:"+srv.URL+"/bad", "if:wan"))
			out := j.run(0)
			if calls.Load() != 1 || j.state().Profiles["dns"].Address != j.ip || strings.Contains(out, "address 8.8") {
				t.Fatal("invalid response selected or leaked")
			}
		})
	}
}
