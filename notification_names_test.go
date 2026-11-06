package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestCLINotificationNamesAndRestart(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	names := []string{"Tunnel Broker", " Home: DNS ", `"quoted"`, "quoted", `\backslash`, "Zürich 東京", "prefix: unchanged: name", `"unterminated`, `"bad\nname"`}
	sort.Strings(names)
	config, mail, cli := "", "", ""
	for _, name := range names {
		config += namedProfile(strconv.Quote(name), srv.URL)
		mail += name + ": confirmed 192.0.2.10\n"
		cli += strconv.Quote(name) + ": confirmed 192.0.2.10\n"
	}
	j.write(config)
	j.mailError = true
	if out := j.run(1); !strings.Contains(out, cli) {
		t.Fatalf("CLI results changed: %q", out)
	}
	if got := j.state().Pending; len(got) != 1 || got[0] != mail {
		t.Fatalf("pending=%q want=%q", got, mail)
	}
	j.run(1)
	if calls != len(names) || len(j.state().Pending) != 1 {
		t.Fatal("restart repeated updates or notification")
	}
	j.mailError = false
	j.run(0)
	if len(j.mails) != 1 || j.mails[0] != mail || len(j.state().Pending) != 0 {
		t.Fatalf("retry changed content: %q", j.mails)
	}
}

func TestCLINotificationPendingCompatibility(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	j.write(namedProfile(`"Tunnel Broker"`, srv.URL))
	pending := []string{"old-name: unchanged\n", "\"Old: \\\"DNS\": confirmed 192.0.2.1\n", "Removed: profile: unchanged\n"}
	state := diskState{Version: 1, Profiles: map[string]profileState{}, Pending: pending}
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(j.dir, "state.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	j.mailError = true
	j.run(1)
	want := append(append([]string{}, pending...), "Tunnel Broker: confirmed 192.0.2.10\n")
	if got := j.state().Pending; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("queue changed: %q", got)
	}
	j.mailError = false
	j.run(0)
	if strings.Join(j.mails, "|") != strings.Join(want, "|") || calls != 1 {
		t.Fatalf("delivery=%q calls=%d", j.mails, calls)
	}
}

func TestCLIRejectMalformedNotificationNames(t *testing.T) {
	for _, body := range []string{"name: unchanged: arbitrary\n", "name: confirmed 999.2.3.4\n", "name: unchanged\nBcc: forged\n", "unsafe\u202e: unchanged\n", "unsafe\x00: unchanged\n", "name: unchanged\r\n", ": unchanged\n", "name: unchanged\nname: unchanged\n"} {
		t.Run(strconv.Quote(body), func(t *testing.T) {
			j := newJourney(t)
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
			defer srv.Close()
			j.write(namedProfile(`"Tunnel Broker"`, srv.URL))
			b, err := json.Marshal(diskState{Version: 1, Profiles: map[string]profileState{}, Pending: []string{body}})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(j.dir, "state.json")
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
			if out := j.run(1); !strings.Contains(out, "preserved") {
				t.Fatalf("output=%q", out)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(b) || calls != 0 || len(j.mails) != 0 {
				t.Fatal("invalid pending state caused side effects")
			}
		})
	}
}
