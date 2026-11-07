package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBuiltCLISenderName(t *testing.T) {
	if _, err := interfaceIPv4("lo"); err != nil {
		t.Skip("Linux loopback lo unavailable")
	}
	bin := buildCLI(t)
	for _, name := range []string{"", "dyndns-updater on gateway-home", `  Gateway \ literal  `, `Gateway: "mail", 100% %s %(hostname)s {{ hostname }} café 世界; $(touch nope)`} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") }))
			defer srv.Close()
			cfg, state, mail := cliConfig(t, srv.URL)
			base, err := os.ReadFile(cfg)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Dir(cfg)
			argsPath := filepath.Join(dir, "args")
			failPath := filepath.Join(dir, "fail")
			script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\000' \"$@\" >> '%s'\ncat >> '%s'\nif [ -f '%s' ]; then echo 'MTA-SECRET' >&2; exit 1; fi\n", argsPath, mail, failPath)
			write := func(path string, b []byte, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(path, b, mode); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(dir, "sendmail"), []byte(script), 0700)
			configure := func(n string) {
				t.Helper()
				b := append([]byte(nil), base...)
				if n != "" {
					b = append(b, []byte("sender_name: "+strconv.Quote(n)+"\n")...)
				}
				write(cfg, b, 0600)
			}
			run := func(fail bool) {
				t.Helper()
				out, err := exec.Command(bin, "--config", cfg).CombinedOutput()
				if (err != nil) != fail || bytes.Contains(out, []byte("MTA-SECRET")) {
					t.Fatalf("run: %v %s", err, out)
				}
			}
			configure(name)
			write(failPath, nil, 0600)
			run(true)
			before, err := readState(state)
			if err != nil || len(before.Pending) != 1 {
				t.Fatalf("state: %v %+v", err, before)
			}
			// Queue another summary; both must retain their bodies and ordering.
			before.Pending = append(before.Pending, "dns: unchanged\n")
			if err := saveState(state, before); err != nil {
				t.Fatal(err)
			}
			oldArgs := []string{"-oi"}
			if name != "" {
				oldArgs = append(oldArgs, "-F", name)
			}
			oldArgs = append(oldArgs, "--", "root")
			got, err := os.ReadFile(argsPath)
			if err != nil || string(got) != strings.Join(oldArgs, "\x00")+"\x00" {
				t.Fatalf("args: %q %v", got, err)
			}
			// Changing presentation alone must not invalidate confirmations or create mail.
			configure("Retry host 100% Ω")
			if err := os.Remove(failPath); err != nil {
				t.Fatal(err)
			}
			run(false)
			run(false)
			after, err := readState(state)
			if err != nil || calls.Load() != 1 || len(after.Pending) != 0 || !reflect.DeepEqual(after.Profiles, before.Profiles) {
				t.Fatalf("retry state: %+v calls=%d err=%v", after, calls.Load(), err)
			}
			retryArgs := "-oi\x00-F\x00Retry host 100% Ω\x00--\x00root\x00"
			got, err = os.ReadFile(argsPath)
			if err != nil || string(got) != strings.Join(oldArgs, "\x00")+"\x00"+retryArgs+retryArgs {
				t.Fatalf("retry args: %q %v", got, err)
			}
			header := "To: root\nSubject: dyndns-updater profile changes\nContent-Type: text/plain; charset=UTF-8\n\n"
			got, err = os.ReadFile(mail)
			want := header + before.Pending[0] + header + before.Pending[0] + header + before.Pending[1]
			if err != nil || string(got) != want {
				t.Fatalf("mail: %q want %q err=%v", got, want, err)
			}
		})
	}
}

func TestCLIRejectUnsafeSenderName(t *testing.T) {
	for _, value := range []string{`""`, `"   "`, `"-froot"`, `" -Xlog"`, `"name\rBcc: other"`, `"name\nBcc: other"`, `"name\x00"`, `"name\tmore"`, `"name\x7f"`, `"name\u0085"`, `"name\u2028"`, `"name\u2029"`, `"name\u202e"`, `[]`, `{}`} {
		t.Run(value, func(t *testing.T) {
			j := newJourney(t)
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") }))
			defer srv.Close()
			p := profile("dns", "desec", srv.URL)
			// Reject before even creating a state directory entry, including a lock.
			for _, existing := range []bool{false, true} {
				j.write(p)
				if existing {
					j.run(0)
				}
				path := filepath.Join(j.dir, "state.json")
				original, _ := os.ReadFile(path)
				entries, _ := os.ReadDir(j.dir)
				providerBefore, mailBefore := calls.Load(), len(j.mailAttempts)
				config, err := os.ReadFile(j.config)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(j.config, append(config, []byte("sender_name: "+value+"\n")...), 0600); err != nil {
					t.Fatal(err)
				}
				out := j.run(1)
				got, _ := os.ReadFile(path)
				afterEntries, _ := os.ReadDir(j.dir)
				if calls.Load() != providerBefore || len(j.mailAttempts) != mailBefore || !bytes.Equal(got, original) || len(entries) != len(afterEntries) || !strings.Contains(out, "[error]") || strings.Contains(out, "\nBcc:") {
					t.Fatalf("side effect or unsafe output: %q", out)
				}
			}
		})
	}
}
