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

func TestCLIConfigurationFileDiagnostics(t *testing.T) {
	for _, kind := range []string{"missing", "directory", "invalid YAML"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config \"quoted\" file.yaml")
			reason := "cannot read configuration"
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				reason = "configuration unreadable or exceeds 1 MiB"
			case "invalid YAML":
				if err := os.WriteFile(path, []byte("token: [private-secret\n"), 0600); err != nil {
					t.Fatal(err)
				}
				reason = "invalid YAML configuration"
			}
			var output bytes.Buffer
			if code := runCLI([]string{"--config", path}, &output, dependencies{}); code != 1 {
				t.Fatalf("exit = %d, want 1", code)
			}
			want := fmt.Sprintf("[error] configuration %q: %s\n", path, reason)
			if output.String() != want {
				t.Fatalf("output = %q, want %q", output.String(), want)
			}
		})
	}
}

func TestCLIMissingStateDirectory(t *testing.T) {
	j := newJourney(t)
	j.write(profile("dns", "desec", "http://127.0.0.1:1"))
	dir := filepath.Join(j.dir, "missing", "state directory")
	content, err := os.ReadFile(j.config)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.ReplaceAll(string(content), filepath.Join(j.dir, "state.json"), filepath.Join(dir, "state.json")))
	if err := os.WriteFile(j.config, content, 0600); err != nil {
		t.Fatal(err)
	}
	out := j.run(1)
	want := fmt.Sprintf("[error] cannot open state lock (state directory %q must exist and be writable)\n", dir)
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestCLIMissingInterfacesReportedOnce(t *testing.T) {
	j := newJourney(t)
	j.write(profile("gateway-dns", "desec", "http://127.0.0.1:1") + strings.ReplaceAll(profile("gateway-tunnel", "tunnelbroker", "http://127.0.0.1:1"), "interface: wan", "interface: uplink"))
	// Exercise real OS lookup through the CLI with two absent interfaces.
	content, err := os.ReadFile(j.config)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(content), "interface: wan", "interface: dyndns-no-if")
	text = strings.ReplaceAll(text, "interface: uplink", "interface: dyndns-no-if2")
	if err := os.WriteFile(j.config, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	d := productionDependencies()
	d.mail = func(Config, string) error { t.Fatal("missing interfaces triggered mail"); return nil }
	var output bytes.Buffer
	if code := runCLI([]string{"--config", j.config}, &output, d); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	out := output.String()
	want := "\"gateway-dns\": interface \"dyndns-no-if\" IPv4 unavailable\n\"gateway-tunnel\": interface \"dyndns-no-if2\" IPv4 unavailable\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
	if len(j.mails) != 0 {
		t.Fatal("missing interfaces triggered mail")
	}
}

func TestCLIUpdateDecisionTrace(t *testing.T) {
	j := newJourney(t)
	calls := 0
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer server.Close()
	// Quoted interface names must remain unambiguous in the trace.
	base := strings.ReplaceAll(profile("dns", "desec", server.URL), "interface: wan", `interface: 'wan "quoted"'`)
	j.write(base)
	check := func(reason, result string, code, wantCalls int) {
		t.Helper()
		want := fmt.Sprintf("\"dns\": interface %q detected IPv4 %s\n\"dns\": %s\n\"dns\": %s\n", `wan "quoted"`, j.ip, reason, result)
		if out := j.run(code); out != want {
			t.Fatalf("output = %q, want %q", out, want)
		}
		if calls != wantCalls {
			t.Fatalf("provider calls = %d, want %d", calls, wantCalls)
		}
	}
	check("updating provider: no confirmed state", "confirmed "+j.ip, 0, 1)
	check("skipping update: address unchanged and refresh not due", "unchanged", 0, 1)
	j.ip = "192.0.2.20"
	check("updating provider: address changed", "confirmed "+j.ip, 0, 2)
	j.now = j.now.Add(600 * time.Hour)
	check("updating provider: refresh due", "confirmed "+j.ip, 0, 3)
	fail = true
	j.ip = "192.0.2.30"
	check("updating provider: address changed", "provider HTTP failure (401)", 1, 4)
	check("updating provider: retrying previous failure", "provider HTTP failure (401)", 1, 5)
	fail = false
	check("updating provider: retrying previous failure", "confirmed "+j.ip, 0, 6)
	j.write(strings.ReplaceAll(base, "example.invalid", "changed.invalid"))
	check("updating provider: profile identity changed", "confirmed "+j.ip, 0, 7)
	if strings.Contains(strings.Join(j.mails, "\n"), "updating provider:") {
		t.Fatal("operational trace must not enter notification summaries")
	}
}

func TestCLIProfileAndMailFailuresReportedOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	j := newJourney(t)
	j.write(profile("gateway-dns", "desec", server.URL))
	j.mailError = true
	out := j.run(1)
	want := "\"gateway-dns\": interface \"wan\" detected IPv4 192.0.2.10\n\"gateway-dns\": updating provider: no confirmed state\n\"gateway-dns\": provider HTTP failure (503)\n[error] mail failed\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}
