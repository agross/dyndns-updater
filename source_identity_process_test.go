package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBuiltCLISourceEditQueuedMail(t *testing.T) {
	var calls, discovery atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/discover") {
			discovery.Add(1)
			fmt.Fprint(w, "8.8.4.4")
			return
		}
		calls.Add(1)
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	bin := buildCLI(t)
	cfg, state, mail := cliConfig(t, srv.URL)
	original, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(original), "\n")
	for i, line := range lines {
		if strings.Contains(line, "sources:") {
			lines[i] = fmt.Sprintf("    sources: [{url: %q}]", srv.URL+"/discover")
		}
	}
	base := strings.Join(lines, "\n")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(cfg, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(failed bool) string {
		t.Helper()
		out, err := exec.Command(bin, "--config", cfg).CombinedOutput()
		if (err != nil) != failed {
			t.Fatalf("CLI: %v %s", err, out)
		}
		return string(out)
	}
	write(base)
	run(false)
	confirmed, err := readState(state)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(mail)
	if err != nil {
		t.Fatal(err)
	}
	// Inject legacy queued delivery intent, not a new event. Source edits must
	// retry these bytes and must not manufacture another provider confirmation.
	s := confirmed
	s.Pending = []string{"dns: confirmed 8.8.4.4\n"}
	if err := saveState(state, s); err != nil {
		t.Fatal(err)
	}
	write(strings.ReplaceAll(base, "/discover", "/discover-replacement"))
	out := run(false)
	after, err := os.ReadFile(mail)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readState(state)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || discovery.Load() != 2 || !strings.Contains(out, "dns: unchanged") || !got.Profiles["dns"].Updated.Equal(confirmed.Profiles["dns"].Updated) || got.Profiles["dns"].Address != confirmed.Profiles["dns"].Address || len(got.Pending) != 0 || !strings.HasSuffix(string(after), s.Pending[0]) || strings.Count(string(after[len(before):]), "Subject:") != 1 {
		t.Fatal("source edit changed confirmation or queued delivery")
	}
	run(false)
	final, err := os.ReadFile(mail)
	if err != nil {
		t.Fatal(err)
	}
	if string(final) != string(after) || calls.Load() != 1 || discovery.Load() != 3 {
		t.Fatal("restart repeated update/mail or skipped discovery")
	}
}
