package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBuiltCLIDiscoveryFailureMailRestart(t *testing.T) {
	var bad atomic.Bool
	var providerCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/discover" {
			if bad.Load() {
				fmt.Fprint(w, "10.0.0.1 untrusted-secret")
			} else {
				fmt.Fprint(w, "8.8.4.4")
			}
			return
		}
		providerCalls.Add(1)
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	bin := buildCLI(t)
	cfg, state, mail := cliConfig(t, srv.URL)
	b, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		if strings.Contains(line, "sources:") {
			lines[i] = fmt.Sprintf("    sources: [{url: %q}]", srv.URL+"/discover")
		}
	}
	if err := os.WriteFile(cfg, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(wantFailure bool) {
		t.Helper()
		out, err := exec.Command(bin, "--config", cfg).CombinedOutput()
		if (err != nil) != wantFailure {
			t.Fatalf("CLI %v: %s", err, out)
		}
		if strings.Contains(string(out), "untrusted-secret") {
			t.Fatal("response leaked")
		}
	}
	run(false)
	confirmed, err := readState(state)
	if err != nil {
		t.Fatal(err)
	}
	sm := filepath.Join(filepath.Dir(cfg), "sendmail")
	original, err := os.ReadFile(sm)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sm, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	bad.Store(true)
	run(true)
	failed, err := readState(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed.Pending) != 1 || !failed.Profiles["dns"].Failing || failed.Profiles["dns"].Address != confirmed.Profiles["dns"].Address || !failed.Profiles["dns"].Updated.Equal(confirmed.Profiles["dns"].Updated) {
		t.Fatal("failure intent/confirmation not preserved")
	}
	queued := failed.Pending[0]
	run(true)
	failed, err = readState(state)
	if err != nil || len(failed.Pending) != 1 || failed.Pending[0] != queued {
		t.Fatal("restart duplicated failure intent")
	}
	if err := os.WriteFile(sm, original, 0700); err != nil {
		t.Fatal(err)
	}
	run(true)
	got, err := os.ReadFile(mail)
	if err != nil || !strings.HasSuffix(string(got), queued) || strings.Count(string(got), "Subject:") != 2 {
		t.Fatal("queued mail not retried unchanged")
	}
	bad.Store(false)
	run(false)
	run(false)
	got, err = os.ReadFile(mail)
	if err != nil || strings.Count(string(got), "Subject:") != 3 || providerCalls.Load() != 2 {
		t.Fatal("provider-confirmed recovery/noop failed")
	}
}
