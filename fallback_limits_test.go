package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestCLIFallbackSharedDNSAndHTTPDeadline(t *testing.T) {
	system := startDNS(t, "127.0.0.1:0")
	system.mode.Store(1)
	localSystemDNS(t, system)
	resolver := startDNS(t, "127.0.0.1:0")
	resolver.delay.Store(int64(30 * time.Millisecond))
	var calls atomic.Int32
	endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(80 * time.Millisecond)
		fmt.Fprint(w, "good")
	})
	j := newJourney(t)
	j.write(profile("dns", "desec", endpoint))
	j.fallback(resolver.address)
	j.run(1)
	if calls.Load() != 1 || j.state().Profiles["dns"].Address != "" {
		t.Fatal("DNS and HTTP did not share one deadline")
	}
}

func TestCLIFallbackUnavailableAndDisabled(t *testing.T) {
	system := startDNS(t, "127.0.0.1:0")
	system.mode.Store(1)
	localSystemDNS(t, system)
	endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "good") })
	// A closed local UDP socket produces an immediate transport failure.
	closed, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	unavailable := closed.LocalAddr().String()
	closed.Close()
	later := startDNS(t, "127.0.0.1:0")
	j := newJourney(t)
	j.write(profile("dns", "desec", endpoint))
	j.fallback(unavailable, later.address)
	j.run(0)
	if later.udp.Load() == 0 {
		t.Fatal("unavailable resolver prevented later fallback")
	}
	count := later.udp.Load()
	j.write(profile("dns", "desec", endpoint))
	j.fallback()
	j.ip = "192.0.2.11"
	j.run(1)
	if later.udp.Load() != count || j.state().Profiles["dns"].Address != "192.0.2.10" {
		t.Fatal("explicit empty list changed system-only failure behavior")
	}
}

func TestCLIFallbackPreservesProxyPolicy(t *testing.T) {
	system := startDNS(t, "127.0.0.1:0")
	system.mode.Store(1)
	localSystemDNS(t, system)
	fallback := startDNS(t, "127.0.0.1:0")
	endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) { t.Error("failed proxy bypassed") })
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "CONNECT" {
			t.Error("HTTPS proxy policy changed")
		}
		w.WriteHeader(502)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	http.DefaultTransport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
	j := newJourney(t)
	j.write(profile("dns", "desec", endpoint))
	j.fallback(fallback.address)
	j.run(1)
	if calls.Load() != 1 || system.udp.Load() != 0 || fallback.udp.Load() != 0 {
		t.Fatal("proxy failure triggered provider DNS or bypassed proxy")
	}
}

func TestCLIValidFallbackAddressSyntax(t *testing.T) {
	for _, value := range []string{"192.0.2.53", "2001:db8::53", "192.0.2.53:5353", "[2001:db8::53]:5353"} {
		t.Run(value, func(t *testing.T) {
			j := newJourney(t)
			j.write(profile("dns", "desec", "https://example.invalid/"))
			j.fallback(value)
			// No interface address => no DNS/provider/mail; configuration still passes.
			j.missing = true
			j.run(1)
			if _, err := os.Stat(j.dir + "/state.json"); err != nil {
				t.Fatal("valid resolver syntax rejected", err)
			}
		})
	}
}
