package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type journey struct {
	t               *testing.T
	dir, config, ip string
	now             time.Time
	missing         bool
	mailError       bool
	mails           []string
}

func newJourney(t *testing.T) *journey {
	return &journey{t: t, dir: t.TempDir(), ip: "192.0.2.10", now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}
func (j *journey) write(profiles string) {
	j.t.Helper()
	j.config = filepath.Join(j.dir, "config.yaml")
	b := fmt.Sprintf("state: %s\nrefresh: 600h\ntimeout: 100ms\nprofiles:\n%s", filepath.Join(j.dir, "state.json"), profiles)
	if e := os.WriteFile(j.config, []byte(b), 0600); e != nil {
		j.t.Fatal(e)
	}
}
func profile(name, provider, endpoint string) string {
	return fmt.Sprintf("  %q:\n    provider: %s\n    interface: wan\n    target: example.invalid\n    token: synthetic-secret\n    username: synthetic-user\n    password: synthetic-password\n    endpoint: %s\n", name, provider, endpoint)
}
func (j *journey) run(want int) string {
	j.t.Helper()
	var out bytes.Buffer
	d := dependencies{func(string) (string, error) {
		if j.missing {
			return "", errors.New("missing")
		}
		return j.ip, nil
	}, func() time.Time { return j.now }, func(_ Config, b string) error {
		if j.mailError {
			return errors.New("mail failed")
		}
		j.mails = append(j.mails, b)
		return nil
	}}
	code := runCLI([]string{"--config", j.config}, &out, d)
	if code != want {
		j.t.Fatalf("exit=%d want=%d output=%s", code, want, out.String())
	}
	for _, secret := range []string{"synthetic-secret", "synthetic-password", "synthetic-user"} {
		if strings.Contains(out.String(), secret) {
			j.t.Fatal("credential leaked")
		}
	}
	return out.String()
}
func (j *journey) state() diskState {
	j.t.Helper()
	s, e := readState(filepath.Join(j.dir, "state.json"))
	if e != nil {
		j.t.Fatal(e)
	}
	return s
}

func TestFailureTransitionsAndIndependentProfiles(t *testing.T) {
	j := newJourney(t)
	bad := false
	errorBody := "badauth synthetic-secret"
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path
		calls[name]++
		if name == "/tunnel" {
			u, p, ok := r.BasicAuth()
			if !ok || u != "synthetic-user" || p != "synthetic-password" || r.URL.Query().Get("myip") != j.ip {
				t.Error("invalid Tunnelbroker contract")
			}
			if bad {
				fmt.Fprint(w, errorBody)
				return
			}
			fmt.Fprint(w, "nochg "+r.URL.Query().Get("myip"))
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer server.Close()
	j.write(profile("dns", "desec", server.URL+"/dns") + profile("tunnel", "tunnelbroker", server.URL+"/tunnel"))
	j.run(0)
	bad = true
	j.now = j.now.Add(601 * time.Hour)
	j.run(1)
	if len(j.mails) != 2 || !strings.Contains(j.mails[1], "\"dns\":") || !strings.Contains(j.mails[1], "\"tunnel\":") {
		t.Fatal("expected all-profile failure summary")
	}
	dnsCalls := calls["/dns"]
	j.run(1)
	errorBody = "911 different error"
	j.run(1)
	if len(j.mails) != 2 || calls["/dns"] != dnsCalls {
		t.Fatal("repeated failure notified or independent success lost")
	}
	j.missing = true
	j.run(1)
	if len(j.mails) != 2 {
		t.Fatal("missing interface notified")
	}
	j.missing = false
	bad = false
	j.run(0)
	if len(j.mails) != 3 {
		t.Fatal("recovery not notified")
	}
	j.missing = true
	j.run(1)
	j.missing = false
	j.run(0)
	if len(j.mails) != 3 {
		t.Fatal("unchanged interface return notified")
	}
}

func TestPendingMailAndCrashBoundary(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	j.write(profile("dns", "desec", srv.URL))
	j.mailError = true
	j.run(1)
	if len(j.state().Pending) != 1 {
		t.Fatal("notification intent lost")
	}
	j.mailError = false
	j.run(0)
	if calls != 1 || len(j.mails) != 1 || len(j.state().Pending) != 0 {
		t.Fatal("mail retry must not repeat provider update")
	}
	// Reconstruct the durable pre-acceptance snapshot: local acceptance followed
	// by process death before its acknowledgement write must cause a duplicate.
	s := j.state()
	s.Pending = []string{j.mails[0]}
	if e := saveState(filepath.Join(j.dir, "state.json"), s); e != nil {
		t.Fatal(e)
	}
	j.run(0)
	if len(j.mails) != 2 || calls != 1 {
		t.Fatal("at least once retry failed")
	}
	// A crash between independent profile persistence and summary creation.
	s = j.state()
	s.Dirty = true
	if e := saveState(filepath.Join(j.dir, "state.json"), s); e != nil {
		t.Fatal(e)
	}
	j.run(0)
	if len(j.mails) != 3 {
		t.Fatal("durable notification intent lost")
	}
}

func TestIdentityAndCorruptState(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("myip") != "" {
			fmt.Fprint(w, "good "+r.URL.Query().Get("myip"))
		} else {
			fmt.Fprint(w, "good")
		}
	}))
	defer srv.Close()
	base := profile("dns", "desec", srv.URL)
	j.write(base)
	j.run(0)
	j.write(strings.Replace(base, "synthetic-secret", "rotated", 1))
	j.run(0)
	if calls != 1 {
		t.Fatal("credential rotation invalidated state")
	}
	for _, change := range []string{strings.Replace(base, "example.invalid", "new.invalid", 1), strings.Replace(base, "interface: wan", "interface: other", 1), profile("dns", "tunnelbroker", srv.URL)} {
		j.write(change)
		j.run(0)
	}
	if calls != 4 {
		t.Fatal("identity changes did not force updates")
	}
	path := filepath.Join(j.dir, "state.json")
	for _, bad := range []string{"{", `{}`, `{"version":1,"profiles":{"dns":{"identity":"x","address":"not-ip"}}}`, `{"version":1,"profiles":{}} trailing`} {
		if e := os.WriteFile(path, []byte(bad), 0600); e != nil {
			t.Fatal(e)
		}
		j.run(1)
		b, _ := os.ReadFile(path)
		if string(b) != bad || calls != 4 {
			t.Fatal("corrupt state changed or provider called")
		}
	}
	if e := os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	j.run(1)
	if calls != 4 {
		t.Fatal("unreadable state called provider")
	}
}

