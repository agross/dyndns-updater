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

func TestKilledAfterLocalMailAcceptanceRetries(t *testing.T) {
	if _, err := interfaceIPv4("lo"); err != nil {
		t.Skip("Linux loopback lo unavailable")
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") }))
	defer srv.Close()
	bin := buildCLI(t)
	cfg, state, mail := cliConfig(t, srv.URL)
	marker := filepath.Join(filepath.Dir(cfg), "accepted")
	// This fake MTA accepts the message, then kills the updater before it can
	// persist acceptance. No real email is submitted.
	script := "#!/bin/sh\ncat >> '" + mail + "'\nif [ ! -f '" + marker + "' ]; then\n  touch '" + marker + "'\n  kill -KILL \"$PPID\"\nfi\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(cfg), "sendmail"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bin, "--config", cfg).CombinedOutput(); err == nil {
		t.Fatalf("expected killed process, got %s", out)
	}
	s, err := readState(state)
	if err != nil || len(s.Pending) != 1 {
		t.Fatal("pre-submission state not durable")
	}
	if out, err := exec.Command(bin, "--config", cfg).CombinedOutput(); err != nil {
		t.Fatalf("restart: %v %s", err, out)
	}
	b, err := os.ReadFile(mail)
	if err != nil {
		t.Fatal(err)
	}
	want := "To: root\nSubject: dyndns-updater profile changes\nContent-Type: text/plain; charset=UTF-8\n\n" + s.Pending[0]
	if calls.Load() != 1 || string(b) != strings.Repeat(want, 2) || !strings.HasPrefix(s.Pending[0], "Summary generated at: ") {
		t.Fatal("crash window did not retry accepted mail independently")
	}
	s, err = readState(state)
	if err != nil || len(s.Pending) != 0 {
		t.Fatal("retry acceptance not persisted")
	}
}
