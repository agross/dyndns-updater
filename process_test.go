package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "dyndns-updater")
	if b, e := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); e != nil {
		t.Fatalf("build: %v %s", e, b)
	}
	return bin
}
func cliConfig(t *testing.T, endpoint string) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	mail := filepath.Join(dir, "mail")
	sendmail := filepath.Join(dir, "sendmail")
	script := "#!/bin/sh\n[ \"$1\" = -oi ] && [ \"$2\" = -- ] && [ \"$3\" = root ] || exit 2\ncat >> '" + mail + "'\n"
	if e := os.WriteFile(sendmail, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	cfg := filepath.Join(dir, "config.yaml")
	state := filepath.Join(dir, "state.json")
	body := fmt.Sprintf("state: %s\nsendmail: %s\ntimeout: 5s\nprofiles:\n%s", state, sendmail, strings.Replace(profile("dns", "desec", endpoint), "interface: wan", "interface: lo", 1))
	if e := os.WriteFile(cfg, []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
	return cfg, state, mail
}

func TestBuiltCLIWithLocalSendmail(t *testing.T) {
	if _, err := interfaceIPv4("lo"); err != nil {
		t.Skip("Linux loopback lo unavailable")
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("myipv4") != "127.0.0.1" || r.URL.Query().Get("myipv6") != "preserve" {
			t.Error("wrong request")
		}
		fmt.Fprint(w, "good")
	}))
	defer srv.Close()
	bin := buildCLI(t)
	cfg, state, mail := cliConfig(t, srv.URL)
	for i := range 2 {
		b, e := exec.Command(bin, "--config", cfg).CombinedOutput()
		if e != nil {
			t.Fatalf("run: %v %s", e, b)
		}
		decision, result := "updating provider: no confirmed state", "confirmed 127.0.0.1"
		if i == 1 {
			decision, result = "skipping update: address unchanged and refresh not due", "unchanged"
		}
		want := "dns: interface \"lo\" detected IPv4 127.0.0.1\ndns: " + decision + "\ndns: " + result + "\n"
		if string(b) != want {
			t.Fatalf("built CLI output = %q, want %q", b, want)
		}
		if _, err := os.Stat(state + ".lock"); err != nil {
			t.Fatalf("stable lock file missing after exit: %v", err)
		}
	}
	b, e := os.ReadFile(mail)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Count(string(b), "Subject:") != 1 || calls.Load() != 1 {
		t.Fatal("CLI noop/mail behavior failed")
	}
	info, e := os.Stat(state)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("state must be private")
	}
	// Actual executable sendmail failure followed by local retry.
	if e := os.Remove(mail); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(state); e != nil {
		t.Fatal(e)
	}
	sm := filepath.Join(filepath.Dir(cfg), "sendmail")
	script, e := os.ReadFile(sm)
	if e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(sm, []byte("#!/bin/sh\nexit 1\n"), 0700); e != nil {
		t.Fatal(e)
	}
	if b, e := exec.Command(bin, "--config", cfg).CombinedOutput(); e == nil {
		t.Fatalf("mail error exit zero: %s", b)
	}
	if _, err := os.Stat(state + ".lock"); err != nil {
		t.Fatalf("stable lock file missing after failed exit: %v", err)
	}
	if e := os.WriteFile(sm, script, 0700); e != nil {
		t.Fatal(e)
	}
	if b, e := exec.Command(bin, "--config", cfg).CombinedOutput(); e != nil {
		t.Fatalf("retry: %v %s", e, b)
	}
	if calls.Load() != 2 {
		t.Fatal("mail retry repeated update")
	}
	if _, e := os.ReadFile(mail); e != nil {
		t.Fatal("notification retry missing")
	}
}

func TestConcurrentCLIAndKilledLockHolder(t *testing.T) {
	if _, err := interfaceIPv4("lo"); err != nil {
		t.Skip("Linux loopback lo unavailable")
	}
	for _, kill := range []bool{false, true} {
		t.Run(fmt.Sprint("kill=", kill), func(t *testing.T) {
			entered := make(chan struct{}, 4)
			release := make(chan struct{})
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				entered <- struct{}{}
				if n == 1 {
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				fmt.Fprint(w, "good")
			}))
			defer srv.Close()
			bin := buildCLI(t)
			cfg, state, _ := cliConfig(t, srv.URL)
			first := exec.Command(bin, "--config", cfg)
			var firstOut bytes.Buffer
			first.Stdout = &firstOut
			first.Stderr = &firstOut
			if e := first.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() {
				if first.ProcessState == nil {
					first.Process.Kill()
					first.Wait()
				}
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("first process never called provider")
			}
			second := exec.Command(bin, "--config", cfg)
			var secondOut bytes.Buffer
			second.Stdout = &secondOut
			second.Stderr = &secondOut
			if e := second.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() {
				if second.ProcessState == nil {
					second.Process.Kill()
					second.Wait()
				}
			}()
			select {
			case <-entered:
				t.Fatal("overlap reached provider while lock held")
			case <-time.After(150 * time.Millisecond):
			}
			if kill {
				if e := first.Process.Kill(); e != nil {
					t.Fatal(e)
				}
				first.Wait()
			} else {
				close(release)
				if e := first.Wait(); e != nil {
					t.Fatalf("first: %v %s", e, firstOut.String())
				}
			}
			done := make(chan error, 1)
			go func() { done <- second.Wait() }()
			select {
			case e := <-done:
				if e != nil {
					t.Fatalf("second: %v %s", e, secondOut.String())
				}
			case <-time.After(6 * time.Second):
				t.Fatal("lock not released")
			}
			want := int32(1)
			if kill {
				want = 2
			}
			if calls.Load() != want {
				t.Fatalf("calls=%d want=%d", calls.Load(), want)
			}
			s, e := readState(state)
			if e != nil || s.Profiles["dns"].Address != "127.0.0.1" {
				t.Fatal("state not intact")
			}
		})
	}
}
