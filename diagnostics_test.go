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
	j.write(profile("gateway-dns", "desec", "http://127.0.0.1:1") + strings.ReplaceAll(profile("gateway-tunnel", "tunnelbroker", "http://127.0.0.1:1"), "sources: [{if: wan}]", "sources: [{if: uplink}]"))
	// Exercise real OS lookup through the CLI with two absent interfaces.
	content, err := os.ReadFile(j.config)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(content), "sources: [{if: wan}]", "sources: [{if: dyndns-no-if}]")
	text = strings.ReplaceAll(text, "sources: [{if: uplink}]", "sources: [{if: dyndns-no-if2}]")
	if err := os.WriteFile(j.config, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	d := productionDependencies()
	d.mail = func(_ Config, body string) error { j.mails = append(j.mails, body); return nil }
	var output bytes.Buffer
	if code := runCLI([]string{"--config", j.config}, &output, d); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	out := output.String()
	want := "gateway-dns: source 1 (if) attempting IPv4 detection\ngateway-dns: source 1 failed IPv4 detection\ngateway-tunnel: source 1 (if) attempting IPv4 detection\ngateway-tunnel: source 1 failed IPv4 detection\ngateway-dns: IPv4 detection failed (all sources exhausted)\ngateway-tunnel: IPv4 detection failed (all sources exhausted)\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
	if len(j.mails) != 1 {
		t.Fatal("exhaustion must persist one all-profile failure summary")
	}
}

func TestCLIUpdateDecisionTrace(t *testing.T) {
	j := newJourney(t)
	calls := 0
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "good")
	}))
	defer server.Close()
	// Quoted interface names must remain unambiguous in the trace.
	base := strings.ReplaceAll(profile("dns", "desec", server.URL), "sources: [{if: wan}]", `sources: [{if: 'wan "quoted"'}]`)
	j.write(base)
	check := func(reason, result string, code, wantCalls int) {
		t.Helper()
		want := fmt.Sprintf("dns: source 1 (if) attempting IPv4 detection\ndns: interface %q detected IPv4 %s\ndns: %s\ndns: %s\n", `wan "quoted"`, j.ip, reason, result)
		if out := j.run(code); out != want {
			t.Fatalf("output = %q, want %q", out, want)
		}
		if calls != wantCalls {
			t.Fatalf("provider calls = %d, want %d", calls, wantCalls)
		}
	}
	check("updating provider: no confirmed state", "confirmed "+j.ip, 0, 1)
	check("skipping update: address unchanged and refresh not due", "unchanged", 0, 1)
	j.ip = "8.0.2.20"
	check("updating provider: address changed", "confirmed "+j.ip, 0, 2)
	j.now = j.now.Add(600 * time.Hour)
	check("updating provider: refresh due", "confirmed "+j.ip, 0, 3)
	fail = true
	j.ip = "8.0.2.30"
	check("updating provider: address changed", "provider HTTP failure (500)", 1, 4)
	check("updating provider: retrying previous failure", "provider HTTP failure (500)", 1, 5)
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
	want := "gateway-dns: source 1 (if) attempting IPv4 detection\ngateway-dns: interface \"wan\" detected IPv4 8.0.2.10\ngateway-dns: updating provider: no confirmed state\ngateway-dns: provider HTTP failure (503)\n[error] mail failed\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}
