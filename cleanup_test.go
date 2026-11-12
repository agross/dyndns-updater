package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCLIProfileCleanup(t *testing.T) {
	for _, mode := range []string{"unchanged", "unavailable", "independent failures"} {
		t.Run(mode, func(t *testing.T) {
			j := newJourney(t)
			calls := map[string]int{}
			bad := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls[r.URL.Path]++
				if bad && r.URL.Path == "/keep" {
					w.WriteHeader(503)
					return
				}
				fmt.Fprint(w, "good")
			}))
			defer srv.Close()
			keep := profile("keep", "desec", srv.URL+"/keep")
			removed := profile("removed", "desec", srv.URL+"/removed")
			j.write(keep + removed)
			j.run(0)
			confirmed := j.state().Profiles["keep"]
			j.write(keep)
			code := 0
			switch mode {
			case "unavailable":
				j.missing = true
				code = 1
			case "independent failures":
				bad = true
				j.ip = "8.0.2.20"
				code = 1
				j.write(keep + profile("success", "desec", srv.URL+"/success"))
			}
			out := j.run(code)
			s := j.state()
			if _, ok := s.Profiles["removed"]; ok {
				t.Fatal("obsolete confirmation survived cleanup")
			}
			retained := s.Profiles["keep"]
			if mode == "independent failures" {
				if !retained.Failing || s.Profiles["success"].Address != j.ip || calls["/success"] != 1 || calls["/keep"] != 2 || len(j.mails) != 2 {
					t.Fatal("independent outcomes lost", s, calls, j.mails)
				}
				retained.Failing = false
			} else if mode == "unavailable" {
				if !retained.Failing || calls["/keep"] != 1 || len(j.mails) != 2 {
					t.Fatal("detection failure not independent of cleanup")
				}
				retained.Failing = false
			} else if calls["/keep"] != 1 || len(j.mails) != 1 {
				t.Fatal("cleanup triggered provider or mail", calls, j.mails)
			}
			if retained != confirmed || calls["/removed"] != 1 {
				t.Fatal("retained confirmation changed", s, calls)
			}
			if mode == "unavailable" && out != "keep: source 1 (if) attempting IPv4 detection\nkeep: source 1 failed IPv4 detection\nkeep: IPv4 detection failed (all sources exhausted)\n" {
				t.Fatalf("output=%q", out)
			}
			if mode == "unchanged" {
				want := "keep: source 1 (if) attempting IPv4 detection\nkeep: interface \"wan\" detected IPv4 8.0.2.10\nkeep: skipping update: address unchanged and refresh not due\nkeep: unchanged\n"
				if out != want {
					t.Fatalf("output=%q want=%q", out, want)
				}
				// Absence was persisted; restoring the same identity/address must update.
				j.write(keep + removed)
				j.run(0)
				if calls["/removed"] != 2 || calls["/keep"] != 1 || j.state().Profiles["removed"].Address != j.ip || len(j.mails) != 2 {
					t.Fatal("restoration reused stale confirmation")
				}
			}
		})
	}
}

