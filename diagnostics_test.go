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
)

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
	want := fmt.Sprintf("dyndns-updater: error: cannot open state lock (state directory %q must exist and be writable)\n", dir)
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
	want := "gateway-dns: interface \"dyndns-no-if\" IPv4 unavailable\ngateway-tunnel: interface \"dyndns-no-if2\" IPv4 unavailable\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
	if len(j.mails) != 0 {
		t.Fatal("missing interfaces triggered mail")
	}
}

func TestCLIProfileAndMailFailuresReportedOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	j := newJourney(t)
	j.write(profile("gateway-dns", "desec", server.URL))
	j.mailError = true
	out := j.run(1)
	want := "gateway-dns: provider HTTP failure (503)\ndyndns-updater: error: mail failed\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}
