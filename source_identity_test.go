package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// Historical fixtures deliberately do not call the current identity/retry helpers.
func historicalDigest(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func legacySourceState(j *journey, endpoint, kind string) {
	j.t.Helper()
	id := historicalDigest(struct {
		Provider, Target, Endpoint string
		Sources                    []map[string]string
	}{"desec", "example.invalid", endpoint, []map[string]string{{"Selector": "if", "Value": "wan"}}})
	// Sources historically marshal as exported Selector/Value fields in order.
	p := map[string]any{"identity": id, "address": "8.0.2.10", "updated": "2026-01-01T00:00:00Z", "failing": kind != ""}
	if kind != "" {
		salt := strings.Repeat("ab", 32)
		p["retry"] = map[string]any{"kind": kind, "salt": salt, "key": historicalDigest([]string{salt, id, "synthetic-secret"})}
	}
	b, err := json.Marshal(map[string]any{"version": 1, "profiles": map[string]any{"dns": p}, "dirty": false, "pending": []string{"dns: confirmed 8.0.2.10\n"}})
	if err != nil {
		j.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(j.dir, "state.json"), b, 0600); err != nil {
		j.t.Fatal(err)
	}
}

func TestCLILegacySourceConfirmation(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(fmt.Sprint(edited), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") }))
			defer srv.Close()
			j := newJourney(t)
			j.write(sourceProfile("dns", srv.URL, "if:wan"))
			legacySourceState(j, srv.URL, "")
			if edited {
				j.write(sourceProfile("dns", srv.URL, "if:other"))
			}
			j.now = j.now.Add(time.Minute)
			j.run(0)
			want := int32(0)
			if edited {
				want = 1
			}
			if calls.Load() != want {
				t.Fatalf("legacy verification: calls=%d want=%d", calls.Load(), want)
			}
			if !edited && !j.state().Profiles["dns"].Updated.Equal(j.now.Add(-time.Minute)) {
				t.Fatal("legacy timestamp advanced")
			}
			j.write(sourceProfile("dns", srv.URL, "if:third", "if:wan"))
			j.run(0)
			if calls.Load() != want {
				t.Fatal("migration not durable")
			}
			if len(j.mails) == 0 || j.mails[0] != "dns: confirmed 8.0.2.10\n" {
				t.Fatal("legacy queued mail changed")
			}
		})
	}
}

func TestCLILegacySourceIntervention(t *testing.T) {
	for _, kind := range []string{"desec-auth", "desec-hostname", "desec-target"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") }))
			defer srv.Close()
			j := newJourney(t)
			legacySourceState(j, srv.URL, kind)
			j.write(sourceProfile("dns", srv.URL, "if:other"))
			j.run(1)
			j.run(1)
			if calls.Load() != 0 || !j.state().Profiles["dns"].Failing {
				t.Fatal("legacy source edit unlocked intervention")
			}
			j.write(strings.ReplaceAll(sourceProfile("dns", srv.URL, "if:third"), "synthetic-secret", "corrected-secret"))
			j.run(0)
			if calls.Load() != 1 || j.state().Profiles["dns"].Failing {
				t.Fatal("credential correction failed")
			}
		})
	}
}

func TestCLISourceOnlyConfirmation(t *testing.T) {
	var calls, discoveries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/discover") {
			discoveries.Add(1)
			fmt.Fprint(w, "8.0.2.10")
			return
		}
		calls.Add(1)
		if r.URL.Query().Get("myipv6") != "preserve" {
			t.Error("IPv6 not preserved")
		}
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	j := newJourney(t)
	web := "url:" + srv.URL + "/discover"
	j.write(sourceProfile("dns", srv.URL+"/provider", web))
	j.run(0)
	confirmed := j.state().Profiles["dns"]
	for _, sources := range [][]string{
		{"if:wan"}, {web}, {web, "if:wan"}, {web}, {"url:" + srv.URL + "/discover2"}, {"if:wan", web}, {web, "if:wan"},
	} {
		j.now = j.now.Add(time.Minute)
		j.write(sourceProfile("dns", srv.URL+"/provider", sources...))
		before := discoveries.Load()
		out := j.run(0)
		got := j.state().Profiles["dns"]
		if calls.Load() != 1 || len(j.mailAttempts) != 1 || got.Address != confirmed.Address || !got.Updated.Equal(confirmed.Updated) || !strings.Contains(out, "dns: unchanged") {
			t.Fatalf("source-only edit reconfirmed: calls=%d mail=%d state=%+v output=%s", calls.Load(), len(j.mailAttempts), got, out)
		}
		if strings.HasPrefix(sources[0], "url:") && discoveries.Load() != before+1 {
			t.Fatal("detection did not run")
		}
	}
	j.now = confirmed.Updated.Add(600 * time.Hour)
	j.run(0)
	if calls.Load() != 2 || len(j.mails) != 1 {
		t.Fatal("refresh deadline moved or refresh notified")
	}
}
