package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdateJourney(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("myipv6") != "preserve" || r.URL.Query().Get("myipv4") == "" || r.Header.Get("Authorization") != "Token synthetic-secret" {
			t.Error("invalid deSEC request")
		}
		fmt.Fprint(w, "good")
	}))
	defer server.Close()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	text := fmt.Sprintf("state: %s\nrefresh: 600h\nprofiles:\n  dns:\n    provider: desec\n    interface: wan\n    target: example.invalid\n    token: synthetic-secret\n    endpoint: %s\n", filepath.Join(dir, "state.json"), server.URL)
	if err := os.WriteFile(cfg, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	ip := "192.0.2.1"
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mails := []string{}
	d := dependencies{address: func(string) (string, error) { return ip, nil }, now: func() time.Time { return now }, mail: func(_ Config, body string) error { mails = append(mails, body); return nil }}
	run := func() {
		t.Helper()
		var out bytes.Buffer
		if code := runCLI([]string{"--config", cfg}, &out, d); code != 0 {
			t.Fatalf("exit %d: %s", code, out.String())
		}
		if strings.Contains(out.String(), "synthetic-secret") {
			t.Fatal("secret leaked")
		}
	}
	run()
	run()
	if calls != 1 || len(mails) != 1 {
		t.Fatalf("initial/noop: calls=%d mails=%d", calls, len(mails))
	}
	now = now.Add(601 * time.Hour)
	run()
	if calls != 2 || len(mails) != 1 {
		t.Fatal("same-address refresh should be quiet")
	}
	ip = "192.0.2.2"
	run()
	if calls != 3 || len(mails) != 2 {
		t.Fatal("changed address not reported")
	}
	if err := os.Remove(filepath.Join(dir, "state.json")); err != nil {
		t.Fatal(err)
	}
	run()
	if calls != 4 {
		t.Fatal("missing state must force update")
	}
}