func TestCLIRenamedProfileRequiresSuccess(t *testing.T) {
	j := newJourney(t)
	calls, bad := 0, false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if bad {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	j.write(profile("old", "desec", srv.URL))
	j.run(0)
	j.write(profile("new", "desec", srv.URL))
	bad = true
	j.run(1)
	s := j.state()
	if _, ok := s.Profiles["old"]; ok {
		t.Fatal("old name survived rename")
	}
	if calls != 2 || s.Profiles["new"].Address != "" || !s.Profiles["new"].Updated.IsZero() || !s.Profiles["new"].Failing {
		t.Fatal("rename reused confirmation or confirmed failed request", s)
	}
	bad = false
	j.run(0)
	if calls != 3 || j.state().Profiles["new"].Address != j.ip {
		t.Fatal("renamed profile did not retry")
	}
}

func TestCLICleanupPreservesQueuedMailAndIntent(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	keep := profile("keep", "desec", srv.URL)
	j.write(keep + profile("removed", "desec", srv.URL))
	j.mailError = true
	j.run(1)
	path := filepath.Join(j.dir, "state.json")
	s := j.state()
	s.Pending = append(s.Pending, "removed: unchanged\n")
	s.Dirty = true
	if err := saveState(path, s); err != nil {
		t.Fatal(err)
	}
	queued := append([]string(nil), s.Pending...)
	j.write(keep)
	j.run(1)
	s = j.state()
	want := append(queued, "Summary generated at: 2026-01-01T00:00:00Z\nkeep: unchanged\n")
	if len(s.Profiles) != 1 || s.Dirty || !reflect.DeepEqual(s.Pending, want) || calls != 2 {
		t.Fatal("cleanup altered queue/intent", s)
	}
	j.mailError = false
	j.run(0)
	if !reflect.DeepEqual(j.mails, want) || len(j.state().Pending) != 0 || calls != 2 {
		t.Fatal("queue not retried intact in order", j.mails)
	}
}

func TestCLICleanupRejectsInvalidInputsWithoutWrites(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	keep := profile("keep", "desec", srv.URL)
	j.write(keep + profile("removed", "desec", srv.URL))
	j.run(0)
	path := filepath.Join(j.dir, "state.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	j.write(keep)
	config, err := os.ReadFile(j.config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.config, append(config, []byte("unknown: true\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	j.run(1)
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(original) {
		t.Fatal("invalid config changed state", err)
	}
	j.write(keep)
	corrupt := strings.TrimSpace(string(original)) + " trailing"
	if err := os.WriteFile(path, []byte(corrupt), 0600); err != nil {
		t.Fatal(err)
	}
	j.run(1)
	got, err = os.ReadFile(path)
	if err != nil || string(got) != corrupt || calls != 2 || len(j.mails) != 1 {
		t.Fatal("invalid input altered state or caused external activity", err)
	}
}

func TestBuiltCLICleanupRetriesRemovedProfileMail(t *testing.T) {
	if _, err := interfaceIPv4("lo"); err != nil {
		t.Skip("Linux loopback lo unavailable")
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") }))
	defer srv.Close()
	bin := buildCLI(t)
	cfg, state, mail := cliConfig(t, srv.URL)
	config, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	both := append(append([]byte(nil), config...), []byte(strings.Replace(profile("removed", "desec", srv.URL), "sources: [{if: wan}]", localDiscoverySetting(t), 1))...)
	if err := os.WriteFile(cfg, both, 0600); err != nil {
		t.Fatal(err)
	}
	sm := filepath.Join(filepath.Dir(cfg), "sendmail")
	script, err := os.ReadFile(sm)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sm, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bin, "--config", cfg).CombinedOutput(); err == nil {
		t.Fatalf("mail failure exited zero: %s", out)
	}
	before, err := readState(state)
	if err != nil || len(before.Pending) != 1 {
		t.Fatal("missing queued summary", err)
	}
	if err := os.WriteFile(cfg, config, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sm, script, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		out, err := exec.Command(bin, "--config", cfg).CombinedOutput()
		want := "dns: source 1 (url) attempting IPv4 detection\ndns: source 1 detected IPv4 8.0.2.10\ndns: skipping update: address unchanged and refresh not due\ndns: unchanged\n"
		if err != nil || string(out) != want {
			t.Fatalf("cleanup/retry: %v %q", err, out)
		}
	}
	after, err := readState(state)
	if err != nil || len(after.Profiles) != 1 || after.Profiles["dns"] != before.Profiles["dns"] || len(after.Pending) != 0 || calls.Load() != 2 {
		t.Fatal("cleanup/retry state or provider counts wrong", after, err)
	}
	body, err := os.ReadFile(mail)
	want := "To: root\nSubject: dyndns-updater profile changes\nContent-Type: text/plain; charset=UTF-8\n\n" + before.Pending[0]
	if err != nil || string(body) != want {
		t.Fatalf("queued summary not submitted intact exactly once: %v %q", err, body)
	}
}
