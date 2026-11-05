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
	"time"
)

func TestInterruptedLaterProfileKeepsEarlierSuccess(t *testing.T) {
	for _, removeConfirmed := range []bool{false, true} {
		t.Run(fmt.Sprintf("remove-confirmed=%t", removeConfirmed), func(t *testing.T) {
			if _, err := interfaceIPv4("lo"); err != nil {
				t.Skip("Linux loopback lo unavailable")
			}
			var firstCalls, secondCalls atomic.Int32
			blocked := make(chan struct{}, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/first" {
					firstCalls.Add(1)
				} else if secondCalls.Add(1) == 1 {
					blocked <- struct{}{}
					<-r.Context().Done()
					return
				}
				fmt.Fprint(w, "good")
			}))
			defer srv.Close()
			bin := buildCLI(t)
			cfg, state, mail := cliConfig(t, srv.URL+"/first")
			b, err := os.ReadFile(cfg)
			if err != nil {
				t.Fatal(err)
			}
			b = append(b, []byte(strings.Replace(profile("second", "desec", srv.URL+"/second"), "interface: wan", "interface: lo", 1))...)
			if err := os.WriteFile(cfg, b, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, "--config", cfg)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			}()
			select {
			case <-blocked:
			case <-time.After(5 * time.Second):
				t.Fatal("later provider never reached")
			}
			// The first outcome is already committed, even though the second is blocked.
			s, err := readState(state)
			if err != nil || s.Profiles["dns"].Address != "127.0.0.1" || !s.Dirty {
				t.Fatal("earlier success/notification intent not checkpointed")
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			cmd.Wait()
			if removeConfirmed {
				// Remove the checkpointed profile after actual process termination.
				// Restart must preserve Dirty while pruning its confirmation.
				start := strings.Index(string(b), "profiles:\n") + len("profiles:\n")
				config := string(b[:start]) + strings.Replace(profile("second", "desec", srv.URL+"/second"), "interface: wan", "interface: lo", 1)
				if err := os.WriteFile(cfg, []byte(config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if out, err := exec.Command(bin, "--config", cfg).CombinedOutput(); err != nil {
				t.Fatalf("restart: %v %s", err, out)
			}
			if firstCalls.Load() != 1 || secondCalls.Load() != 2 {
				t.Fatal("restart discarded independent success")
			}
			b, err = os.ReadFile(mail)
			if err != nil || strings.Contains(string(b), "\"dns\":") == removeConfirmed || !strings.Contains(string(b), "\"second\":") {
				t.Fatal("restart lost all-profile notification")
			}
			s, err = readState(state)
			if err != nil || s.Dirty || len(s.Pending) != 0 {
				t.Fatal("recovery not persisted", err)
			}
			if _, ok := s.Profiles["dns"]; ok == removeConfirmed {
				t.Fatal("incorrect recovered profile set", s)
			}
		})
	}
}