func TestInvalidConfigDoesNotLeakOrMail(t *testing.T) {
	j := newJourney(t)
	for _, content := range []string{"token: synthetic-secret\nprofiles: [", "unknown: synthetic-secret", "profiles: []", "---\nprofiles: []\n---\nsecret: synthetic-password"} {
		j.config = filepath.Join(j.dir, "invalid.yaml")
		if e := os.WriteFile(j.config, []byte(content), 0600); e != nil {
			t.Fatal(e)
		}
		j.run(1)
	}
	for _, endpoint := range []string{"http://example.invalid/", "https://user:synthetic-password@example.invalid/", "https://example.invalid/?password=synthetic-password"} {
		j.write(profile("dns", "desec", endpoint))
		j.run(1)
	}
	if len(j.mails) != 0 {
		t.Fatal("config error sent mail")
	}
}

func TestRejectedResponsesAndTimeout(t *testing.T) {
	for _, tc := range []struct {
		provider, body string
		status         int
	}{
		{"desec", "good 192.0.2.10", 200}, {"desec", "good\nbadauth", 200}, {"desec", "", 200}, {"desec", strings.Repeat("x", 4097), 200}, {"desec", "good", 401}, {"desec", "good", 302}, {"tunnelbroker", "good 192.0.2.99", 200}, {"tunnelbroker", "good", 200}, {"tunnelbroker", "good 192.0.2.10 extra", 200},
	} {
		t.Run(fmt.Sprintf("%s-%d-%d", tc.provider, tc.status, len(tc.body)), func(t *testing.T) {
			j := newJourney(t)
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			j.write(profile("dns", tc.provider, srv.URL))
			j.run(1)
			j.run(1)
			wantCalls := 2
			if tc.provider == "desec" && tc.status == 401 {
				wantCalls = 1
			}
			if calls != wantCalls || j.state().Profiles["dns"].Address != "" || len(j.mails) != 1 {
				t.Fatal("rejection recorded success or failed retry/dedup")
			}
		})
	}
	j := newJourney(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	j.write(profile("dns", "desec", srv.URL))
	j.run(1)
}

func TestAddressChangeDuringUpdate(t *testing.T) {
	j := newJourney(t)
	received := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.URL.Query().Get("myipv4"))
		j.ip = "192.0.2.20"
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	j.write(profile("dns", "desec", srv.URL))
	j.run(0)
	if j.state().Profiles["dns"].Address != "192.0.2.10" {
		t.Fatal("recorded unconfirmed address")
	}
	j.run(0)
	if len(received) != 2 || received[1] != "192.0.2.20" {
		t.Fatal("next run missed change")
	}
}

func TestStateContainsNoCredentials(t *testing.T) {
	j := newJourney(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "good") }))
	defer srv.Close()
	j.write(profile("dns", "desec", srv.URL))
	j.run(0)
	b, e := json.Marshal(j.state())
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "synthetic-") {
		t.Fatal("credentials persisted")
	}
}
