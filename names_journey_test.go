package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCLILegacyStateAndPendingMail(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	j.write(namedProfile("dns", srv.URL))
	// Version 1 fixture produced before mapping syntax. Identity algorithm remains
	// unchanged, and a preexisting pending body must be submitted byte-for-byte.
	p := Profile{Provider: "desec", Target: "example.invalid", Interface: "wan", Endpoint: srv.URL}
	legacy := fmt.Sprintf(`{"version":1,"profiles":{"dns":{"identity":%q,"address":"192.0.2.10","updated":"2026-01-01T00:00:00Z","failing":false}},"dirty":false,"pending":["dns: confirmed 192.0.2.10\n","removed-profile: interface \"wan \\\"quoted\\\"\" IPv4 unavailable\n"]}`, identity(p))
	path := filepath.Join(j.dir, "state.json")
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	var snapshot diskState
	if err := json.Unmarshal([]byte(legacy), &snapshot); err != nil {
		t.Fatal(err)
	}
	j.mailError = true
	j.run(1)
	if calls != 0 {
		t.Fatal("mapping syntax invalidated existing confirmed state")
	}
	j.mailError = false
	j.run(0)
	if calls != 0 || strings.Join(j.mails, "|") != strings.Join(snapshot.Pending, "|") {
		t.Fatalf("pending content/order changed: %q", j.mails)
	}
}

func TestCLIExactDecodedNameIdentity(t *testing.T) {
	j := newJourney(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "good") }))
	defer srv.Close()
	names := []string{"Home DNS", " Home DNS", "home DNS", "café", "cafe\u0301", " ", "17", "true"}
	configured := ""
	for _, name := range names {
		configured += namedProfile(strconv.Quote(name), srv.URL)
		j.write(configured)
		j.run(0)
	}
	if calls != len(names) || len(j.state().Profiles) != len(names) {
		t.Fatal("names trimmed, case folded, Unicode normalized, or rejected")
	}
	j.write(namedProfile(`"Home DNS"`, srv.URL))
	j.run(0)
	if calls != len(names) {
		t.Fatal("existing exact name not reused")
	}
	// A changed identity still updates even with the same name and address.
	j.write(strings.Replace(namedProfile(`"Home DNS"`, srv.URL), "example.invalid", "changed.invalid", 1))
	out := j.run(0)
	if calls != len(names)+1 || !strings.Contains(out, `"Home DNS": updating provider: profile identity changed`) {
		t.Fatal("identity change suppressed")
	}
	j.now = j.now.Add(601 * time.Hour)
	j.run(0)
	if calls != len(names)+2 {
		t.Fatal("refresh suppressed")
	}
	j.ip = "192.0.2.20"
	j.run(0)
	if calls != len(names)+3 || j.state().Profiles["Home DNS"].Address != j.ip {
		t.Fatal("address change suppressed")
	}
}

func TestBuiltCLIMappingNameAndFakeMail(t *testing.T) {
	if _, err := interfaceIPv4("lo"); err != nil {
		t.Skip("Linux loopback lo unavailable")
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") }))
	defer srv.Close()
	bin := buildCLI(t)
	cfg, state, mail := cliConfig(t, srv.URL)
	b, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	name := `Home DNS: "quoted" \ route`
	content := strings.Replace(string(b), `"dns":`, strconv.Quote(name)+":", 1)
	if err := os.WriteFile(cfg, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		b, err = exec.Command(bin, "--config", cfg).CombinedOutput()
		if err != nil {
			t.Fatalf("CLI: %v %s", err, b)
		}
		decision, result := "updating provider: no confirmed state", "confirmed 127.0.0.1"
		if i == 1 {
			decision, result = "skipping update: address unchanged and refresh not due", "unchanged"
		}
		q := strconv.Quote(name)
		want := q + ": interface \"lo\" detected IPv4 127.0.0.1\n" + q + ": " + decision + "\n" + q + ": " + result + "\n"
		if string(b) != want {
			t.Fatalf("output=%q want=%q", b, want)
		}
	}
	b, err = os.ReadFile(mail)
	if err != nil {
		t.Fatal(err)
	}
	wantMail := "To: root\nSubject: dyndns-updater profile changes\nContent-Type: text/plain; charset=UTF-8\n\n" + name + ": confirmed 127.0.0.1\n"
	if string(b) != wantMail || calls.Load() != 1 {
		t.Fatalf("mail=%q calls=%d", b, calls.Load())
	}
	b, err = os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	var s diskState
	if err := json.Unmarshal(b, &s); err != nil || s.Profiles[name].Address != "127.0.0.1" {
		t.Fatal("decoded name not persisted")
	}
	// Invalid configuration must not consume even an existing valid queue, update
	// the provider, or invoke the executable MTA.
	s.Pending = []string{strconv.Quote(name) + ": confirmed 127.0.0.1\n"}
	b, err = json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, b, 0600); err != nil {
		t.Fatal(err)
	}
	originalState := string(b)
	content = strings.Replace(content, strconv.Quote(name)+":", `"bad\nBcc: forged":`, 1)
	if err := os.WriteFile(cfg, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command(bin, "--config", cfg).CombinedOutput(); err == nil || !strings.HasPrefix(string(b), "[error] configuration ") || strings.Contains(string(b), "forged") {
		t.Fatalf("invalid config: %v %s", err, b)
	}
	b, err = os.ReadFile(state)
	if err != nil || string(b) != originalState || calls.Load() != 1 {
		t.Fatal("invalid config changed state/provider")
	}
	b, err = os.ReadFile(mail)
	if err != nil || string(b) != wantMail {
		t.Fatal("invalid config submitted mail")
	}
	content = strings.Replace(content, `"bad\nBcc: forged":`, strconv.Quote(name)+":", 1)
	if err := os.WriteFile(cfg, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	unsafe := `{"version":1,"profiles":{},"pending":["dns: unchanged\nBcc: forged\n"]}`
	if err := os.WriteFile(state, []byte(unsafe), 0600); err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command(bin, "--config", cfg).CombinedOutput(); err == nil || string(b) != "[error] invalid pending notification state; preserved\n" {
		t.Fatalf("unsafe state: %v %q", err, b)
	}
	b, err = os.ReadFile(state)
	if err != nil || string(b) != unsafe || calls.Load() != 1 {
		t.Fatal("unsafe state changed state/provider")
	}
	b, err = os.ReadFile(mail)
	if err != nil || string(b) != wantMail {
		t.Fatal("unsafe state submitted mail")
	}
}
