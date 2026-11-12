package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCLIDiscoveryDNSFallback(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		system, first                    int32
		status                           int
		wantWeb, wantFallback, wantLater bool
	}{
		{"system first", 0, 0, 200, true, false, false},
		{"first fallback", 1, 0, 200, true, true, false},
		{"negative then later", 1, 1, 200, true, true, true},
		{"empty then later", 2, 2, 200, true, true, true},
		{"unusable then later", 6, 6, 200, true, true, true},
		{"IPv6 only then later", 7, 7, 200, true, true, true},
		{"SERVFAIL then later", 5, 5, 200, true, true, true},
		{"UDP to TCP", 1, 3, 200, true, true, false},
		{"timeout", 1, 4, 200, false, true, false},
		{"exhaustion", 1, 1, 200, false, true, true},
		{"HTTP failure is not DNS failure", 0, 0, 503, true, false, false},
		{"TLS failure", 0, 0, 200, false, false, false},
		{"connection failure", 0, 0, 200, false, false, false},
		{"redirect is not DNS failure", 0, 0, 302, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			system := startDNS(t, "127.0.0.1:0")
			system.mode.Store(tc.system)
			localSystemDNS(t, system)
			first := startDNS(t, "127.0.0.1:0")
			first.mode.Store(tc.first)
			later := startDNS(t, "127.0.0.1:0")
			if tc.name == "exhaustion" {
				later.mode.Store(1)
			}
			var web, providers atomic.Int32
			endpoint, server := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) {
				web.Add(1)
				if !strings.HasPrefix(r.Host, "example.com:") || r.TLS.ServerName != "example.com" || r.Header.Get("Authorization") != "" {
					t.Error("Host/SNI/auth changed")
				}
				w.Header().Set("Location", "/must-not-follow")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, "8.8.4.4")
			})
			if tc.name == "TLS failure" {
				endpoint = strings.Replace(endpoint, "example.com", "wrong.invalid", 1)
			}
			if tc.name == "connection failure" {
				server.Close()
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { providers.Add(1); fmt.Fprint(w, "good") }))
			defer provider.Close()
			j := newJourney(t)
			j.ip = "9.9.9.9"
			j.write(sourceProfile("dns", provider.URL, "url:"+endpoint, "if:wan"))
			j.fallback(first.address, later.address)
			out := j.run(0)
			if (web.Load() > 0) != tc.wantWeb || (first.udp.Load() > 0) != tc.wantFallback || (later.udp.Load() > 0) != tc.wantLater || providers.Load() != 1 {
				t.Fatalf("web=%d first=%d later=%d provider=%d: %s", web.Load(), first.udp.Load(), later.udp.Load(), providers.Load(), out)
			}
			expected := "9.9.9.9"
			if tc.wantWeb && tc.status == 200 {
				expected = "8.8.4.4"
			}
			if j.state().Profiles["dns"].Address != expected {
				t.Fatal("wrong fallback result")
			}
			if tc.first == 3 && first.tcp.Load() == 0 {
				t.Fatal("truncated DNS never reached TCP")
			}
			if tc.first == 4 && strings.Contains(out, "DNS lookup succeeded") {
				t.Fatal("deadline invented DNS success")
			}
		})
	}
}

func TestCLIDiscoverySharedDNSAndHTTPBudget(t *testing.T) {
	system := startDNS(t, "127.0.0.1:0")
	system.mode.Store(1)
	localSystemDNS(t, system)
	resolver := startDNS(t, "127.0.0.1:0")
	resolver.delay.Store(int64(30 * time.Millisecond))
	var calls atomic.Int32
	endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(80 * time.Millisecond)
		fmt.Fprint(w, "8.8.4.4")
	})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "good") }))
	defer provider.Close()
	j := newJourney(t)
	j.write(sourceProfile("dns", provider.URL, "url:"+endpoint, "if:wan"))
	j.fallback(resolver.address)
	out := j.run(0)
	if calls.Load() != 1 || j.state().Profiles["dns"].Address != j.ip || !strings.Contains(out, "DNS lookup succeeded") || !strings.Contains(out, "source 1 failed") {
		t.Fatal("DNS and HTTP did not share one source budget", out)
	}
}

func TestCLIDiscoveryLiteralSkipsDNS(t *testing.T) {
	system := startDNS(t, "127.0.0.1:0")
	system.mode.Store(4)
	localSystemDNS(t, system)
	fallback := startDNS(t, "127.0.0.1:0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/discover" {
			fmt.Fprint(w, "8.8.4.4")
		} else {
			fmt.Fprint(w, "good")
		}
	}))
	defer srv.Close()
	j := newJourney(t)
	j.write(sourceProfile("dns", srv.URL, "url:"+srv.URL+"/discover"))
	j.fallback(fallback.address)
	j.run(0)
	if system.udp.Load() != 0 || fallback.udp.Load() != 0 {
		t.Fatal("literal endpoint used DNS")
	}
}
