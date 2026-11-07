package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCLIFallbackDefaultsAndOverrides(t *testing.T) {
	for _, kind := range []string{"omitted", "empty", "custom"} {
		t.Run(kind, func(t *testing.T) {
			system := startDNS(t, "127.0.0.1:0")
			system.mode.Store(1)
			localSystemDNS(t, system)
			first := startDNS(t, "127.0.0.1:0")
			first.mode.Store(1)
			second := startDNS(t, "127.0.0.1:0")
			second.mode.Store(1)
			third := startDNS(t, "127.0.0.1:0")
			custom := startDNS(t, "127.0.0.1:0")
			defaults := []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}
			routes := map[string]string{defaults[0]: first.address, defaults[1]: second.address, defaults[2]: third.address, custom.address: custom.address}
			previous := fallbackDNSDial
			var mu sync.Mutex
			var attempted []string
			fallbackDNSDial = func(ctx context.Context, network, address string) (net.Conn, error) {
				mu.Lock()
				// Go may issue A/AAAA and retries to the same resolver concurrently.
				if len(attempted) == 0 || attempted[len(attempted)-1] != address {
					attempted = append(attempted, address)
				}
				local, ok := routes[address]
				mu.Unlock()
				if !ok {
					return nil, fmt.Errorf("unexpected resolver destination")
				}
				return (&net.Dialer{Resolver: &net.Resolver{}}).DialContext(ctx, network, local)
			}
			t.Cleanup(func() { fallbackDNSDial = previous })
			var calls atomic.Int32
			endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") })
			j := newJourney(t)
			j.write(profile("dns", "desec", endpoint))
			expected := defaults
			wantExit, wantCalls := 0, int32(1)
			switch kind {
			case "empty":
				j.fallback()
				expected = []string{}
				wantExit, wantCalls = 1, 0
			case "custom":
				j.fallback(custom.address)
				expected = []string{custom.address}
			}
			c, err := loadConfig(j.config)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c.FallbackDNS, expected) {
				t.Fatalf("configured resolvers=%v, want %v", c.FallbackDNS, expected)
			}
			j.run(wantExit)
			mu.Lock()
			got := append([]string{}, attempted...)
			mu.Unlock()
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("resolver order=%v, want %v", got, expected)
			}
			if system.udp.Load() == 0 || calls.Load() != wantCalls {
				t.Fatal("system-first or provider request behavior changed")
			}
			saved := j.state().Profiles["dns"]
			if kind == "empty" {
				if saved.Address != "" || !saved.Failing {
					t.Fatal("disabled fallback claimed confirmation")
				}
			} else if saved.Address != j.ip {
				t.Fatal("fallback did not confirm update")
			}
		})
	}
}
